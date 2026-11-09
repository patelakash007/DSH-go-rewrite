//

//       · NewIncrementalFolder() / FolderFromEvents()

package session

import (
	"reflect"
	"sync"
)

// ============================================================================

// ============================================================================

func (m Message) Equal(o ProjectionState) bool {
	other, ok := o.(Message)
	if !ok {
		return false
	}
	return m.Role == other.Role &&
		m.Content == other.Content &&
		m.Seq == other.Seq &&
		strSliceEq(m.ToolCallIDs, other.ToolCallIDs)
}

func messageSliceEq(a, b []Message) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !a[i].Equal(b[i]) {
			return false
		}
	}
	return true
}

func strSliceEq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (r RequestHeaderFold) Equal(o ProjectionState) bool {
	other, ok := o.(RequestHeaderFold)
	if !ok {
		return false
	}
	return r.Present == other.Present &&
		r.ConfigEpoch == other.ConfigEpoch &&
		r.SystemHash == other.SystemHash &&
		r.ToolCount == other.ToolCount
}

func (s EffectiveSandboxMode) Equal(o ProjectionState) bool {
	other, ok := o.(EffectiveSandboxMode)
	if !ok {
		return false
	}
	return s.Present == other.Present && s.Mode == other.Mode &&
		s.Enforced == other.Enforced && s.Root == other.Root
}

func (a EffectiveApprovalPolicy) Equal(o ProjectionState) bool {
	other, ok := o.(EffectiveApprovalPolicy)
	if !ok {
		return false
	}
	return a.Present == other.Present && a.Policy == other.Policy && a.Source == other.Source
}

func (p PermissionPresetFold) Equal(o ProjectionState) bool {
	other, ok := o.(PermissionPresetFold)
	if !ok {
		return false
	}
	return p.Present == other.Present && p.Preset == other.Preset
}

func (g GoalFold) Equal(o ProjectionState) bool {
	other, ok := o.(GoalFold)
	if !ok {
		return false
	}
	if g.Present != other.Present ||
		g.GoalID != other.GoalID ||
		g.Phase != other.Phase ||
		g.Description != other.Description ||
		g.MaxRounds != other.MaxRounds ||
		g.Revision != other.Revision {
		return false
	}

	if (g.BlockReason == nil) != (other.BlockReason == nil) {
		return false
	}
	if g.BlockReason != nil {
		if g.BlockReason.Code != other.BlockReason.Code ||
			g.BlockReason.Message != other.BlockReason.Message {
			return false
		}
	}
	return true
}

func (t TodoFold) Equal(o ProjectionState) bool {
	other, ok := o.(TodoFold)
	if !ok {
		return false
	}
	if t.Present != other.Present {
		return false
	}
	return strSliceEq(t.Items, other.Items)
}

func (p PlanModeFold) Equal(o ProjectionState) bool {
	other, ok := o.(PlanModeFold)
	if !ok {
		return false
	}
	return p.Present == other.Present && p.Mode == other.Mode
}

func (s SessionTitleFold) Equal(o ProjectionState) bool {
	other, ok := o.(SessionTitleFold)
	if !ok {
		return false
	}
	return s.Present == other.Present && s.Title == other.Title
}

func SessionProjectionEqual(a, b SessionProjection) bool {
	return messageSliceEq(a.Messages, b.Messages) &&
		a.RequestHeader.Equal(b.RequestHeader) &&
		a.SandboxMode.Equal(b.SandboxMode) &&
		a.Approval.Equal(b.Approval) &&
		a.Preset.Equal(b.Preset) &&
		a.Goal.Equal(b.Goal) &&
		a.Todo.Equal(b.Todo) &&
		a.PlanMode.Equal(b.PlanMode) &&
		a.Title.Equal(b.Title)
}

// ============================================================================

// ============================================================================

