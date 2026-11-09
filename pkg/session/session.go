//

//

//

package session

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/JopenChen/dsh-go/pkg/brand"
	"github.com/JopenChen/dsh-go/pkg/invariant"
)

// ============================================================================

// ============================================================================

type EventType string

const (
	EventTurnStart         EventType = "turn/start"
	EventTurnEnd           EventType = "turn/end"
	EventTurnStopping      EventType = "turn/stopping"
	EventStepStart         EventType = "step/start"
	EventStepEnd           EventType = "step/end"
	EventAgentError        EventType = "agent/error"
	EventAgentPreStep      EventType = "agent/pre-step"
	EventAgentRequest      EventType = "agent/request"
	EventAgentStatus       EventType = "agent/status"
	EventAgentInboxSpliced EventType = "agent/inbox/spliced"
)

const (
	EventUserMessage        EventType = "user/message"
	EventAssistantMessage   EventType = "assistant/message"
	EventAssistantChunk     EventType = "assistant/chunk"
	EventAssistantReasoning EventType = "assistant/reasoning"
	EventInjectionContext   EventType = "injection/context"
	EventSessionTitle       EventType = "session/title"
)

const (
	EventToolCall         EventType = "tool/call"
	EventToolResult       EventType = "tool/result"
	EventToolError        EventType = "tool/error"
	EventToolObserved     EventType = "tool/observed"
	EventToolPresentation EventType = "tool/presentation"
)

const (
	EventPlanMode     EventType = "plan/mode"
	EventPlanApproval EventType = "plan/approval"
	EventGoalChange   EventType = "goal/change"
	EventGoalRound    EventType = "goal/round"
	EventTodoWrite    EventType = "todo/write"
	EventEndSeed      EventType = "session/end-seed"
	EventPlanUpdate   EventType = "project/plan-update"
)

const (
	EventRequestHeader     EventType = "request/header"
	EventRequestContext    EventType = "request/context"
	EventApprovalAsked     EventType = "approval/asked"
	EventApprovalDecided   EventType = "approval/decided"
	EventPresetChange      EventType = "permission/preset-change"
	EventAgentPresetChange EventType = "agent/preset-change"
	EventCommandRun        EventType = "command/run"
	EventCommandDone       EventType = "command/done"
)

const (
	EventFSWriteIntent EventType = "fs/write-intent"
	EventFSEditIntent  EventType = "fs/edit-intent"
	EventFSObserved    EventType = "fs/observed"
	EventFSVersionBump EventType = "fs/version-bump"
	EventFSRead        EventType = "fs/read"
	EventFSDirList     EventType = "fs/dir-list"
)

const (
	EventSkillsChange  EventType = "skills/change"
	EventSkillsCatalog EventType = "skills/catalog"
	EventSkillInject   EventType = "skill/inject"
)

const (
	EventLLMRequest EventType = "llm/request"
	EventLLMStream  EventType = "llm/stream"
	EventLLMRetry   EventType = "llm/retry"
	EventLLMError   EventType = "llm/error"
	EventLLMDone    EventType = "llm/done"
)

const (
	EventSurfaceReplace          EventType = "surface/replace"
	EventAttachmentAdded         EventType = "attachment/added"
	EventSessionProjectionUpdate EventType = "session/projection-updated"
	EventWorkspaceChange         EventType = "workspace/change"
	EventUserQuestion            EventType = "user/question"
	EventSandboxMode             EventType = "sandbox/mode"
)

var AllEventTypes = []EventType{
	EventTurnStart, EventTurnEnd, EventTurnStopping, EventStepStart, EventStepEnd,
	EventAgentError, EventAgentPreStep, EventAgentRequest, EventAgentStatus, EventAgentInboxSpliced,
	EventUserMessage, EventAssistantMessage, EventAssistantChunk, EventAssistantReasoning,
	EventInjectionContext, EventSessionTitle,
	EventToolCall, EventToolResult, EventToolError, EventToolObserved, EventToolPresentation,
	EventPlanMode, EventPlanApproval, EventGoalChange, EventGoalRound, EventTodoWrite,
	EventEndSeed, EventPlanUpdate,
	EventRequestHeader, EventRequestContext, EventApprovalAsked, EventApprovalDecided,
	EventPresetChange, EventAgentPresetChange, EventCommandRun, EventCommandDone,
	EventFSWriteIntent, EventFSEditIntent, EventFSObserved, EventFSVersionBump, EventFSRead, EventFSDirList,
	EventSkillsChange, EventSkillsCatalog, EventSkillInject,
	EventLLMRequest, EventLLMStream, EventLLMRetry, EventLLMError, EventLLMDone,
	EventSurfaceReplace, EventAttachmentAdded, EventSessionProjectionUpdate, EventWorkspaceChange,
	EventUserQuestion, EventSandboxMode,
}

