//

//

package settings

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/JopenChen/dsh-go/pkg/scope"
)

type Path string

func splitPath(p Path) []string {
	if p == "" {
		return nil
	}
	return strings.Split(string(p), ".")
}

// ============================================================================
// SettingsNamespace
// ============================================================================

type SettingsNamespace struct {
	Name string

	SecretPaths map[Path]struct{}
}

func NewNamespace(name string) *SettingsNamespace {
	return &SettingsNamespace{Name: name, SecretPaths: map[Path]struct{}{}}
}

func (ns *SettingsNamespace) MarkSecret(p Path) {
	ns.SecretPaths[p] = struct{}{}
}

func (ns *SettingsNamespace) IsSecret(p Path) bool {
	_, ok := ns.SecretPaths[p]
	return ok
}

// ============================================================================
// SettingsScope
// ============================================================================

type PathOp struct {
	Set *json.RawMessage `json:"set,omitempty"`
}

func NewSetOp(v any) PathOp {
	raw, _ := json.Marshal(v)
	rm := json.RawMessage(raw)
	return PathOp{Set: &rm}
}

func NewUnsetOp() PathOp {
	return PathOp{}
}

type SettingsScope struct {
	mu        sync.Mutex
	ns        *SettingsNamespace
	revision  uint64
	hostLayer *scope.Layer[string]
	sessLayer *scope.Layer[string]
}

func NewSettingsScope(ns *SettingsNamespace) *SettingsScope {
	return &SettingsScope{
		ns:        ns,
		hostLayer: scope.NewLayer[string](scope.Key("host")),
		sessLayer: scope.NewLayer[string](scope.Key("session")),
	}
}

func (s *SettingsScope) Revision() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.revision
}

func (s *SettingsScope) ApplyHost(path Path, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	s.hostLayer.Register(string(path), string(raw), 0)
	return nil
}

func (s *SettingsScope) Get(path Path) (json.RawMessage, bool) {
	scoped := scope.NewScopedLayers[string]().Push(s.hostLayer).Push(s.sessLayer)
	if v, ok := scoped.Get(string(path)); ok {
		return json.RawMessage(v), true
	}
	return nil, false
}

func (s *SettingsScope) Update(expectedRevision uint64, ops map[Path]PathOp) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.revision != expectedRevision {
		return &ErrRevisionMismatch{Expected: expectedRevision, Actual: s.revision}
	}
	for path, op := range ops {
		if op.Set == nil {

			s.sessLayer.Unregister(string(path))
		} else {
			s.sessLayer.Register(string(path), string(*op.Set), 0)
		}
	}
	s.revision++
	return nil
}

func (s *SettingsScope) Replace(expectedRevision uint64, values map[Path]any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.revision != expectedRevision {
		return &ErrRevisionMismatch{Expected: expectedRevision, Actual: s.revision}
	}

	ns := scope.NewLayer[string](scope.Key("session"))
	for path, v := range values {
		raw, err := json.Marshal(v)
		if err != nil {
			return err
		}
		ns.Register(string(path), string(raw), 0)
	}
	s.sessLayer = ns
	s.revision++
	return nil
}

type ErrRevisionMismatch struct {
	Expected uint64
	Actual   uint64
}

func (e *ErrRevisionMismatch) Error() string {
	return fmt.Sprintf("settings: revision mismatch: expected %d, actual %d", e.Expected, e.Actual)
}

func IsRevisionMismatch(err error) bool {
	_, ok := err.(*ErrRevisionMismatch)
	return ok
}

type DescribedSetting struct {
	Path   Path   `json:"path"`
	Op     string `json:"op"` // "set" / "unset" / "default"
	Value  any    `json:"value,omitempty"`
	Secret bool   `json:"secret,omitempty"`
}

func (s *SettingsScope) Describe(redactSecrets bool) []DescribedSetting {
	s.mu.Lock()
	defer s.mu.Unlock()

	scoped := scope.NewScopedLayers[string]().Push(s.hostLayer).Push(s.sessLayer)
	merged := scoped.Merge()
	pathSet := map[Path]struct{}{}
	for _, e := range merged {
		if e.Name != "" {
			pathSet[Path(e.Name)] = struct{}{}
		}
	}
	paths := make([]Path, 0, len(pathSet))
	for p := range pathSet {
		paths = append(paths, p)
	}
	sort.Slice(paths, func(i, j int) bool { return paths[i] < paths[j] })

	var out []DescribedSetting
	for _, p := range paths {
		secret := s.ns.IsSecret(p)

		sessSet := s.sessLayer.Has(string(p))
		hostSet := s.hostLayer.Has(string(p))
		op := "unset"
		if sessSet {
			op = "set"
		} else if hostSet {
			op = "default"
		}
		ds := DescribedSetting{Path: p, Op: op, Secret: secret}
		if redactSecrets && secret {

		} else if v, ok := s.Get(p); ok {
			var anyV any
			_ = json.Unmarshal(v, &anyV)
			ds.Value = anyV
		}
		out = append(out, ds)
	}
	return out
}
