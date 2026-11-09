package tests

import (
	"testing"

	"github.com/JopenChen/dsh-go/pkg/brand"
	"github.com/JopenChen/dsh-go/pkg/session"
)

func TestEndSeedBracket(t *testing.T) {
	sl := session.NewSessionLog(brand.NewSessionID("s1"))

	_, _ = sl.Append(session.UserMessageData{Content: "seed1"})
	_, _ = sl.Append(session.UserMessageData{Content: "seed2"})

	if _, err := session.MarkEndSeed(sl, brand.SessionID{}); err != nil {
		t.Fatalf("MarkEndSeed failed: %v", err)
	}

	_, _ = sl.Append(session.UserMessageData{Content: "live1"})

	seedSeq := session.SeedEndSeq(sl.Events())
	if seedSeq == 0 {
		t.Fatal("end-seed should exist")
	}

	if !session.IsAfterEndSeed(sl.Events(), seedSeq+1) {
		t.Fatal("live1 should be after end-seed")
	}
	if session.IsAfterEndSeed(sl.Events(), 1) {
		t.Fatal("seed1 should not be after end-seed")
	}
}

func TestEndSeedForkParent(t *testing.T) {
	parentID := brand.NewSessionID("parent_s")
	sl := session.NewSessionLog(brand.NewSessionID("child_s"))

	_, err := session.MarkEndSeed(sl, parentID)
	if err != nil {
		t.Fatalf("MarkEndSeed failed: %v", err)
	}

	events := sl.Events()
	last := events[len(events)-1]
	if last.Type != session.EventEndSeed {
		t.Fatalf("last event should be end-seed: %+v", last)
	}
	d, ok := last.Data.(session.EndSeedData)
	if !ok || d.ParentSession.Raw() != "parent_s" {
		t.Fatalf("end-seed should carry parent session: %+v", last.Data)
	}
}

func TestEndSeedHeaderLength(t *testing.T) {
	h := session.NewSessionHeader(brand.NewSessionID("s1"), "/ws")
	h.SeedLength = 42
	if session.EndSeedMarker(h) != 42 {
		t.Fatalf("SeedLength boundary = %d, want 42", session.EndSeedMarker(h))
	}
}
