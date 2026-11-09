//

package tests

import (
	"testing"

	"github.com/JopenChen/dsh-go/pkg/sysprompt"
)

func TestN05ContextHashStableAndChanges(t *testing.T) {
	reg := sysprompt.NewContextRegistry()
	reg.Register("plan", 1, "plan mode: on")
	text1, hash1 := reg.Compute()
	text1b, hash1b := reg.Compute()
	if text1 != text1b || hash1 != hash1b {
		t.Fatal("hash should be stable when unchanged")
	}

	reg.Register("plan", 1, "plan mode: off")
	_, hash2 := reg.Compute()
	if hash2 == hash1 {
		t.Fatal("hash should change after state change")
	}
	_ = text1
}

func TestN05ChangeOnlyInject(t *testing.T) {
	reg := sysprompt.NewContextRegistry()
	reg.Register("goal", 1, "goal: ship it")
	in := sysprompt.NewChangeOnlyInjector(reg, "")

	if _, injected := in.MightInject(); !injected {
		t.Fatal("should inject on first round")
	}

	for i := 0; i < 50; i++ {
		if _, injected := in.MightInject(); injected {
			t.Fatalf("round %d unchanged should not inject", i)
		}
	}
	if in.InjectCount() != 1 {
		t.Fatalf("should inject only once, got %d", in.InjectCount())
	}

	reg.Register("goal", 1, "goal: ship it now")
	if _, injected := in.MightInject(); !injected {
		t.Fatal("should inject after state change")
	}
	if in.InjectCount() != 2 {
		t.Fatalf("should have two injections, got %d", in.InjectCount())
	}
}

func TestN05GoalRoundStopsOnComplete(t *testing.T) {
	active := sysprompt.GoalRoundContext{Active: true, Round: 3, GoalDesc: "build"}
	if active.Render() == "" {
		t.Fatal("should render continue prompt when active")
	}
	complete := sysprompt.GoalRoundContext{Active: false, Round: 3, GoalDesc: "build"}
	if complete.Render() != "" {
		t.Fatal("should stop injection after complete (empty)")
	}
}

func TestN05CompactionPreserveSnapshot(t *testing.T) {
	reg := sysprompt.NewContextRegistry()
	reg.Register("ctx1", 1, "snapshot-A")
	cp := &sysprompt.CompactPreserve{}

	textA, hashA := reg.Compute()
	cp.Record(textA, hashA)

	reg.Register("ctx2", 2, "snapshot-B")
	textB, hashB := reg.Compute()
	cp.Record(textB, hashB)

	latest, hash, ok := cp.Latest()
	if !ok || hash != hashB || latest != textB {
		t.Fatalf("should retain latest snapshot B after compaction: ok=%v", ok)
	}
	if hash == hashA {
		t.Fatal("should not retain old snapshot A")
	}
}

func TestN05PersistedHashRecovery(t *testing.T) {
	reg := sysprompt.NewContextRegistry()
	reg.Register("p1", 1, "stable")
	reg.Register("p2", 2, "settings: x")

	in1 := sysprompt.NewChangeOnlyInjector(reg, "")
	_, _ = in1.MightInject()
	persisted := in1.LastHash()

	in2 := sysprompt.NewChangeOnlyInjector(reg, persisted)
	if _, injected := in2.MightInject(); injected {
		t.Fatal("state unchanged after persistence recovery should not re-inject")
	}
	if in2.InjectCount() != 0 {
		t.Fatalf("should have 0 injections, got %d", in2.InjectCount())
	}
}
