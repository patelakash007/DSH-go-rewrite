package sysprompt

import "testing"

func TestChangeOnlyInjectorCleared(t *testing.T) {
	reg := NewContextRegistry()
	inj := NewChangeOnlyInjector(reg, "")

	if _, ok := inj.MightInject(); ok {
		t.Fatal("empty first time must not inject")
	}

	reg.Register("a", 10, "hello")
	text, ok := inj.MightInject()
	if !ok || text != "hello" {
		t.Fatalf("expected hello, got %q ok=%v", text, ok)
	}

	reg.Unregister("a")
	text, ok = inj.MightInject()
	if !ok || text != ClearedContextText {
		t.Fatalf("expected cleared marker, got %q ok=%v", text, ok)
	}
}
