//

package tests

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/JopenChen/dsh-go/pkg/brand"
	"github.com/JopenChen/dsh-go/pkg/persistence"
	"github.com/JopenChen/dsh-go/pkg/session"
)

func genEventsH05(n int) []session.SessionEvent {
	evs := make([]session.SessionEvent, 0, n)
	for i := 0; i < n; i++ {
		seq := uint64(i + 1)
		switch i % 2 {
		case 0:
			evs = append(evs, session.SessionEvent{
				Seq: seq, Time: fixedTestTimeH02(),
				Type: session.EventUserMessage,
				Data: session.UserMessageData{Content: "hello-" + itoa(i)},
			})
		case 1:
			evs = append(evs, session.SessionEvent{
				Seq: seq, Time: fixedTestTimeH02(),
				Type: session.EventAssistantMessage,
				Data: session.AssistantMessageData{Content: "world-" + itoa(i)},
			})
		}
	}
	return evs
}

// ============================================================================

// ============================================================================

func TestH05PersistenceRoundTripEquivalent(t *testing.T) {
	dir := t.TempDir()

	persistence.ResetJSONLIOStats()

	j, err := persistence.NewJSONL(filepath.Join(dir, "h05"), 50, persistence.WithShardCount(4))
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()

	ctx := context.Background()
	id := brand.NewSessionID("h05-eq")
	_ = j.SaveHeader(ctx, session.NewSessionHeader(id, "/ws"))

	events := genEventsH05(200)

	for _, ev := range events {
		if err := j.Append(ctx, id, ev); err != nil {
			t.Fatalf("Append failed: %v", err)
		}
	}
	if err := j.Flush(ctx, id); err != nil {
		t.Fatalf("Flush failed: %v", err)
	}

	_, got, err := j.Load(ctx, id)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if len(got) != len(events) {
		t.Fatalf("Load event count = %d, want %d", len(got), len(events))
	}
	for i, e := range got {
		if e.Seq != events[i].Seq || e.Type != events[i].Type {
			t.Fatalf("event %d mismatch: got %+v want %+v", i, e, events[i])
		}
	}

	stats := persistence.ReadJSONLIOStats()
	if stats.MarshaledEvents != uint64(len(events)) {
		t.Fatalf("MarshaledEvents = %d, want %d", stats.MarshaledEvents, len(events))
	}

	minHits := uint64(len(events)) + 1
	if stats.PooledBufferHits < minHits {
		t.Fatalf("PooledBufferHits = %d, want >= %d", stats.PooledBufferHits, minHits)
	}
	if stats.MarshaledBytes == 0 {
		t.Fatal("MarshaledBytes should be > 0 (pooled path was not taken)")
	}
}

func TestH05RewriteEquivalent(t *testing.T) {
	dir := t.TempDir()
	persistence.ResetJSONLIOStats()
	j, err := persistence.NewJSONL(filepath.Join(dir, "h05-rw"), 100)
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	ctx := context.Background()
	id := brand.NewSessionID("h05-rw")
	_ = j.SaveHeader(ctx, session.NewSessionHeader(id, "/ws"))

	events := genEventsH05(120)
	for _, ev := range events {
		_ = j.Append(ctx, id, ev)
	}

	n, err := j.Snapshot(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if n != len(events) {
		t.Fatalf("Snapshot N = %d, want %d", n, len(events))
	}

	_, got, err := j.Load(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(events) {
		t.Fatalf("Load event count mismatch after rewrite: %d vs %d", len(got), len(events))
	}
	for i := range got {
		if got[i].Seq != events[i].Seq {
			t.Fatalf("Seq mismatch after rewrite i=%d got=%d want=%d", i, got[i].Seq, events[i].Seq)
		}
	}
	stats := persistence.ReadJSONLIOStats()
	if stats.MarshaledEvents < uint64(len(events)) {
		t.Fatalf("MarshaledEvents = %d, want >= %d", stats.MarshaledEvents, len(events))
	}

	_ = stats.HeaderMarshaledBytes
}

// ============================================================================

// ============================================================================

func BenchmarkH05PersistencePooledIO(b *testing.B) {
	dir := b.TempDir()

	j0, _ := persistence.NewJSONL(filepath.Join(dir, "warm"), 200)
	warm := genEventsH05(200)
	for i := 0; i < 100; i++ {
		id := brand.NewSessionID("warm-" + itoa(i))
		for _, ev := range warm {
			_ = j0.Append(context.Background(), id, ev)
		}
		_ = j0.Flush(context.Background(), id)
	}
	_ = j0.Close()

	persistence.ResetJSONLIOStats()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		runDir := filepath.Join(dir, "r-"+itoa(i))
		j, err := persistence.NewJSONL(runDir, 200, persistence.WithShardCount(8))
		if err != nil {
			b.Fatal(err)
		}
		ctx := context.Background()
		evs := genEventsH05(200)
		for s := 0; s < 100; s++ {
			id := brand.NewSessionID("s-" + itoa(s))
			for _, ev := range evs {
				if aerr := j.Append(ctx, id, ev); aerr != nil {
					b.Fatal(aerr)
				}
			}
			if ferr := j.Flush(ctx, id); ferr != nil {
				b.Fatal(ferr)
			}
		}
		_ = j.Close()
	}
}
