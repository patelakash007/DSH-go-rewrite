package tests

import (
	"strings"
	"testing"

	"github.com/JopenChen/dsh-go/pkg/mcp"
)

func TestPublicNameClean(t *testing.T) {
	got := mcp.PublicToolName("srv", "get_x")
	if got != "mcp__srv__get_x" {
		t.Fatalf("clean name = %q", got)
	}
}

func TestPublicNameInvalidChar(t *testing.T) {
	got := mcp.PublicToolName("srv", "a.b")

	if !strings.HasPrefix(got, "mcp__srv__a_b_") {
		t.Fatalf("normalized name = %q", got)
	}
	if len(got) > mcp.MaxPublicNameLength {
		t.Fatal("name within length bound")
	}
}

func TestPublicNameDistinctHash(t *testing.T) {

	a := mcp.PublicToolName("s", "a.b")
	b := mcp.PublicToolName("s", "a/b")
	if a == b {
		t.Fatalf("distinct identities must not collapse: %q", a)
	}
}