func ApplyMessage(msgs []Message, ev SessionEvent) ([]Message, bool) {
	dirty := false
	if ev.SurfaceOp != nil && ev.SurfaceOp.Op == SurfaceReplace {
		dirty = true
	}
	switch ev.Type {
	case EventUserMessage:
		if d, ok := ev.Data.(UserMessageData); ok {
			msgs = append(msgs, Message{Role: "user", Content: d.Content, Seq: ev.Seq})
		}
	case EventAssistantMessage:
		if d, ok := ev.Data.(AssistantMessageData); ok {
			msgs = append(msgs, Message{
				Role:        "assistant",
				Content:     d.Content,
				ToolCallIDs: append([]string(nil), d.ToolCallIDs...),
				Seq:         ev.Seq,
			})
		}
	}
	return msgs, dirty
}

// ApplyRequestHeader latest-wins。
func ApplyRequestHeader(s RequestHeaderFold, ev SessionEvent) RequestHeaderFold {
	if ev.Type != EventRequestHeader {
		return s
	}
	if d, ok := ev.Data.(RequestHeaderData); ok {
		return RequestHeaderFold{
			Present:     true,
			ConfigEpoch: d.ConfigEpoch,
			SystemHash:  d.SystemHash,
			ToolCount:   d.ToolCount,
		}
	}
	return s
}

// ApplyPermissionPreset latest-wins。
func ApplyPermissionPreset(s PermissionPresetFold, ev SessionEvent) PermissionPresetFold {
	if ev.Type != EventPresetChange {
		return s
	}
	if d, ok := ev.Data.(PresetChangeData); ok {
		return PermissionPresetFold{Present: true, Preset: d.Preset}
	}
	return s
}

