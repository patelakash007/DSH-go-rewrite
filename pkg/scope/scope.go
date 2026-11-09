//

//

//

package scope

import (
	"sort"
	"sync"
)

type Key string

type entry[V any] struct {
	name  string
	value V
	rank  int
	seq   uint64
}

type Layer[V any] struct {
	key     Key
	entries []entry[V]
	mu      sync.RWMutex
	seq     uint64
}

func NewLayer[V any](key Key) *Layer[V] {
	return &Layer[V]{key: key}
}

func (l *Layer[V]) Key() Key {
	return l.key
}

func (l *Layer[V]) Register(name string, value V, rank int) uint64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.seq++
	l.entries = append(l.entries, entry[V]{name: name, value: value, rank: rank, seq: l.seq})
	return l.seq
}

func (l *Layer[V]) get(name string) (V, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	var (
		found  bool
		winner entry[V]
	)
	for _, e := range l.entries {
		if e.name != name {
			continue
		}
		if !found || e.rank > winner.rank || (e.rank == winner.rank && e.seq > winner.seq) {
			found = true
			winner = e
		}
	}
	return winner.value, found
}

func (l *Layer[V]) Get(name string) (V, bool) {
	return l.get(name)
}

func (l *Layer[V]) Has(name string) bool {
	_, ok := l.get(name)
	return ok
}

func (l *Layer[V]) Unregister(name string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := l.entries[:0]
	for _, e := range l.entries {
		if e.name == name {
			continue
		}
		out = append(out, e)
	}
	l.entries = out
}

func (l *Layer[V]) snapshot() []entry[V] {
	l.mu.RLock()
	defer l.mu.RUnlock()
	out := make([]entry[V], len(l.entries))
	copy(out, l.entries)
	return out
}

type ScopedLayers[V any] struct {
	mu     sync.RWMutex
	layers []*Layer[V]
}

func NewScopedLayers[V any]() *ScopedLayers[V] {
	return &ScopedLayers[V]{}
}

func (sl *ScopedLayers[V]) Push(layer *Layer[V]) *ScopedLayers[V] {
	sl.mu.Lock()
	defer sl.mu.Unlock()
	sl.layers = append(sl.layers, layer)
	return sl
}

func (sl *ScopedLayers[V]) Get(name string) (V, bool) {
	if name == "" {
		var zero V
		return zero, false
	}
	sl.mu.RLock()
	defer sl.mu.RUnlock()

	for i := len(sl.layers) - 1; i >= 0; i-- {
		if v, ok := sl.layers[i].get(name); ok {
			return v, true
		}
	}
	var zero V
	return zero, false
}

type ResolvedEntry[V any] struct {
	Name  string
	Value V
	Rank  int
}

//

func (sl *ScopedLayers[V]) Merge() []ResolvedEntry[V] {
	sl.mu.RLock()
	layers := make([]*Layer[V], len(sl.layers))
	copy(layers, sl.layers)
	sl.mu.RUnlock()

	nameSet := map[string]struct{}{}
	var anonymous []ResolvedEntry[V]
	for _, layer := range layers {
		for _, e := range layer.snapshot() {
			if e.name == "" {
				anonymous = append(anonymous, ResolvedEntry[V]{Name: "", Value: e.value, Rank: e.rank})
				continue
			}
			nameSet[e.name] = struct{}{}
		}
	}

	names := make([]string, 0, len(nameSet))
	for n := range nameSet {
		names = append(names, n)
	}

	sort.Strings(names)

	resolved := make([]ResolvedEntry[V], 0, len(names)+len(anonymous))
	for _, n := range names {
		if v, ok := sl.Get(n); ok {
			resolved = append(resolved, ResolvedEntry[V]{Name: n, Value: v, Rank: 0})
		}
	}
	resolved = append(resolved, anonymous...)
	return resolved
}
