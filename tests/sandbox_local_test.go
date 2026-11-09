package tests

import (
	"runtime"
	"testing"

	"github.com/JopenChen/dsh-go/pkg/sandbox"
)

func TestLocalProviderBackendType(t *testing.T) {
	if string(sandbox.BackendBwrap) != "bwrap" {
		t.Fatal("BackendBwrap should be bwrap")
	}
	if string(sandbox.BackendLandlock) != "landlock" {
		t.Fatal("BackendLandlock should be landlock")
	}
	if string(sandbox.BackendSeatbelt) != "seatbelt" {
		t.Fatal("BackendSeatbelt should be seatbelt")
	}
	if string(sandbox.BackendWindowsACL) != "windows-acl" {
		t.Fatal("BackendWindowsACL should be windows-acl")
	}
}

func TestLocalProviderConfig(t *testing.T) {
	p := sandbox.NewLocalProvider(&sandbox.LocalConfig{
		RunnerCommand: "custom-runner",
		BackendType:   sandbox.BackendBwrap,
	})
	if p.Backend() != sandbox.BackendBwrap {
		t.Fatalf("should force bwrap, got %s", p.Backend())
	}
	if p.Runner() != "custom-runner" {
		t.Fatalf("should use custom-runner, got %s", p.Runner())
	}
}

func TestLocalProviderAvailable(t *testing.T) {
	p := sandbox.NewLocalProvider(nil)

	if runtime.GOOS == "windows" {
		if !p.Available() {
			t.Fatal("windows-acl backend should be available on Windows")
		}
		if p.Backend() != sandbox.BackendWindowsACL {
			t.Fatalf("windows-acl should be selected on Windows, got %s", p.Backend())
		}
	}
}

func TestLocalProviderConfineWindowsACL(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("test windows-acl only on Windows")
	}
	p := sandbox.NewLocalProvider(nil)
	policy := sandbox.SandboxPolicy{
		SandboxExecutionPolicy: sandbox.SandboxExecutionPolicy{
			Mode:          sandbox.ModeReadOnly,
			WorkspaceRoot: "/workspace",
		},
		Mode: sandbox.ConfinedReadOnly,
	}
	result, err := p.Confine([]string{"echo", "hello"}, policy)
	if err != nil {
		t.Fatalf("Confine should not error: %v", err)
	}
	if result.Enforcement != sandbox.EnforcementFull {
		t.Fatalf("enforcement should be full, got %s", result.Enforcement)
	}
	if p.Backend() != sandbox.BackendWindowsACL {
		t.Fatalf("backend should be windows-acl, got %s", p.Backend())
	}
}

func TestLocalProviderConfineCustomBwrap(t *testing.T) {
	p := sandbox.NewLocalProvider(&sandbox.LocalConfig{
		RunnerCommand: "bwrap",
		BackendType:   sandbox.BackendBwrap,
	})
	policy := sandbox.SandboxPolicy{
		SandboxExecutionPolicy: sandbox.SandboxExecutionPolicy{
			Mode:          sandbox.ModeReadOnly,
			WorkspaceRoot: "/workspace",
		},
		Mode: sandbox.ConfinedReadOnly,
	}
	result, err := p.Confine([]string{"echo", "hello"}, policy)
	if err != nil {
		t.Fatalf("Confine should not error: %v", err)
	}

	if result.Argv[0] != "bwrap" {
		t.Fatalf("argv[0] should be bwrap, got %s", result.Argv[0])
	}

	foundSep := false
	for _, a := range result.Argv {
		if a == "--" {
			foundSep = true
		}
	}
	if !foundSep {
		t.Fatal("argv should contain -- separator")
	}

	if result.Argv[len(result.Argv)-2] != "echo" || result.Argv[len(result.Argv)-1] != "hello" {
		t.Fatalf("argv should end with echo hello, got %v", result.Argv)
	}
}

func TestLocalProviderDenialSignatures(t *testing.T) {
	p := sandbox.NewLocalProvider(&sandbox.LocalConfig{
		RunnerCommand: "bwrap",
		BackendType:   sandbox.BackendBwrap,
	})
	sigs := p.DenialSignatures()
	if len(sigs) == 0 {
		t.Fatal("bwrap should have rejection signature")
	}

	found := false
	for _, s := range sigs {
		if s == "Read-only file system" {
			found = true
		}
	}
	if !found {
		t.Fatalf("bwrap should contain 'Read-only file system' signature, got %v", sigs)
	}
}
