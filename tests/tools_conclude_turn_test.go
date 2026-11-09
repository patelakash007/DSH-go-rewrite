package tests

import (
	"testing"

	"github.com/JopenChen/dsh-go/pkg/tools"
)

func TestToolsConcludeTurn(t *testing.T) {
	concluded := ""
	tc := tools.NewToolRunContext(func(reason string) { concluded = reason })

	if !tc.ConcludeTurn("goal-blocker") {
		t.Fatal("first conclude should return true")
	}
	if concluded != "goal-blocker" {
		t.Fatalf("conclude reason = %q, want goal-blocker", concluded)
	}
	if !tc.IsConcluded() {
		t.Fatal("IsConcluded should be true after conclude")
	}

	if tc.ConcludeTurn("again") {
		t.Fatal("duplicate conclude should return false")
	}
}

func TestToolsDeferContext(t *testing.T) {
	var order []string
	tc := tools.NewToolRunContext(nil)

	tc.DeferContext(func() { order = append(order, "d1") })
	tc.DeferContext(func() { order = append(order, "d2") })

	_ = tc.ConcludeTurn("cleanup")

	if len(order) != 2 || order[0] != "d2" || order[1] != "d1" {
		t.Fatalf("deferred should execute in LIFO order: %v", order)
	}
}

func TestToolsRunContextMeta(t *testing.T) {
	tc := tools.NewToolRunContext(nil)
	tc.SetMeta("started_ms", 100)
	if v, ok := tc.GetMeta("started_ms"); !ok || v != 100 {
		t.Fatalf("meta read/write unexpected: %v %v", v, ok)
	}
}

func TestToolsConcludeContextPropagation(t *testing.T) {
	called := false
	tc := tools.NewToolRunContext(func(string) { called = true })

	ctx := tc.Context()
	got, ok := tools.ToolRunContextFrom(ctx)
	if !ok {
		t.Fatal("should be able to get ToolRunContext from context")
	}
	got.ConcludeTurn("x")
	if !called {
		t.Fatal("conclude callback should propagate via context and trigger")
	}
}
