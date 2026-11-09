//

//

//

package eventbus

import (
	"sync"
)

type Listener[T any, R any] func(payload T) R

type Bus[T any, R any] struct {
	mu        sync.RWMutex
	listeners []listenerSlot[T, R]
	nextID    uint64
}

type listenerSlot[T any, R any] struct {
	id uint64
	fn Listener[T, R]
}

func New[T any, R any]() *Bus[T, R] {
	return &Bus[T, R]{}
}

func (b *Bus[T, R]) On(fn Listener[T, R]) func() {
	b.mu.Lock()
	b.nextID++
	id := b.nextID
	b.listeners = append(b.listeners, listenerSlot[T, R]{id: id, fn: fn})
	b.mu.Unlock()

	var removed bool
	return func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if removed {
			return
		}
		removed = true
		for i := range b.listeners {
			if b.listeners[i].id == id {
				b.listeners = append(b.listeners[:i], b.listeners[i+1:]...)
				return
			}
		}
	}
}

func (b *Bus[T, R]) snapshot() []Listener[T, R] {
	b.mu.RLock()
	out := make([]Listener[T, R], len(b.listeners))
	for i := range b.listeners {
		out[i] = b.listeners[i].fn
	}
	b.mu.RUnlock()
	return out
}

func (b *Bus[T, R]) Len() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.listeners)
}

func (b *Bus[T, R]) Emit(payload T) {
	for _, fn := range b.snapshot() {
		fn(payload)
	}
}

func (b *Bus[T, R]) Bail(payload T, shouldShort func(R) bool) (R, bool) {
	var zero R
	for _, fn := range b.snapshot() {
		r := fn(payload)
		if shouldShort != nil && shouldShort(r) {
			return r, true
		}
	}
	return zero, false
}

func (b *Bus[T, R]) Serial(payload T) []R {
	fns := b.snapshot()
	out := make([]R, 0, len(fns))
	for _, fn := range fns {
		out = append(out, fn(payload))
	}
	return out
}

type Signal[T any] struct {
	inner *Bus[T, struct{}]
}

func NewSignal[T any]() *Signal[T] {
	return &Signal[T]{inner: New[T, struct{}]()}
}

func (s *Signal[T]) On(fn func(T)) func() {
	return s.inner.On(func(payload T) struct{} {
		fn(payload)
		return struct{}{}
	})
}

func (s *Signal[T]) Emit(payload T) {
	s.inner.Emit(payload)
}

func (s *Signal[T]) Len() int { return s.inner.Len() }
