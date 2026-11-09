//

//

//

package credentials

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/JopenChen/dsh-go/pkg/brand"
)

// ============================================================================

// ============================================================================

type AuthStatus string

const (
	AuthPending   AuthStatus = "pending"
	AuthCompleted AuthStatus = "completed"
	AuthCancelled AuthStatus = "cancelled"
)

type AuthSession struct {
	ID        string              `json:"id"`
	FlowID    FlowID              `json:"flowId"`
	Status    AuthStatus          `json:"status"`
	StartedAt time.Time           `json:"startedAt"`
	CredRef   brand.CredentialRef `json:"credRef,omitempty"`
	HasValue  bool                `json:"hasValue"`
}

// ============================================================================

// ============================================================================

type AuthService struct {
	auth     *AuthorizationService
	store    *Store
	mu       sync.Mutex
	sessions map[string]*AuthSession
	next     int
}

func NewAuthService(auth *AuthorizationService, store *Store) *AuthService {
	return &AuthService{
		auth:     auth,
		store:    store,
		sessions: map[string]*AuthSession{},
	}
}

func (s *AuthService) newSessionID() string {
	s.next++
	return fmt.Sprintf("auth-%04d", s.next)
}

func (s *AuthService) Begin(ctx context.Context, flowID FlowID) (token string, err error) {

	hint, err := s.auth.Begin(ctx, flowID)
	if err != nil {
		return "", err
	}
	_ = hint

	s.mu.Lock()
	defer s.mu.Unlock()
	id := s.newSessionID()
	sess := &AuthSession{ID: id, FlowID: flowID, Status: AuthPending, StartedAt: time.Now()}
	s.sessions[id] = sess
	return id, nil
}

func (s *AuthService) Complete(ctx context.Context, token string, value string) error {
	s.mu.Lock()
	sess, ok := s.sessions[token]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("credentials: auth session %q not found", token)
	}
	if sess.Status != AuthPending {
		s.mu.Unlock()
		return fmt.Errorf("credentials: auth session %q already %s", token, sess.Status)
	}

	ref := sess.FlowID
	s.mu.Unlock()

	if err := s.store.Set(ctx, ref, value); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sess.Status = AuthCompleted
	sess.CredRef = ref
	sess.HasValue = true
	return nil
}

func (s *AuthService) Cancel(ctx context.Context, token string) error {
	s.mu.Lock()
	sess, ok := s.sessions[token]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("credentials: auth session %q not found", token)
	}
	if sess.Status != AuthPending {
		s.mu.Unlock()
		return fmt.Errorf("credentials: auth session %q already %s", token, sess.Status)
	}
	flowID := sess.FlowID
	sess.Status = AuthCancelled
	s.mu.Unlock()

	return s.auth.Cancel(ctx, flowID)
}

func (s *AuthService) Status(token string) (AuthSession, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[token]
	if !ok {
		return AuthSession{}, false
	}
	cp := *sess
	return cp, true
}

func (s *AuthService) List() []AuthSession {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]AuthSession, 0, len(s.sessions))
	for _, v := range s.sessions {
		out = append(out, *v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
