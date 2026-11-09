package tests

import (
	"errors"
	"strings"
	"testing"

	"github.com/JopenChen/dsh-go/pkg/invariant"
)

func TestInvariantPkgAttribution(t *testing.T) {
	reg := invariant.NewRegistry()

	err := reg.Register("pkg/session", func() error {
		return &invariant.InvariantError{Pkg: "pkg/session", Msg: "turn/start while turn already open"}
	})
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	failures := reg.Run()
	if len(failures) != 1 {
		t.Fatalf("Run() should return 1 failure, got %d", len(failures))
	}
	if !strings.HasPrefix(failures[0].Error(), "INVARIANT [pkg/session]: ") {
		t.Fatalf("error prefix mismatch: %q", failures[0].Error())
	}

	ledger := reg.Ledger()
	if len(ledger) != 1 {
		t.Fatalf("Ledger should have 1 entry, got %d", len(ledger))
	}
}

func TestInvariantMultiplePkgsOrdered(t *testing.T) {
	reg := invariant.NewRegistry()

	_ = reg.Register("pkg/zeta", func() error { return errors.New("z bug") })
	_ = reg.Register("pkg/alpha", func() error { return errors.New("a bug") })

	failures := reg.Run()
	if len(failures) != 2 {
		t.Fatalf("Run() should return 2 failures, got %d", len(failures))
	}

	if !strings.HasPrefix(failures[0].Error(), "INVARIANT [pkg/alpha]: ") {
		t.Fatalf("first failure should attribute to pkg/alpha: %q", failures[0].Error())
	}
	if !strings.HasPrefix(failures[1].Error(), "INVARIANT [pkg/zeta]: ") {
		t.Fatalf("second failure should attribute to pkg/zeta: %q", failures[1].Error())
	}
}

func TestInvariantRegisterValidation(t *testing.T) {
	reg := invariant.NewRegistry()

	if err := reg.Register("", func() error { return nil }); err == nil {
		t.Fatal("registration with empty pkg should error")
	}
	if err := reg.Register("pkg/x", nil); err == nil {
		t.Fatal("registration with nil checker should error")
	}
}

func TestInvariantDisableInProduction(t *testing.T) {
	reg := invariant.NewRegistry()
	_ = reg.Register("pkg/session", func() error {
		return errors.New("should not run when disabled")
	})

	original := invariant.IsEnabled()
	defer invariant.SetEnabled(original)

	invariant.SetEnabled(false)
	if invariant.IsEnabled() {
		t.Fatal("IsEnabled should be false after SetEnabled(false)")
	}
	if failures := reg.Run(); len(failures) != 0 {
		t.Fatalf("Run should return empty when disabled, got %d failures", len(failures))
	}

	invariant.SetEnabled(true)
	if failures := reg.Run(); len(failures) != 1 {
		t.Fatalf("Run should return 1 failure after re-enabling, got %d", len(failures))
	}
}

func TestInvariantCheckerPanicIsolated(t *testing.T) {
	reg := invariant.NewRegistry()
	_ = reg.Register("pkg/panicky", func() error {
		panic("boom")
	})
	_ = reg.Register("pkg/ok", func() error { return nil })

	failures := reg.Run()
	if len(failures) != 1 {
		t.Fatalf("panic checker should be caught as 1 failure, got %d", len(failures))
	}
	if !strings.Contains(failures[0].Error(), "panicked") {
		t.Fatalf("should indicate checker panic: %q", failures[0].Error())
	}
}

func TestInvariantMustSatisfy(t *testing.T) {
	reg := invariant.NewRegistry()

	_ = reg.Register("pkg/clean", func() error { return nil })
	func() {
		defer func() {
			if p := recover(); p != nil {
				t.Fatalf("should not panic: %v", p)
			}
		}()
		reg.MustSatisfy()
	}()

	_ = reg.Register("pkg/dirty", func() error { return errors.New("bad state") })
	func() {
		defer func() {
			if p := recover(); p == nil {
				t.Fatal("MustSatisfy should panic on violation")
			}
		}()
		reg.MustSatisfy()
	}()
}
