package tests

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/JopenChen/dsh-go/pkg/brand"
	"github.com/JopenChen/dsh-go/pkg/session"
)

func TestFoldHotVsReplay(t *testing.T) {
	sl := session.NewSessionLog(brand.NewSessionID("fold_1"))

	const turns = 520
	for i := 0; i < turns; i++ {
		_, _ = sl.Append(session.UserMessageData{Content: "user msg"})
		_, _ = sl.Append(session.AssistantMessageData{Content: "assistant msg"})
	}

	_, _ = sl.Append(session.PresetChangeData{Preset: "safe"})
	_, _ = sl.Append(session.GoalChangeData{GoalID: "g1", Phase: "active", Description: "do x", MaxRounds: 5, Revision: 1})
	_, _ = sl.Append(session.TodoWriteData{Items: []string{"a", "b", "c"}})
	_, _ = sl.Append(session.PlanModeData{Mode: "on"})
	_, _ = sl.Append(session.SessionTitleData{Title: "My Session"})
	_, _ = sl.Append(session.RequestHeaderData{ConfigEpoch: 2, SystemHash: "h", ToolCount: 3})
	_, _ = sl.Append(session.PresetChangeData{Preset: "danger"})

	if sl.Len() < 1000 {
		t.Fatalf("event count less than 1000: %d", sl.Len())
	}

	var hot session.SessionProjection
	events := sl.Events()
	for i := 1; i <= len(events); i++ {
		hot = session.FoldAll(events[:i])
	}

	replay := session.FoldAll(events)

	if !reflect.DeepEqual(hot, replay) {
		t.Fatalf("warm/cold fold mismatch\nwarm: %+v\ncold: %+v", hot, replay)
	}

	if len(replay.Messages) != turns*2 {
		t.Fatalf("message count = %d, want %d", len(replay.Messages), turns*2)
	}
	if replay.Preset.Preset != "danger" {
		t.Fatalf("preset should be latest danger: %+v", replay.Preset)
	}
	if replay.SandboxMode.Mode != session.SandboxDangerFullAccess {
		t.Fatalf("danger preset should map full-access: %+v", replay.SandboxMode)
	}
	if replay.Approval.Policy != session.ApprovalAllowAll {
		t.Fatalf("danger preset should map allow-all: %+v", replay.Approval)
	}
	if replay.Goal.GoalID != "g1" || replay.Goal.Revision != 1 {
		t.Fatalf("goal fold unexpected: %+v", replay.Goal)
	}
	if !replay.Todo.Present || len(replay.Todo.Items) != 3 {
		t.Fatalf("todo fold unexpected: %+v", replay.Todo)
	}
	if replay.PlanMode.Mode != "on" {
		t.Fatalf("plan mode fold unexpected: %+v", replay.PlanMode)
	}
	if replay.Title.Title != "My Session" {
		t.Fatalf("title fold unexpected: %+v", replay.Title)
	}
	if !replay.RequestHeader.Present || replay.RequestHeader.ConfigEpoch != 2 {
		t.Fatalf("request header fold unexpected: %+v", replay.RequestHeader)
	}
}

func TestDeriveMessagesAfterCompactionReplace(t *testing.T) {

	var events []session.SessionEvent
	time := fixedTestTime()
	for i := 1; i <= 10; i++ {
		role := "user"
		content := "old message"
		if i%2 == 0 {
			role = "assistant"
		}
		var data session.EventData
		if role == "user" {
			data = session.UserMessageData{Content: content}
		} else {
			data = session.AssistantMessageData{Content: content}
		}
		events = append(events, session.SessionEvent{Seq: uint64(i), Time: time, Type: data.EventType(), Data: data})
	}

	summary := session.AssistantMessageData{Content: "[Compacted Summary] first 10 messages"}
	replaceEv := session.SessionEvent{
		Seq:       11,
		Time:      time,
		Type:      session.EventAssistantMessage,
		Data:      summary,
		SurfaceOp: session.NewReplaceOp(1, 10, nil),
	}
	events = append(events, replaceEv)

	events = append(events, session.SessionEvent{
		Seq: 12, Time: time, Type: session.EventUserMessage,
		Data: session.UserMessageData{Content: "New question"},
	})

	msgs := session.DeriveMessages(events)

	if len(msgs) != 2 {
		t.Fatalf("message count after compaction = %d, want 2 (summary+new message)", len(msgs))
	}
	if msgs[0].Content != "[Compacted Summary] first 10 messages" {
		t.Fatalf("first item should be compacted summary: %q", msgs[0].Content)
	}
	if msgs[1].Content != "New question" {
		t.Fatalf("last item should be new message: %q", msgs[1].Content)
	}
	if msgs[0].Seq != 11 || msgs[1].Seq != 12 {
		t.Fatalf("seq marker unexpected: %+v", msgs)
	}
}

func TestFoldDeterministicAcrossReplay(t *testing.T) {
	sl := session.NewSessionLog(brand.NewSessionID("fold_2"))
	_, _ = sl.Append(session.PresetChangeData{Preset: "review"})
	_, _ = sl.Append(session.TodoWriteData{Items: []string{"x"}})
	_, _ = sl.Append(session.SessionTitleData{Title: "T"})
	events := sl.Events()

	p1 := session.FoldAll(events)
	p2 := session.FoldAll(events)

	b1, _ := json.Marshal(p1)
	b2, _ := json.Marshal(p2)
	if string(b1) != string(b2) {
		t.Fatalf("fold output non-deterministic\n%s\n%s", b1, b2)
	}
}
