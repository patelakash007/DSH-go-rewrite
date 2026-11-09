//

package tests

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/JopenChen/dsh-go/pkg/skills"
)

func TestN04CatalogStable(t *testing.T) {
	root := t.TempDir()
	r := skills.New(root)
	for _, n := range []string{"b", "a", "c"} {
		writeSkill(t, filepath.Join(root, ".dsh", "skills"), n, "desc of "+n)
	}
	r.Scan()
	first := r.CatalogText()
	firstHash := r.CatalogHash()
	if first == "" {
		t.Fatal("CatalogText should not be empty")
	}
	for i := 0; i < 1000; i++ {
		if got := r.CatalogText(); got != first {
			t.Fatalf("CatalogText mismatch on attempt %d", i)
		}
		if got := r.CatalogHash(); got != firstHash {
			t.Fatalf("CatalogHash unstable")
		}
	}

	if indexOf(first, "a ") > indexOf(first, "b ") {
		t.Fatalf("catalog should be lexicographical by name: %q", first)
	}
}

func TestN04ChangeOnlyInject(t *testing.T) {
	root := t.TempDir()
	r := skills.New(root)
	proj := filepath.Join(root, ".dsh", "skills")
	writeSkill(t, proj, "alpha", "AAA")
	r.Scan()

	in := skills.NewInjector(r)

	content, injected := in.MaybeInject()
	if !injected || content == "" {
		t.Fatal("should inject on first round")
	}

	for i := 0; i < 50; i++ {
		if _, injected := in.MaybeInject(); injected {
			t.Fatalf("round %d should not inject when skills unchanged", i)
		}
	}
	if in.InjectCount() != 1 {
		t.Fatalf("should inject only once, got %d", in.InjectCount())
	}

	writeSkill(t, proj, "beta", "BBB")
	r.Scan()
	if _, injected := in.MaybeInject(); !injected {
		t.Fatal("should re-inject after adding skill")
	}
	if in.InjectCount() != 2 {
		t.Fatalf("should have two injections, got %d", in.InjectCount())
	}

	if err := os.Remove(filepath.Join(proj, "alpha.md")); err != nil {
		t.Fatal(err)
	}
	r.Scan()
	if _, injected := in.MaybeInject(); !injected {
		t.Fatal("should re-inject after removing skill")
	}
}

func TestN04InjectorBlockedInAvailableSkills(t *testing.T) {
	root := t.TempDir()
	r := skills.New(root)
	writeSkill(t, filepath.Join(root, ".dsh", "skills"), "refactor", "tool tips")
	r.Scan()
	in := skills.NewInjector(r)
	content, _ := in.MaybeInject()
	if indexOf(content, "<available_skills>") < 0 || indexOf(content, "</available_skills>") < 0 {
		t.Fatalf("injection should contain <available_skills> block: %q", content)
	}
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