// ============================================================================

// ============================================================================

type EventData interface {
	EventType() EventType
}

type TurnStartData struct {
	Turn uint64 `json:"turn"`
}

func (TurnStartData) EventType() EventType { return EventTurnStart }

type TurnEndReason string

func (r *TurnEndReason) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		*r = TurnEndReason(s)
		return nil
	}
	var m struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(data, &m); err == nil {
		*r = TurnEndReason(m.Kind)
		return nil
	}
	return nil
}

// completed / aborted / blocked / error / max-tokens / interrupted
const (
	ReasonCompleted   TurnEndReason = "completed"
	ReasonFinished    TurnEndReason = "finished"
	ReasonInterrupted TurnEndReason = "interrupted"
	ReasonAborted     TurnEndReason = "aborted"
	ReasonBlocked     TurnEndReason = "blocked"
	ReasonError       TurnEndReason = "error"
	ReasonMaxTokens   TurnEndReason = "max-tokens"
)

type TurnEndData struct {
	Turn   uint64        `json:"turn"`
	Reason TurnEndReason `json:"reason"`
}

func (TurnEndData) EventType() EventType { return EventTurnEnd }

type TurnStoppingData struct {
	Reason string `json:"reason,omitempty"`
}

func (TurnStoppingData) EventType() EventType { return EventTurnStopping }

type StepStartData struct {
	Turn    uint64 `json:"turn"`
	Step    uint64 `json:"step"`
	StepSeq uint64 `json:"stepSeq,omitempty"`
}

func (StepStartData) EventType() EventType { return EventStepStart }

type StepEndData struct {
	Turn    uint64 `json:"turn"`
	Step    uint64 `json:"step"`
	StepSeq uint64 `json:"stepSeq,omitempty"`
}

func (StepEndData) EventType() EventType { return EventStepEnd }

type AgentErrorData struct {
	Message string `json:"message"`
	Pkg     string `json:"pkg,omitempty"`
}

func (AgentErrorData) EventType() EventType { return EventAgentError }

type AgentPreStepData struct {
	Blocked bool `json:"blocked,omitempty"`
}

func (AgentPreStepData) EventType() EventType { return EventAgentPreStep }

type AgentRequestData struct {
	Provider string `json:"provider"`
	Model    string `json:"model,omitempty"`
}

func (AgentRequestData) EventType() EventType { return EventAgentRequest }

type AgentStatusData struct {
	Status string `json:"status"`
}

func (AgentStatusData) EventType() EventType { return EventAgentStatus }

type InboxTarget string

const (
	InboxNextTurn InboxTarget = "next-turn"

	InboxNextStep InboxTarget = "next-step"
)

type InboxSplicedData struct {
	Target       InboxTarget `json:"target"`
	Start        int         `json:"start"`
	RemovedCount int         `json:"removedCount,omitempty"`
	Inserted     []string    `json:"inserted"`
	Outcome      string      `json:"outcome,omitempty"`
}

func (InboxSplicedData) EventType() EventType { return EventAgentInboxSpliced }

type UserMessageData struct {
	Content string `json:"content"`
	Source  string `json:"source,omitempty"` // e.g. "user" / "reference" / "injected"
	Refs    []any  `json:"refs,omitempty"`
}

func (u *UserMessageData) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if cRaw, ok := raw["content"]; ok {
		var str string
		if err := json.Unmarshal(cRaw, &str); err == nil {
			u.Content = str
		} else {
			var parts []map[string]any
			if err := json.Unmarshal(cRaw, &parts); err == nil {
				var sb strings.Builder
				for _, p := range parts {
					if t, ok := p["text"].(string); ok {
						sb.WriteString(t)
					}
				}
				u.Content = sb.String()
			}
		}
	}
	if sRaw, ok := raw["source"]; ok {
		var str string
		if err := json.Unmarshal(sRaw, &str); err == nil {
			u.Source = str
		} else {
			u.Source = string(sRaw)
		}
	}
	if rRaw, ok := raw["refs"]; ok {
		_ = json.Unmarshal(rRaw, &u.Refs)
	}
	return nil
}

func (UserMessageData) EventType() EventType { return EventUserMessage }

type AssistantMessageData struct {
	Content     string   `json:"content"`
	ToolCallIDs []string `json:"toolCallIds,omitempty"`
}

