//

package tests

import (
	"testing"

	"github.com/JopenChen/dsh-go/pkg/sysprompt"
)

func TestN03SystemPromptPureFunction(t *testing.T) {
	a := sysprompt.New()
	a.Register("persona", sysprompt.SectionOrderPersona, "You are a helpful agent.")
	a.Register("policy", sysprompt.SectionOrderPolicy, "Follow plan mode policy.")
	a.Register("tools", sysprompt.SectionOrderToolsSchema, "# tools schema")

	first := a.Assemble()
	firstHash := a.AssembleHash()
	for i := 0; i < 1000; i++ {
		if got := a.Assemble(); got != first {
			t.Fatalf("render mismatch on attempt %d (pure function violated)", i)
		}
		if got := a.AssembleHash(); got != firstHash {
			t.Fatalf("hash unstable on attempt %d: %s != %s", i, got, firstHash)
		}
	}
}

func TestN03StaticCheckCatchesBanned(t *testing.T) {
	bad := `Today is ` + "`time.Now()`" + ` in $(os.Getwd())`

	dirty := "const now = time.Now(); const cwd = os.Getwd(); use math/rand"
	violations := sysprompt.StaticCheck(dirty)
	if len(violations) == 0 {
		t.Fatalf("should catch time.Now/os.Getwd/rand anti-patterns")
	}
	if sysprompt.IsPure(dirty) {
		t.Fatal("text with anti-patterns should not be pure function")
	}
	_ = bad
}

func TestN03StaticCheckClean(t *testing.T) {
	clean := "You are a helpful assistant. Follow the workspace policy."
	if v := sysprompt.StaticCheck(clean); len(v) != 0 {
		t.Fatalf("clean text should have no violations: %v", v)
	}
	if !sysprompt.IsPure(clean) {
		t.Fatal("clean text should be pure function")
	}
}

func TestN03CheckSections(t *testing.T) {
	a := sysprompt.New()
	a.Register("persona", sysprompt.SectionOrderPersona, "You are helpful.")
	a.Register("bad", sysprompt.SectionOrderPolicy, "os.Gos.Getwd() dynamic")
	_, forbidden := sysprompt.CheckSections(a)
	_ = forbidden

	if !forbidden {
		t.Fatal("section with anti-patterns should be forbidden")
	}

	a.Unregister("bad")
	viol, forb := sysprompt.CheckSections(a)
	if len(viol) != 0 || forb {
		t.Fatalf("should have 0 violations after removal: %v", viol)
	}
}

func TestN03OrderConst(t *testing.T) {
	if sysprompt.SectionOrderPersona >= sysprompt.SectionOrderPolicy {
		t.Fatal("persona(100) should be less than policy(200)")
	}

	a := sysprompt.New()
	a.Register("policy", sysprompt.SectionOrderPolicy, "p")
	s := a.Sections()[0]
	if s.Order != sysprompt.SectionOrderPolicy {
		t.Fatalf("order should come from constant 200, got %d", s.Order)
	}
}
