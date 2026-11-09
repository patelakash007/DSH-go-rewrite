//

//

package shell

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/JopenChen/dsh-go/pkg/subprocess"
)

// ============================================================================

// ============================================================================

type ExecRequest struct {
	Command string

	Cwd string

	TimeoutMs int

	Env map[string]string
}

type ExecSpec struct {
	Argv []string

	Cwd string

	TimeoutMs int

	Env map[string]string

	SandboxMode string
}

type RunResult struct {
	ExitCode int    `json:"exitCode"`
	Signal   string `json:"signal,omitempty"`

	TimedOut bool `json:"timedOut"`

	Aborted bool `json:"aborted"`

	TimeoutMs int `json:"timeoutMs"`
}

type BashResult struct {
	Output string `json:"output"`

	Truncated bool `json:"truncated"`

	SpillPath string `json:"spillPath,omitempty"`

	Result RunResult `json:"result"`
}

// ============================================================================

// ============================================================================

type Shell struct {
	rt      *subprocess.LocalRuntime
	sandbox string
}

func New(sandboxMode string) *Shell {
	mode := sandboxMode
	if mode == "" {
		mode = "workspace-write"
	}
	return &Shell{rt: subprocess.NewLocal(), sandbox: mode}
}

func (s *Shell) Resolve(req ExecRequest) ExecSpec {
	argv := shellArgv(req.Command)
	timeout := req.TimeoutMs
	if timeout < 0 {
		timeout = 0
	}
	return ExecSpec{
		Argv:        argv,
		Cwd:         req.Cwd,
		TimeoutMs:   timeout,
		Env:         req.Env,
		SandboxMode: s.sandbox,
	}
}

func shellArgv(cmd string) []string {
	switch runtime.GOOS {
	case "windows":
		return []string{"cmd", "/c", cmd}
	default:
		return []string{"bash", "-c", cmd}
	}
}

func (s *Shell) Run(req ExecRequest) (RunResult, BashResult, error) {
	spec := s.Resolve(req)
	spillRoot := tmpDir()

	memCap := 64 * 1024
	h := s.rt.Spawn(subprocess.SubprocessSpawnSpec{
		Argv:           spec.Argv,
		Cwd:            spec.Cwd,
		StdoutMaxBytes: memCap,
		SpillRoot:      spillRoot,
		GraceMs:        500,
		Env:            spec.Env,
	})

	var res RunResult
	res.TimeoutMs = spec.TimeoutMs
	timedCh := make(chan struct{})
	if spec.TimeoutMs > 0 {
		time.AfterFunc(time.Duration(spec.TimeoutMs)*time.Millisecond, func() { close(timedCh) })
	}

	var outcome subprocess.SubprocessOutcome
	select {
	case outcome = <-h.Done():
		res.ExitCode = outcome.ExitCode
		res.Signal = outcome.Signal
	case <-timedCh:

		h.Terminate()
		res.TimedOut = true
		res.ExitCode = -1
		<-h.Done()
	}

	co := h.Collected().Stdout().ReadOutput(0)
	br := BashResult{
		Output:    co.Text,
		Truncated: co.Truncated,
		SpillPath: co.SpillPath,
		Result:    res,
	}
	return res, br, nil
}

func tmpDir() string {
	dir, err := os.MkdirTemp("", "dsh-shell-spill-*")
	if err != nil {
		return os.TempDir()
	}
	return dir
}

// ============================================================================

// ============================================================================

type BashTool struct {
	shell *Shell
}

func NewBashTool(sandboxMode string) *BashTool {
	return &BashTool{shell: New(sandboxMode)}
}

func (b *BashTool) Exec(req ExecRequest) (*BashResult, error) {
	_, br, err := b.shell.Run(req)
	if err != nil {
		return nil, err
	}
	return &br, nil
}

func FormatOutput(r *BashResult) string {
	if !r.Truncated {
		return r.Output
	}
	return fmt.Sprintf("%s\n...[truncated; full output at %s]", strings.TrimRight(r.Output, "\n"), r.SpillPath)
}
