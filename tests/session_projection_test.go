package tests

import (
	"testing"

	"github.com/JopenChen/dsh-go/pkg/brand"
	"github.com/JopenChen/dsh-go/pkg/session"
)

type intState int

func (s intState) Equal(other session.ProjectionState) bool {
	o, ok := other.(intState)
	return ok && s == o
}

func TestProjectionRegisterAndSubscribe(t *testing.T) {
	reg := session.NewProjectionRegistry()

	todoDef := &session.ProjectionDefinition[intState]{
		ID:   brand.NewProjectionID("todo"),
		Init: 0,
		Apply: func(state intState, ev session.SessionEvent) intState {
			if ev.Type == session.EventTodoWrite {
				return state + 1
			}
			return state
		},
	}
	todoProj, err := session.RegisterProjection(reg, todoDef)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	reg.ApplyEvents([]session.SessionEvent{
		{Seq: 1, Type: session.EventTodoWrite, Data: session.TodoWriteData{Items: []string{"a"}}},
		{Seq: 2, Type: session.EventTodoWrite, Data: session.TodoWriteData{Items: []string{"a", "b"}}},
		{Seq: 3, Type: session.EventUserMessage, Data: session.UserMessageData{Content: "hi"}},
	})

	if todoProj.State() != 2 {
		t.Fatalf("todo projection state = %d, want 2", todoProj.State())
	}

	ch := reg.Changelog()
	if len(ch) != 3 {
		t.Fatalf("changelog length = %d, want 3", len(ch))
	}
	changedCount := 0
	for _, c := range ch {
		if c.Changed {
			changedCount++
		}
	}
	if changedCount != 2 {
		t.Fatalf("changed count = %d, want 2", changedCount)
	}
}

func TestProjectionSnapshotMatchesFold(t *testing.T) {

	events := []session.SessionEvent{
		{Seq: 1, Type: session.EventTodoWrite, Data: session.TodoWriteData{Items: []string{"a"}}},
		{Seq: 2, Type: session.EventPlanMode, Data: session.PlanModeData{Mode: "on"}},
	}

	events[0].Time = fixedTestTime()
	events[1].Time = fixedTestTime()

	reg := session.NewProjectionRegistry()

	todoDef := &session.ProjectionDefinition[intState]{
		ID:   brand.NewProjectionID("todo"),
		Init: 0,
		Merge: func(evs []session.SessionEvent) intState {
			if session.FoldTodoWrite(evs).Present {
				return intState(len(session.FoldTodoWrite(evs).Items))
			}
			return 0
		},
	}
	_, err := session.RegisterProjection(reg, todoDef)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	reg.ApplyEvents(events)

	reg.Rebuild(events)

	items := session.FoldTodoWrite(events).Items
	expectedState := intState(len(items))
	if got := reg.SnapshotAll()[brand.NewProjectionID("todo")]; got != expectedState {
		t.Fatalf("snapshot = %v, fold = %v (should match)", got, expectedState)
	}
}
