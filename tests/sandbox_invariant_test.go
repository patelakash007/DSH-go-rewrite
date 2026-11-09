package tests

import (
	"testing"

	"github.com/JopenChen/dsh-go/pkg/sandbox"
)

func TestInvariantValidMode(t *testing.T) {
	sandbox.InvariantEnabled = true
	defer func() { sandbox.InvariantEnabled = true }()

	sandbox.AssertValidMode(sandbox.ModeReadOnly)
	sandbox.AssertValidMode(sandbox.ModeWorkspaceWrite)
	sandbox.AssertValidMode(sandbox.ModeDangerFullAccess)
}

func TestInvariantInvalidMode(t *testing.T) {
	sandbox.InvariantEnabled = true
	defer func() { sandbox.InvariantEnabled = true }()
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("invalid mode should panic")
		}
	}()
	sandbox.AssertValidMode(sandbox.SandboxMode("invalid"))
}

func TestInvariantExecutionPolicy(t *testing.T) {
	sandbox.InvariantEnabled = true
	defer func() { sandbox.InvariantEnabled = true }()

	sandbox.AssertExecutionPolicy(sandbox.SandboxExecutionPolicy{
		Mode:          sandbox.ModeWorkspaceWrite,
		WorkspaceRoot: "/workspace",
	})

	sandbox.AssertExecutionPolicy(sandbox.SandboxExecutionPolicy{
		Mode: sandbox.ModeReadOnly,
	})
}

func TestInvariantExecutionPolicyNoRoot(t *testing.T) {
	sandbox.InvariantEnabled = true
	defer func() { sandbox.InvariantEnabled = true }()
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("workspace-write without root should panic")
		}
	}()
	sandbox.AssertExecutionPolicy(sandbox.SandboxExecutionPolicy{
		Mode: sandbox.ModeWorkspaceWrite,
	})
}

func TestInvariantDisabled(t *testing.T) {
	sandbox.InvariantEnabled = false
	defer func() { sandbox.InvariantEnabled = true }()

	sandbox.AssertValidMode(sandbox.SandboxMode("invalid"))
	sandbox.AssertExecutionPolicy(sandbox.SandboxExecutionPolicy{
		Mode: sandbox.ModeWorkspaceWrite,
	})
}
