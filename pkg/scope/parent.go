//

package scope

import (
	"errors"
	"sync"
)

var ErrAlreadyBound = errors.New("scope: key already bound to a parent")

var ErrCycle = errors.New("scope: parent link would form a cycle")

type ParentTree struct {
	mu      sync.RWMutex
	parents map[Key]Key
}

func NewParentTree() *ParentTree {
	return &ParentTree{parents: map[Key]Key{}}
}

func (t *ParentTree) Bind(key, parent Key) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, exists := t.parents[key]; exists {
		return ErrAlreadyBound
	}

	for cursor := parent; ; {
		if cursor == key {
			return ErrCycle
		}
		next, ok := t.parents[cursor]
		if !ok {
			break
		}
		cursor = next
	}
	t.parents[key] = parent
	return nil
}

func (t *ParentTree) Rebind(key, parent Key) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	for cursor := parent; ; {
		if cursor == key {
			return ErrCycle
		}
		next, ok := t.parents[cursor]
		if !ok {
			break
		}
		cursor = next
	}
	t.parents[key] = parent
	return nil
}

func (t *ParentTree) ParentOf(key Key) (Key, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	p, ok := t.parents[key]
	return p, ok
}

func (t *ParentTree) ChainOf(key Key) []Key {
	t.mu.RLock()
	defer t.mu.RUnlock()
	chain := []Key{}
	visited := map[Key]struct{}{}
	for cursor := key; ; {
		if _, seen := visited[cursor]; seen {
			break
		}
		visited[cursor] = struct{}{}
		chain = append(chain, cursor)
		next, ok := t.parents[cursor]
		if !ok {
			break
		}
		cursor = next
	}
	return chain
}