func (a *AssistantMessageData) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if cRaw, ok := raw["content"]; ok {
		var str string
		if err := json.Unmarshal(cRaw, &str); err == nil {
			a.Content = str
		} else {
			var parts []map[string]any
			if err := json.Unmarshal(cRaw, &parts); err == nil {
				var sb strings.Builder
				for _, p := range parts {
					if t, ok := p["text"].(string); ok {
						sb.WriteString(t)
					}
				}
				a.Content = sb.String()
			}
		}
	} else if mRaw, ok := raw["message"]; ok {
		var msg struct {
			Content json.RawMessage `json:"content"`
		}
		if err := json.Unmarshal(mRaw, &msg); err == nil {
			var str string
			if err := json.Unmarshal(msg.Content, &str); err == nil {
				a.Content = str
			} else {
				var parts []map[string]any
				if err := json.Unmarshal(msg.Content, &parts); err == nil {
					var sb strings.Builder
					for _, p := range parts {
						if t, ok := p["text"].(string); ok {
							sb.WriteString(t)
						}
					}
					a.Content = sb.String()
				}
			}
		}
	}
	if tcRaw, ok := raw["toolCallIds"]; ok {
		_ = json.Unmarshal(tcRaw, &a.ToolCallIDs)
	}
	return nil
}

func (AssistantMessageData) EventType() EventType { return EventAssistantMessage }

type AssistantChunkData struct {
	Text string `json:"text,omitempty"`
}

func (AssistantChunkData) EventType() EventType { return EventAssistantChunk }

type AssistantReasoningData struct {
	Text string `json:"text,omitempty"`
}

func (AssistantReasoningData) EventType() EventType { return EventAssistantReasoning }

type InjectionContextData struct {
	Name    string `json:"name"`
	Content string `json:"content"`
	Hash    string `json:"hash,omitempty"`
}

func (InjectionContextData) EventType() EventType { return EventInjectionContext }

type SessionTitleData struct {
	Title string `json:"title"`
}

func (SessionTitleData) EventType() EventType { return EventSessionTitle }

type ToolCallData struct {
	CallID brand.ToolCallID `json:"callId"`
	Tool   string           `json:"tool"`
	Input  json.RawMessage  `json:"input,omitempty"`
}

func (ToolCallData) EventType() EventType { return EventToolCall }

type ToolResultData struct {
	CallID    brand.ToolCallID `json:"callId"`
	IsError   bool             `json:"isError,omitempty"`
	Output    string           `json:"output,omitempty"`
	Truncated bool             `json:"truncated,omitempty"`
	SpillPath string           `json:"spillPath,omitempty"`
}

func (ToolResultData) EventType() EventType { return EventToolResult }

type ToolErrorData struct {
	CallID brand.ToolCallID `json:"callId"`
	Error  string           `json:"error"`
}

func (ToolErrorData) EventType() EventType { return EventToolError }

type ToolObservedData struct {
	CallID brand.ToolCallID `json:"callId"`
	Kind   string           `json:"kind"`
}

func (ToolObservedData) EventType() EventType { return EventToolObserved }

type ToolPresentationData struct {
	CallID brand.ToolCallID `json:"callId"`
	Card   string           `json:"card"`
}

func (ToolPresentationData) EventType() EventType { return EventToolPresentation }

type PlanModeData struct {
	Mode string `json:"mode"` // "on" / "off"
}

func (PlanModeData) EventType() EventType { return EventPlanMode }

type PlanApprovalData struct {
	Approved bool `json:"approved"`
}

func (PlanApprovalData) EventType() EventType { return EventPlanApproval }

type GoalPhase string

type GoalBlockReason struct {
	Code string `json:"code"`

	Message string `json:"message"`
}

type GoalChangeData struct {
	GoalID      string    `json:"goalId"`
	Phase       GoalPhase `json:"phase"`
	Description string    `json:"description,omitempty"`
	MaxRounds   int       `json:"maxRounds,omitempty"`
	Revision    uint64    `json:"revision"`

	BlockReason *GoalBlockReason `json:"blockedReason,omitempty"`
}

func (GoalChangeData) EventType() EventType { return EventGoalChange }

type GoalRoundData struct {
	Round int `json:"round"`
}

func (GoalRoundData) EventType() EventType { return EventGoalRound }

type TodoEntry struct {
	Content string `json:"content"`
	Status  string `json:"status"` // pending | in_progress | completed
}

type TodoWriteData struct {
	Items   []string    `json:"items,omitempty"`
	Entries []TodoEntry `json:"entries,omitempty"`
}

func (TodoWriteData) EventType() EventType { return EventTodoWrite }

type EndSeedData struct {
	ParentSession brand.SessionID `json:"parentSession,omitempty"`
}

func (EndSeedData) EventType() EventType { return EventEndSeed }

type PlanUpdateData struct {
	Content string `json:"content,omitempty"`
}

func (PlanUpdateData) EventType() EventType { return EventPlanUpdate }

type RequestHeaderData struct {
	ConfigEpoch uint64 `json:"configEpoch"`
	SystemHash  string `json:"systemHash,omitempty"`
	ToolCount   int    `json:"toolCount,omitempty"`
}

func (RequestHeaderData) EventType() EventType { return EventRequestHeader }

type RequestContextData struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Window   int    `json:"contextWindow,omitempty"`
	Reason   string `json:"reason,omitempty"` // initial/resume/change/series
}

