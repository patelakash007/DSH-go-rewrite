package nativetools

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/JopenChen/dsh-go/pkg/nativefs"
)

func TestRegistryFileTools(t *testing.T) {
	files, err := nativefs.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	registry := New(files, files.Root)
	write := registry.Lookup("write_file")
	if _, err := write.Execute(context.Background(), map[string]any{"path": "note.txt", "content": "hello native\nsecond"}); err != nil {
		t.Fatal(err)
	}
	read := registry.Lookup("read_file")
	value, err := read.Execute(context.Background(), map[string]any{"path": "note.txt"})
	if err != nil {
		t.Fatal(err)
	}
	page := value.(nativefs.TextPage)
	if page.Text != "hello native\nsecond" {
		t.Fatalf("read = %q", page.Text)
	}
	search := registry.Lookup("search_files")
	value, err = search.Execute(context.Background(), map[string]any{"query": "native"})
	if err != nil {
		t.Fatal(err)
	}
	if len(value.(map[string]any)["matches"].([]SearchMatch)) != 1 {
		t.Fatalf("search = %#v", value)
	}
}

func TestShellUsesWorkspace(t *testing.T) {
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skip("system shell unavailable")
	}
	files, err := nativefs.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	registry := New(files, files.Root)
	value, err := registry.Lookup("shell").Execute(context.Background(), map[string]any{
		"command": "printf native > shell.txt && pwd", "timeoutMs": 5_000,
	})
	if err != nil {
		t.Fatal(err)
	}
	result := value.(map[string]any)
	if result["exitCode"] != 0 {
		t.Fatalf("shell = %#v", result)
	}
	data, err := os.ReadFile(filepath.Join(files.Root(), "shell.txt"))
	if err != nil || string(data) != "native" {
		t.Fatalf("shell output file = %q, %v", data, err)
	}
}
