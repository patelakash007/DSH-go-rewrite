//

//

//

package registry

import (
	"errors"
	"sync"
)

var ErrFrozen = errors.New("registry: frozen, mutation not allowed")

type Freezable[K comparable, V any] struct {
	mu sync.RWMutex
	m  map[K]V

	frozen   bool
	snapshot map[K]V
}

func NewFreezable[K comparable, V any]() *Freezable[K, V] {
	return &Freezable[K, V]{m: map[K]V{}}
}

func NewFreezableFrom[K comparable, V any](entries map[K]V) *Freezable[K, V] {
	cp := make(map[K]V, len(entries))
	for k, v := range entries {
		cp[k] = v
	}
	return &Freezable[K, V]{m: cp}
}

func (r *Freezable[K, V]) Put(k K, v V) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.frozen {
		return ErrFrozen
	}
	r.m[k] = v
	return nil
}

func (r *Freezable[K, V]) Remove(k K) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.frozen {
		return ErrFrozen
	}
	delete(r.m, k)
	return nil
}

func (r *Freezable[K, V]) snapshotRef() map[K]V {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.snapshot
}

func (r *Freezable[K, V]) Get(k K) (V, bool) {
	if snap := r.snapshotRef(); snap != nil {
		v, ok := snap[k]
		return v, ok
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	v, ok := r.m[k]
	return v, ok
}

func (r *Freezable[K, V]) Contains(k K) bool {
	if snap := r.snapshotRef(); snap != nil {
		_, ok := snap[k]
		return ok
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.m[k]
	return ok
}

func (r *Freezable[K, V]) Len() int {
	if snap := r.snapshotRef(); snap != nil {
		return len(snap)
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.m)
}

func (r *Freezable[K, V]) Keys() []K {
	if snap := r.snapshotRef(); snap != nil {
		return keysOf(snap)
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return keysOf(r.m)
}

func keysOf[K comparable, V any](m map[K]V) []K {
	out := make([]K, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func (r *Freezable[K, V]) Freeze() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.frozen {
		return
	}
	snap := make(map[K]V, len(r.m))
	for k, v := range r.m {
		snap[k] = v
	}

	r.snapshot = snap
	r.frozen = true

	r.m = nil
}

func (r *Freezable[K, V]) IsFrozen() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.frozen
}

func (r *Freezable[K, V]) Clone() *Freezable[K, V] {
	r.mu.RLock()
	src := r.m
	if r.frozen {
		src = r.snapshot
	}
	cp := make(map[K]V, len(src))
	for k, v := range src {
		cp[k] = v
	}
	r.mu.RUnlock()
	return &Freezable[K, V]{m: cp}
}
