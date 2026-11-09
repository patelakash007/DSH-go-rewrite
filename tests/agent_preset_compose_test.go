package tests

import (
	"testing"

	"github.com/JopenChen/dsh-go/pkg/presets"
)

func TestAgentPresetCompose(t *testing.T) {
	code := &presets.AgentPreset{Key: "code", Tools: []string{"bash", "fs_read"}, Prompt: "You are a coding assistant"}
	research := &presets.AgentPreset{Key: "research", Tools: []string{"web_fetch"}, Prompt: "You are a researcher"}

	combo := presets.ComposeFrom("code_research", code, research)
	if len(combo.Tools) != 3 {
		t.Fatalf("combined tool count = %d, want 3: %v", len(combo.Tools), combo.Tools)
	}

	if combo.Tools[0] != "bash" || combo.Tools[2] != "web_fetch" {
		t.Fatalf("tool ordering unexpected: %v", combo.Tools)
	}
	if combo.Prompt == "" {
		t.Fatal("combined preset should join prompt")
	}
}

func TestAgentPresetMountSelect(t *testing.T) {
	reg := presets.NewPresetRegistry()

	code := &presets.AgentPreset{Key: "code", Tools: []string{"bash"}, Prompt: "Coding"}
	reg.Mount(code)

	got, ok := reg.Select("code")
	if !ok || got.Key != "code" || len(got.Tools) != 1 {
		t.Fatalf("Select(code) unexpected: %+v ok=%v", got, ok)
	}

	if _, ok := reg.Select("nope"); ok {
		t.Fatal("unmounted key should not be selected")
	}
}

func TestAgentPresetIndependence(t *testing.T) {
	reg := presets.NewPresetRegistry()
	reg.Mount(&presets.AgentPreset{Key: "a", Tools: []string{"t1"}, Prompt: "A"})
	reg.Mount(&presets.AgentPreset{Key: "b", Tools: []string{"t2"}, Prompt: "B"})

	pa, _ := reg.Select("a")
	pb, _ := reg.Select("b")

	if len(pa.Tools) != 1 || len(pb.Tools) != 1 {
		t.Fatalf("two presets should be independent: a=%v b=%v", pa.Tools, pb.Tools)
	}
	if pa.Tools[0] == pb.Tools[0] {
		t.Fatal("tool sets of two presets should differ")
	}
}