func ApplyGoalChange(s GoalFold, ev SessionEvent) GoalFold {
	if ev.Type != EventGoalChange {
		return s
	}
	if d, ok := ev.Data.(GoalChangeData); ok {
		if !s.Present || d.Revision > s.Revision {
			return GoalFold{
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
	return s
}

func ApplyTodoWrite(s TodoFold, ev SessionEvent) TodoFold {
	if ev.Type != EventTodoWrite {
		return s
	}
	if d, ok := ev.Data.(TodoWriteData); ok {
		return TodoFold{
			Present: true,
			Items:   append([]string(nil), d.Items...),
			Entries: append([]TodoEntry(nil), d.Entries...),
		}
	}
	return s
}

// ApplyPlanMode latest-wins。
func ApplyPlanMode(s PlanModeFold, ev SessionEvent) PlanModeFold {
	if ev.Type != EventPlanMode {
		return s
	}
	if d, ok := ev.Data.(PlanModeData); ok {
		return PlanModeFold{Present: true, Mode: d.Mode}
	}
	return s
}

// ApplySessionTitle latest-wins。
func ApplySessionTitle(s SessionTitleFold, ev SessionEvent) SessionTitleFold {
	if ev.Type != EventSessionTitle {
		return s
	}
	if d, ok := ev.Data.(SessionTitleData); ok {
		return SessionTitleFold{Present: true, Title: d.Title}
	}
	return s
}

func deriveSandboxAndApproval(preset PermissionPresetFold) (EffectiveSandboxMode, EffectiveApprovalPolicy) {
	if !preset.Present {
		return EffectiveSandboxMode{}, EffectiveApprovalPolicy{}
	}
	mode, ok := presetSandbox[preset.Preset]
	if !ok {
		mode = SandboxWorkspaceWrite
	}
	policy, ok := presetApproval[preset.Preset]
	if !ok {
		policy = ApprovalAskDangerous
	}
	return EffectiveSandboxMode{Present: true, Mode: mode, Enforced: true},
		EffectiveApprovalPolicy{Present: true, Policy: policy, Source: "preset"}
}

// ============================================================================

// ============================================================================

//

//    f := NewIncrementalFolder()
//    f.Append(ev1)
//    f.Append(ev2)

//

type IncrementalFolder struct {
	messages  []Message
	reqHeader RequestHeaderFold
	preset    PermissionPresetFold
	sandbox   EffectiveSandboxMode
	approval  EffectiveApprovalPolicy
	goal      GoalFold
	todo      TodoFold
	planMode  PlanModeFold
	title     SessionTitleFold

	messagesDirty bool

	Stats FolderStats

	statsMu sync.Mutex
}

type FolderStats struct {
	IncrementalAppends uint64 `json:"incrementalAppends"`

	DirtyRebuilds uint64 `json:"dirtyRebuilds"`

	EventsScanned uint64 `json:"eventsScanned"`
}

func NewIncrementalFolder() *IncrementalFolder {
	return &IncrementalFolder{}
}

func FolderFromEvents(events []SessionEvent) *IncrementalFolder {
	f := NewIncrementalFolder()
	proj := FoldAll(events)
	f.messages = proj.Messages
	f.reqHeader = proj.RequestHeader
	f.preset = proj.Preset
	f.sandbox = proj.SandboxMode
	f.approval = proj.Approval
	f.goal = proj.Goal
	f.todo = proj.Todo
	f.planMode = proj.PlanMode
	f.title = proj.Title

	f.statsMu.Lock()
	f.Stats.EventsScanned += uint64(len(events))
	f.statsMu.Unlock()
	return f
}

func (f *IncrementalFolder) Append(ev SessionEvent) {
	var dirty bool
	f.messages, dirty = ApplyMessage(f.messages, ev)
	if dirty {
		f.messagesDirty = true
	}
	f.reqHeader = ApplyRequestHeader(f.reqHeader, ev)

	oldPreset := f.preset
	f.preset = ApplyPermissionPreset(f.preset, ev)

	if !reflect.DeepEqual(oldPreset, f.preset) {
		f.sandbox, f.approval = deriveSandboxAndApproval(f.preset)
	}

	f.goal = ApplyGoalChange(f.goal, ev)
	f.todo = ApplyTodoWrite(f.todo, ev)
	f.planMode = ApplyPlanMode(f.planMode, ev)
	f.title = ApplySessionTitle(f.title, ev)

	f.statsMu.Lock()
	f.Stats.IncrementalAppends++
	f.Stats.EventsScanned++
	f.statsMu.Unlock()
}

func (f *IncrementalFolder) AppendMany(evs []SessionEvent) {
	for _, ev := range evs {
		f.Append(ev)
	}
}

//

func (f *IncrementalFolder) Snapshot(events []SessionEvent) SessionProjection {
	return f.snapshot(events, true)
}

func (f *IncrementalFolder) SnapshotMeta(events []SessionEvent) SessionProjection {
	return f.snapshot(events, false)
}

func (f *IncrementalFolder) snapshot(events []SessionEvent, copyMessages bool) SessionProjection {
	messages := f.messages
	if f.messagesDirty {

		messages = deriveMessages(events)
		f.messagesDirty = false
		f.messages = messages
		f.statsMu.Lock()
		f.Stats.DirtyRebuilds++
		f.Stats.EventsScanned += uint64(len(events))
		f.statsMu.Unlock()
	}
	p := SessionProjection{
		RequestHeader: f.reqHeader,
		SandboxMode:   f.sandbox,
		Approval:      f.approval,
		Preset:        f.preset,
		Goal:          f.goal,
		Todo:          f.todo,
		PlanMode:      f.planMode,
		Title:         f.title,
	}
	if copyMessages {
		p.Messages = append([]Message(nil), messages...)
	}
	return p
}

func (f *IncrementalFolder) FullRebuild(events []SessionEvent) SessionProjection {
	proj := FoldAll(events)
	f.messages = proj.Messages
	f.reqHeader = proj.RequestHeader
	f.preset = proj.Preset
	f.sandbox = proj.SandboxMode
	f.approval = proj.Approval
	f.goal = proj.Goal
	f.todo = proj.Todo
	f.planMode = proj.PlanMode
	f.title = proj.Title
	f.messagesDirty = false
	return proj
}

func (f *IncrementalFolder) StatsCopy() FolderStats {
	f.statsMu.Lock()
	defer f.statsMu.Unlock()
	return f.Stats
}

func (f *IncrementalFolder) MarkDirty() { f.messagesDirty = true }

// ============================================================================

// ============================================================================

//
