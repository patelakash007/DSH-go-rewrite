//

package tests

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/JopenChen/dsh-go/pkg/skills"
)

func writeSkill(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+".md"), []byte("Description: "+name+"\n"+body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSkillRegistryDynamicDiscovery(t *testing.T) {
	root := t.TempDir()
	r := skills.New(root)
	projSkills := filepath.Join(root, ".dsh", "skills")

	r.Scan()
	if got := len(r.List()); got != 0 {
		t.Fatalf("initial should be empty, got %d", got)
	}

	writeSkill(t, projSkills, "my-skill", "do the thing")
	r.Scan()
	if _, ok := r.Get("my-skill"); !ok {
		t.Fatal("new skill should be discoverable")
	}
	if len(r.List()) != 1 {
		t.Fatalf("should have 1 skill, got %d", len(r.List()))
	}

	if err := os.Remove(filepath.Join(projSkills, "my-skill.md")); err != nil {
		t.Fatal(err)
	}
	r.Scan()
	if _, ok := r.Get("my-skill"); ok {
		t.Fatal("should not be discovered after deletion")
	}
}

func TestSkillRankPrecedence(t *testing.T) {
	root := t.TempDir()
	r := skills.New(root)
	writeSkill(t, filepath.Join(root, ".dsh", "skills"), "cmd", "project version")
	writeSkill(t, filepath.Join(root, ".config", "skills"), "cmd", "custom version") // rank 2

	r.Scan()
	s, ok := r.Get("cmd")
	if !ok {
		t.Fatal("should have cmd")
	}
	if s.Rank != 0 {
		t.Fatalf("project-dsh should win (rank 0), got rank %d", s.Rank)
	}
	if !containsStr(s.Content, "project version") {
		t.Fatalf("content should take project version, got %q", s.Content)
	}

	cands := r.Candidates()
	if len(cands) != 1 || cands[0].Rank != 0 {
		t.Fatalf("candidates should be 1 item with rank 0, got %+v", cands)
	}
}

func TestSkillToolInjection(t *testing.T) {
	root := t.TempDir()
	r := skills.New(root)
	writeSkill(t, filepath.Join(root, ".dsh", "skills"), "refactor", "Refactor tips")
	r.Scan()

	tool := skills.NewSkillTool(r)
	content, err := tool.Resolve("refactor", true) // by model
	if err != nil {
		t.Fatal(err)
	}
	if !containsStr(content, "Refactor tips") {
		t.Fatalf("should inject body, got %q", content)
	}

	if _, err := tool.Resolve("nope", true); err == nil {
		t.Fatal("non-existent skill should error")
	}
}

func TestSkillWatchChange(t *testing.T) {
	root := t.TempDir()
	r := skills.New(root)
	stop := make(chan struct{})
	var mu sync.Mutex
	calls := 0
	go r.Watch(stop, 20*time.Millisecond, func([]string) {
		mu.Lock()
		calls++
		mu.Unlock()
	})

	time.Sleep(60 * time.Millisecond)
	writeSkill(t, filepath.Join(root, ".dsh", "skills"), "newskill", "body")
	time.Sleep(120 * time.Millisecond)
	close(stop)

	mu.Lock()
	n := calls
	mu.Unlock()
	if n < 1 {
		t.Fatal("watch should trigger onChange after directory added")
	}
	if _, ok := r.Get("newskill"); !ok {
		t.Fatal("skill should be visible after watch")
	}
}
