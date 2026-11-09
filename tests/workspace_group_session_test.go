//

package tests

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/JopenChen/dsh-go/pkg/brand"
	"github.com/JopenChen/dsh-go/pkg/storage"
	"github.com/JopenChen/dsh-go/pkg/workspace"
)

func TestWorkspaceSameRootSameID(t *testing.T) {
	reg := workspace.New(storage.NewMemoryKV())
	ctx := context.Background()
	root := filepath.Join("some", "dir", "proj")

	id1, _, err := reg.Create(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	id2, _, err := reg.Create(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if id1 != id2 {
		t.Fatalf("same root should return same id, got %v vs %v", id1, id2)
	}

	ws, _, err := reg.Get(ctx, id1)
	if err != nil {
		t.Fatal(err)
	}
	if ws.ID != id1 {
		t.Fatalf("record.ID should equal id1, got %v", ws.ID)
	}
}

func TestWorkspaceDifferentRootDifferentID(t *testing.T) {
	reg := workspace.New(storage.NewMemoryKV())
	ctx := context.Background()
	a, _, _ := reg.Create(ctx, "proj/a")
	b, _, _ := reg.Create(ctx, "proj/b")
	if a == b {
		t.Fatalf("different roots should have different ids, got %v for both", a)
	}
}

func TestWorkspaceResumeOnOpen(t *testing.T) {
	reg := workspace.New(storage.NewMemoryKV())
	ctx := context.Background()
	id, _, _ := reg.Create(ctx, "proj/p1")
	sid := brand.NewSessionID("sess-123")

	if _, err := reg.SetResumeOnOpen(ctx, id, sid); err != nil {
		t.Fatal(err)
	}
	ws, _, err := reg.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if ws.ResumeOnOpen == nil || *ws.ResumeOnOpen != sid {
		t.Fatalf("resume-on-open should be %v, got %v", sid, ws.ResumeOnOpen)
	}

	if _, err := reg.ClearResumeOnOpen(ctx, id); err != nil {
		t.Fatal(err)
	}
	ws2, _, _ := reg.Get(ctx, id)
	if ws2.ResumeOnOpen != nil {
		t.Fatalf("resume-on-open should be nil after clearing, got %v", ws2.ResumeOnOpen)
	}
}

func TestWorkspaceSessionGroup(t *testing.T) {
	reg := workspace.New(storage.NewMemoryKV())
	ctx := context.Background()
	id, _, _ := reg.Create(ctx, "proj/p2")
	if _, err := reg.SetSessionGroup(ctx, id, "team-alpha"); err != nil {
		t.Fatal(err)
	}
	ws, _, _ := reg.Get(ctx, id)
	if ws.SessionGroup != "team-alpha" {
		t.Fatalf("sessionGroup should be team-alpha, got %q", ws.SessionGroup)
	}
}

func TestWorkspaceList(t *testing.T) {
	reg := workspace.New(storage.NewMemoryKV())
	ctx := context.Background()
	_, _, _ = reg.Create(ctx, "b")
	_, _, _ = reg.Create(ctx, "a")
	_, _, _ = reg.Create(ctx, "c")
	wsList, err := reg.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(wsList) != 3 {
		t.Fatalf("should have 3 workspaces, got %d", len(wsList))
	}

	seen := map[string]bool{}
	for _, ws := range wsList {
		if seen[ws.Root] {
			t.Fatalf("duplicate root %q", ws.Root)
		}
		seen[ws.Root] = true
	}
}

func TestWorkspaceMissingRootRejected(t *testing.T) {
	reg := workspace.New(storage.NewMemoryKV())
	ctx := context.Background()
	if _, _, err := reg.Create(ctx, "   "); err == nil {
		t.Fatal("empty root should error")
	}
}
