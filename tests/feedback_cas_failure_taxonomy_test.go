//

package tests

import (
	"context"
	"testing"

	"github.com/JopenChen/dsh-go/pkg/brand"
	"github.com/JopenChen/dsh-go/pkg/feedback"
	"github.com/JopenChen/dsh-go/pkg/storage"
)

func TestFeedbackWriteRead(t *testing.T) {
	ctx := context.Background()
	store := feedback.New(storage.NewMemoryKV())
	sid := brand.NewSessionID("s1")

	v1, err := store.Put(ctx, sid, "msg1", feedback.RatingThumbsUp, "great", nil)
	if err != nil {
		t.Fatal(err)
	}
	if v1 != 1 {
		t.Fatalf("initial version should be 1, got %d", v1)
	}
	fb, err := store.Get(ctx, sid, "msg1")
	if err != nil {
		t.Fatal(err)
	}
	if fb.Rating != feedback.RatingThumbsUp || fb.Note != "great" {
		t.Fatalf("read inaccurate: %+v", fb)
	}
	if fb.Version != 1 {
		t.Fatalf("version should be 1, got %d", fb.Version)
	}

	v2, _ := store.Put(ctx, sid, "msg1", feedback.RatingThumbsDown, "revised", nil)
	if v2 != 2 {
		t.Fatalf("version after update should be 2, got %d", v2)
	}
}

func TestFeedbackCASConflict(t *testing.T) {
	ctx := context.Background()
	store := feedback.New(storage.NewMemoryKV())
	sid := brand.NewSessionID("s1")
	if _, err := store.Put(ctx, sid, "m", feedback.RatingThumbsUp, "", nil); err != nil {
		t.Fatal(err)
	}

	exp := uint64(0)
	_, err := store.Put(ctx, sid, "m", feedback.RatingThumbsDown, "", &exp)
	if feedback.CodeOf(err) != feedback.CodeVersionConflict {
		t.Fatalf("CAS conflict should be VERSION_CONFLICT, got %v", err)
	}
}

func TestFeedbackNotFound(t *testing.T) {
	ctx := context.Background()
	store := feedback.New(storage.NewMemoryKV())
	_, err := store.Get(ctx, brand.NewSessionID("s1"), "ghost")
	if feedback.CodeOf(err) != feedback.CodeNotFound {
		t.Fatalf("should be NOT_FOUND, got %v", err)
	}
}

func TestFeedbackSessionNotFound(t *testing.T) {
	ctx := context.Background()
	store := feedback.New(storage.NewMemoryKV())
	_, err := store.Put(ctx, brand.SessionID{}, "m", feedback.RatingThumbsUp, "", nil)
	if feedback.CodeOf(err) != feedback.CodeSessionNotFound {
		t.Fatalf("should be SESSION_NOT_FOUND, got %v", err)
	}
}

func TestFeedbackListAndDelete(t *testing.T) {
	ctx := context.Background()
	sid := brand.NewSessionID("sX")
	other := brand.NewSessionID("sY")
	store := feedback.New(storage.NewMemoryKV())
	for _, m := range []string{"a", "b"} {
		_, _ = store.Put(ctx, sid, m, feedback.RatingThumbsUp, "", nil)
	}
	_, _ = store.Put(ctx, other, "c", feedback.RatingThumbsUp, "", nil)
	list, err := store.List(ctx, sid)
	if err != nil || len(list) != 2 {
		t.Fatalf("sX should list 2 items, got %d (%v)", len(list), err)
	}
	if err := store.Delete(ctx, sid, "a"); err != nil {
		t.Fatal(err)
	}
	list, _ = store.List(ctx, sid)
	if len(list) != 1 {
		t.Fatalf("should have 1 item left after deletion, got %d", len(list))
	}
}
