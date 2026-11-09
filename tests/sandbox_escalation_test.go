//

package tests

import (
	"context"
	"errors"
	"testing"

	"github.com/JopenChen/dsh-go/pkg/sandbox"
)

func TestWiderModes(t *testing.T) {

	if !sandbox.CanEscalateTo(sandbox.ModeReadOnly, sandbox.ModeWorkspaceWrite) {
		t.Fatal("read-only should escalate to workspace-write")
	}
	if !sandbox.CanEscalateTo(sandbox.ModeReadOnly, sandbox.ModeDangerFullAccess) {
		t.Fatal("read-only should escalate to danger-full-access")
	}

	if !sandbox.CanEscalateTo(sandbox.ModeWorkspaceWrite, sandbox.ModeDangerFullAccess) {
		t.Fatal("workspace-write should escalate to danger-full-access")
	}

	if sandbox.CanEscalateTo(sandbox.ModeReadOnly, sandbox.ModeReadOnly) {
		t.Fatal("read-only should not escalate to itself")
	}
	if sandbox.CanEscalateTo(sandbox.ModeWorkspaceWrite, sandbox.ModeReadOnly) {
		t.Fatal("workspace-write should not downgrade to read-only")
	}
	if sandbox.CanEscalateTo(sandbox.ModeDangerFullAccess, sandbox.ModeReadOnly) {
		t.Fatal("danger should not downgrade")
	}
}

func TestEscalationTargets(t *testing.T) {
	targets := sandbox.EscalationTargets()
	if len(targets) != 2 {
		t.Fatalf("should have 2 escalation targets, got %d", len(targets))
	}

	for _, target := range targets {
		if target == sandbox.ModeReadOnly {
			t.Fatal("read-only should not be in escalation targets")
		}
	}
}

func TestValidateEscalationArgs(t *testing.T) {

	if err := sandbox.ValidateEscalationArgs("workspace-write", "Need to write log file"); err != nil {
		t.Fatalf("paired parameters should pass: %v", err)
	}

	if err := sandbox.ValidateEscalationArgs("workspace-write", ""); err == nil {
		t.Fatal("missing justification should error")
	}

	if err := sandbox.ValidateEscalationArgs("", "Need to write"); err == nil {
		t.Fatal("missing sandbox_permissions should error")
	}

	if err := sandbox.ValidateEscalationArgs("workspace-write", "   "); err == nil {
		t.Fatal("blank justification should error")
	}
}

func TestSandboxDenialMarker(t *testing.T) {
	marker := sandbox.SandboxDenialMarker(sandbox.ModeReadOnly)
	expected := "[sandbox: file access denied under read-only mode]"
	if marker != expected {
		t.Fatalf("rejection marker format mismatch\nexpected: %s\ngot: %s", expected, marker)
	}
}

func TestEscalationHintMarker(t *testing.T) {
	hint := sandbox.EscalationHintMarker("command")
	if hint == "" {
		t.Fatal("escalation prompt should not be empty")
	}

	if !containsStr(hint, "sandbox_permissions") {
		t.Fatalf("escalation prompt should contain sandbox_permissions, got: %s", hint)
	}

	if !containsStr(hint, "justification") {
		t.Fatalf("escalation prompt should contain justification, got: %s", hint)
	}
}

type fakeApprover struct {
	outcome sandbox.EscalationOutcome
	err     error
}

func (f *fakeApprover) Request(ctx context.Context, req sandbox.EscalationApprovalRequest) (sandbox.EscalationOutcome, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.outcome, nil
}

func TestApproveEscalationAllowed(t *testing.T) {
	approver := &fakeApprover{outcome: sandbox.EscalationAllowedOnce}
	result, err := sandbox.ApproveEscalation(context.Background(), sandbox.EscalationRequest{
		RequestedMode: sandbox.ModeWorkspaceWrite,
		Justification: "Need to write log",
		EffectiveMode: sandbox.ModeReadOnly,
		Subject:       "command",
		ToolName:      "bash",
		CallID:        "call-1",
	}, approver)
	if err != nil {
		t.Fatalf("approval should not error: %v", err)
	}
	if result != sandbox.ModeWorkspaceWrite {
		t.Fatalf("should return workspace-write, got %s", result)
	}
}

func TestApproveEscalationNotWider(t *testing.T) {
	approver := &fakeApprover{outcome: sandbox.EscalationAllowedOnce}
	_, err := sandbox.ApproveEscalation(context.Background(), sandbox.EscalationRequest{
		RequestedMode: sandbox.ModeReadOnly,
		Justification: "Need to write",
		EffectiveMode: sandbox.ModeReadOnly,
		Subject:       "command",
		ToolName:      "bash",
		CallID:        "call-1",
	}, approver)
	if err == nil {
		t.Fatal("non-strictly wider should error")
	}
}

func TestApproveEscalationRejected(t *testing.T) {
	approver := &fakeApprover{outcome: sandbox.EscalationRejected}
	_, err := sandbox.ApproveEscalation(context.Background(), sandbox.EscalationRequest{
		RequestedMode: sandbox.ModeWorkspaceWrite,
		Justification: "Need to write",
		EffectiveMode: sandbox.ModeReadOnly,
		Subject:       "command",
		ToolName:      "bash",
		CallID:        "call-1",
	}, approver)
	if err == nil {
		t.Fatal("user denial should error")
	}
	if !errors.Is(err, sandbox.ErrEscalationRejected) {
		t.Fatalf("should return ErrEscalationRejected, got: %v", err)
	}
}

func TestApproveEscalationCancelled(t *testing.T) {
	approver := &fakeApprover{outcome: sandbox.EscalationCancelled}
	_, err := sandbox.ApproveEscalation(context.Background(), sandbox.EscalationRequest{
		RequestedMode: sandbox.ModeWorkspaceWrite,
		Justification: "Need to write",
		EffectiveMode: sandbox.ModeReadOnly,
		Subject:       "command",
		ToolName:      "bash",
		CallID:        "call-1",
	}, approver)
	if err == nil {
		t.Fatal("approval cancellation should error")
	}
}

func TestApproveEscalationUnavailable(t *testing.T) {
	approver := &fakeApprover{outcome: sandbox.EscalationUnavailable}
	_, err := sandbox.ApproveEscalation(context.Background(), sandbox.EscalationRequest{
		RequestedMode: sandbox.ModeWorkspaceWrite,
		Justification: "Need to write",
		EffectiveMode: sandbox.ModeReadOnly,
		Subject:       "command",
		ToolName:      "bash",
		CallID:        "call-1",
	}, approver)
	if err == nil {
		t.Fatal("approval unavailable should error")
	}
}

func TestApproveEscalationNoApprover(t *testing.T) {
	_, err := sandbox.ApproveEscalation(context.Background(), sandbox.EscalationRequest{
		RequestedMode: sandbox.ModeWorkspaceWrite,
		Justification: "Need to write",
		EffectiveMode: sandbox.ModeReadOnly,
		Subject:       "command",
		ToolName:      "bash",
		CallID:        "call-1",
	}, nil)
	if err == nil {
		t.Fatal("no approver should fail-closed")
	}
}
