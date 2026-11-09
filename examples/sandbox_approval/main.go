//

//

//

//

//   go run ./examples/sandbox_approval
//

package main

import (
	"fmt"

	"github.com/JopenChen/dsh-go/pkg/approval"
	"github.com/JopenChen/dsh-go/pkg/brand"
	"github.com/JopenChen/dsh-go/pkg/sandbox"
	"github.com/JopenChen/dsh-go/pkg/userq"
)

func main() {
	// ------------------------------------------------------------------

	// ------------------------------------------------------------------

	presetPolice := func(preset string) (approval.Policy, bool) {
		switch preset {
		case "safe":
			return approval.PolicyDenyAll, true
		case "dev":
			return approval.PolicyAskDangerous, true
		default:
			return "", false
		}
	}

	uq := userq.New(userq.NewStub())
	svc := approval.New(presetPolice, uq)

	eff := svc.Resolve("dev", "")
	fmt.Println("— A1. Policy resolution (preset dev) —")
	fmt.Printf("  Effective policy=%s, source=%s\n", eff.Policy, eff.Source)

	sessID := "sess-1"
	svc.SetSessionPolicy(sessID, approval.PolicyDenyAll)
	eff2 := svc.Resolve("dev", sessID)
	fmt.Println("— A2. Session override (nearest-scope-wins) —")
	fmt.Printf("  Effective policy=%s, source=%s\n", eff2.Policy, eff2.Source)

	d, err := svc.Evaluate(approval.Request{Tool: "fs_read", Preset: "dev", SessionID: sessID})
	fmt.Println("— A3. Decision tri-state (deny-all) —")
	fmt.Printf("  fs_read => %s (err=%v)\n", d, err)

	d2, _ := svc.Evaluate(approval.Request{Tool: "shell", Preset: "dev"})
	d3, _ := svc.Evaluate(approval.Request{Tool: "fs_read", Preset: "dev"})
	fmt.Println("— A4. ask-dangerous (stub default reject = fail-closed) —")
	fmt.Printf("  shell(dangerous) => %s\n  fs_read(non-dangerous) => %s\n", d2, d3)
	fmt.Printf("  (Total ask count=%d)\n", svc.AskCount())

	// ------------------------------------------------------------------

	// ------------------------------------------------------------------

	ms := &memSession{id: brand.NewSessionID("sbox-sess"), cwd: "/workspace/app", mode: sandbox.ModeWorkspaceWrite}
	ps := sandbox.NewPolicyService(sandbox.ModeReadOnly, "/fallback")

	p1 := ps.Resolve(sandbox.SandboxPolicyRequest{Session: ms})
	fmt.Println("— B1. Sandbox policy (session override workspace-write) —")
	fmt.Printf("  mode=%s, root=%s, danger=%v\n", p1.Mode, p1.WorkspaceRoot, p1.IsDanger())

	danger := sandbox.ModeDangerFullAccess
	p2 := ps.Resolve(sandbox.SandboxPolicyRequest{Session: ms, Mode: &danger})
	fmt.Println("— B2. Explicit override danger-full-access —")
	fmt.Printf("  mode=%s, danger=%v (bypass sandbox, raw argv)\n", p2.Mode, p2.IsDanger())

	p3 := ps.Resolve(sandbox.SandboxPolicyRequest{})
	fmt.Println("— B3. agentless fallback —")
	fmt.Printf("  mode=%s, root=%s, danger=%v\n", p3.Mode, p3.WorkspaceRoot, p3.IsDanger())

	fmt.Println("\n— Conclusion: Approval governs permission, Sandbox governs boundaries —")
}

type memSession struct {
	id   brand.SessionID
	cwd  string
	mode sandbox.SandboxMode
}

func (m *memSession) ID() brand.SessionID { return m.id }
func (m *memSession) Cwd() string         { return m.cwd }
func (m *memSession) SandboxMode() (sandbox.SandboxMode, bool) {
	if m.mode == "" {
		return "", false
	}
	return m.mode, true
}
