//

package tests

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/JopenChen/dsh-go/pkg/brand"
	"github.com/JopenChen/dsh-go/pkg/persistence"
	"github.com/JopenChen/dsh-go/pkg/session"
)

func fixedTestTimeH02() time.Time { return time.Unix(1700000000, 0).UTC() }

func newJSONLNoCheckH02(t *testing.T, dir string) *persistence.JSONLBackend {
	t.Helper()
	b, err := persistence.NewJSONL(dir, 1024)
	if err != nil {
		t.Fatalf("NewJSONL failed: %v", err)
	}
	t.Cleanup(func() { _ = b.Close() })
	return b
}

func TestH02ShardCountDefault(t *testing.T) {
	dir := t.TempDir()
	b, err := persistence.NewJSONL(dir, 100)
	if err != nil {
		t.Fatalf("NewJSONL failed: %v", err)
	}
	defer b.Close()

	if got := b.ShardCount(); got != persistence.DefaultShardCount {
		t.Fatalf("default ShardCount = %d, want %d", got, persistence.DefaultShardCount)
	}

	n := b.ShardCount()
	if n&(n-1) != 0 {
		t.Fatalf("ShardCount %d is not a power of 2", n)
	}
}

func TestH02WithShardCountRoundsUp(t *testing.T) {
	dir := t.TempDir()
	b, err := persistence.NewJSONL(dir, 100, persistence.WithShardCount(7))
	if err != nil {
		t.Fatalf("NewJSONL failed: %v", err)
	}
	defer b.Close()
	if got := b.ShardCount(); got != 8 {
		t.Fatalf("WithShardCount(7) should round up to 8, got %d", got)
	}
}

func TestH02AppendAsyncFlushByInterval(t *testing.T) {
	dir := t.TempDir()

	interval := 20 * time.Millisecond
	b, err := persistence.NewJSONL(dir, 10000, persistence.WithFlushInterval(interval))
	if err != nil {
		t.Fatalf("NewJSONL failed: %v", err)
	}
	defer b.Close()

	ctx := context.Background()
	id := brand.NewSessionID("h02_async_int")
	hdr := session.NewSessionHeader(id, "/ws")
	_ = b.SaveHeader(ctx, hdr)

	for i := uint64(1); i <= 2; i++ {
		_ = b.Append(ctx, id, session.SessionEvent{
			Seq: i, Time: fixedTestTimeH02(),
			Type: session.EventUserMessage,
			Data: session.UserMessageData{Content: "msg"},
		})
	}

	probe := newJSONLNoCheckH02(t, dir)
	_, before, _ := probe.Load(ctx, id)
	if len(before) != 0 {
		t.Fatalf("Load immediately after Append should not see uncommitted events, got %d", len(before))
	}

	time.Sleep(interval*3 + 10*time.Millisecond)

	_, after, err := probe.Load(ctx, id)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if len(after) != 2 {
		t.Fatalf("window flush should yield 2 events, got %d", len(after))
	}
}

func TestH02CloseFlushesEverything(t *testing.T) {
	dir := t.TempDir()

	b, err := persistence.NewJSONL(dir, 100000,
		persistence.WithFlushInterval(5*time.Second),
		persistence.WithShardCount(4))
	if err != nil {
		t.Fatalf("NewJSONL failed: %v", err)
	}

	ctx := context.Background()
	const N = 50
	ids := make([]brand.SessionID, N)
	for i := 0; i < N; i++ {
		id := brand.NewSessionID(fmt.Sprintf("h02_close_%d", i))
		ids[i] = id
		_ = b.SaveHeader(ctx, session.NewSessionHeader(id, "/ws"))
		_ = b.Append(ctx, id, session.SessionEvent{
			Seq: 1, Time: fixedTestTimeH02(),
			Type: session.EventTurnStart, Data: session.TurnStartData{Turn: 0},
		})
		_ = b.Append(ctx, id, session.SessionEvent{
			Seq: 2, Time: fixedTestTimeH02(),
			Type: session.EventTurnEnd,
			Data: session.TurnEndData{Turn: 0, Reason: session.ReasonFinished},
		})
	}

	if err := b.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	probe, _ := persistence.NewJSONL(dir, 1)
	defer probe.Close()
	for i := 0; i < N; i++ {
		_, evs, err := probe.Load(ctx, ids[i])
		if err != nil {
			t.Fatalf("session %d Load failed: %v", i, err)
		}
		if len(evs) != 2 {
			t.Fatalf("session %d event count = %d, want 2", i, len(evs))
		}
	}
}

