//

package tests

import (
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/JopenChen/dsh-go/pkg/subprocess"
)

func envCheckArgv(varName string) []string {
	if runtime.GOOS == "windows" {

		return []string{"cmd", "/c", "if defined " + varName + " (echo PRESENT) else (echo EMPTY)"}
	}
	return []string{"sh", "-c", `if [ -z "$` + varName + `" ]; then echo EMPTY; else echo PRESENT; fi`}
}

func TestSubprocessScrubEnv(t *testing.T) {
	t.Setenv("DSH_TEST_FACT", "secret")
	rt := subprocess.NewLocal()
	h := rt.Spawn(subprocess.SubprocessSpawnSpec{
		Argv: envCheckArgv("DSH_TEST_FACT"),
		Cwd:  t.TempDir(),
	})
	out := h.Collected().Stdout()
	res := <-h.Done()
	if res.ExitCode != 0 {
		t.Fatalf("subprocess exit code should be 0, got %d", res.ExitCode)
	}
	co := out.ReadOutput(0)
	if !strings.Contains(co.Text, "EMPTY") {
		t.Fatalf("subprocess should read empty after DSH_* scrubbed: %q", co.Text)
	}
}

func TestSubprocessCollectedSpill(t *testing.T) {
	var argv []string
	if runtime.GOOS == "windows" {
		argv = []string{"cmd", "/c", "for /L %i in (1,1,3000) do @echo line of output no %i"}
	} else {
		argv = []string{"sh", "-c", "seq 1 3000"}
	}
	rt := subprocess.NewLocal()
	h := rt.Spawn(subprocess.SubprocessSpawnSpec{
		Argv:           argv,
		Cwd:            t.TempDir(),
		StdoutMaxBytes: 1024,
		SpillRoot:      t.TempDir(),
	})
	res := <-h.Done()
	if res.ExitCode != 0 {
		t.Fatalf("exit code should be 0, got %d", res.ExitCode)
	}
	co := h.Collected().Stdout().ReadOutput(0)
	if !co.Truncated {
		t.Fatal("output exceeding threshold should be truncated")
	}
	if co.SpillPath == "" {
		t.Fatal("spill path should be generated when spill enabled")
	}

	data := readSpill(t, co.SpillPath)
	if len(data) <= 1024 {
		t.Fatalf("spill should contain full stream (>1KB), got %d bytes", len(data))
	}
}

func readSpill(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read spill failed: %v", err)
	}
	return b
}

func TestSubprocessTerminate(t *testing.T) {
	var argv []string
	if runtime.GOOS == "windows" {
		argv = []string{"cmd", "/c", "ping -n 60 127.0.0.1 >nul"}
	} else {
		argv = []string{"sh", "-c", "sleep 1000"}
	}
	rt := subprocess.NewLocal()
	h := rt.Spawn(subprocess.SubprocessSpawnSpec{Argv: argv, Cwd: t.TempDir(), GraceMs: 100})
	h.Terminate()
	select {
	case res := <-h.Done():

		if res.ExitCode == 0 {
			t.Fatalf("exit code after termination should not be 0, got %+v", res)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Done did not resolve within timeout after terminate")
	}
}

func TestSubprocessOutcomeSuccess(t *testing.T) {
	var argv []string
	if runtime.GOOS == "windows" {
		argv = []string{"cmd", "/c", "exit 0"}
	} else {
		argv = []string{"sh", "-c", "exit 0"}
	}
	rt := subprocess.NewLocal()
	h := rt.Spawn(subprocess.SubprocessSpawnSpec{Argv: argv, Cwd: t.TempDir()})
	res := <-h.Done()
	if res.ExitCode != 0 {
		t.Fatalf("normal exit code should be 0, got %+v", res)
	}
}
