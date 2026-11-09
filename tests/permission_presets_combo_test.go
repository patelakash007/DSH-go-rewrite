//

package tests

import (
	"testing"

	"github.com/JopenChen/dsh-go/pkg/approval"
	"github.com/JopenChen/dsh-go/pkg/presets"
	"github.com/JopenChen/dsh-go/pkg/sandbox"
)

func TestPermissionPresetsCombo(t *testing.T) {
	cases := []struct {
		name    string
		sandbox sandbox.SandboxMode
		approx  approval.Policy
	}{
		{presets.PresetSafe, sandbox.ModeReadOnly, approval.PolicyAskDangerous},
		{presets.PresetDanger, sandbox.ModeDangerFullAccess, approval.PolicyAllowAll},
		{presets.PresetReview, sandbox.ModeReadOnly, approval.PolicyAskDangerousEdit},
		{presets.PresetCustom, sandbox.ModeWorkspaceWrite, approval.PolicyAskDangerous},
	}
	for _, c := range cases {
		p, ok := presets.Resolve(c.name)
		if !ok {
			t.Fatalf("preset %q Resolve should succeed", c.name)
		}
		if p.SandboxMode != c.sandbox || p.ApprovalPolicy != c.approx {
			t.Errorf("preset %q combination error: got %+v, want sandbox=%s approval=%s", c.name, p, c.sandbox, c.approx)
		}
	}

	names := presets.PresetNames()
	if len(names) != 4 {
		t.Fatalf("should have 4 presets, got %v", names)
	}
}

func TestPermissionPresetResolveUnknownFallsBackToCustom(t *testing.T) {
	p, ok := presets.Resolve("nonexistent-preset")
	if !ok {
		t.Fatal("unknown preset should fallback custom with ok=true")
	}
	if p.Name != presets.PresetCustom {
		t.Fatalf("should fallback custom, got %s", p.Name)
	}
	if p.SandboxMode != sandbox.ModeWorkspaceWrite {
		t.Fatalf("custom sandbox should be workspace-write, got %s", p.SandboxMode)
	}
}

func TestPermissionPresetMappers(t *testing.T) {
	am := presets.ApprovalMapper()
	sb := presets.SandboxMapper()
	pol, _ := am("danger")
	if pol != approval.PolicyAllowAll {
		t.Fatalf("danger->approval should allow-all, got %s", pol)
	}
	scale, _ := sb("review")
	if scale != sandbox.ModeReadOnly {
		t.Fatalf("review->sandbox should read-only, got %s", scale)
	}
}

func TestPermissionPresetDerive(t *testing.T) {

	st := presets.Derive(presets.PresetSafe, nil, nil)
	if st.IsCustom {
		t.Fatal("should not be custom without override")
	}
	if st.ActivePreset != presets.PresetSafe || st.SandboxMode != sandbox.ModeReadOnly {
		t.Fatalf("safe derived state error: %+v", st)
	}

	ws := sandbox.ModeDangerFullAccess
	allow := approval.PolicyAllowAll
	st = presets.Derive(presets.PresetCustom, &ws, &allow)
	if !st.IsCustom {
		t.Fatal("custom + override should have IsCustom=true")
	}
	if st.SandboxMode != sandbox.ModeDangerFullAccess || st.ApprovalPolicy != approval.PolicyAllowAll {
		t.Fatalf("override derived fields error: %+v", st)
	}
}
