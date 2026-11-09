//

package tests

import (
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/JopenChen/dsh-go/pkg/shell"
)

func bigSeqCmd() string {
	if runtime.GOOS == "windows" {
		return "for /L %i in (1,1,20000) do @echo line %i"
	}
	return "seq 1 100000"
}

func TestBashSpillFullContent(t *testing.T) {
	s := shell.New("workspace-write")
	res, br, err := s.Run(shell.ExecRequest{Command: bigSeqCmd(), Cwd: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("normal command should have exitCode=0, got %+v", res)
	}
	if res.TimeoutMs != 0 {
		t.Fatalf("unset timeout should have timeoutMs=0, got %d", res.TimeoutMs)
	}
	if !br.Truncated {
		t.Fatal("large output should be truncated")
	}
	if br.SpillPath == "" {
		t.Fatal("truncated output should provide spillPath")
	}

	data, err := os.ReadFile(br.SpillPath)
	if err != nil {
		t.Fatalf("read spill failed: %v", err)
	}
	rows := strings.Count(string(data), "\n")
	want := 100000
	if runtime.GOOS == "windows" {
		want = 20000
	}
	if rows < want-10 {
		t.Fatalf("spill should contain around %d lines, got %d lines", want, rows)
	}

	formatted := shell.FormatOutput(&br)
	if !strings.Contains(formatted, "full output at") {
		t.Fatalf("formatted output should guide reading spill: %q", formatted)
	}
}

func TestBashTimeout(t *testing.T) {
	s := shell.New("workspace-write")
	var cmd string
	if runtime.GOOS == "windows" {
		cmd = "ping -n 30 127.0.0.1 >nul"
	} else {
		cmd = "sleep 30"
	}
	res, br, err := s.Run(shell.ExecRequest{Command: cmd, Cwd: t.TempDir(), TimeoutMs: 500})
	if err != nil {
		t.Fatal(err)
	}
	if !res.TimedOut {
		t.Fatalf("should have timedOut=true, got %+v", res)
	}
	if res.TimeoutMs != 500 {
		t.Fatalf("timeoutMs should record 500, got %d", res.TimeoutMs)
	}
	_ = br
}

func TestBashOutputLines(t *testing.T) {
	s := shell.New("workspace-write")
	var cmd string
	if runtime.GOOS == "windows" {
		cmd = "echo hello"
	} else {
		cmd = "echo hello"
	}
	_, br, err := s.Run(shell.ExecRequest{Command: cmd, Cwd: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if br.Truncated {
		t.Fatal("short output should not be truncated")
	}
	if !strings.Contains(br.Output, "hello") {
		t.Fatalf("output should contain hello, got %q", br.Output)
	}
}
