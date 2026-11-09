//

//

//

package tokenmeter

import (
	"fmt"
	"sync"
)

type TokenUsage struct {
	PromptTokens int `json:"promptTokens"`

	CompletionTokens int `json:"completionTokens"`
}

func (u TokenUsage) Total() int {
	return u.PromptTokens + u.CompletionTokens
}

type RequestRecord struct {
	Seq   int        `json:"seq"`
	Usage TokenUsage `json:"usage"`
}

type Meter struct {
	mu        sync.Mutex
	budget    int
	seq       int
	records   []RequestRecord
	totalProm int
	totalComp int
	denied    bool
}

func New(budget int) *Meter {
	return &Meter{budget: budget}
}

func (m *Meter) Budget() int {
	return m.budget
}

func (m *Meter) Record(u TokenUsage) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seq++
	m.totalProm += u.PromptTokens
	m.totalComp += u.CompletionTokens
	m.records = append(m.records, RequestRecord{Seq: m.seq, Usage: u})
	if m.budget > 0 && m.totalProm+m.totalComp >= m.budget {
		m.denied = true
	}
}

func (m *Meter) HasBudget() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return !m.denied
}

func (m *Meter) BudgetDenied() bool {
	return !m.HasBudget()
}

func (m *Meter) Totals() (prompt, completion, total int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.totalProm, m.totalComp, m.totalProm + m.totalComp
}

func (m *Meter) Records() []RequestRecord {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]RequestRecord, len(m.records))
	copy(out, m.records)
	return out
}

const BudgetDeniedError = "token-meter: session budget exceeded; request rejected before LLM call"

func (m *Meter) Check() error {
	if m.BudgetDenied() {
		return fmt.Errorf("%s (prompt+completion >= %d)", BudgetDeniedError, m.budget)
	}
	return nil
}

type SurfacePricing map[string]int

var defaultSurfacePricing = SurfacePricing{
	"assistant/message": 4,
	"tool/result":       2,
	"user/message":      1,
}

func (p SurfacePricing) CostOf(entries []SurfaceNode) int {
	total := 0
	for _, e := range entries {
		if w, ok := p[e.Kind]; ok {
			total += w * e.Count
		}
	}
	return total
}

type SurfaceNode struct {
	Kind string

	Count int
}
