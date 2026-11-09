//

//

package session

import (
	"sort"
)

// ============================================================================

// ============================================================================

func EffectiveEvents(events []SessionEvent) []SessionEvent {
	hidden := make(map[uint64]bool, len(events))
	for _, ev := range events {
		if ev.SurfaceOp != nil && ev.SurfaceOp.Op == SurfaceReplace {
			for s := ev.SurfaceOp.Start; s <= ev.SurfaceOp.End && s <= ev.Seq; s++ {
				hidden[s] = true
			}
		}
	}
	out := make([]SessionEvent, 0, len(events))
	for _, ev := range events {
		if hidden[ev.Seq] {
			continue
		}
		out = append(out, ev)
	}
	return out
}

// ============================================================================

// ============================================================================

type Message struct {
	Role        string   `json:"role"` // "user" / "assistant"
	Content     string   `json:"content"`
	ToolCallIDs []string `json:"toolCallIds,omitempty"`
	Seq         uint64   `json:"seq"`
}

func deriveMessages(events []SessionEvent) []Message {
	effective := EffectiveEvents(events)
	var msgs []Message
	for _, ev := range effective {
		switch ev.Type {
		case EventUserMessage:
			if d, ok := ev.Data.(UserMessageData); ok {
				msgs = append(msgs, Message{Role: "user", Content: d.Content, Seq: ev.Seq})
			}
		case EventAssistantMessage:
			if d, ok := ev.Data.(AssistantMessageData); ok {
				msgs = append(msgs, Message{Role: "assistant", Content: d.Content, ToolCallIDs: d.ToolCallIDs, Seq: ev.Seq})
			}
		}
	}
	return msgs
}

func DeriveMessages(events []SessionEvent) []Message {
	return deriveMessages(events)
}

// ============================================================================

// ============================================================================

type RequestHeaderFold struct {
	Present     bool   `json:"present"`
	ConfigEpoch uint64 `json:"configEpoch,omitempty"`
	SystemHash  string `json:"systemHash,omitempty"`
	ToolCount   int    `json:"toolCount,omitempty"`
}

func foldRequestHeader(events []SessionEvent) RequestHeaderFold {
	var out RequestHeaderFold
	for _, ev := range events {
		if ev.Type != EventRequestHeader {
			continue
		}
		if d, ok := ev.Data.(RequestHeaderData); ok {
			out = RequestHeaderFold{Present: true, ConfigEpoch: d.ConfigEpoch, SystemHash: d.SystemHash, ToolCount: d.ToolCount}
		}
	}
	return out
}

func FoldRequestHeader(events []SessionEvent) RequestHeaderFold {
	return foldRequestHeader(events)
}

type SandboxMode string

const (
	SandboxReadOnly         SandboxMode = "read-only"
	SandboxWorkspaceWrite   SandboxMode = "workspace-write"
	SandboxDangerFullAccess SandboxMode = "danger-full-access"
)

type EffectiveSandboxMode struct {
	Present  bool        `json:"present"`
	Mode     SandboxMode `json:"mode,omitempty"`
	Enforced bool        `json:"enforced,omitempty"`
	Root     string      `json:"root,omitempty"`
}

type ApprovalPolicy string

const (
	ApprovalAllowAll         ApprovalPolicy = "allow-all"
	ApprovalDenyAll          ApprovalPolicy = "deny-all"
	ApprovalAskDangerous     ApprovalPolicy = "ask-dangerous"
	ApprovalAskDangerousEdit ApprovalPolicy = "ask-dangerous-tool-edit"
)

type EffectiveApprovalPolicy struct {
	Present bool           `json:"present"`
	Policy  ApprovalPolicy `json:"policy,omitempty"`
	Source  string         `json:"source,omitempty"` // preset / user / session
}

type PermissionPresetFold struct {
	Present bool   `json:"present"`
	Preset  string `json:"preset,omitempty"`
}

var presetSandbox = map[string]SandboxMode{
	"safe":    SandboxReadOnly,
	"danger":  SandboxDangerFullAccess,
	"review":  SandboxReadOnly,
	"default": SandboxWorkspaceWrite,
}

var presetApproval = map[string]ApprovalPolicy{
	"safe":    ApprovalAskDangerous,
	"danger":  ApprovalAllowAll,
	"review":  ApprovalAskDangerousEdit,
	"default": ApprovalAskDangerous,
}

func foldPermissionPreset(events []SessionEvent) PermissionPresetFold {
	var out PermissionPresetFold
	for _, ev := range events {
		if ev.Type != EventPresetChange {
			continue
		}
		if d, ok := ev.Data.(PresetChangeData); ok {
			out = PermissionPresetFold{Present: true, Preset: d.Preset}
		}
	}
	return out
}

func FoldPermissionPreset(events []SessionEvent) PermissionPresetFold {
	return foldPermissionPreset(events)
}

func foldEffectiveSandboxMode(events []SessionEvent) EffectiveSandboxMode {
	preset := foldPermissionPreset(events)
	if !preset.Present {
		return EffectiveSandboxMode{}
	}
	mode, ok := presetSandbox[preset.Preset]
	if !ok {
		mode = SandboxWorkspaceWrite
	}
	return EffectiveSandboxMode{Present: true, Mode: mode, Enforced: true}
}