func (RequestContextData) EventType() EventType { return EventRequestContext }

type ApprovalRequestIDData struct {
	RequestID brand.ApprovalRequestID `json:"requestId"`
	Tool      string                  `json:"tool,omitempty"`
	Summary   string                  `json:"summary,omitempty"`
}

func (ApprovalRequestIDData) EventType() EventType { return EventApprovalAsked }

type ApprovalDecidedData struct {
	RequestID brand.ApprovalRequestID `json:"requestId"`
	Allowed   bool                    `json:"allowed"`
}

func (ApprovalDecidedData) EventType() EventType { return EventApprovalDecided }

type PresetChangeData struct {
	Preset string `json:"preset"`
}

func (PresetChangeData) EventType() EventType { return EventPresetChange }

type AgentPresetChangeData struct {
	Preset string `json:"preset"`
}

func (AgentPresetChangeData) EventType() EventType { return EventAgentPresetChange }

type CommandRunData struct {
	Command string `json:"command"`
	Args    string `json:"args,omitempty"`
}

func (CommandRunData) EventType() EventType { return EventCommandRun }

type CommandDoneData struct {
	Command string `json:"command"`
}

func (CommandDoneData) EventType() EventType { return EventCommandDone }

type FsIntentData struct {
	Path     string `json:"path"`
	Kind     string `json:"kind,omitempty"`
	Version  uint64 `json:"version,omitempty"`
	Observed bool   `json:"observed,omitempty"`
}

func (FsIntentData) EventType() EventType { return "" }

type FsWriteIntentData struct {
	Path     string `json:"path"`
	Version  uint64 `json:"version,omitempty"`
	Observed bool   `json:"observed,omitempty"`
}

func (FsWriteIntentData) EventType() EventType { return EventFSWriteIntent }

type FsEditIntentData struct {
	Path    string `json:"path"`
	Version uint64 `json:"version,omitempty"`
}

func (FsEditIntentData) EventType() EventType { return EventFSEditIntent }

type FsObservedData struct {
	Path string `json:"path"`
}

func (FsObservedData) EventType() EventType { return EventFSObserved }

type FsVersionBumpData struct {
	Path    string `json:"path"`
	Version uint64 `json:"version"`
}

func (FsVersionBumpData) EventType() EventType { return EventFSVersionBump }

type FsReadData struct {
	Path string `json:"path"`
}

func (FsReadData) EventType() EventType { return EventFSRead }

type FsDirListData struct {
	Path string `json:"path"`
}

func (FsDirListData) EventType() EventType { return EventFSDirList }

type SkillsChangeData struct {
	Added   []string `json:"added,omitempty"`
	Removed []string `json:"removed,omitempty"`
	Changed []string `json:"changed,omitempty"`
}

func (SkillsChangeData) EventType() EventType { return EventSkillsChange }

type SkillsCatalogData struct {
	Catalog string `json:"catalog"`
	Hash    string `json:"hash"`
}

func (SkillsCatalogData) EventType() EventType { return EventSkillsCatalog }

type SkillInjectData struct {
	SkillName string `json:"skillName"`
	Content   string `json:"content,omitempty"`
}

func (SkillInjectData) EventType() EventType { return EventSkillInject }

type LLMRequestData struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

func (LLMRequestData) EventType() EventType { return EventLLMRequest }

type LLMStreamData struct {
	Text string `json:"text,omitempty"`
}

func (LLMStreamData) EventType() EventType { return EventLLMStream }

type LLMRetryData struct {
	Attempt   int    `json:"attempt"`
	BackoffMs int64  `json:"backoffMs"`
	Error     string `json:"error"`
}

func (LLMRetryData) EventType() EventType { return EventLLMRetry }

type LLMErrorData struct {
	Kind  string `json:"kind"` // overload/rate-limit/refusal/context-overflow
	Error string `json:"error"`
}

func (LLMErrorData) EventType() EventType { return EventLLMError }

type LLMDoneData struct {
	PromptTokens     int `json:"promptTokens"`
	CompletionTokens int `json:"completionTokens"`
}

func (LLMDoneData) EventType() EventType { return EventLLMDone }

type SurfaceReplaceData struct {
	Start uint64          `json:"start"`
	End   uint64          `json:"end"`
	Data  json.RawMessage `json:"data,omitempty"`
}

func (SurfaceReplaceData) EventType() EventType { return EventSurfaceReplace }

type AttachmentAddedData struct {
	AttachmentID brand.AttachmentID `json:"attachmentId"`
	Name         string             `json:"name,omitempty"`
}

func (AttachmentAddedData) EventType() EventType { return EventAttachmentAdded }

type ProjectionUpdateData struct {
	ProjectionID brand.ProjectionID `json:"projectionId"`
	Hash         string             `json:"hash,omitempty"`
}

