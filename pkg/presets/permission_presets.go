//

//

//

package presets

import (
	"sort"

	"github.com/JopenChen/dsh-go/pkg/approval"
	"github.com/JopenChen/dsh-go/pkg/sandbox"
)

type PermissionPreset struct {
	Name string `json:"name"`

	SandboxMode sandbox.SandboxMode `json:"sandboxMode"`

	ApprovalPolicy approval.Policy `json:"approvalPolicy"`
}

const (
	PresetSafe   = "safe"
	PresetDanger = "danger"
	PresetReview = "review"
	PresetCustom = "custom"
)

var presetTable = map[string]PermissionPreset{
	PresetSafe: {
		Name:           PresetSafe,
		SandboxMode:    sandbox.ModeReadOnly,
		ApprovalPolicy: approval.PolicyAskDangerous,
	},
	PresetDanger: {
		Name:           PresetDanger,
		SandboxMode:    sandbox.ModeDangerFullAccess,
		ApprovalPolicy: approval.PolicyAllowAll,
	},
	PresetReview: {
		Name:           PresetReview,
		SandboxMode:    sandbox.ModeReadOnly,
		ApprovalPolicy: approval.PolicyAskDangerousEdit,
	},

	PresetCustom: {
		Name:           PresetCustom,
		SandboxMode:    sandbox.ModeWorkspaceWrite,
		ApprovalPolicy: approval.PolicyAskDangerous,
	},
}

func PresetNames() []string {
	names := make([]string, 0, len(presetTable))
	for n := range presetTable {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func Resolve(name string) (PermissionPreset, bool) {
	p, ok := presetTable[name]
	if !ok {

		return presetTable[PresetCustom], true
	}
	return p, true
}

func ApprovalMapper() func(string) (approval.Policy, bool) {
	return func(name string) (approval.Policy, bool) {
		p, _ := Resolve(name)
		return p.ApprovalPolicy, true
	}
}

func SandboxMapper() func(string) (sandbox.SandboxMode, bool) {
	return func(name string) (sandbox.SandboxMode, bool) {
		p, _ := Resolve(name)
		return p.SandboxMode, true
	}
}

type DerivedState struct {
	ActivePreset string `json:"activePreset"`

	SandboxMode sandbox.SandboxMode `json:"sandboxMode"`

	ApprovalPolicy approval.Policy `json:"approvalPolicy"`

	IsCustom bool `json:"isCustom"`
}

func Derive(presetName string, sandboxOverride *sandbox.SandboxMode, approvalOverride *approval.Policy) DerivedState {
	base, _ := Resolve(presetName)
	isCustom := sandboxOverride != nil || approvalOverride != nil
	if presetName != PresetCustom {
		isCustom = false
	}
	if sandboxOverride != nil {
		base.SandboxMode = *sandboxOverride
	}
	if approvalOverride != nil {
		base.ApprovalPolicy = *approvalOverride
	}
	return DerivedState{
		ActivePreset:   base.Name,
		SandboxMode:    base.SandboxMode,
		ApprovalPolicy: base.ApprovalPolicy,
		IsCustom:       isCustom,
	}
}
