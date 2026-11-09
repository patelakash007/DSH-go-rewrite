//

package tests

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/JopenChen/dsh-go/pkg/sandbox"
)

func TestCanonicalPathExisting(t *testing.T) {
	dir := t.TempDir()
	canonical := sandbox.CanonicalPath(dir)
	if canonical == "" {
		t.Fatal("canonicalPath should not return empty")
	}

	if _, err := os.Stat(canonical); err != nil {
		t.Fatalf("canonicalized path should exist: %v", err)
	}
}

func TestCanonicalPathMissing(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nonexistent", "path")
	result := sandbox.CanonicalPath(missing)
	if result != missing {
		t.Fatalf("non-existent path should return original path, want %q, got %q", missing, result)
	}
}

func TestWritableRootsReadOnly(t *testing.T) {
	policy := sandbox.SandboxExecutionPolicy{
		Mode:          sandbox.ModeReadOnly,
		WorkspaceRoot: "/workspace/app",
	}
	roots := sandbox.WritableRoots(policy)
	if len(roots) != 0 {
		t.Fatalf("read-only mode should return empty list, got %v", roots)
	}
}

func TestWritableRootsDanger(t *testing.T) {
	policy := sandbox.SandboxExecutionPolicy{
		Mode:          sandbox.ModeDangerFullAccess,
		WorkspaceRoot: "/workspace/app",
	}
	roots := sandbox.WritableRoots(policy)
	if len(roots) != 0 {
		t.Fatalf("danger mode should return empty list, got %v", roots)
	}
}

func TestWritableRootsWorkspaceWrite(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("/tmp semantics differ on Windows, skipping")
	}
	policy := sandbox.SandboxExecutionPolicy{
		Mode:          sandbox.ModeWorkspaceWrite,
		WorkspaceRoot: "/workspace/app",
	}
	roots := sandbox.WritableRoots(policy)
	if len(roots) < 2 {
		t.Fatalf("workspace-write should return at least workspaceRoot and tmpdir, got %v", roots)
	}

	foundWorkspace := false
	for _, r := range roots {
		if r == "/workspace/app" {
			foundWorkspace = true
		}
	}
	if !foundWorkspace {
		t.Fatalf("should contain workspaceRoot /workspace/app, got %v", roots)
	}
}

func TestWritableRootsDeduplication(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skip on Windows")
	}

	policy := sandbox.SandboxExecutionPolicy{
		Mode:          sandbox.ModeWorkspaceWrite,
		WorkspaceRoot: "/tmp",
	}
	roots := sandbox.WritableRoots(policy)
	seen := map[string]bool{}
	for _, r := range roots {
		if seen[r] {
			t.Fatalf("duplicate path: %s, all %v", r, roots)
		}
		seen[r] = true
	}
}

func TestWritableRootsCanonical(t *testing.T) {
	dir := t.TempDir()
	policy := sandbox.SandboxExecutionPolicy{
		Mode:          sandbox.ModeWorkspaceWrite,
		WorkspaceRoot: dir,
	}
	roots := sandbox.WritableRoots(policy)

	found := false
	for _, r := range roots {
		if r == sandbox.CanonicalPath(dir) {
			found = true
		}
	}
	if !found {
		t.Fatalf("should contain canonicalized workspaceRoot, got %v", roots)
	}
}
