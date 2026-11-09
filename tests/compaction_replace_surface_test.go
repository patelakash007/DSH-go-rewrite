package tests

import (
	"testing"

	"github.com/JopenChen/dsh-go/pkg/session"
)

func TestCompactionReplaceSurface(t *testing.T) {

	var events []session.SessionEvent
	time := fixedTestTime()
	for i := 1; i <= 100; i++ {
		var data session.EventData
		if i%2 == 1 {
			data = session.UserMessageData{Content: "u"}
		} else {
			data = session.AssistantMessageData{Content: "a"}
		}
		events = append(events, session.SessionEvent{Seq: uint64(i), Time: time, Type: data.EventType(), Data: data})
	}

	summary := session.AssistantMessageData{Content: "[Summary coverage 3..80]"}
	events = append(events, session.SessionEvent{
		Seq: 101, Time: time, Type: session.EventAssistantMessage, Data: summary,
		SurfaceOp: session.NewReplaceOp(3, 80, nil),
	})

	nodes, replacements := session.FoldSurface(events)
	if len(nodes) != 23 {
		t.Fatalf("effective node count = %d, want 23 (seq1,2 + 81..100 + replacement event)", len(nodes))
	}
	if len(replacements) != 1 || replacements[0].Start != 3 || replacements[0].End != 80 {
		t.Fatalf("replacement range unexpected: %+v", replacements)
	}

	if !nodes[len(nodes)-1].Replaced {
		t.Fatal("replacement event should mark Replaced")
	}

	msgs := session.DeriveMessages(events)
	if len(msgs) != len(nodes) {
		t.Fatalf("deriveMessages count = %d, foldSurface count = %d, should match", len(msgs), len(nodes))
	}
}

func TestSurfaceReplacePurity(t *testing.T) {
	time := fixedTestTime()

	var events []session.SessionEvent
	for i := 1; i <= 3; i++ {
		events = append(events, session.SessionEvent{
			Seq: uint64(i), Time: time, Type: session.EventUserMessage,
			Data: session.UserMessageData{Content: "orig"},
		})
	}

	srcSeq1Before := events[0].Seq

	events = append(events, session.SessionEvent{
		Seq: 4, Time: time, Type: session.EventAssistantMessage,
		Data:      session.AssistantMessageData{Content: "summary"},
		SurfaceOp: session.NewReplaceOp(1, 3, nil),
	})

	if events[0].Seq != srcSeq1Before {
		t.Fatal("source events were modified")
	}

	if len(events) != 4 {
		t.Fatalf("source events should be retained: %d", len(events))
	}
}
