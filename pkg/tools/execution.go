//

//

package tools

import (
	"context"
	"sync"
)

type concludeKey struct{}

type ConcludeFunc func(reason string)

func WithConclude(ctx context.Context, fn ConcludeFunc) context.Context {
	return context.WithValue(ctx, concludeKey{}, fn)
}

func ConcludeFrom(ctx context.Context) ConcludeFunc {
	if fn, ok := ctx.Value(concludeKey{}).(ConcludeFunc); ok {
		return fn
	}
	return nil
}

type ToolRunContext struct {
	mu        sync.Mutex
	meta      map[string]any
	deferred  []func()
	concluded bool
	conclude  ConcludeFunc
}

func NewToolRunContext(conclude ConcludeFunc) *ToolRunContext {
	return &ToolRunContext{meta: map[string]any{}, conclude: conclude}
}

func (tc *ToolRunContext) SetMeta(k string, v any) {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	tc.meta[k] = v
}

func (tc *ToolRunContext) GetMeta(k string) (any, bool) {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	v, ok := tc.meta[k]
	return v, ok
}

func (tc *ToolRunContext) DeferContext(fn func()) {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	tc.deferred = append(tc.deferred, fn)
}

func (tc *ToolRunContext) runDeferred() {
	tc.mu.Lock()
	defers := make([]func(), len(tc.deferred))
	copy(defers, tc.deferred)
	tc.deferred = nil
	tc.mu.Unlock()
	for i := len(defers) - 1; i >= 0; i-- {
		if defers[i] != nil {
			defers[i]()
		}
	}
}

func (tc *ToolRunContext) ConcludeTurn(reason string) bool {
	tc.mu.Lock()
	if tc.concluded {
		tc.mu.Unlock()
		return false
	}
	tc.concluded = true
	conclude := tc.conclude
	tc.mu.Unlock()

	tc.runDeferred()
	if conclude != nil {
		conclude(reason)
	}
	return true
}

func (tc *ToolRunContext) IsConcluded() bool {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	return tc.concluded
}

func (tc *ToolRunContext) Context() context.Context {
	return withRunContext(context.Background(), tc)
}

type runContextKey struct{}

func withRunContext(ctx context.Context, tc *ToolRunContext) context.Context {
	return context.WithValue(ctx, runContextKey{}, tc)
}

func ToolRunContextFrom(ctx context.Context) (*ToolRunContext, bool) {
	tc, ok := ctx.Value(runContextKey{}).(*ToolRunContext)
	return tc, ok
}
