//

// - Provider fail-closed（SANDBOX_UNAVAILABLE）
package tests

import (
	"testing"

	"github.com/JopenChen/dsh-go/pkg/brand"
	"github.com/JopenChen/dsh-go/pkg/sandbox"
)

type fakeSession struct {
	id   string
	cwd  string
	mode sandbox.SandboxMode
	has  bool
}

func (f *fakeSession) ID() brand.SessionID { return brand.NewSessionID(f.id) }
func (f *fakeSession) Cwd() string         { return f.cwd }
func (f *fakeSession) SandboxMode() (sandbox.SandboxMode, bool) {
	return f.mode, f.has
}

func TestSandboxBothConsumersConsistentPolicy(t *testing.T) {
	svc := sandbox.NewPolicyService(sandbox.ModeReadOnly, "/fallback")
	sess := &fakeSession{id: "s1", cwd: `C:\work`, mode: sandbox.ModeWorkspaceWrite, has: true}

	bashPolicy := svc.Resolve(sandbox.SandboxPolicyRequest{Session: sess})
	fsPolicy := svc.Resolve(sandbox.SandboxPolicyRequest{Session: sess})

	if bashPolicy.Mode != fsPolicy.Mode || bashPolicy.WorkspaceRoot != fsPolicy.WorkspaceRoot {
		t.Fatalf("Bash/FS policy mismatch: bash=%+v fs=%+v", bashPolicy, fsPolicy)
	}
	if bashPolicy.Mode != sandbox.ModeWorkspaceWrite {
		t.Fatalf("should apply session override workspace-write, got %s", bashPolicy.Mode)
	}
	if bashPolicy.WorkspaceRoot != `C:\work` {
		t.Fatalf("workspace root should take session cwd, got %q", bashPolicy.WorkspaceRoot)
	}
	if bashPolicy.IsDanger() {
		t.Fatal("workspace-write should not be danger")
	}
}

func TestSandboxModePrecedence(t *testing.T) {
	svc := sandbox.NewPolicyService(sandbox.ModeReadOnly, "/root")

	p := svc.Resolve(sandbox.SandboxPolicyRequest{})
	if p.Mode != sandbox.ModeReadOnly {
		t.Fatalf("no input should fallback to default read-only, got %s", p.Mode)
	}

	sess := &fakeSession{id: "s", cwd: "/ws", mode: sandbox.ModeDangerFullAccess, has: true}
	p = svc.Resolve(sandbox.SandboxPolicyRequest{Session: sess})
	if p.Mode != sandbox.ModeDangerFullAccess {
		t.Fatalf("session override should be danger, got %s", p.Mode)
	}
	if !p.IsDanger() {
		t.Fatal("IsDanger() should be true")
	}

	explicit := sandbox.ModeWorkspaceWrite
	p = svc.Resolve(sandbox.SandboxPolicyRequest{Session: sess, Mode: &explicit})
	if p.Mode != sandbox.ModeWorkspaceWrite {
		t.Fatalf("explicit override should be workspace-write, got %s", p.Mode)
	}
}

func TestSandboxWorkspaceRootFallback(t *testing.T) {
	svc := sandbox.NewPolicyService(sandbox.ModeWorkspaceWrite, "/fallback")

	p := svc.Resolve(sandbox.SandboxPolicyRequest{})
	if p.WorkspaceRoot == "" || p.WorkspaceRoot != svc.Resolve(sandbox.SandboxPolicyRequest{}).WorkspaceRoot {
		t.Fatalf("fallback root unexpected: %q", p.WorkspaceRoot)
	}

	sess := &fakeSession{id: "s", cwd: "relative/x"}
	p = svc.Resolve(sandbox.SandboxPolicyRequest{Session: sess})
	if p.WorkspaceRoot == "relative/x" {
		t.Fatalf("workspaceRoot should normalize to absolute path, got %q", p.WorkspaceRoot)
	}
}

func TestSandboxToConfined(t *testing.T) {
	base := sandbox.SandboxExecutionPolicy{
		Mode:          sandbox.ModeWorkspaceWrite,
		WorkspaceRoot: "/ws",
	}
	confined, err := base.ToConfined()
	if err != nil {
		t.Fatalf("workspace-write should be degradable: %v", err)
	}
	if confined.Mode != sandbox.ConfinedWorkspaceWrite {
		t.Fatalf("degraded mode should be workspace-write, got %s", confined.Mode)
	}

	danger := base
	danger.Mode = sandbox.ModeDangerFullAccess
	if _, err := danger.ToConfined(); err == nil {
		t.Fatal("danger-full-access downgrade should error")
	}
}

func TestSandboxProviderFailClosed(t *testing.T) {
	prov := &sandbox.UnavailableProvider{}
	_, err := prov.Confine(nil, sandbox.SandboxPolicy{})
	if err == nil {
		t.Fatal("must return error when no backend available, silent pass disallowed")
	}
	if err.Error() != sandbox.SandboxUnavailableErrorCode && !containsStr(err.Error(), sandbox.SandboxUnavailableErrorCode) {
		t.Fatalf("should carry SANDBOX_UNAVAILABLE code, got: %v", err)
	}
}

func TestSandboxRunnerFailureRule(t *testing.T) {
	rule := sandbox.RunnerFailureRule{
		AllowedExitCodes:   []int{125},
		FatalSignatures:    []string{"cannot start sandbox runner", "exec format error"},
		InformationalLines: []string{"landlock: fallback to baseline"},
	}

	if _, ok := rule.Matches(1, "boom"); ok {
		t.Fatal("exit code 1 not in allowlist, should not judge failure")
	}

	stderr := "landlock: fallback to baseline\ncannot start sandbox runner: permission denied\n"
	sig, ok := rule.Matches(125, stderr)
	if !ok {
		t.Fatal("should judge runner failure")
	}
	if sig == "" {
		t.Fatal("should return matched fatal signature")
	}
}

func containsStr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
