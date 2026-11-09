//

package tests

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/JopenChen/dsh-go/pkg/commands"
	"github.com/JopenChen/dsh-go/pkg/registry"
	"github.com/JopenChen/dsh-go/pkg/session"
)

// ============================================================================

// ============================================================================

func TestH07FreezableBasic(t *testing.T) {
	r := registry.NewFreezable[string, int]()
	if r.Len() != 0 {
		t.Fatal("initial should be empty")
	}
	if err := r.Put("a", 1); err != nil {
		t.Fatal(err)
	}
	if err := r.Put("b", 2); err != nil {
		t.Fatal(err)
	}
	if v, ok := r.Get("a"); !ok || v != 1 {
		t.Fatalf("Get(a) = %v, ok=%v", v, ok)
	}
	if !r.Contains("b") || r.Contains("z") {
		t.Fatal("Contains check unexpected")
	}
	if r.Len() != 2 {
		t.Fatalf("Len = %d, want 2", r.Len())
	}
	// Remove
	if err := r.Remove("a"); err != nil {
		t.Fatal(err)
	}
	if r.Contains("a") {
		t.Fatal("should not contain a after Remove")
	}
}

// ============================================================================

// ============================================================================

func TestH07FreezableSnapshotConsistency(t *testing.T) {
	r := registry.NewFreezable[string, int]()
	_ = r.Put("x", 10)
	_ = r.Put("y", 20)
	r.Freeze()
	if !r.IsFrozen() {
		t.Fatal("IsFrozen should be true after Freeze")
	}

	if v, ok := r.Get("x"); !ok || v != 10 {
		t.Fatalf("Get(x) after Freeze = %v ok=%v", v, ok)
	}
	if v, ok := r.Get("y"); !ok || v != 20 {
		t.Fatalf("Get(y) after Freeze = %v ok=%v", v, ok)
	}
	if r.Len() != 2 || !r.Contains("y") {
		t.Fatal("Len/Contains unexpected after Freeze")
	}
}

// ============================================================================

// ============================================================================

func TestH07FreezeRejectsMutation(t *testing.T) {
	r := registry.NewFreezable[string, int]()
	_ = r.Put("a", 1)
	r.Freeze()
	if err := r.Put("b", 2); !errors.Is(err, registry.ErrFrozen) {
		t.Fatalf("Put after Freeze should return ErrFrozen, got %v", err)
	}
	if err := r.Remove("a"); !errors.Is(err, registry.ErrFrozen) {
		t.Fatalf("Remove after Freeze should return ErrFrozen, got %v", err)
	}

	if r.Len() != 1 || !r.Contains("a") {
		t.Fatal("rejected writes after Freeze should not affect snapshot")
	}
}

// ============================================================================

// ============================================================================

func TestH07FreezeIdempotent(t *testing.T) {
	r := registry.NewFreezable[string, int]()
	_ = r.Put("a", 42)
	r.Freeze()
	r.Freeze()
	r.Freeze()
	if blob, ok := r.Get("a"); !ok || blob != 42 {
		t.Fatalf("snapshot should hold after multiple Freezes: %v %v", blob, ok)
	}
}

// ============================================================================

// ============================================================================

func TestH07ConcurrentReadAfterFreeze(t *testing.T) {
	const n = 64
	r := registry.NewFreezable[string, int]()
	for i := 0; i < n; i++ {
		_ = r.Put(itoa(i), i)
	}
	r.Freeze()
	var wg sync.WaitGroup
	stop := make(chan struct{})

	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(seed int) {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				for i := 0; i < n; i++ {
					v, ok := r.Get(itoa(i))
					if !ok || v != i {
						t.Errorf("concurrent read unexpected: key %d got %v ok=%v", i, v, ok)
						return
					}
				}
			}
		}(g)
	}

	_ = stop
	close(stop)
	wg.Wait()
}

// ============================================================================

// ============================================================================

func TestH07CommandsFreeze(t *testing.T) {
	r := commands.NewRegistry()

	if _, ok := r.Get("plan"); !ok {
		t.Fatal("built-in plan command missing")
	}
	if r.IsFrozen() {
		t.Fatal("should not be frozen initially")
	}

	if err := r.Register(&commands.CommandDefinition{Name: "resume", Description: "continue", Handler: func(ctx context.Context, args string, sl *session.SessionLog) (string, error) { return "", nil }}); err != nil {
		t.Fatal(err)
	}
	r.Freeze()
	if !r.IsFrozen() {
		t.Fatal("IsFrozen should be true after Freeze")
	}

	if _, ok := r.Get("resume"); !ok {
		t.Fatal("should be able to read registered commands after Freeze")
	}
	if err := r.Register(&commands.CommandDefinition{Name: "late", Description: "x", Handler: nil}); !errors.Is(err, registry.ErrFrozen) {
		t.Fatalf("Register after Freeze should return ErrFrozen, got %v", err)
	}

	names := r.List()
	foundPlan, foundResume := false, false
	for _, n := range names {
		if n == "plan" {
			foundPlan = true
		}
		if n == "resume" {
			foundResume = true
		}
	}
	if !foundPlan || !foundResume {
		t.Fatalf("List should contain plan and resume, got %v", names)
	}
}

// ============================================================================

// ============================================================================

func BenchmarkH07ReadBeforeFreeze(b *testing.B) {
	r := registry.NewFreezable[string, int]()
	for i := 0; i < 100; i++ {
		_ = r.Put(itoa(i), i)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = r.Get(itoa(i % 100))
	}
}

func BenchmarkH07ReadAfterFreeze(b *testing.B) {
	r := registry.NewFreezable[string, int]()
	for i := 0; i < 100; i++ {
		_ = r.Put(itoa(i), i)
	}
	r.Freeze()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = r.Get(itoa(i % 100))
	}
}
