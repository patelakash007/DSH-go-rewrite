//

//

package sysprompt

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
	"sync"
)

type RuntimeContext struct {
	Name string

	Order int

	Content string
}

type ContextRegistry struct {
	mu       sync.RWMutex
	ctxs     map[string]*RuntimeContext
	lastHash string
}

func NewContextRegistry() *ContextRegistry {
	return &ContextRegistry{ctxs: map[string]*RuntimeContext{}}
}

func (r *ContextRegistry) Register(name string, order int, content string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.ctxs[name]; exists && r.ctxs[name].Content == content && r.ctxs[name].Order == order {
		return false
	}
	r.ctxs[name] = &RuntimeContext{Name: name, Order: order, Content: content}
	return true
}

func (r *ContextRegistry) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.ctxs, name)
}

func (r *ContextRegistry) Compute() (text string, hash string) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := make([]*RuntimeContext, 0, len(r.ctxs))
	for _, c := range r.ctxs {
		items = append(items, c)
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].Order < items[j].Order })
	parts := make([]string, 0, len(items))
	for _, c := range items {
		if c.Content != "" {
			parts = append(parts, c.Content)
		}
	}
	text = strings.Join(parts, "\n")
	if text == "" {
		hash = ""
	} else {
		sum := sha256.Sum256([]byte(text))
		hash = hex.EncodeToString(sum[:])
	}
	return text, hash
}

func (r *ContextRegistry) Snapshot() map[string]string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := map[string]string{}
	for name, c := range r.ctxs {
		out[name] = c.Content
	}
	return out
}

func (r *ContextRegistry) LastHash() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.lastHash
}

func (r *ContextRegistry) SetLastHash(h string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastHash = h
}

type GoalRoundContext struct {
	Active bool

	Round int

	GoalDesc string
}

func (g *GoalRoundContext) Render() string {
	if !g.Active {
		return ""
	}
	return "[Goal continuation round #" + itoa(g.Round) + "] " + g.GoalDesc
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
