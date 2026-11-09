//

//

package sandbox

import (
	"fmt"
	"strings"
)

var InvariantEnabled = true

type invariantError struct {
	pkg     string
	message string
}

func (e *invariantError) Error() string {
	return fmt.Sprintf("[%s invariant] %s", e.pkg, e.message)
}

func assert(condition bool, format string, args ...any) {
	if !InvariantEnabled {
		return
	}
	if !condition {
		panic(&invariantError{pkg: "sandbox", message: fmt.Sprintf(format, args...)})
	}
}

func AssertValidMode(mode SandboxMode) {
	assert(mode == ModeReadOnly || mode == ModeWorkspaceWrite || mode == ModeDangerFullAccess,
		"invalid sandbox mode %q (expected read-only, workspace-write, or danger-full-access)", mode)
}

func AssertExecutionPolicy(policy SandboxExecutionPolicy) {
	AssertValidMode(policy.Mode)

	if policy.Mode == ModeWorkspaceWrite {
		assert(strings.TrimSpace(policy.WorkspaceRoot) != "",
			"workspace-write mode requires non-empty workspaceRoot")
	}
}

func AssertConfinedPolicy(policy SandboxPolicy) {
	assert(policy.Mode == ConfinedReadOnly || policy.Mode == ConfinedWorkspaceWrite,
		"confined policy mode must be read-only or workspace-write, got %q", policy.Mode)
	if policy.Mode == ConfinedWorkspaceWrite {
		assert(strings.TrimSpace(policy.WorkspaceRoot) != "",
			"workspace-write confined policy requires non-empty workspaceRoot")
	}
}

func AssertNotDangerBeforeConfine(mode SandboxMode) {
	assert(mode != ModeDangerFullAccess,
		"cannot confine danger-full-access policy (must bypass sandbox and spawn raw argv)")
}

func AssertEscalationTarget(target SandboxMode) {
	assert(target == ModeWorkspaceWrite || target == ModeDangerFullAccess,
		"escalation target %q is not in closed target vocabulary [workspace-write, danger-full-access]", target)
}

func AssertConfinedArgv(result ConfinedArgv) {
	assert(len(result.Argv) > 0, "confined argv must not be empty")
	assert(result.Enforcement == EnforcementFull || result.Enforcement == EnforcementPartial,
		"invalid enforcement level %q (expected full or partial)", result.Enforcement)
}

func AssertWritableRoots(mode SandboxMode, roots []string) {
	if mode == ModeWorkspaceWrite {
		assert(len(roots) > 0, "workspace-write mode should have non-empty writable roots")
	} else {
		assert(len(roots) == 0, "mode %q should have empty writable roots, got %v", mode, roots)
	}
}