func (ProjectionUpdateData) EventType() EventType { return EventSessionProjectionUpdate }

type WorkspaceChangeData struct {
	WorkspaceID brand.WorkspaceID `json:"workspaceId"`
	Root        string            `json:"root,omitempty"`
}

func (WorkspaceChangeData) EventType() EventType { return EventWorkspaceChange }

type UserQuestionData struct {
	Question string `json:"question"`
	Answer   string `json:"answer,omitempty"`
}

func (UserQuestionData) EventType() EventType { return EventUserQuestion }

type SandboxModeData struct {
	Mode   string `json:"mode"`
	Source string `json:"source,omitempty"`
}

func (SandboxModeData) EventType() EventType { return EventSandboxMode }

// ============================================================================

// ============================================================================

func newEventData(t EventType) (EventData, error) {
	switch t {
	case EventTurnStart:
		return TurnStartData{}, nil
	case EventTurnEnd:
		return TurnEndData{}, nil
	case EventTurnStopping:
		return TurnStoppingData{}, nil
	case EventStepStart:
		return StepStartData{}, nil
	case EventStepEnd:
		return StepEndData{}, nil
	case EventAgentError:
		return AgentErrorData{}, nil
	case EventAgentPreStep:
		return AgentPreStepData{}, nil
	case EventAgentRequest:
		return AgentRequestData{}, nil
	case EventAgentStatus:
		return AgentStatusData{}, nil
	case EventAgentInboxSpliced:
		return InboxSplicedData{}, nil
	case EventUserMessage:
		return UserMessageData{}, nil
	case EventAssistantMessage:
		return AssistantMessageData{}, nil
	case EventAssistantChunk:
		return AssistantChunkData{}, nil
	case EventAssistantReasoning:
		return AssistantReasoningData{}, nil
	case EventInjectionContext:
		return InjectionContextData{}, nil
	case EventSessionTitle:
		return SessionTitleData{}, nil
	case EventToolCall:
		return ToolCallData{}, nil
	case EventToolResult:
		return ToolResultData{}, nil
	case EventToolError:
		return ToolErrorData{}, nil
	case EventToolObserved:
		return ToolObservedData{}, nil
	case EventToolPresentation:
		return ToolPresentationData{}, nil
	case EventPlanMode:
		return PlanModeData{}, nil
	case EventPlanApproval:
		return PlanApprovalData{}, nil
	case EventGoalChange:
		return GoalChangeData{}, nil
	case EventGoalRound:
		return GoalRoundData{}, nil
	case EventTodoWrite:
		return TodoWriteData{}, nil
	case EventEndSeed:
		return EndSeedData{}, nil
	case EventPlanUpdate:
		return PlanUpdateData{}, nil
	case EventRequestHeader:
		return RequestHeaderData{}, nil
	case EventRequestContext:
		return RequestContextData{}, nil
	case EventApprovalAsked:
		return ApprovalRequestIDData{}, nil
	case EventApprovalDecided:
		return ApprovalDecidedData{}, nil
	case EventPresetChange:
		return PresetChangeData{}, nil
	case EventAgentPresetChange:
		return AgentPresetChangeData{}, nil
	case EventCommandRun:
		return CommandRunData{}, nil
	case EventCommandDone:
		return CommandDoneData{}, nil
	case EventFSWriteIntent:
		return FsWriteIntentData{}, nil
	case EventFSEditIntent:
		return FsEditIntentData{}, nil
	case EventFSObserved:
		return FsObservedData{}, nil
	case EventFSVersionBump:
		return FsVersionBumpData{}, nil
	case EventFSRead:
		return FsReadData{}, nil
	case EventFSDirList:
		return FsDirListData{}, nil
	case EventSkillsChange:
		return SkillsChangeData{}, nil
	case EventSkillsCatalog:
		return SkillsCatalogData{}, nil
	case EventSkillInject:
		return SkillInjectData{}, nil
	case EventLLMRequest:
		return LLMRequestData{}, nil
	case EventLLMStream:
		return LLMStreamData{}, nil
	case EventLLMRetry:
		return LLMRetryData{}, nil
	case EventLLMError:
		return LLMErrorData{}, nil
	case EventLLMDone:
		return LLMDoneData{}, nil
	case EventSurfaceReplace:
		return SurfaceReplaceData{}, nil
	case EventAttachmentAdded:
		return AttachmentAddedData{}, nil
	case EventSessionProjectionUpdate:
		return ProjectionUpdateData{}, nil
	case EventWorkspaceChange:
		return WorkspaceChangeData{}, nil
	case EventUserQuestion:
		return UserQuestionData{}, nil
	case EventSandboxMode:
		return SandboxModeData{}, nil
	default:
		return nil, fmt.Errorf("session: unknown event type %q", t)
	}
}

