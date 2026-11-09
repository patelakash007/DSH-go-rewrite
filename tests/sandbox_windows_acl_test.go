//go:build windows
// +build windows

package tests

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/JopenChen/dsh-go/pkg/sandbox"
)

func TestWindowsACLProviderAvailable(t *testing.T) {
	p := sandbox.NewWindowsACLProvider(t.TempDir())
	if !p.Available() {
		t.Fatal("Windows ACL backend should be available on Windows")
	}
}

func TestWindowsACLProviderConfineReadOnly(t *testing.T) {
	p := sandbox.NewWindowsACLProvider(t.TempDir())
	policy := sandbox.SandboxPolicy{
		SandboxExecutionPolicy: sandbox.SandboxExecutionPolicy{
			Mode:          sandbox.ModeReadOnly,
			WorkspaceRoot: t.TempDir(),
		},
		Mode: sandbox.ConfinedReadOnly,
	}
	result, err := p.Confine([]string{"echo", "hello"}, policy)
	if err != nil {
		t.Fatalf("Confine should not error: %v", err)
	}

	if result.Enforcement != sandbox.EnforcementFull && result.Enforcement != sandbox.EnforcementPartial {
		t.Fatalf("enforcement should be full or partial, got %s", result.Enforcement)
	}

	if len(result.Argv) != 2 || result.Argv[0] != "echo" || result.Argv[1] != "hello" {
		t.Fatalf("argv should remain unchanged, got %v", result.Argv)
	}
}

func TestWindowsACLProviderCreateRestrictedToken(t *testing.T) {
	p := sandbox.NewWindowsACLProvider(t.TempDir())
	token, err := p.CreateRestrictedToken()
	if err != nil {
		t.Fatalf("CreateRestrictedToken should not error: %v", err)
	}

	token2, err := p.CreateRestrictedToken()
	if err != nil {
		t.Fatalf("second CreateRestrictedToken should not error: %v", err)
	}
	if token != token2 {
		t.Fatal("should return cached token")
	}
}

func TestWindowsACLProviderSetupTempDir(t *testing.T) {
	p := sandbox.NewWindowsACLProvider(t.TempDir())
	tempDir, err := p.SetupTempDir()
	if err != nil {
		t.Fatalf("SetupTempDir should not error: %v", err)
	}
	if tempDir == "" {
		t.Fatal("temp directory should not be empty")
	}

	if _, err := os.Stat(tempDir); err != nil {
		t.Fatalf("temp directory should exist: %v", err)
	}

	tempDir2, err := p.SetupTempDir()
	if err != nil {
		t.Fatalf("second SetupTempDir should not error: %v", err)
	}
	if tempDir != tempDir2 {
		t.Fatal("should return cached temp directory")
	}
}

func TestWindowsACLProviderGrantWorkspaceAccess(t *testing.T) {
	dir := t.TempDir()
	p := sandbox.NewWindowsACLProvider(dir)
	err := p.GrantWorkspaceAccess(dir)
	if err != nil {
		t.Fatalf("GrantWorkspaceAccess should not error: %v", err)
	}

	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("directory should exist: %v", err)
	}
}

func TestWindowsACLProviderCleanup(t *testing.T) {
	p := sandbox.NewWindowsACLProvider(t.TempDir())

	if _, err := p.CreateRestrictedToken(); err != nil {
		t.Fatalf("CreateRestrictedToken: %v", err)
	}
	tempDir, err := p.SetupTempDir()
	if err != nil {
		t.Fatalf("SetupTempDir: %v", err)
	}

	if err := p.Cleanup(); err != nil {
		t.Fatalf("Cleanup should not error: %v", err)
	}

	if _, err := os.Stat(tempDir); !os.IsNotExist(err) {
		t.Fatalf("temp dir should be deleted, but still exists: %v", err)
	}
}

func TestWindowsACLProviderSpawnWithToken(t *testing.T) {
	p := sandbox.NewWindowsACLProvider(t.TempDir())
	workDir := t.TempDir()

	outputFile := filepath.Join(workDir, "output.txt")

	proc, err := p.SpawnWithToken(
		[]string{"cmd", "/c", "echo", "hello", ">", outputFile},
		nil,
		workDir,
	)
	if err != nil {
		t.Skipf("SpawnWithToken may require special privileges, skipping: %v", err)
	}
	if proc == nil {
		t.Fatal("process should not be nil")
	}

	state, err := proc.Wait()
	if err != nil {
		t.Fatalf("Wait should not error: %v", err)
	}
	if !state.Success() {
		t.Fatalf("process should exit successfully, got %v", state)
	}
}
