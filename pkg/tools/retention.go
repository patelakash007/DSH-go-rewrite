//

//

package tools

import (
	"bytes"
	"encoding/json"
	"sync"
)

// ============================================================================

// ============================================================================

type RetainedEntry struct {
	raw      []byte
	retained bool
}

type ResultRetention struct {
	mu   sync.RWMutex
	data map[string]RetainedEntry
}

func NewResultRetention() *ResultRetention {
	return &ResultRetention{data: map[string]RetainedEntry{}}
}

func (r *ResultRetention) Retain(callID string, value []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.data[callID] = RetainedEntry{
		raw:      append([]byte(nil), value...),
		retained: true,
	}
}

func (r *ResultRetention) Read(callID string) ([]byte, bool) {
	r.mu.RLock()
	e, ok := r.data[callID]
	r.mu.RUnlock()
	if !ok || !e.retained {
		return nil, false
	}

	return append([]byte(nil), e.raw...), true
}

func (r *ResultRetention) Remove(callID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e, ok := r.data[callID]; ok {
		e.retained = false
		r.data[callID] = e
	}
}

func (r *ResultRetention) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.data)
}

// ============================================================================

// ============================================================================

func Canonicalize(value any) []byte {
	switch v := value.(type) {
	case nil:
		return nil
	case []byte:
		return v
	case string:
		return []byte(v)
	default:
		b, err := json.Marshal(value)
		if err != nil {
			return []byte{}
		}
		return b
	}
}

func (r *ResultRetention) RetainResult(res *ToolCallResult) {
	if res == nil {
		return
	}
	r.Retain(res.CallID.Raw(), Canonicalize(res.Value))
}

func Equal(a, b []byte) bool {
	return bytes.Equal(a, b)
}
