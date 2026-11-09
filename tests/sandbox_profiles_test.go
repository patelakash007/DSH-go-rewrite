//

package tests

import (
	"runtime"
	"strings"
	"testing"

	"github.com/JopenChen/dsh-go/pkg/sandbox"
)

func TestBwrapProfileReadOnly(t *testing.T) {
	policy := sandbox.SandboxPolicy{
		SandboxExecutionPolicy: sandbox.SandboxExecutionPolicy{
			Mode:          sandbox.ModeReadOnly,
			WorkspaceRoot: "/workspace",
		},
		Mode: sandbox.ConfinedReadOnly,
	}
	args := sandbox.BwrapProfileArgs(policy)

	expected := []string{"--ro-bind", "/", "/", "--dev", "/dev", "--unshare-pid", "--proc", "/proc", "--die-with-parent"}
	for i, e := range expected {
		if args[i] != e {
			t.Fatalf("arg %d should be %q, got %q", i, e, args[i])
		}
	}

	for _, a := range args {
		if a == "--bind" {
			t.Fatal("read-only should not contain --bind")
		}
	}
}

func TestBwrapProfileWorkspaceWrite(t *testing.T) {
	policy := sandbox.SandboxPolicy{
		SandboxExecutionPolicy: sandbox.SandboxExecutionPolicy{
			Mode:          sandbox.ModeWorkspaceWrite,
			WorkspaceRoot: "/workspace/app",
		},
		Mode: sandbox.ConfinedWorkspaceWrite,
	}
	args := sandbox.BwrapProfileArgs(policy)

	foundTmpfs := false
	foundBind := false
	for i, a := range args {
		if a == "--tmpfs" && i+1 < len(args) && args[i+1] == "/tmp" {
			foundTmpfs = true
		}
		if a == "--bind" && i+1 < len(args) && args[i+1] == "/workspace/app" {
			foundBind = true
		}
	}
	if !foundTmpfs {
		t.Fatal("workspace-write should contain --tmpfs /tmp")
	}
	if !foundBind {
		t.Fatal("workspace-write should contain --bind workspaceRoot")
	}
}

func TestLandlockProfileReadOnly(t *testing.T) {
	policy := sandbox.SandboxPolicy{
		SandboxExecutionPolicy: sandbox.SandboxExecutionPolicy{
			Mode:          sandbox.ModeReadOnly,
			WorkspaceRoot: "/workspace",
		},
		Mode: sandbox.ConfinedReadOnly,
	}
	args := sandbox.LandlockProfileArgs(policy)
	// read-only：readOnly=[/], readWrite=[/dev/null]

	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--read-only /") {
		t.Fatalf("should contain --read-only /, got: %s", joined)
	}
	if !strings.Contains(joined, "--read-write /dev/null") {
		t.Fatalf("should contain --read-write /dev/null, got: %s", joined)
	}

	if strings.Contains(joined, "--read-write /workspace") {
		t.Fatalf("read-only should not contain workspace in read-write, got: %s", joined)
	}
}

func TestLandlockProfileWorkspaceWrite(t *testing.T) {
	policy := sandbox.SandboxPolicy{
		SandboxExecutionPolicy: sandbox.SandboxExecutionPolicy{
			Mode:          sandbox.ModeWorkspaceWrite,
			WorkspaceRoot: "/workspace/app",
		},
		Mode: sandbox.ConfinedWorkspaceWrite,
	}
	args := sandbox.LandlockProfileArgs(policy)
	joined := strings.Join(args, " ")

	if !strings.Contains(joined, "--read-write /dev/null") {
		t.Fatalf("should contain --read-write /dev/null, got: %s", joined)
	}
	if !strings.Contains(joined, "--read-write /tmp") {
		t.Fatalf("should contain --read-write /tmp, got: %s", joined)
	}
	if !strings.Contains(joined, "--read-write /workspace/app") {
		t.Fatalf("should contain --read-write /workspace/app, got: %s", joined)
	}
}

func TestSeatbeltProfileReadOnly(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("seatbelt is only meaningful on macOS")
	}
	policy := sandbox.SandboxPolicy{
		SandboxExecutionPolicy: sandbox.SandboxExecutionPolicy{
			Mode:          sandbox.ModeReadOnly,
			WorkspaceRoot: "/workspace",
		},
		Mode: sandbox.ConfinedReadOnly,
	}
	args := sandbox.SeatbeltProfileArgs(policy)

	if args[0] != "-p" {
		t.Fatalf("first arg should be -p, got %q", args[0])
	}

	sbpl := args[1]
	if !strings.Contains(sbpl, "(version 1)") {
		t.Fatalf("SBPL should contain (version 1), got: %s", sbpl)
	}
	if !strings.Contains(sbpl, "(allow default)") {
		t.Fatalf("SBPL should contain (allow default), got: %s", sbpl)
	}
	if !strings.Contains(sbpl, "(deny file-write*)") {
		t.Fatalf("SBPL should contain (deny file-write*), got: %s", sbpl)
	}
	if !strings.Contains(sbpl, "/dev/null") {
		t.Fatalf("SBPL should contain /dev/null, got: %s", sbpl)
	}
}

func TestSeatbeltProfileWorkspaceWrite(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("seatbelt is only meaningful on macOS")
	}
	policy := sandbox.SandboxPolicy{
		SandboxExecutionPolicy: sandbox.SandboxExecutionPolicy{
			Mode:          sandbox.ModeWorkspaceWrite,
			WorkspaceRoot: "/workspace/app",
		},
		Mode: sandbox.ConfinedWorkspaceWrite,
	}
	args := sandbox.SeatbeltProfileArgs(policy)
	sbpl := args[1]

	if !strings.Contains(sbpl, "/workspace/app") {
		t.Fatalf("SBPL should contain workspaceRoot, got: %s", sbpl)
	}
	if !strings.Contains(sbpl, "(subpath") {
		t.Fatalf("SBPL should contain (subpath ...) authorization, got: %s", sbpl)
	}
}
