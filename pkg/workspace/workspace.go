//

//

package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/JopenChen/dsh-go/pkg/brand"
	"github.com/JopenChen/dsh-go/pkg/storage"
)

// ============================================================================

// ============================================================================

type Workspace struct {
	ID           brand.WorkspaceID `json:"id"`
	Root         string            `json:"root"`
	SessionGroup string            `json:"sessionGroup,omitempty"`
	ResumeOnOpen *brand.SessionID  `json:"resumeOnOpen,omitempty"`
}

// ============================================================================
// Registry
// ============================================================================

type Registry struct {
	domain *storage.Domain[Workspace]
}

func New(backend storage.Backend) *Registry {
	return &Registry{domain: storage.NewDomain[Workspace](backend, "workspace/")}
}

func normalize(root string) (string, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return "", fmt.Errorf("workspace: empty root path")
	}

	abs, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return "", fmt.Errorf("workspace: resolve root %q: %w", root, err)
	}
	return abs, nil
}

func idFromRoot(normalizedRoot string) brand.WorkspaceID {
	sum := sha256.Sum256([]byte(normalizedRoot))
	return brand.NewWorkspaceID("ws_" + hex.EncodeToString(sum[:12]))
}

func (r *Registry) Create(ctx context.Context, root string) (brand.WorkspaceID, uint64, error) {
	normalized, err := normalize(root)
	if err != nil {
		return brand.WorkspaceID{}, 0, err
	}
	id := idFromRoot(normalized)

	if ws, ver, gerr := r.domain.Get(ctx, id.Raw()); gerr == nil {
		_ = ws
		return id, ver, nil
	} else if !storage.IsKeyNotFound(gerr) {
		return brand.WorkspaceID{}, 0, gerr
	}

	rec := Workspace{ID: id, Root: normalized}
	ver, err := r.domain.Put(ctx, id.Raw(), rec, 0)
	if err != nil {
		return brand.WorkspaceID{}, 0, fmt.Errorf("workspace: create %q: %w", normalized, err)
	}
	return id, ver, nil
}

func (r *Registry) Get(ctx context.Context, id brand.WorkspaceID) (Workspace, uint64, error) {
	if id.IsZero() {
		return Workspace{}, 0, fmt.Errorf("workspace: missing id")
	}
	return r.domain.Get(ctx, id.Raw())
}

func (r *Registry) List(ctx context.Context) ([]Workspace, error) {
	keys, err := r.domain.List(ctx)
	if err != nil {
		return nil, err
	}
	var out []Workspace
	for _, k := range keys {
		ws, _, err := r.domain.Get(ctx, k)
		if err != nil {
			return nil, err
		}
		out = append(out, ws)
	}
	return out, nil
}

func (r *Registry) SetSessionGroup(ctx context.Context, id brand.WorkspaceID, group string) (uint64, error) {
	return r.update(ctx, id, func(ws *Workspace) {
		ws.SessionGroup = group
	})
}

func (r *Registry) SetResumeOnOpen(ctx context.Context, id brand.WorkspaceID, sid brand.SessionID) (uint64, error) {
	return r.update(ctx, id, func(ws *Workspace) {
		cp := sid
		ws.ResumeOnOpen = &cp
	})
}

func (r *Registry) ClearResumeOnOpen(ctx context.Context, id brand.WorkspaceID) (uint64, error) {
	return r.update(ctx, id, func(ws *Workspace) {
		ws.ResumeOnOpen = nil
	})
}

func (r *Registry) update(ctx context.Context, id brand.WorkspaceID, mutate func(*Workspace)) (uint64, error) {
	if id.IsZero() {
		return 0, fmt.Errorf("workspace: missing id")
	}
	ws, ver, err := r.domain.Get(ctx, id.Raw())
	if err != nil {
		return 0, err
	}
	mutate(&ws)
	return r.domain.Put(ctx, id.Raw(), ws, ver)
}