func FoldEffectiveSandboxMode(events []SessionEvent) EffectiveSandboxMode {
	return foldEffectiveSandboxMode(events)
}

func foldEffectiveApprovalPolicy(events []SessionEvent) EffectiveApprovalPolicy {
	preset := foldPermissionPreset(events)
	if !preset.Present {
		return EffectiveApprovalPolicy{}
	}
	policy, ok := presetApproval[preset.Preset]
	if !ok {
		policy = ApprovalAskDangerous
	}
	return EffectiveApprovalPolicy{Present: true, Policy: policy, Source: "preset"}
}

func FoldEffectiveApprovalPolicy(events []SessionEvent) EffectiveApprovalPolicy {
	return foldEffectiveApprovalPolicy(events)
}

type GoalFold struct {
	Present     bool      `json:"present"`
	GoalID      string    `json:"goalId,omitempty"`
	Phase       GoalPhase `json:"phase,omitempty"`
	Description string    `json:"description,omitempty"`
	MaxRounds   int       `json:"maxRounds,omitempty"`
	Revision    uint64    `json:"revision,omitempty"`

	BlockReason *GoalBlockReason `json:"blockedReason,omitempty"`
}

func foldGoalChange(events []SessionEvent) GoalFold {
	var out GoalFold
	for _, ev := range events {
		if ev.Type != EventGoalChange {
			continue
		}
		if d, ok := ev.Data.(GoalChangeData); ok {

			if !out.Present || d.Revision > out.Revision {
				out = GoalFold{
					Present:     true,
					GoalID:      d.GoalID,
					Phase:       d.Phase,
					Description: d.Description,
					MaxRounds:   d.MaxRounds,
					Revision:    d.Revision,
					BlockReason: d.BlockReason,
				}
			}
		}
	}
	return out
}

func FoldGoalChange(events []SessionEvent) GoalFold {
	return foldGoalChange(events)
}

type TodoFold struct {
	Present bool        `json:"present"`
	Items   []string    `json:"items,omitempty"`
	Entries []TodoEntry `json:"entries,omitempty"`
}

func foldTodoWrite(events []SessionEvent) TodoFold {
	var out TodoFold
	for _, ev := range events {
		if ev.Type != EventTodoWrite {
			continue
		}
		if d, ok := ev.Data.(TodoWriteData); ok {
			out = TodoFold{
				Present: true,
				Items:   append([]string(nil), d.Items...),
				Entries: append([]TodoEntry(nil), d.Entries...),
			}
		}
	}
	return out
}

func FoldTodoWrite(events []SessionEvent) TodoFold {
	return foldTodoWrite(events)
}

type PlanModeFold struct {
	Present bool   `json:"present"`
	Mode    string `json:"mode,omitempty"` // "on" / "off"
}

func foldPlanMode(events []SessionEvent) PlanModeFold {
	var out PlanModeFold
	for _, ev := range events {
		if ev.Type != EventPlanMode {
			continue
		}
		if d, ok := ev.Data.(PlanModeData); ok {
			out = PlanModeFold{Present: true, Mode: d.Mode}
		}
	}
	return out
}

func FoldPlanMode(events []SessionEvent) PlanModeFold {
	return foldPlanMode(events)
}

type SessionTitleFold struct {
	Present bool   `json:"present"`
	Title   string `json:"title,omitempty"`
}

func foldSessionTitle(events []SessionEvent) SessionTitleFold {
	var out SessionTitleFold
	for _, ev := range events {
		if ev.Type != EventSessionTitle {
			continue
		}
		if d, ok := ev.Data.(SessionTitleData); ok {
			out = SessionTitleFold{Present: true, Title: d.Title}
		}
	}
	return out
}

func FoldSessionTitle(events []SessionEvent) SessionTitleFold {
	return foldSessionTitle(events)
}

// ============================================================================

// ============================================================================

type SessionProjection struct {
	Messages      []Message               `json:"messages"`
	RequestHeader RequestHeaderFold       `json:"requestHeader"`
	SandboxMode   EffectiveSandboxMode    `json:"sandboxMode"`
	Approval      EffectiveApprovalPolicy `json:"approvalPolicy"`
	Preset        PermissionPresetFold    `json:"preset"`
	Goal          GoalFold                `json:"goal"`
	Todo          TodoFold                `json:"todo"`
	PlanMode      PlanModeFold            `json:"planMode"`
	Title         SessionTitleFold        `json:"title"`
}

func FoldAll(events []SessionEvent) SessionProjection {
	effective := EffectiveEvents(events)
	return SessionProjection{
		Messages:      deriveMessages(effective),
		RequestHeader: foldRequestHeader(effective),
		SandboxMode:   foldEffectiveSandboxMode(effective),
		Approval:      foldEffectiveApprovalPolicy(effective),
		Preset:        foldPermissionPreset(effective),
		Goal:          foldGoalChange(effective),
		Todo:          foldTodoWrite(effective),
		PlanMode:      foldPlanMode(effective),
		Title:         foldSessionTitle(effective),
	}
}

func FoldAllFromLog(sl *SessionLog) SessionProjection {
	return FoldAll(sl.Events())
}

func sortMessagesBySeq(msgs []Message) {
	sort.SliceStable(msgs, func(i, j int) bool { return msgs[i].Seq < msgs[j].Seq })
}
