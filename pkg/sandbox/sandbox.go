//

//

//

package sandbox

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/JopenChen/dsh-go/pkg/brand"
)

// ============================================================================

// ============================================================================

type SandboxMode string

const (
	ModeReadOnly         SandboxMode = "read-only"
	ModeWorkspaceWrite   SandboxMode = "workspace-write"
	ModeDangerFullAccess SandboxMode = "danger-full-access"
)

type ConfinedSandboxMode string

const (
	ConfinedReadOnly       ConfinedSandboxMode = "read-only"
	ConfinedWorkspaceWrite ConfinedSandboxMode = "workspace-write"
)

type SandboxEnforcement string

const (
	EnforcementFull    SandboxEnforcement = "full"
	EnforcementPartial SandboxEnforcement = "partial"
)

// ============================================================================

// ============================================================================

type SandboxExecutionPolicy struct {
	Mode SandboxMode `json:"mode"`

	WorkspaceRoot string `json:"workspaceRoot"`

	SessionID brand.SessionID `json:"sessionId,omitempty"`
}

func (p SandboxExecutionPolicy) IsDanger() bool {
	return p.Mode == ModeDangerFullAccess
}

type SandboxPolicy struct {
	SandboxExecutionPolicy

	Mode ConfinedSandboxMode `json:"mode"`
}

// ============================================================================

// ============================================================================

type Session interface {
	ID() brand.SessionID

	Cwd() string

	SandboxMode() (SandboxMode, bool)
}

type SandboxPolicyRequest struct {
	Session Session

	Mode *SandboxMode
}

type SandboxPolicyService struct {
	DefaultMode SandboxMode

	FallbackRoot string
}

func NewPolicyService(defaultMode SandboxMode, fallbackRoot string) *SandboxPolicyService {
	return &SandboxPolicyService{DefaultMode: defaultMode, FallbackRoot: fallbackRoot}
}

func (s *SandboxPolicyService) OverrideOf(session Session) (SandboxMode, bool) {
	if session == nil {
		return "", false
	}
	return session.SandboxMode()
}

//

func (s *SandboxPolicyService) Resolve(req SandboxPolicyRequest) SandboxExecutionPolicy {
	mode := s.DefaultMode
	if req.Session != nil {
		if m, ok := req.Session.SandboxMode(); ok {
			mode = m
		}
	}
	if req.Mode != nil {
		mode = *req.Mode
	}

	root := s.FallbackRoot
	if req.Session != nil {
		if cwd := req.Session.Cwd(); cwd != "" {
			root = cwd
		}
	}
	root = canonicalize(root)

	var sid brand.SessionID
	if req.Session != nil {
		sid = req.Session.ID()
	}
	return SandboxExecutionPolicy{
		Mode:          mode,
		WorkspaceRoot: root,
		SessionID:     sid,
	}
}

func (p SandboxExecutionPolicy) ToConfined() (SandboxPolicy, error) {
	switch p.Mode {
	case ModeReadOnly:
		return SandboxPolicy{SandboxExecutionPolicy: p, Mode: ConfinedReadOnly}, nil
	case ModeWorkspaceWrite:
		return SandboxPolicy{SandboxExecutionPolicy: p, Mode: ConfinedWorkspaceWrite}, nil
	default:
		return SandboxPolicy{}, fmt.Errorf("sandbox: cannot confine danger-full-access policy %+v", p)
	}
}

func canonicalize(root string) string {
	if root == "" {
		return root
	}
	if filepath.IsAbs(root) {
		return filepath.Clean(root)
	}

	if len(root) >= 3 && ((root[0] >= 'A' && root[0] <= 'Z') || (root[0] >= 'a' && root[0] <= 'z')) && root[1] == ':' && (root[2] == '\\' || root[2] == '/') {
		return filepath.Clean(root)
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return filepath.Clean(root)
	}
	return filepath.Clean(abs)
}

// ============================================================================

// ============================================================================

type RunnerFailureRule struct {
	AllowedExitCodes []int `json:"allowedExitCodes,omitempty"`

	FatalSignatures []string `json:"fatalSignatures"`

	InformationalLines []string `json:"informationalLines,omitempty"`
}

func (r RunnerFailureRule) Matches(exitCode int, stderr string) (string, bool) {
	if exitCode == 0 {
		return "", false
	}
	if len(r.AllowedExitCodes) > 0 {
		allowed := false
		for _, c := range r.AllowedExitCodes {
			if c == exitCode {
				allowed = true
				break
			}
		}
		if !allowed {
			return "", false
		}
	}

	info := map[string]bool{}
	for _, l := range r.InformationalLines {
		info[strings.ToLower(l)] = true
	}
	var remaining []string
	for _, line := range strings.Split(stderr, "\n") {
		if !info[strings.ToLower(strings.TrimSpace(line))] {
			remaining = append(remaining, line)
		}
	}

	joined := strings.Join(remaining, "\n")
	lower := strings.ToLower(joined)
	for _, sig := range r.FatalSignatures {
		if strings.Contains(lower, strings.ToLower(sig)) {
			return sig, true
		}
	}
	return "", false
}

type ConfinedArgv struct {
	Argv []string `json:"argv"`

	Enforcement SandboxEnforcement `json:"enforcement"`

	DenialSignatures []string `json:"denialSignatures"`

	RunnerFailureRules []RunnerFailureRule `json:"runnerFailureRules"`
}

type SandboxProvider interface {
	Confine(argv []string, policy SandboxPolicy) (ConfinedArgv, error)
}

const SandboxUnavailableErrorCode = "SANDBOX_UNAVAILABLE"

type SandboxUnavailableError struct {
	Msg string
}

func (e *SandboxUnavailableError) Error() string {
	return fmt.Sprintf("%s: %s", SandboxUnavailableErrorCode, e.Msg)
}

type UnavailableProvider struct{}

func (*UnavailableProvider) Confine([]string, SandboxPolicy) (ConfinedArgv, error) {
	return ConfinedArgv{}, &SandboxUnavailableError{Msg: "no usable sandbox backend"}
}
