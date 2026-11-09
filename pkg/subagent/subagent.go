//

//

package subagent

import (
	"context"
	"sync"

	"github.com/JopenChen/dsh-go/pkg/brand"
)

// ============================================================================

// ============================================================================

type ForkLineage struct {
	Parent  *brand.SessionID `json:"parent,omitempty"`
	Session brand.SessionID  `json:"session"`
}

type SpawnRequest struct {
	Parent    *brand.SessionID `json:"parent,omitempty"`
	Input     string           `json:"input"`
	MaxRounds int              `json:"maxRounds,omitempty"`
}

type SpawnResult struct {
	Lineage ForkLineage `json:"lineage"`
	Output  string      `json:"output"`
	Err     string      `json:"err,omitempty"`
}

type Provider interface {
	Name() string

	Spawn(ctx context.Context, req SpawnRequest) (*Handle, error)
}

type Handle struct {
	Provider string
	Lineage  ForkLineage
	mu       sync.Mutex
	disposed bool
	drained  bool
	done     chan struct{}
}

func newHandle(provider string, lineage ForkLineage) *Handle {
	return &Handle{Provider: provider, Lineage: lineage, done: make(chan struct{})}
}

func (h *Handle) markDrained() {
	h.mu.Lock()
	if !h.drained {
		h.drained = true
		close(h.done)
	}
	h.mu.Unlock()
}

func (h *Handle) Drain(ctx context.Context) error {
	select {
	case <-h.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (h *Handle) Dispose() {
	h.mu.Lock()
	h.disposed = true
	h.mu.Unlock()

	h.markDrained()
}

type acpProvider struct{}

func (acpProvider) Name() string { return "acp" }

func (acpProvider) Spawn(ctx context.Context, req SpawnRequest) (*Handle, error) {
	sid := brand.NewSessionID("acp-" + req.Input)
	h := newHandle("acp", ForkLineage{Parent: req.Parent, Session: sid})

	h.markDrained()
	return h, nil
}

type forkCopyProvider struct{}

func (forkCopyProvider) Name() string { return "fork-copy" }

func (forkCopyProvider) Spawn(ctx context.Context, req SpawnRequest) (*Handle, error) {
	sid := brand.NewSessionID("fork-" + req.Input)
	h := newHandle("fork-copy", ForkLineage{Parent: req.Parent, Session: sid})
	h.markDrained()
	return h, nil
}

// ============================================================================
// Runtime
// ============================================================================

type Runtime struct {
	mu        sync.RWMutex
	providers map[string]Provider
	handles   map[string]*Handle
}

func NewRuntime() *Runtime {
	r := &Runtime{providers: map[string]Provider{}, handles: map[string]*Handle{}}

	_ = r.RegisterProvider(acpProvider{})
	_ = r.RegisterProvider(forkCopyProvider{})
	return r
}

func (r *Runtime) RegisterProvider(p Provider) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[p.Name()] = p
	return nil
}

func (r *Runtime) Provider(name string) (Provider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[name]
	return p, ok
}

func (r *Runtime) Spawn(ctx context.Context, providerName string, req SpawnRequest) (*Handle, error) {
	p, ok := r.Provider(providerName)
	if !ok {
		return nil, &ErrUnknownProvider{Name: providerName}
	}
	h, err := p.Spawn(ctx, req)
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	r.handles[h.Lineage.Session.Raw()] = h
	r.mu.Unlock()
	return h, nil
}

func (r *Runtime) DisposeOwner(parent brand.SessionID) {
	r.mu.RLock()
	handles := make([]*Handle, 0, len(r.handles))
	for _, h := range r.handles {
		if h.Lineage.Parent != nil && *h.Lineage.Parent == parent {
			handles = append(handles, h)
		}
	}
	r.mu.RUnlock()
	for _, h := range handles {
		h.Dispose()
	}
}

func (r *Runtime) ActiveCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.handles)
}

// ============================================================================

// ============================================================================

type ErrUnknownProvider struct{ Name string }

func (e *ErrUnknownProvider) Error() string {
	return "subagent: unknown provider " + e.Name
}

func IsUnknownProvider(err error) bool {
	_, ok := err.(*ErrUnknownProvider)
	return ok
}