func EventTypeOf(d EventData) EventType {
	return d.EventType()
}

// ============================================================================

// ============================================================================

type SessionEvent struct {
	Seq uint64 `json:"seq"`

	Time time.Time `json:"time"`

	Type EventType `json:"type"`

	Data EventData `json:"data"`

	SurfaceOp *SurfaceOp `json:"surfaceOp,omitempty"`

	SourceEventSeqs []uint64 `json:"sourceEventSeqs,omitempty"`
}

type eventEnvelope struct {
	Seq             uint64          `json:"seq"`
	Time            time.Time       `json:"time"`
	Type            EventType       `json:"type"`
	Data            json.RawMessage `json:"data"`
	SurfaceOp       *SurfaceOp      `json:"surfaceOp,omitempty"`
	SourceEventSeqs []uint64        `json:"sourceEventSeqs,omitempty"`
}

func (env *eventEnvelope) UnmarshalJSON(data []byte) error {
	type rawEnv struct {
		Seq             uint64          `json:"seq"`
		TimeRaw         json.RawMessage `json:"time"`
		Type            EventType       `json:"type"`
		Data            json.RawMessage `json:"data"`
		SurfaceOp       *SurfaceOp      `json:"surfaceOp,omitempty"`
		SourceEventSeqs []uint64        `json:"sourceEventSeqs,omitempty"`
	}
	var r rawEnv
	if err := json.Unmarshal(data, &r); err != nil {
		return err
	}
	env.Seq = r.Seq
	env.Type = r.Type
	env.Data = r.Data
	env.SurfaceOp = r.SurfaceOp
	env.SourceEventSeqs = r.SourceEventSeqs

	if len(r.TimeRaw) > 0 {
		var t time.Time
		if err := json.Unmarshal(r.TimeRaw, &t); err == nil {
			env.Time = t
		} else {
			var ms int64
			if err := json.Unmarshal(r.TimeRaw, &ms); err == nil {
				env.Time = time.UnixMilli(ms)
			}
		}
	}
	return nil
}

func (e SessionEvent) MarshalJSON() ([]byte, error) {
	dataBytes, err := json.Marshal(e.Data)
	if err != nil {
		return nil, fmt.Errorf("session: marshal data for %q: %w", e.Type, err)
	}
	env := eventEnvelope{
		Seq:             e.Seq,
		Time:            e.Time,
		Type:            e.Type,
		Data:            dataBytes,
		SurfaceOp:       e.SurfaceOp,
		SourceEventSeqs: e.SourceEventSeqs,
	}
	return json.Marshal(env)
}

func (e *SessionEvent) UnmarshalJSON(data []byte) error {
	var env eventEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		return fmt.Errorf("session: unmarshal envelope: %w", err)
	}
	instance, err := newEventData(env.Type)
	if err != nil {
		return err
	}
	if len(env.Data) > 0 {

		pv := reflect.New(reflect.TypeOf(instance))
		if err := json.Unmarshal(env.Data, pv.Interface()); err != nil {
			return fmt.Errorf("session: unmarshal data for %q: %w", env.Type, err)
		}
		instance = pv.Elem().Interface().(EventData)
	}
	e.Seq = env.Seq
	e.Time = env.Time
	e.Type = env.Type
	e.Data = instance
	e.SurfaceOp = env.SurfaceOp
	e.SourceEventSeqs = env.SourceEventSeqs
	return nil
}

// ============================================================================

// ============================================================================

type sessionState struct {
	turnOpen  bool
	stepOpen  bool
	openTurn  uint64
	openStep  uint64
	nextTurn  uint64
	nextStep  uint64
	toolCalls map[string]bool
	lastTime  time.Time
}

type SessionLog struct {
	mu         sync.Mutex
	sessionID  brand.SessionID
	events     []SessionEvent
	seq        uint64
	state      sessionState
	invariants *invariant.Registry

	folder *IncrementalFolder
}

//

func (sl *SessionLog) EnableIncrementalProjection() {
	sl.mu.Lock()
	defer sl.mu.Unlock()
	if sl.folder != nil {
		return
	}
	if len(sl.events) == 0 {
		sl.folder = NewIncrementalFolder()
	} else {
		sl.folder = FolderFromEvents(sl.events)
	}
}

func (sl *SessionLog) DisableIncrementalProjection() {
	sl.mu.Lock()
	defer sl.mu.Unlock()
	sl.folder = nil
}

func (sl *SessionLog) IncrementalEnabled() bool {
	sl.mu.Lock()
	defer sl.mu.Unlock()
	return sl.folder != nil
}

func (sl *SessionLog) Projection() SessionProjection {
	sl.mu.Lock()
	defer sl.mu.Unlock()
	if sl.folder != nil {
		return sl.folder.Snapshot(sl.events)
	}
	return FoldAll(sl.events)
}

