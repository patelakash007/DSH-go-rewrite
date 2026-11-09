package subprocess

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

const DSHEnvPrefix = "DSH_"

func writeSpillFile(dir, existing string, data []byte) string {
	if existing == "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return ""
		}
		f, err := os.CreateTemp(dir, "spill-*.txt")
		if err != nil {
			return ""
		}
		path := f.Name()
		if _, err := f.Write(data); err != nil {
			_ = f.Close()
			return path
		}
		_ = f.Close()
		return path
	}
	f, err := os.OpenFile(existing, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return existing
	}
	_, _ = f.Write(data)
	_ = f.Close()
	return existing
}

func ScrubParentEnv() []string {
	return scrubEnv(os.Environ())
}

func scrubEnv(applied []string) []string {
	out := make([]string, 0, len(applied))
	for _, kv := range applied {

		if strings.HasPrefix(kv, DSHEnvPrefix) {
			continue
		}

		if !strings.Contains(kv, "=") {
			continue
		}
		out = append(out, kv)
	}
	return out
}

func mergeEnv(base []string, overrides map[string]string) []string {
	m := make(map[string]string, len(base))
	for _, kv := range base {
		if i := strings.IndexByte(kv, '='); i >= 0 {
			m[kv[:i]] = kv[i+1:]
		}
	}
	for k, v := range overrides {
		if v == "" {
			delete(m, k)
		} else {
			m[k] = v
		}
	}
	out := make([]string, 0, len(m))
	for k, v := range m {
		out = append(out, k+"="+v)
	}
	return out
}

type LocalRuntime struct{}

func NewLocal() *LocalRuntime { return &LocalRuntime{} }

func (r *LocalRuntime) Spawn(spec SubprocessSpawnSpec) *SubprocessHandle {
	stdoutR := &outputReader{}
	stderrR := &outputReader{}
	col := &collectedOutputs{stdout: stdoutR, stderr: stderrR}

	cmd := exec.Command(spec.Argv[0], spec.Argv[1:]...)
	cmd.Dir = spec.Cwd
	if spec.GraceMs <= 0 {
		spec.GraceMs = 2000
	}

	cmd.Env = mergeEnv(ScrubParentEnv(), spec.Env)

	if runtime.GOOS != "windows" {

		platformSetSysProcAttr(cmd)
	}

	var stdin io.WriteCloser
	if spec.StdinData != "" {
		cmd.Stdin = strings.NewReader(spec.StdinData)
	} else {
		stdin, _ = cmd.StdinPipe()
	}

	outPipe, _ := cmd.StdoutPipe()
	errPipe, _ := cmd.StderrPipe()

	startErr := cmd.Start()
	pid := -1
	if startErr == nil {
		pid = cmd.Process.Pid
	}

	done := make(chan SubprocessOutcome, 1)
	doneErr := make(chan error, 1)
	h := &SubprocessHandle{
		Pid:       pid,
		collected: col,
		done:      done,
		doneErr:   doneErr,
		terminateFn: func() {
			if cmd.Process == nil {
				return
			}
			killTree(cmd, spec.GraceMs)
		},
	}
	_ = stdin

	if startErr != nil {
		doneErr <- fmt.Errorf("subprocess: spawn %q: %w", spec.Argv[0], startErr)
		close(doneErr)
		return h
	}

	memOut := spec.StdoutMaxBytes
	if memOut <= 0 {
		memOut = 64 * 1024
	}
	memErr := spec.StderrMaxBytes
	if memErr <= 0 {
		memErr = 64 * 1024
	}

	var pipesWg sync.WaitGroup
	pipesWg.Add(2)
	go func() {
		defer pipesWg.Done()
		pipeToReader(outPipe, stdoutR, memOut, spec.SpillRoot)
	}()
	go func() {
		defer pipesWg.Done()
		pipeToReader(errPipe, stderrR, memErr, spec.SpillRoot)
	}()

	go func() {
		pipesWg.Wait()
		wErr := cmd.Wait()
		res := SubprocessOutcome{}
		if wErr == nil {
			res.ExitCode = 0
		} else if ee, ok := wErr.(*exec.ExitError); ok {
			res.ExitCode = ee.ExitCode()

			if sig := platformExitSignal(ee); sig != "" {
				res.Signal = sig
				res.ExitCode = -1
			}
		} else {
			doneErr <- wErr
			close(doneErr)
			return
		}
		done <- res
		close(done)
	}()
	return h
}

func pipeToReader(r io.Reader, rd *outputReader, memCap int, spillRoot string) {
	buf := make([]byte, 16*1024)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			rd.append(buf[:n], memCap, spillCapFor(spillRoot), spillRoot)
		}
		if err != nil {
			return
		}
	}
}

func spillCapFor(root string) int {
	if root == "" {
		return 0
	}
	return 8 << 20
}

func killTree(cmd *exec.Cmd, graceMs int) {
	pid := cmd.Process.Pid
	if pid <= 0 {
		return
	}
	if runtime.GOOS == "windows" {

		_ = exec.Command("taskkill", "/T", "/F", "/PID", fmt.Sprint(pid)).Run()
		return
	}

	_ = platformSignalGroup(-pid, syscallSIGTERM)
	grace := time.Duration(graceMs) * time.Millisecond
	time.Sleep(grace)
	_ = platformSignalGroup(-pid, syscallSIGKILL)
}
