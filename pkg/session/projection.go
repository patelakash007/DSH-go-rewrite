//

//

package session

import (
	"sort"
	"sync"

	"github.com/JopenChen/dsh-go/pkg/brand"
)

type ProjectionState interface {
	Equal(other ProjectionState) bool
}

type ProjectionDefinition[S ProjectionState] struct {
	ID brand.ProjectionID

	Init S

	Apply func(state S, ev SessionEvent) S

	Merge func(events []SessionEvent) S
}

type Projection[S ProjectionState] struct {
	def   *ProjectionDefinition[S]
	state S
}

func (p *Projection[S]) State() S {
	return p.state
}

type projectionBox interface {
	apply(ev SessionEvent) bool
	projectionID() brand.ProjectionID
	stateValue() any
	rebuild(events []SessionEvent)
}

func (p *Projection[S]) apply(ev SessionEvent) bool {
	if p.def == nil || p.def.Apply == nil {
		return false
	}
	newState := p.def.Apply(p.state, ev)
	changed := !newState.Equal(p.state)
	if changed {
		p.state = newState
	}
	return changed
}

func (p *Projection[S]) projectionID() brand.ProjectionID {
	return p.def.ID
}

func (p *Projection[S]) stateValue() any { return p.state }

func (p *Projection[S]) rebuild(events []SessionEvent) {
	if p.def == nil {
		return
	}
	if p.def.Merge != nil {
		p.state = p.def.Merge(events)
		return
	}
	p.state = p.def.Init
	for _, ev := range events {
		if p.def.Apply != nil {
			p.state = p.def.Apply(p.state, ev)
		}
	}
}

type Registry struct {
	mu        sync.RWMutex
	defs      map[string]projectionBox // ProjectionID.Raw() -> projectionBox
	seq       uint64
	changelog []ProjectionChange
}

type ProjectionChange struct {
	ProjectionID brand.ProjectionID `json:"projectionId"`

	Seq uint64 `json:"seq"`

	EventSeq uint64 `json:"eventSeq"`

	Changed bool `json:"changed"`
}

func NewProjectionRegistry() *Registry {
	return &Registry{defs: map[string]projectionBox{}}
}

func RegisterProjection[S ProjectionState](r *Registry, def *ProjectionDefinition[S]) (*Projection[S], error) {
	if def == nil || def.ID.IsZero() {
		return nil, &projectionError{"nil or zero-id projection"}
	}
	p := &Projection[S]{def: def, state: def.Init}
	r.mu.Lock()
	r.defs[def.ID.Raw()] = p
	r.mu.Unlock()
	return p, nil
}

type projectionError struct{ msg string }

func (e *projectionError) Error() string { return "projection: " + e.msg }

func (r *Registry) ApplyEvents(events []SessionEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, ev := range events {
		r.seq++
		for _, box := range r.defs {
			changed := box.apply(ev)
			r.changelog = append(r.changelog, ProjectionChange{
				ProjectionID: box.projectionID(),
				Seq:          r.seq,
				EventSeq:     ev.Seq,
				Changed:      changed,
			})
		}
	}
}

func (r *Registry) Changelog() []ProjectionChange {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ProjectionChange, len(r.changelog))
	copy(out, r.changelog)
	return out
}

func (r *Registry) SnapshotAll() map[brand.ProjectionID]any {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := map[brand.ProjectionID]any{}
	keys := make([]string, 0, len(r.defs))
	for id := range r.defs {
		keys = append(keys, id)
	}
	sort.Strings(keys)
	for _, id := range keys {
		out[brand.NewProjectionID(id)] = r.defs[id].stateValue()
	}
	return out
}

func (r *Registry) Rebuild(events []SessionEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, box := range r.defs {
		box.rebuild(events)
	}
}