func (sl *SessionLog) ProjectionMeta() SessionProjection {
	sl.mu.Lock()
	defer sl.mu.Unlock()
	if sl.folder != nil {
		return sl.folder.SnapshotMeta(sl.events)
	}

	p := FoldAll(sl.events)
	p.Messages = nil
	return p
}

func (sl *SessionLog) IncrementalStats() FolderStats {
	sl.mu.Lock()
	defer sl.mu.Unlock()
	if sl.folder == nil {
		return FolderStats{}
	}
	return sl.folder.StatsCopy()
}

func NewSessionLog(id brand.SessionID) *SessionLog {
	sl := &SessionLog{
		sessionID:  id,
		state:      sessionState{toolCalls: map[string]bool{}},
		invariants: invariant.NewRegistry(),
	}

	_ = sl.invariants.Register("pkg/session", sl.checkTurnPairing)
	_ = sl.invariants.Register("pkg/session", sl.checkStepPairing)
	_ = sl.invariants.Register("pkg/session", sl.checkToolCallResultPairing)
	return sl
}

func (sl *SessionLog) SessionID() brand.SessionID {
	return sl.sessionID
}

func (sl *SessionLog) SandboxMode() (SandboxMode, bool) {
	evs := sl.Events()
	for i := len(evs) - 1; i >= 0; i-- {
		if evs[i].Type != EventSandboxMode {
			continue
		}
		data, ok := evs[i].Data.(SandboxModeData)
		if !ok {
			continue
		}
		return SandboxMode(data.Mode), true
	}
	return "", false
}

func (sl *SessionLog) Events() []SessionEvent {
	sl.mu.Lock()
	defer sl.mu.Unlock()
	out := make([]SessionEvent, len(sl.events))
	copy(out, sl.events)
	return out
}

func (sl *SessionLog) Len() int {
	sl.mu.Lock()
	defer sl.mu.Unlock()
	return len(sl.events)
}

func (sl *SessionLog) LastSeq() uint64 {
	sl.mu.Lock()
	defer sl.mu.Unlock()
	return sl.seq
}

func (sl *SessionLog) NextTurn() uint64 {
	sl.mu.Lock()
	defer sl.mu.Unlock()
	return sl.state.nextTurn
}

func (sl *SessionLog) NextStep() uint64 {
	sl.mu.Lock()
	defer sl.mu.Unlock()
	return sl.state.nextStep
}

func (sl *SessionLog) OpenTurn() (uint64, bool) {
	sl.mu.Lock()
	defer sl.mu.Unlock()
	return sl.state.openTurn, sl.state.turnOpen
}

func (sl *SessionLog) OpenStep() (uint64, bool) {
	sl.mu.Lock()
	defer sl.mu.Unlock()
	return sl.state.openStep, sl.state.stepOpen
}

func (sl *SessionLog) Append(data EventData) (uint64, error) {
	sl.mu.Lock()
	defer sl.mu.Unlock()

	now := time.Now()
	if !sl.state.lastTime.IsZero() && now.Before(sl.state.lastTime) {
		now = sl.state.lastTime
	}

	sl.seq++
	ev := SessionEvent{
		Seq:  sl.seq,
		Time: now,
		Type: data.EventType(),
		Data: data,
	}

	if err := sl.applyState(ev); err != nil {
		sl.seq--
		return 0, err
	}

	sl.events = append(sl.events, ev)
	sl.state.lastTime = now

	if sl.folder != nil {
		sl.folder.Append(ev)
	}
	return ev.Seq, nil
}

func (sl *SessionLog) Check() []error {
	sl.mu.Lock()
	defer sl.mu.Unlock()
	return sl.invariants.Run()
}

