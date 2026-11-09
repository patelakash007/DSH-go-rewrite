package tests

import (
	"context"
	"testing"

	"github.com/JopenChen/dsh-go/pkg/brand"
	"github.com/JopenChen/dsh-go/pkg/persistence"
	"github.com/JopenChen/dsh-go/pkg/session"
)

func TestPersistenceCrashRepair(t *testing.T) {
	dir := t.TempDir()
	backend, err := persistence.NewJSONL(dir, 1)
	if err != nil {
		t.Fatalf("NewJSONL failed: %v", err)
	}
	ctx := context.Background()

	id := brand.NewSessionID("crash_s")
	header := session.NewSessionHeader(id, "/ws")
	if err := backend.SaveHeader(ctx, header); err != nil {
		t.Fatalf("SaveHeader failed: %v", err)
	}

	events := []session.SessionEvent{
		{Seq: 1, Type: session.EventTurnStart, Data: session.TurnStartData{Turn: 0}},
		{Seq: 2, Type: session.EventStepStart, Data: session.StepStartData{Turn: 0, Step: 1}},
		{Seq: 3, Type: session.EventUserMessage, Data: session.UserMessageData{Content: "hi"}},
		{Seq: 4, Type: session.EventToolCall, Data: session.ToolCallData{CallID: brand.NewToolCallID("call_1"), Tool: "bash"}},
	}
	for i := range events {
		events[i].Time = fixedTestTime()
		if err := backend.Append(ctx, id, events[i]); err != nil {
			t.Fatalf("Append failed: %v", err)
		}
	}
	if err := backend.Flush(ctx, id); err != nil {
		t.Fatalf("Flush failed: %v", err)
	}

	backend2, err := persistence.NewJSONL(dir, 1)
	if err != nil {
		t.Fatalf("reopening failed: %v", err)
	}
	_, loaded, err := backend2.Load(ctx, id)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(loaded) != 6 {
		t.Fatalf("event count after load = %d, want 6 (4 + repair step/end + repair turn/end)", len(loaded))
	}

	last := loaded[len(loaded)-1]
	if last.Type != session.EventTurnEnd {
		t.Fatalf("tail should be repair turn/end: %+v", last)
	}
	if td, ok := last.Data.(session.TurnEndData); !ok || td.Reason != session.ReasonInterrupted {
		t.Fatalf("repair turn/end should be interrupted: %+v", last.Data)
	}

	foundCall := false
	for _, ev := range loaded {
		if ev.Type == session.EventToolCall {
			foundCall = true
		}
	}
	if !foundCall {
		t.Fatal("written tool/call events should be retained")
	}
}

func TestPersistenceForkLineage(t *testing.T) {
	dir := t.TempDir()
	backend, err := persistence.NewJSONL(dir, 1)
	if err != nil {
		t.Fatalf("NewJSONL failed: %v", err)
	}
	ctx := context.Background()

	parent := session.NewSessionHeader(brand.NewSessionID("parent"), "/ws")
	if err := backend.SaveHeader(ctx, parent); err != nil {
		t.Fatalf("SaveHeader failed: %v", err)
	}
	child := parent.Fork(brand.NewSessionID("child"))
	if err := backend.SaveHeader(ctx, child); err != nil {
		t.Fatalf("child SaveHeader failed: %v", err)
	}

	_, parentEvents, err := backend.Load(ctx, parent.ID)
	if err != nil {
		t.Fatalf("parent Load failed: %v", err)
	}
	childHeader, childEvents, err := backend.Load(ctx, child.ID)
	if err != nil {
		t.Fatalf("child Load failed: %v", err)
	}

	if len(parentEvents) != 0 || len(childEvents) != 0 {
		t.Fatalf("empty session event count should be 0: parent=%d child=%d", len(parentEvents), len(childEvents))
	}
	if childHeader.ParentSession.Raw() != "parent" {
		t.Fatalf("child.ParentSession = %q, want parent", childHeader.ParentSession.Raw())
	}

	ids, err := backend.List(ctx)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("List should contain 2 sessions: %v", ids)
	}
}

func TestPersistenceFlushCheckpoint(t *testing.T) {
	dir := t.TempDir()
	backend, err := persistence.NewJSONL(dir, 100)
	if err != nil {
		t.Fatalf("NewJSONL failed: %v", err)
	}
	ctx := context.Background()

	id := brand.NewSessionID("flush_s")
	header := session.NewSessionHeader(id, "/ws")
	_ = backend.SaveHeader(ctx, header)

	_ = backend.Append(ctx, id, session.SessionEvent{Seq: 1, Type: session.EventTurnStart, Data: session.TurnStartData{Turn: 0}})
	_ = backend.Append(ctx, id, session.SessionEvent{Seq: 2, Type: session.EventTurnEnd, Data: session.TurnEndData{Turn: 0, Reason: session.ReasonFinished}})

	backendProbe := newJSONLNoCheck(t, dir)
	_, before, _ := backendProbe.Load(ctx, id)
	if len(before) != 0 {
		t.Fatalf("file should have no events before flush, got %d", len(before))
	}

	// flush checkpoint
	if err := backend.Flush(ctx, id); err != nil {
		t.Fatalf("Flush failed: %v", err)
	}

	_, after, err := backend.Load(ctx, id)
	if err != nil {
		t.Fatalf("reload failed: %v", err)
	}
	if len(after) != 2 {
		t.Fatalf("should have 2 events after flush, got %d", len(after))
	}
}

func newJSONLNoCheck(t *testing.T, dir string) *persistence.JSONLBackend {
	t.Helper()
	b, err := persistence.NewJSONL(dir, 1)
	if err != nil {
		t.Fatalf("NewJSONL failed: %v", err)
	}
	return b
}
