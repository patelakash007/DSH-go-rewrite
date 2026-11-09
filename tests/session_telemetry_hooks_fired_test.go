//

package tests

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/JopenChen/dsh-go/pkg/telemetry"
)

func TestTelemetryThreeHooksFire(t *testing.T) {
	tm := telemetry.New()
	ctx := context.Background()

	var (
		mu                    sync.Mutex
		events, chunks, tools int
		latencies             []time.Duration
	)

	tm.RegisterEventHook(func(_ context.Context, d telemetry.EventHookData) {
		mu.Lock()
		defer mu.Unlock()
		events++
		latencies = append(latencies, d.LatencyMS)
	})
	tm.RegisterChunkHook(func(_ context.Context, d telemetry.ChunkHookData) {
		mu.Lock()
		defer mu.Unlock()
		chunks++
		latencies = append(latencies, d.LatencyMS)
	})
	tm.RegisterToolHook(func(_ context.Context, d telemetry.ToolHookData) {
		mu.Lock()
		defer mu.Unlock()
		tools++
		latencies = append(latencies, d.LatencyMS)
	})

	now := time.Now()
	tm.RecordEvent(ctx, "user/message", now.Add(-2*time.Millisecond))
	tm.RecordChunk(ctx, 1, 12, now.Add(-3*time.Millisecond))
	tm.RecordTool(ctx, "bash", true, now.Add(-5*time.Millisecond))

	mu.Lock()
	defer mu.Unlock()
	if events != 1 || chunks != 1 || tools != 1 {
		t.Fatalf("all 3 hook types should trigger once, got event=%d chunk=%d tool=%d", events, chunks, tools)
	}
	if len(latencies) != 3 {
		t.Fatalf("should collect 3 latency values, got %d", len(latencies))
	}

	for i, l := range latencies {
		if l < 0 {
			t.Fatalf("latency[%d]=%v should not be negative", i, l)
		}
		if l > 100*time.Millisecond {
			t.Fatalf("latency[%d]=%v outside reasonable range", i, l)
		}
	}
}

func TestTelemetryToolOkFlag(t *testing.T) {
	tm := telemetry.New()
	ctx := context.Background()
	var okResult bool
	tm.RegisterToolHook(func(_ context.Context, d telemetry.ToolHookData) {
		okResult = d.Ok
	})
	tm.RecordTool(ctx, "todo_write", false, time.Now())
	if okResult {
		t.Fatal("failed tool should have Ok=false")
	}
}

func TestTelemetryPanicIsolated(t *testing.T) {
	tm := telemetry.New()
	ctx := context.Background()
	fired := 0

	tm.RegisterEventHook(func(_ context.Context, _ telemetry.EventHookData) {
		panic("boom")
	})
	tm.RegisterEventHook(func(_ context.Context, _ telemetry.EventHookData) {
		fired++
	})

	tm.RecordEvent(ctx, "tool/bash", time.Now())
	if fired != 1 {
		t.Fatalf("normal hooks after panicking hook should still be called, got fired=%d", fired)
	}
}

func TestTelemetryNoHooksNoPanic(t *testing.T) {
	tm := telemetry.New()
	ctx := context.Background()
	now := time.Now()
	tm.RecordEvent(ctx, "user/message", now)
	tm.RecordChunk(ctx, 1, 10, now)
	tm.RecordTool(ctx, "bash", true, now)

}

func TestTelemetryConcurrent(t *testing.T) {
	tm := telemetry.New()
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tm.RegisterEventHook(func(_ context.Context, _ telemetry.EventHookData) {})
		}()
	}
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tm.RecordEvent(ctx, "user/message", time.Now())
			tm.RecordChunk(ctx, 1, 5, time.Now())
			tm.RecordTool(ctx, "bash", true, time.Now())
		}()
	}
	wg.Wait()
}