func TestH02ConcurrentAppendNoLoss(t *testing.T) {
	dir := t.TempDir()
	b, err := persistence.NewJSONL(dir, 32,
		persistence.WithFlushInterval(30*time.Millisecond),
		persistence.WithShardCount(8))
	if err != nil {
		t.Fatalf("NewJSONL failed: %v", err)
	}
	defer b.Close()

	ctx := context.Background()
	id := brand.NewSessionID("h02_concurrent")
	_ = b.SaveHeader(ctx, session.NewSessionHeader(id, "/ws"))

	const goroutines = 16
	const perRoutine = 200
	total := goroutines * perRoutine

	var seq int64
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for k := 0; k < perRoutine; k++ {
				s := atomic.AddInt64(&seq, 1)
				_ = b.Append(ctx, id, session.SessionEvent{
					Seq:  uint64(s),
					Time: fixedTestTimeH02(),
					Type: session.EventAssistantMessage,
					Data: session.AssistantMessageData{Content: "x"},
				})
			}
		}()
	}
	wg.Wait()

	if err := b.Flush(ctx, id); err != nil {
		t.Fatalf("Flush failed: %v", err)
	}
	_, evs, err := b.Load(ctx, id)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if len(evs) != total {
		t.Fatalf("event count after concurrent writes = %d, want %d (lost %d)",
			len(evs), total, total-len(evs))
	}
}

func TestH02ExplicitFlushImmediate(t *testing.T) {
	dir := t.TempDir()

	b, err := persistence.NewJSONL(dir, 10000,
		persistence.WithFlushInterval(60*time.Second))
	if err != nil {
		t.Fatalf("NewJSONL failed: %v", err)
	}
	defer b.Close()

	ctx := context.Background()
	id := brand.NewSessionID("h02_exp_flush")
	_ = b.SaveHeader(ctx, session.NewSessionHeader(id, "/ws"))

	_ = b.Append(ctx, id, session.SessionEvent{
		Seq: 1, Time: fixedTestTimeH02(),
		Type: session.EventTurnStart, Data: session.TurnStartData{Turn: 0},
	})
	_ = b.Append(ctx, id, session.SessionEvent{
		Seq: 2, Time: fixedTestTimeH02(),
		Type: session.EventTurnEnd,
		Data: session.TurnEndData{Turn: 0, Reason: session.ReasonFinished},
	})

	if err := b.Flush(ctx, id); err != nil {
		t.Fatalf("Flush failed: %v", err)
	}
	probe := newJSONLNoCheckH02(t, dir)
	_, evs, err := probe.Load(ctx, id)
	if err != nil {
		t.Fatalf("probe Load failed: %v", err)
	}
	if len(evs) != 2 {
		t.Fatalf("explicit Flush should have 2 items, got %d", len(evs))
	}
}

func TestH02BatchThresholdTrigger(t *testing.T) {
	dir := t.TempDir()

	b, err := persistence.NewJSONL(dir, 5,
		persistence.WithFlushInterval(60*time.Second))
	if err != nil {
		t.Fatalf("NewJSONL failed: %v", err)
	}
	defer b.Close()

	ctx := context.Background()
	id := brand.NewSessionID("h02_batch_trig")
	_ = b.SaveHeader(ctx, session.NewSessionHeader(id, "/ws"))

	for i := uint64(1); i <= 5; i++ {
		_ = b.Append(ctx, id, session.SessionEvent{
			Seq: i, Time: fixedTestTimeH02(),
			Type: session.EventAssistantChunk,
			Data: session.AssistantChunkData{Text: "x"},
		})
	}

	time.Sleep(80 * time.Millisecond)

	probe := newJSONLNoCheckH02(t, dir)
	_, evs, err := probe.Load(ctx, id)
	if err != nil {
		t.Fatalf("probe Load failed: %v", err)
	}
	if len(evs) != 5 {
		t.Fatalf("signal flush after batchSize reached should have 5 events, got %d", len(evs))
	}
}