func (sl *SessionLog) applyState(ev SessionEvent) error {
	switch ev.Type {
	case EventTurnStart:
		if sl.state.turnOpen {
			return fmt.Errorf("session invariant: turn/start while turn already open")
		}
		data, ok := ev.Data.(TurnStartData)
		if !ok {
			return fmt.Errorf("session invariant: turn/start data type mismatch")
		}
		if data.Turn != sl.state.nextTurn {
			return fmt.Errorf("session invariant: turn/start expected turn %d, got %d", sl.state.nextTurn, data.Turn)
		}
		sl.state.turnOpen = true
		sl.state.openTurn = data.Turn
		sl.state.nextStep = 1
	case EventTurnEnd:
		if !sl.state.turnOpen {
			return fmt.Errorf("session invariant: turn/end without open turn/start")
		}
		if sl.state.stepOpen {
			return fmt.Errorf("session invariant: turn/end while step still open")
		}
		data, ok := ev.Data.(TurnEndData)
		if !ok {
			return fmt.Errorf("session invariant: turn/end data type mismatch")
		}
		if data.Turn != sl.state.openTurn {
			return fmt.Errorf("session invariant: turn/end turn %d does not match open turn %d", data.Turn, sl.state.openTurn)
		}
		sl.state.turnOpen = false
		sl.state.openTurn = 0
		sl.state.nextTurn++
	case EventTurnStopping:

		if !sl.state.turnOpen {
			return fmt.Errorf("session invariant: turn/stopping without open turn")
		}
	case EventStepStart:
		if !sl.state.turnOpen {
			return fmt.Errorf("session invariant: step/start without open turn")
		}
		if sl.state.stepOpen {
			return fmt.Errorf("session invariant: step/start while step already open")
		}
		data, ok := ev.Data.(StepStartData)
		if !ok {
			return fmt.Errorf("session invariant: step/start data type mismatch")
		}
		if data.Turn != sl.state.openTurn {
			return fmt.Errorf("session invariant: step/start in turn %d but open turn is %d", data.Turn, sl.state.openTurn)
		}
		if data.Step != sl.state.nextStep {
			return fmt.Errorf("session invariant: step/start expected step %d, got %d", sl.state.nextStep, data.Step)
		}
		sl.state.stepOpen = true
		sl.state.openStep = data.Step
		sl.state.nextStep++
	case EventStepEnd:
		if !sl.state.stepOpen {
			return fmt.Errorf("session invariant: step/end without open step/start")
		}
		data, ok := ev.Data.(StepEndData)
		if !ok {
			return fmt.Errorf("session invariant: step/end data type mismatch")
		}
		if data.Turn != sl.state.openTurn {
			return fmt.Errorf("session invariant: step/end in turn %d but open turn is %d", data.Turn, sl.state.openTurn)
		}
		if data.Step != sl.state.openStep {
			return fmt.Errorf("session invariant: step/end step %d does not match open step %d", data.Step, sl.state.openStep)
		}
		sl.state.stepOpen = false
		sl.state.openStep = 0
	case EventToolCall:
		if !sl.state.stepOpen {
			return fmt.Errorf("session invariant: tool/call without open step")
		}
		td, ok := ev.Data.(ToolCallData)
		if !ok {
			return fmt.Errorf("session invariant: tool/call data type mismatch")
		}
		key := td.CallID.Raw()
		if _, exists := sl.state.toolCalls[key]; exists {
			return fmt.Errorf("session invariant: duplicate tool/call %q", key)
		}
		sl.state.toolCalls[key] = false
	case EventToolResult, EventToolError:
		td, ok := ev.Data.(ToolResultData)
		var callID brand.ToolCallID
		if ok {
			callID = td.CallID
		} else if ed, ok2 := ev.Data.(ToolErrorData); ok2 {
			callID = ed.CallID
		} else {
			return fmt.Errorf("session invariant: tool result data type mismatch")
		}
		key := callID.Raw()
		open, exists := sl.state.toolCalls[key]
		if !exists {
			return fmt.Errorf("session invariant: tool/result for unknown call %q", key)
		}
		if open {
			return fmt.Errorf("session invariant: duplicate tool/result for call %q", key)
		}
		sl.state.toolCalls[key] = true
	}
	return nil
}

func (sl *SessionLog) checkTurnPairing() error {
	open := false
	for _, ev := range sl.events {
		switch ev.Type {
		case EventTurnStart:
			open = true
		case EventTurnEnd:
			open = false
		}
	}
	if open {
		return fmt.Errorf("unclosed turn at end of log")
	}
	return nil
}

func (sl *SessionLog) checkStepPairing() error {
	turnOpen, stepOpen := false, false
	for _, ev := range sl.events {
		switch ev.Type {
		case EventTurnStart:
			turnOpen = true
		case EventTurnEnd:
			turnOpen = false
		case EventStepStart:
			stepOpen = turnOpen
		case EventStepEnd:
			stepOpen = false
		}
	}
	if stepOpen {
		return fmt.Errorf("unclosed step at end of log")
	}
	return nil
}

func (sl *SessionLog) checkToolCallResultPairing() error {
	matched := map[string]bool{}
	for _, ev := range sl.events {
		switch ev.Type {
		case EventToolCall:
			if td, ok := ev.Data.(ToolCallData); ok {
				matched[td.CallID.Raw()] = false
			}
		case EventToolResult, EventToolError:
			var callID brand.ToolCallID
			if td, ok := ev.Data.(ToolResultData); ok {
				callID = td.CallID
			} else if ed, ok := ev.Data.(ToolErrorData); ok {
				callID = ed.CallID
			}
			if _, exists := matched[callID.Raw()]; !exists {
				return fmt.Errorf("tool/result for unknown call %q", callID.Raw())
			}
			matched[callID.Raw()] = true
		}
	}
	for k, done := range matched {
		if !done {
			return fmt.Errorf("unmatched tool/call %q", k)
		}
	}
	return nil
}
