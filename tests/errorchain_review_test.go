//

//

package tests

import (
	"errors"
	"fmt"
	"testing"

	"github.com/JopenChen/dsh-go/pkg/llm"
)

type simpleErr struct{}

func (simpleErr) Error() string { return "" }

type llmFailureMessage struct{ msg string }

func (e *llmFailureMessage) Error() string { return e.msg }
func (e *llmFailureMessage) Unwrap() error { return errors.New(e.msg) }

type circularA struct{ next error }

func (circularA) Error() string   { return "A" }
func (c circularA) Unwrap() error { return c.next }

type circularB struct{ next error }

func (circularB) Error() string   { return "B" }
func (c circularB) Unwrap() error { return c.next }

func TestR05SingleCauseChain(t *testing.T) {
	base := errors.New("base failure")
	wrapped := fmt.Errorf("wrap: %w", base)
	got := llm.RenderErrorChain(wrapped)

	if got != "wrap: base failure: base failure" {
		t.Fatalf("single cause chain = %q, want %q", got, "wrap: base failure: base failure")
	}
}

func TestR05RepeatedCauseSkipped(t *testing.T) {
	e := &llmFailureMessage{msg: "dup"}
	got := llm.RenderErrorChain(e)

	if got != "dup" {
		t.Fatalf("repeated cause should be suppressed, got %q", got)
	}
}

func TestR05Aggregate(t *testing.T) {
	m := errors.Join(errors.New("a"), errors.New("b"), errors.New("c"))
	got := llm.RenderErrorChain(m)

	if !contains(got, "[a; b; c]") {
		t.Fatalf("aggregate should contain parenthesized group, got %q", got)
	}
}

func TestR05MessageFieldAndStr(t *testing.T) {

	if got := llm.RenderErrorChain(struct{ Message string }{Message: "hi msg"}); got != "hi msg" {
		t.Fatalf("message field = %q", got)
	}

	if got := llm.RenderErrorChain(42); got != "42" {
		t.Fatalf("plain value = %q", got)
	}
}

func TestR05CircularAndEmpty(t *testing.T) {

	var a circularA
	var b circularB
	a.next = &b
	b.next = &a
	got := llm.RenderErrorChain(&a)
	if !contains(got, "<circular cause>") {
		t.Fatalf("circular should contain <circular cause>, got %q", got)
	}

	if got := llm.RenderErrorChain(simpleErr{}); got != "simpleErr" {
		t.Fatalf("empty message should fall back to type name, got %q", got)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
