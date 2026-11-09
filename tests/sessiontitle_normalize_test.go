package tests

import (
	"testing"

	"github.com/JopenChen/dsh-go/pkg/sessiontitle"
)

func TestCleanTitle(t *testing.T) {
	in := "\x1b[31ma\x1b[0m   b\x00c\t d"
	got := sessiontitle.CleanTitle(in)

	if got != "a bc d" {
		t.Fatalf("clean = %q", got)
	}
}

func TestTruncateUTF8(t *testing.T) {

	if got := sessiontitle.TruncateUTF8("hello 🌍", 8); got != "hello " {
		t.Fatalf("truncate = %q", got)
	}
}

func TestNormalizeTitle(t *testing.T) {
	if got := sessiontitle.Normalize("  hello   world  ", 100); got != "hello world" {
		t.Fatalf("normalize = %q", got)
	}
}
