//

//

package agent

import "sync"

type ModelSelection struct {
	Provider string

	Model string

	ReasoningEffort string
}

type ModelSelectionRef struct {
	mu sync.RWMutex

	current *ModelSelection

	assembled *ModelSelection
}

func NewModelSelectionRef() *ModelSelectionRef {
	return &ModelSelectionRef{}
}

func (r *ModelSelectionRef) Select(sel *ModelSelection) {
	r.mu.Lock()
	r.current = cloneSelection(sel)
	r.mu.Unlock()
}

func (r *ModelSelectionRef) Current() *ModelSelection {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return cloneSelection(r.current)
}

func (r *ModelSelectionRef) Capture() *ModelSelection {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.assembled = cloneSelection(r.current)
	return cloneSelection(r.assembled)
}

func (r *ModelSelectionRef) Assembled() *ModelSelection {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return cloneSelection(r.assembled)
}

type RequestConfig struct {
	Provider        string
	Model           string
	ReasoningEffort string
}

func (r *ModelSelectionRef) Apply(cfg RequestConfig) RequestConfig {
	r.mu.RLock()
	sel := cloneSelection(r.assembled)
	r.mu.RUnlock()

	if sel == nil {
		return cfg
	}
	out := cfg
	out.ReasoningEffort = ""
	out.Provider = sel.Provider
	out.Model = sel.Model
	if sel.ReasoningEffort != "" {
		out.ReasoningEffort = sel.ReasoningEffort
	}
	return out
}

func cloneSelection(s *ModelSelection) *ModelSelection {
	if s == nil {
		return nil
	}
	cp := *s
	return &cp
}
