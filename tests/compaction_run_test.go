//

package tests

import (
	"context"
	"testing"

	"github.com/JopenChen/dsh-go/pkg/compaction"
	"github.com/JopenChen/dsh-go/pkg/session"
)

func buildChatEvents(n int) []session.SessionEvent {
	evs := make([]session.SessionEvent, 0, n*2)
	seq := uint64(1)
	for i := 0; i < n; i++ {
		evs = append(evs, session.SessionEvent{Seq: seq, Time: fixedTestTime(), Type: session.EventUserMessage,
			Data: session.UserMessageData{Content: "User question round N needs to be long enough to consume token budget QQQQ"}})
		seq++
		evs = append(evs, session.SessionEvent{Seq: seq, Time: fixedTestTime(), Type: session.EventAssistantMessage,
			Data: session.AssistantMessageData{Content: "Assistant reply: long response text to trigger token budget threshold AAAA"}})
		seq++
	}
	return evs
}

func TestCompactionTriggersAndReplaces(t *testing.T) {
	ctx := context.Background()
	events := buildChatEvents(60)
	cp := compaction.New(compaction.Config{MaxTokens: 40, KeepTail: 2, SummaryMaxRunes: 200}, nil)

	if !cp.ShouldCompact(events) {
		t.Fatal("events should exceed token budget to trigger compaction")
	}
	res, err := cp.Compact(ctx, events)
	if err != nil {
		t.Fatal(err)
	}
	if res.Start > res.End {
		t.Fatalf("invalid replacement range start=%d end=%d", res.Start, res.End)
	}
	if res.Summary == "" {
		t.Fatal("summary should not be empty")
	}
	newSeq := events[len(events)-1].Seq + 1
	if res.Replacement.Seq != newSeq {
		t.Fatalf("replacement event seq should be %d, got %d", newSeq, res.Replacement.Seq)
	}
	if res.Replacement.SurfaceOp == nil {
		t.Fatal("replacement event must carry SurfaceOp")
	}

	before := len(session.DeriveMessages(events))
	after := len(session.DeriveMessages(res.Effective))
	if after >= before {
		t.Fatalf("effective message count should decrease after compaction: before=%d after=%d", before, after)
	}
}

func TestCompactionTurnContinues(t *testing.T) {
	ctx := context.Background()
	events := buildChatEvents(40)
	cp := compaction.New(compaction.Config{MaxTokens: 30, KeepTail: 2, SummaryMaxRunes: 200}, nil)
	res, err := cp.Compact(ctx, events)
	if err != nil {
		t.Fatal(err)
	}
	effective := res.Effective

	nextSeq := effective[len(effective)-1].Seq + 1
	effective = append(effective, session.SessionEvent{Seq: nextSeq, Time: fixedTestTime(),
		Type: session.EventUserMessage, Data: session.UserMessageData{Content: "New question round after compaction PPPP"}})

	msgs := session.DeriveMessages(effective)
	last := msgs[len(msgs)-1]
	if last.Role != "user" || last.Content != "New question round after compaction PPPP" {
		t.Fatalf("new messages after compaction should be derivable, got last item=%+v", last)
	}
}

func TestCompactionHeaderRebuildNoRawEvents(t *testing.T) {
	ctx := context.Background()
	events := buildChatEvents(40)
	cp := compaction.New(compaction.Config{MaxTokens: 30, KeepTail: 2, SummaryMaxRunes: 200}, nil)
	res, err := cp.Compact(ctx, events)
	if err != nil {
		t.Fatal(err)
	}
	effective := res.Effective

	proj := session.FoldAll(effective)
	if len(proj.Messages) == 0 {
		t.Fatal("should still have derived messages after compaction (summary+retained)")
	}

	for _, m := range proj.Messages {
		if m.Seq <= res.Start {
			t.Fatalf("original messages in replaced range seq<=%d should not appear in derived messages, got seq=%d", res.Start, m.Seq)
		}
	}
}

func TestCompactionBelowThresholdNoOp(t *testing.T) {
	ctx := context.Background()
	events := buildChatEvents(3)
	cp := compaction.New(compaction.Config{MaxTokens: 10000, KeepTail: 2, SummaryMaxRunes: 200}, nil)
	if cp.ShouldCompact(events) {
		t.Fatal("low load should not trigger compaction")
	}
	if _, err := cp.Compact(ctx, events); err == nil {
		t.Fatal("calling Compact below threshold should return error (no compaction needed)")
	}
}
