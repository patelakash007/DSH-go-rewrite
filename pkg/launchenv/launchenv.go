package launchenv

import "runtime"

type Source string

const (
	SourceProcess    Source = "process"
	SourceProjectEnv Source = "project-env"
	SourceUserEnv    Source = "user-env"
)

var sourceOrder = []Source{SourceProcess, SourceProjectEnv, SourceUserEnv}

type Entry struct {
	Value  string
	Source Source
	Path   string
}

type layer struct {
	path   string
	values map[string]string
}

type Snapshot struct {
	bySource map[Source]*layer
}

type LayerInput struct {
	Source Source
	Path   string
	Values map[string]string
}

func New(layers []LayerInput) *Snapshot {
	s := &Snapshot{bySource: map[Source]*layer{}}
	for _, l := range layers {
		cp := &layer{path: l.Path, values: map[string]string{}}
		for k, v := range l.Values {
			cp.values[lookupKey(k)] = v
		}
		s.bySource[l.Source] = cp
	}
	return s
}

func (s *Snapshot) Get(name string) (Entry, bool) {
	return s.GetFrom(name, sourceOrder)
}

func (s *Snapshot) GetFrom(name string, sources []Source) (Entry, bool) {
	key := lookupKey(name)
	for _, src := range sourceOrder {
		if !contains(sources, src) {
			continue
		}
		if l, ok := s.bySource[src]; ok {
			if v, ok := l.values[key]; ok {
				return Entry{Value: v, Source: src, Path: l.path}, true
			}
		}
	}
	return Entry{}, false
}

func lookupKey(name string) string {
	if runtime.GOOS == "windows" {
		return toUpper(name)
	}
	return name
}

func toUpper(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] >= 'a' && b[i] <= 'z' {
			b[i] -= 32
		}
	}
	return string(b)
}

func contains(list []Source, v Source) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
