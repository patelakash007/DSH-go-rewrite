//

package tests

import (
	"context"
	"testing"
	"time"

	"github.com/JopenChen/dsh-go/pkg/brand"
	"github.com/JopenChen/dsh-go/pkg/subagent"
)

func TestSubagentInProcessForkLineage(t *testing.T) {
	ctx := context.Background()
	parent := brand.NewSessionID("parent-1")
	captured := ""

	r := subagent.NewRuntime()

	rp := subagent.NewInProcessProvider(func(_ context.Context, req subagent.SpawnRequest) (brand.SessionID, string, error) {
		sid := brand.NewSessionID("child-1")
		captured = "parent="
		if req.Parent != nil {
			captured += req.Parent.Raw()
		}
		captured += "/session=" + sid.Raw()
		return sid, "Subtask finished", nil
	})
	_ = r.RegisterProvider(rp)

	h, err := r.Spawn(ctx, "in-process", subagent.SpawnRequest{
		Parent: &parent, Input: "fork me",
		MaxRounds: 3,
	})
	if err != nil {
		t.Fatal(err)
	}

	if h.Lineage.Parent == nil || *h.Lineage.Parent != parent {
		t.Fatalf("lineage parent should be %s, got %v", parent, h.Lineage.Parent)
	}
	if h.Lineage.Session.Raw() != "child-1" {
		t.Fatalf("child session should be child-1, got %s", h.Lineage.Session)
	}
	if captured != "parent=parent-1/session=child-1" {
		t.Fatalf("lineage context received by runner unexpected: %q", captured)
	}

	if err := h.Drain(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestSubagentParentDisposeDrainsChild(t *testing.T) {
	ctx := context.Background()
	parent := brand.NewSessionID("parent-2")

	r := subagent.NewRuntime()
	rp := subagent.NewInProcessProvider(nil)
	_ = r.RegisterProvider(rp)

	h, err := r.Spawn(ctx, "in-process", subagent.SpawnRequest{Parent: &parent, Input: "t"})
	if err != nil {
		t.Fatal(err)
	}

	r.DisposeOwner(parent)

	done := make(chan error, 1)
	go func() { done <- h.Drain(ctx) }()
	select {
	case <-done:

		_ = h.Drain(ctx)
	case <-time.After(2 * time.Second):
		t.Fatal("child handle should drain after parent dispose, timed out")
	}
}

func TestSubagentUnknownProvider(t *testing.T) {
	r := subagent.NewRuntime()
	_, err := r.Spawn(context.Background(), "no-such", subagent.SpawnRequest{Input: "x"})
	if !subagent.IsUnknownProvider(err) {
		t.Fatalf("unknown backend should report ErrUnknownProvider, got %v", err)
	}
}

func TestSubagentBuiltinStubs(t *testing.T) {
	r := subagent.NewRuntime()
	for _, name := range []string{"acp", "fork-copy"} {
		h, err := r.Spawn(context.Background(), name, subagent.SpawnRequest{Input: "s"})
		if err != nil {
			t.Fatalf("%s stub spawn failed: %v", name, err)
		}
		if h.Provider != name {
			t.Fatalf("%s handle provider should be %s, got %s", name, name, h.Provider)
		}
	}

	if r.ActiveCount() != 2 {
		t.Fatalf("should have 2 registered handles, got %d", r.ActiveCount())
	}
}
