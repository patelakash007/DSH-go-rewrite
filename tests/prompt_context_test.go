package tests

import (
	"testing"

	"github.com/JopenChen/dsh-go/pkg/sysprompt"
)

func TestPromptContextDynamic(t *testing.T) {
	reg := sysprompt.NewContextRegistry()

	reg.Register("runtime_ctx", 300, "cwd=/workspace")
	reg.Register("plan_policy", 500, "Plan before executing")

	text, hash := reg.Compute()
	if text == "" {
		t.Fatal("should be able to compute content after registration")
	}
	if hash == "" {
		t.Fatal("non-empty snapshot should have hash")
	}

	if _, h2 := reg.Compute(); h2 != hash {
		t.Fatal("hash should be stable")
	}

	reg.Unregister("plan_policy")
	text2, hash2 := reg.Compute()
	if text2 == text || hash2 == hash {
		t.Fatal("compute should change after removal")
	}
}

func TestPromptContextChangeDetection(t *testing.T) {
	reg := sysprompt.NewContextRegistry()
	changed := reg.Register("runtime_ctx", 300, "v1")
	if !changed {
		t.Fatal("first registration should be considered change")
	}
	if !reg.Register("runtime_ctx", 300, "v2") {
		t.Fatal("register should return changed=true after content change")
	}
	if reg.Register("runtime_ctx", 300, "v2") {
		t.Fatal("register should return changed=false when content unchanged (change-only)")
	}
}

func TestGoalRoundContext(t *testing.T) {
	active := sysprompt.GoalRoundContext{Active: true, Round: 2, GoalDesc: "implement task"}
	if active.Render() == "" {
		t.Fatal("should render continue prompt when active")
	}

	inactive := sysprompt.GoalRoundContext{Active: false, Round: 2, GoalDesc: "implement task"}
	if inactive.Render() != "" {
		t.Fatal("should stop injection after goal.complete")
	}
}

func TestPromptContextSnapshotPersist(t *testing.T) {
	reg := sysprompt.NewContextRegistry()
	reg.Register("runtime_ctx", 300, "snapshot-value")

	snap := reg.Snapshot()
	if snap["runtime_ctx"] != "snapshot-value" {
		t.Fatalf("snapshot content unexpected: %v", snap)
	}
}
