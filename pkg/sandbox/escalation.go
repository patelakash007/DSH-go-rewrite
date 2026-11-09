//

//

package sandbox

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// ============================================================================

// ============================================================================

var widerModes = map[SandboxMode][]SandboxMode{
	ModeReadOnly:       {ModeWorkspaceWrite, ModeDangerFullAccess},
	ModeWorkspaceWrite: {ModeDangerFullAccess},
}

func CanEscalateTo(effectiveMode, targetMode SandboxMode) bool {
	targets, ok := widerModes[effectiveMode]
	if !ok {
		return false
	}
	for _, t := range targets {
		if t == targetMode {
			return true
		}
	}
	return false
}

func EscalationTargets() []SandboxMode {
	return []SandboxMode{ModeWorkspaceWrite, ModeDangerFullAccess}
}

// ============================================================================

// ============================================================================

//

func ValidateEscalationArgs(sandboxPermissions, justification string) error {
	if sandboxPermissions != "" && justification == "" {
		return errors.New("invalid escalation: sandbox_permissions requires a justification")
	}
	if justification != "" && sandboxPermissions == "" {
		return errors.New("invalid escalation: justification is only valid together with sandbox_permissions")
	}
	if justification != "" && strings.TrimSpace(justification) == "" {
		return errors.New("invalid justification: expected a non-empty sentence")
	}
	return nil
}

// ============================================================================

// ============================================================================

func SandboxDenialMarker(mode SandboxMode) string {
	return fmt.Sprintf("[sandbox: file access denied under %s mode]", mode)
}

//

func EscalationHintMarker(subject string) string {
	return fmt.Sprintf(
		"[sandbox: escalation available — retry this exact %s once with sandbox_permissions (the narrowest wider mode that suffices) + justification; the approval prompt asks the user]",
		subject,
	)
}

// ============================================================================

// ============================================================================

type EscalationOutcome string

const (
	EscalationAllowedOnce EscalationOutcome = "allowed-once"

	EscalationRejected EscalationOutcome = "rejected"

	EscalationCancelled EscalationOutcome = "cancelled"

	EscalationUnavailable EscalationOutcome = "unavailable"
)

type EscalationApprovalRequest struct {
	ToolName string

	CallID string

	Reason string
}

type EscalationApprover interface {
	Request(ctx context.Context, req EscalationApprovalRequest) (EscalationOutcome, error)
}

type EscalationRequest struct {
	RequestedMode SandboxMode

	Justification string

	EffectiveMode SandboxMode

	Subject string

	ToolName string

	CallID string
}

// ============================================================================

// ============================================================================

var (
	ErrEscalationNotWider = errors.New("escalation not strictly wider")

	ErrEscalationNoApprover = errors.New("escalation requires approval, but no approver is available")

	ErrEscalationRejected = errors.New("escalation rejected by user")

	ErrEscalationCancelled = errors.New("escalation approval cancelled")

	ErrEscalationUnavailable = errors.New("escalation requires approval, but no approval channel is available")
)

// ============================================================================

// ============================================================================

//

func ApproveEscalation(ctx context.Context, req EscalationRequest, approver EscalationApprover) (SandboxMode, error) {

	if !CanEscalateTo(req.EffectiveMode, req.RequestedMode) {
		return "", fmt.Errorf("%w: sandbox escalation to %q is not strictly wider than this call's current %q mode",
			ErrEscalationNotWider, req.RequestedMode, req.EffectiveMode)
	}

	if err := ValidateEscalationArgs(string(req.RequestedMode), req.Justification); err != nil {
		return "", err
	}

	if approver == nil {
		return "", fmt.Errorf("%w: escalation to %q requires approval, but no approver is composed",
			ErrEscalationNoApprover, req.RequestedMode)
	}

	outcome, err := approver.Request(ctx, EscalationApprovalRequest{
		ToolName: req.ToolName,
		CallID:   req.CallID,
		Reason:   fmt.Sprintf("escalate sandbox to %s: %s", req.RequestedMode, req.Justification),
	})
	if err != nil {
		return "", fmt.Errorf("escalation approval request failed: %w", err)
	}

	switch outcome {
	case EscalationAllowedOnce:
		return req.RequestedMode, nil
	case EscalationRejected:
		return "", fmt.Errorf("%w: the user rejected escalating this %s to %q",
			ErrEscalationRejected, req.Subject, req.RequestedMode)
	case EscalationCancelled:
		return "", fmt.Errorf("%w: approval for escalating to %q was cancelled",
			ErrEscalationCancelled, req.RequestedMode)
	case EscalationUnavailable:
		return "", fmt.Errorf("%w: escalation to %q requires approval, but no approval channel is available",
			ErrEscalationUnavailable, req.RequestedMode)
	default:
		return "", fmt.Errorf("unknown escalation outcome: %q", outcome)
	}
}
