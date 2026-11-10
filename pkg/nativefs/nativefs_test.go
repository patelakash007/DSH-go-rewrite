package nativefs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveConfinesTraversalAndSymlinks(t *testing.T) {
	root := t.TempDir()
	service, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Resolve(root, "../outside"); Code(err) != ErrNotAllowed {
		t.Fatalf("traversal error = %v, want %s", err, ErrNotAllowed)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Resolve(root, "escape/file"); Code(err) != ErrNotAllowed {
		t.Fatalf("symlink escape error = %v, want %s", err, ErrNotAllowed)
	}
}

func TestLegacyWorkspaceMappingAndReadWindows(t *testing.T) {
	root := t.TempDir()
	service, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "hello.txt"), []byte("one\ntwo\nthree\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	resolved, err := service.Resolve("/root/workspace", "/root/workspace/hello.txt")
	if err != nil || resolved != filepath.Join(root, "hello.txt") {
		t.Fatalf("legacy resolve = %q, %v", resolved, err)
	}
	page, err := service.ReadText(root, "hello.txt", 2, 1, DefaultMaxTextBytes)
	if err != nil {
		t.Fatal(err)
	}
	if page.Text != "two" || page.Lines != 1 || page.EOF {
		t.Fatalf("page = %+v", page)
	}
	window, err := service.ReadWindow(root, "hello.txt", 4, 3)
	if err != nil {
		t.Fatal(err)
	}
	if string(window.Data) != "two" || window.EOF {
		t.Fatalf("window = %+v", window)
	}
}

func TestAtomicWriteAndEdit(t *testing.T) {
	service, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	first, err := service.WriteText(service.Root(), "nested/file.txt", "alpha beta alpha")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.EditText(service.Root(), "nested/file.txt", "alpha", "x", false); Code(err) != ErrInvalid {
		t.Fatalf("ambiguous edit = %v", err)
	}
	second, err := service.EditText(service.Root(), "nested/file.txt", "alpha", "x", true)
	if err != nil {
		t.Fatal(err)
	}
	if first.Version == second.Version {
		t.Fatal("version did not change")
	}
	data, _, err := service.ReadWhole(service.Root(), "nested/file.txt", 1024)
	if err != nil || string(data) != "x beta x" {
		t.Fatalf("edited data = %q, %v", data, err)
	}
}
