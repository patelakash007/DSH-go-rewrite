//

//

package credentials

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/JopenChen/dsh-go/pkg/brand"
	"github.com/JopenChen/dsh-go/pkg/storage"
)

// ============================================================================

// ============================================================================

type envRecord struct {
	Values map[string]string `json:"values"`
}

type Store struct {
	mu       sync.Mutex
	domain   *storage.Domain[envRecord]
	writable map[string]bool
}

func NewStore(backend storage.Backend) *Store {
	return &Store{
		domain:   storage.NewDomain[envRecord](backend, "credentials"),
		writable: map[string]bool{},
	}
}

func NewMemoryStore() *Store {
	return NewStore(storage.NewMemoryKV())
}

func (s *Store) load() (envRecord, uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, version, err := s.domain.Get(context.Background(), "env")
	if err != nil {
		if storage.IsKeyNotFound(err) {
			return envRecord{Values: map[string]string{}}, 0
		}
		return envRecord{Values: map[string]string{}}, 0
	}
	if rec.Values == nil {
		rec.Values = map[string]string{}
	}
	return rec, version
}

func (s *Store) Resolve(ctx context.Context, ref brand.CredentialRef) (string, bool) {
	rec, _ := s.load()
	v, ok := rec.Values[ref.Raw()]
	return v, ok
}

func (s *Store) Set(ctx context.Context, ref brand.CredentialRef, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, version, err := s.domain.Get(ctx, "env")
	if err != nil && !storage.IsKeyNotFound(err) {
		return err
	}
	if rec.Values == nil {
		rec.Values = map[string]string{}
	}
	rec.Values[ref.Raw()] = value
	if _, err := s.domain.Put(ctx, "env", rec, version); err != nil {
		return err
	}
	s.writable[ref.Raw()] = true
	return nil
}

func (s *Store) Unset(ctx context.Context, ref brand.CredentialRef) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, version, err := s.domain.Get(ctx, "env")
	if err != nil && !storage.IsKeyNotFound(err) {
		return err
	}
	if rec.Values == nil {
		return nil
	}
	delete(rec.Values, ref.Raw())
	delete(s.writable, ref.Raw())
	_, err = s.domain.Put(ctx, "env", rec, version)
	return err
}

type CredentialInfo struct {
	Ref      brand.CredentialRef `json:"ref"`
	Writable bool                `json:"writable"`
	HasValue bool                `json:"hasValue"`
}

func (s *Store) Describe(ctx context.Context) []CredentialInfo {
	rec, _ := s.load()
	refs := make([]string, 0, len(rec.Values))
	for k := range rec.Values {
		refs = append(refs, k)
	}
	sort.Strings(refs)
	out := make([]CredentialInfo, 0, len(refs))
	for _, k := range refs {
		ref := brand.NewCredentialRef(k)
		_, has := rec.Values[k]
		out = append(out, CredentialInfo{Ref: ref, Writable: s.writable[k], HasValue: has})
	}
	return out
}

// ============================================================================
// AuthorizationFlow & AuthorizationService
// ============================================================================

type FlowID = brand.CredentialRef

type Flow struct {
	ID FlowID

	Name string

	Begin func(ctx context.Context) (string, error)

	Cancel func(ctx context.Context) error
}

type AuthorizationService struct {
	mu    sync.RWMutex
	flows map[string]*Flow
}

func NewAuthorizationService() *AuthorizationService {
	return &AuthorizationService{flows: map[string]*Flow{}}
}

func (a *AuthorizationService) Register(f *Flow) error {
	if f == nil || f.ID.Raw() == "" {
		return fmt.Errorf("credentials: flow with empty id cannot register")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.flows[f.ID.Raw()] = f
	return nil
}

func (a *AuthorizationService) List() []*Flow {
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := make([]*Flow, 0, len(a.flows))
	for _, f := range a.flows {
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID.Raw() < out[j].ID.Raw() })
	return out
}

func (a *AuthorizationService) Begin(ctx context.Context, id FlowID) (string, error) {
	a.mu.RLock()
	f, ok := a.flows[id.Raw()]
	a.mu.RUnlock()
	if !ok {
		return "", fmt.Errorf("credentials: flow %q not found", id.Raw())
	}
	if f.Begin == nil {
		return "", fmt.Errorf("credentials: flow %q has no begin handler", id.Raw())
	}
	return f.Begin(ctx)
}

func (a *AuthorizationService) Cancel(ctx context.Context, id FlowID) error {
	a.mu.RLock()
	f, ok := a.flows[id.Raw()]
	a.mu.RUnlock()
	if !ok {
		return fmt.Errorf("credentials: flow %q not found", id.Raw())
	}
	if f.Cancel == nil {
		return nil
	}
	return f.Cancel(ctx)
}
