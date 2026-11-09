package fs

import (
	"sync"
)

type FsObservation struct {
	Kind    string // "present" | "absent"
	Version FsVersion
}

//

type ObservationPolicy struct {
	mu    sync.Mutex
	state map[string]map[FsTargetKey]FsObservation // owner -> targetKey -> observation
}

func NewObservationPolicy() *ObservationPolicy {
	return &ObservationPolicy{state: map[string]map[FsTargetKey]FsObservation{}}
}

func (p *ObservationPolicy) Record(owner string, key FsTargetKey, obs FsObservation) {
	p.mu.Lock()
	defer p.mu.Unlock()
	m, ok := p.state[owner]
	if !ok {
		m = map[FsTargetKey]FsObservation{}
		p.state[owner] = m
	}
	m[key] = obs
}

type Decision struct {
	WriteIntent *FsWriteIntent

	EditVersion *FsVersion

	Deny bool

	DenyCode FsErrorCode
}

//

func (p *ObservationPolicy) DecideWrite(owner string, key FsTargetKey) Decision {
	p.mu.Lock()
	obs, ok := p.state[owner][key]
	p.mu.Unlock()
	if !ok {
		return Decision{WriteIntent: &FsWriteIntent{Kind: "createIfAbsent"}}
	}
	if obs.Kind == "absent" {
		return Decision{WriteIntent: &FsWriteIntent{Kind: "createIfAbsent"}}
	}
	return Decision{WriteIntent: &FsWriteIntent{Kind: "replaceIfVersion", Version: obs.Version}}
}

//

func (p *ObservationPolicy) DecideEdit(owner string, key FsTargetKey) Decision {
	p.mu.Lock()
	obs, ok := p.state[owner][key]
	p.mu.Unlock()
	if !ok {
		return Decision{Deny: true, DenyCode: FSErrNotObserved}
	}
	if obs.Kind == "absent" {
		return Decision{Deny: true, DenyCode: FSErrNotFound}
	}
	v := obs.Version
	return Decision{EditVersion: &v}
}

func (p *ObservationPolicy) DropOwner(owner string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.state, owner)
}

// ============================================================================

// ============================================================================

type PolicyFS struct {
	Local *LocalFS
	Obs   *ObservationPolicy
}

func NewPolicyFS(local *LocalFS, obs *ObservationPolicy) *PolicyFS {
	return &PolicyFS{Local: local, Obs: obs}
}

func (p *PolicyFS) ReflectObservation(owner string, t FsTarget) {
	if v, ok := p.Local.ObservedVersion(t); ok {
		p.Obs.Record(owner, t.TargetKey, FsObservation{Kind: "present", Version: v})
	}
}

func (p *PolicyFS) Write(owner string, t FsTarget, content string) (FsWriteOutcome, error) {
	dec := p.Obs.DecideWrite(owner, t.TargetKey)
	return p.Local.WriteText(t, content, dec.WriteIntent)
}

func (p *PolicyFS) Edit(owner string, t FsTarget, edit FsEditRequest) (FsEditOutcome, error) {
	dec := p.Obs.DecideEdit(owner, t.TargetKey)
	if dec.Deny {
		return FsEditOutcome{}, fsErr(dec.DenyCode, "edit %q blocked by observation policy", t.DisplayPath)
	}
	return p.Local.EditText(t, edit, dec.EditVersion)
}
