//

package tests

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/JopenChen/dsh-go/pkg/jobs"
)

// ============================================================================

// ============================================================================

func TestH08SupervisorGracefulShutdown(t *testing.T) {
	sup := jobs.New()
	var exited atomic.Int32
	for i := 0; i < 8; i++ {
		name := "worker-" + itoa(i)
		ok := sup.Go(name, func(ctx context.Context) {
			defer exited.Add(1)

			<-ctx.Done()
		})
		if !ok {
			t.Fatalf("start worker %s failed", name)
		}
	}
	if sup.WorkerCount() != 8 {
		t.Fatalf("WorkerCount = %d, want 8", sup.WorkerCount())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := sup.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}
	if exited.Load() != 8 {
		t.Fatalf("exited worker count = %d, want 8 (leak detected)", exited.Load())
	}

	if sup.Healthy(ctx) {
		t.Fatal("should have no alive workers after Shutdown")
	}
}

// ============================================================================

// ============================================================================

func TestH08ParentCtxCascade(t *testing.T) {
	sup := jobs.New()
	sawCancel := make(chan struct{}, 1)
	sup.Go("cascade", func(ctx context.Context) {
		<-ctx.Done()
		sawCancel <- struct{}{}
	})
	ctx, cancelCtx := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelCtx()
	if err := sup.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-sawCancel:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("worker did not receive cascaded cancellation signal")
	}
}

// ============================================================================

// ============================================================================

func TestH08IdempotentShutdown(t *testing.T) {
	sup := jobs.New()
	sup.Go("w1", func(ctx context.Context) { <-ctx.Done() })
	ctx := context.Background()
	if err := sup.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	if err := sup.Shutdown(ctx); err != nil {
		t.Fatalf("second Shutdown should succeed idempotently: %v", err)
	}
	sup.Close()
	sup.Close()

	if sup.Go("late", func(ctx context.Context) {}) {
		t.Fatal("Go after Shutdown should be rejected")
	}
}

// ============================================================================

// ============================================================================

func TestH08ShutdownTimeout(t *testing.T) {
	sup := jobs.New()

	sup.Go("stubborn", func(ctx context.Context) { select {} })
	sup.Go("ok", func(ctx context.Context) { <-ctx.Done() })

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	err := sup.Shutdown(ctx)
	if err == nil {
		t.Fatal("Shutdown should return error when unyielding worker exists")
	}
}

// ============================================================================

// ============================================================================

func TestH08SingleWorkerCancel(t *testing.T) {
	sup := jobs.New()
	var exited atomic.Bool
	sup.Go("target", func(ctx context.Context) {
		<-ctx.Done()
		exited.Store(true)
	})
	sup.Go("other", func(ctx context.Context) { <-ctx.Done() })
	sup.CancelWorker("target")
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	if !sup.WaitWorker(ctx, "target") {
		t.Fatal("WaitWorker(target) should be true shortly after cancel")
	}
	if !exited.Load() {
		t.Fatal("target worker should have exited")
	}

	if sup.WorkerCount() != 2 {
		t.Fatalf("WorkerCount should remain 2, got %d", sup.WorkerCount())
	}
}

// ============================================================================

// ============================================================================

func TestH08TickerWorkerViaSupervisor(t *testing.T) {
	sup := jobs.New()
	var flushed atomic.Int64

	sup.Go("jsonl-shard-0", func(ctx context.Context) {
		tk := time.NewTicker(10 * time.Millisecond)
		defer tk.Stop()
		for {
			select {
			case <-tk.C:
				flushed.Add(1)
			case <-ctx.Done():
				return
			}
		}
	})

	time.Sleep(60 * time.Millisecond)
	before := flushed.Load()
	if before == 0 {
		t.Fatal("ticker worker should have triggered flush")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	if err := sup.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown ticker worker failed: %v", err)
	}

	tk := time.NewTicker(10 * time.Millisecond)
	defer tk.Stop()
	select {
	case <-tk.C:

	case <-time.After(50 * time.Millisecond):
	}

	if flushed.Load() < before {
		t.Fatalf("flushed should increase monotonically: before=%d now=%d", before, flushed.Load())
	}
}
