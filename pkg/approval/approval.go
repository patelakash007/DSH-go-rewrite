//

//

//

//

package approval

import (
	"context"
	"fmt"
	"sync"

	"github.com/JopenChen/dsh-go/pkg/scope"
	"github.com/JopenChen/dsh-go/pkg/userq"
)

// ============================================================================

// ============================================================================

type Policy string

const (
	PolicyAllowAll         Policy = "allow-all"
	PolicyDenyAll          Policy = "deny-all"
	PolicyAskDangerous     Policy = "ask-dangerous"
	PolicyAskDangerousEdit Policy = "ask-dangerous-tool-edit"
)

type EffSource string

const (
	SourcePreset  EffSource = "preset"
	SourceUser    EffSource = "user"
	SourceSession EffSource = "session"
)

type Effective struct {
	Policy Policy    `json:"policy"`
	Source EffSource `json:"source"`
}

type Decision int

const (
	DecideAllow Decision = iota
	DecideDeny
	DecideAsk
)

func (d Decision) String() string {
	switch d {
	case DecideAllow:
		return "allow"
	case DecideDeny:
		return "deny"
	case DecideAsk:
		return "ask"
	default:
		return "unknown"
	}
}

// ============================================================================
// Service
// ============================================================================

type PresetPolice func(preset string) (Policy, bool)

func (p PresetPolice) resolve(preset string) (Policy, bool) {
	if p == nil {
		return PolicyDenyAll, false
	}
	pol, ok := p(preset)
	if !ok {
		return PolicyDenyAll, false
	}
	return pol, true
}

type DangerFunc func(tool string) bool

type Request struct {
	Tool string

	CallID string

	Preset string

	SessionID string

	DangerousOverride DangerFunc
}

type Service struct {
	preset PresetPolice
	uq     *userq.Service

	userLayer    *scope.Layer[Policy]
	sessionLayer *scope.Layer[Policy]

	dangerous map[string]bool

	mu   sync.Mutex
	asks int
}

func New(preset PresetPolice, uq *userq.Service) *Service {
	return &Service{
		preset:       preset,
		uq:           uq,
		userLayer:    scope.NewLayer[Policy](scope.Key("user")),
		sessionLayer: scope.NewLayer[Policy](scope.Key("session")),
		dangerous:    defaultDangerous(),
	}
}

func (s *Service) SetDangerous(m map[string]bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dangerous = m
}

func defaultDangerous() map[string]bool {
	return map[string]bool{
		"shell":      true,
		"bash":       true,
		"subprocess": true,
		"fs_write":   true,
		"fs_edit":    true,
		"rm":         true,
	}
}

func (s *Service) SetUserPolicy(p Policy) {
	s.userLayer.Unregister("policy")
	s.userLayer.Register("policy", p, 100)
}

func (s *Service) SetUserPolicyByKey(userKey string, p Policy) {
	s.userLayer.Unregister(userKey)
	s.userLayer.Register(userKey, p, 100)
}

func (s *Service) SetSessionPolicy(sessionID string, p Policy) {
	s.sessionLayer.Unregister(sessionID)
	s.sessionLayer.Register(sessionID, p, 100)
}

func (s *Service) ClearSessionPolicy(sessionID string) {
	s.sessionLayer.Unregister(sessionID)
}

func (s *Service) AskCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.asks
}

func (s *Service) Resolve(preset string, sessionID string) Effective {

	if sessionID != "" {
		if p, ok := s.sessionLayer.Get(sessionID); ok {
			return Effective{Policy: p, Source: SourceSession}
		}
	}

	if p, ok := s.userLayer.Get("policy"); ok {
		return Effective{Policy: p, Source: SourceUser}
	}

	if p, ok := s.preset.resolve(preset); ok {
		return Effective{Policy: p, Source: SourcePreset}
	}

	return Effective{Policy: PolicyDenyAll, Source: SourcePreset}
}

//
//   - allow-all → allow；deny-all → deny；

func (s *Service) Evaluate(req Request) (Decision, error) {
	eff := s.Resolve(req.Preset, req.SessionID)
	switch eff.Policy {
	case PolicyAllowAll:
		return DecideAllow, nil
	case PolicyDenyAll:
		return DecideDeny, nil
	case PolicyAskDangerous, PolicyAskDangerousEdit:
		if !s.isDangerous(req.Tool, req.DangerousOverride) {
			return DecideAllow, nil
		}
		allowed, err := s.ask(req)
		if err != nil {

			return DecideDeny, err
		}
		if allowed {
			return DecideAllow, nil
		}
		return DecideDeny, nil
	default:
		return DecideDeny, fmt.Errorf("approval: unknown policy %q", eff.Policy)
	}
}

func (s *Service) isDangerous(tool string, fn DangerFunc) bool {
	if fn != nil {
		return fn(tool)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.dangerous[tool]
}

func (s *Service) ask(req Request) (bool, error) {
	s.mu.Lock()
	s.asks++
	s.mu.Unlock()
	if s.uq == nil {
		return false, fmt.Errorf("approval: no userq service for ask on %q", req.Tool)
	}
	idx, _, err := s.uq.Ask(context.Background(), userq.QuestionOptions{
		Prompt:       fmt.Sprintf("Approve risky tool call %s?", req.Tool),
		Choices:      []string{"Approve", "Deny"},
		DefaultIndex: 1,
		Intent:       "approval",
	})
	if err != nil {

		return false, err
	}

	return idx == 0, nil
}
