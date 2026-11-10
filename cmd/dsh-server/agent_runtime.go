package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/JopenChen/dsh-go/pkg/agent"
	"github.com/JopenChen/dsh-go/pkg/brand"
	"github.com/JopenChen/dsh-go/pkg/llm"
	"github.com/JopenChen/dsh-go/pkg/llm/provider_deepseek"
	"github.com/JopenChen/dsh-go/pkg/nativetools"
	"github.com/JopenChen/dsh-go/pkg/session"
	"github.com/JopenChen/dsh-go/pkg/sysprompt"
	"github.com/JopenChen/dsh-go/pkg/tools"
	"github.com/JopenChen/dsh-go/pkg/uuid"
	"github.com/JopenChen/dsh-go/pkg/waterfall"
)

type pendingApproval struct {
	id        string
	sessionID string
	tool      string
	callID    string
	frame     map[string]any
	result    chan bool
	once      sync.Once
}

type nativeAdapter struct{ server *Server }

func (a nativeAdapter) Name() string {
	if a.server.adapter == nil {
		return "unavailable"
	}
	return a.server.adapter.Name()
}

func (a nativeAdapter) Chat(ctx context.Context, request llm.ChatRequest, callback func(llm.StreamChunk)) (llm.Usage, error) {
	adapter := a.server.adapter
	if adapter == nil {
		return llm.Usage{}, fmt.Errorf("native engine has no model adapter")
	}
	if _, deepSeek := adapter.(*provider_deepseek.DeepSeek); deepSeek {
		key, configured := a.server.credStore.Resolve(ctx, brand.NewCredentialRef("DEEPSEEK_API_KEY"))
		if !configured || strings.TrimSpace(key) == "" {
			callback(llm.StreamChunk{Kind: llm.ChunkText, Text: "Welcome to DSH Native! Configure DEEPSEEK_API_KEY in Settings to chat with DeepSeek."})
			callback(llm.StreamChunk{Kind: llm.ChunkDone})
			return llm.Usage{}, nil
		}
	}
	return adapter.Chat(ctx, request, callback)
}

func (s *Server) attachSession(rec *SessionRecord) {
	if rec == nil || rec.Log == nil || rec.stopLog != nil {
		return
	}
	rec.stopLog = rec.Log.OnAppend(func(event session.SessionEvent) {
		s.onSessionEvent(rec, event)
	})
}

func (s *Server) recoverInterruptedSession(rec *SessionRecord) {
	if rec == nil || rec.Log == nil {
		return
	}
	turn, turnOpen := rec.Log.OpenTurn()
	step, stepOpen := rec.Log.OpenStep()
	if stepOpen {
		_, _ = rec.Log.Append(session.StepEndData{Turn: turn, Step: step})
	}
	if turnOpen {
		_, _ = rec.Log.Append(session.TurnEndData{Turn: turn, Reason: session.ReasonInterrupted})
	}
}

func (s *Server) ensureAgent(rec *SessionRecord) *agent.Agent {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if rec.Agent != nil {
		rec.Agent.SetModel("deepseek", rec.Model, "")
		return rec.Agent
	}
	rec.Tools = nativetools.New(s.files, func() string {
		rec.mu.Lock()
		defer rec.mu.Unlock()
		return rec.Cwd
	})
	rec.Skills = s.ensureSkillRegistryLocked(rec)

	pipeline := tools.NewPipeline()
	pipeline.UsePre(func(execution *tools.ExecContext, next waterfall.NextFunc) error {
		if execution == nil || execution.Request == nil || !nativetools.IsDangerous(execution.Request.Tool) {
			return next()
		}
		rec.mu.Lock()
		preset := rec.Permission
		rec.mu.Unlock()
		switch preset {
		case "danger-full-access":
			return next()
		case "read-only":
			execution.Denied = true
			execution.Result.Error = "the read-only permission preset blocks this tool"
			return nil
		default:
			if !s.requestToolApproval(execution.Ctx, rec, execution.Request) {
				execution.Denied = true
				execution.Result.Error = "tool approval was rejected or cancelled"
				return nil
			}
			return next()
		}
	})
	dispatch := &tools.Tool{Name: "native-dispatch", Execute: func(ctx context.Context, input map[string]any) (any, error) {
		execution, ok := tools.ExecContextFrom(ctx)
		if !ok || execution.Request == nil {
			return nil, fmt.Errorf("native tool dispatch context is missing")
		}
		implementation := rec.Tools.Lookup(execution.Request.Tool)
		if implementation == nil || implementation.Execute == nil {
			return nil, fmt.Errorf("native tool %q is unavailable", execution.Request.Tool)
		}
		return implementation.Execute(ctx, input)
	}}
	pipeline.WithTool(dispatch)

	system := sysprompt.New()
	system.Register("native-runtime", 100, "You are running inside DSH's standalone Android-native Go engine. Use the provided workspace tools. Do not assume Ubuntu, PRoot, Termux, Node.js, npm, bash, or paths under /root exist. Keep file operations inside the app-owned workspace.")
	if catalog := rec.Skills.CatalogText(); catalog != "" {
		system.Register("skills", 90, catalog)
	}
	rec.Agent = agent.NewAgent(rec.ID, rec.Log, system, pipeline, nativeAdapter{server: s}, agent.AgentOptions{
		Provider: "deepseek", Model: rec.Model, DefaultTimeout: 15 * time.Minute,
	})
	definitions := rec.Tools.Tools()
	definitions = append(definitions, nativeSkillTool(rec.Skills))
	rec.Agent.SetTools(definitions...)
	rec.Agent.SetToolProvider(func(name string) *tools.Tool {
		if name == "load_skill" {
			return nativeSkillTool(rec.Skills)
		}
		return rec.Tools.Lookup(name)
	})
	return rec.Agent
}

func (s *Server) onSessionEvent(rec *SessionRecord, event session.SessionEvent) {
	if err := s.dbStore.Append(context.Background(), rec.ID, event); err != nil {
		log.Printf("persist native session %s seq %d: %v", rec.ID.Raw(), event.Seq, err)
	}
	mapped := mapSessionEvent(event)
	rec.mu.Lock()
	rec.Events = append(rec.Events, mapped)
	rec.UpdatedAt = event.Time
	switch data := event.Data.(type) {
	case session.SessionTitleData:
		rec.Title = data.Title
	case session.ModelSelectionData:
		if data.Model != "" {
			rec.Model = data.Model
		}
	case session.PresetChangeData:
		if data.Preset != "" {
			rec.Permission = data.Preset
		}
	case session.AgentStatusData:
		if data.Status == string(agent.StatusIdle) {
			rec.Cancel = nil
			rec.RunID = ""
		}
	}
	frame := map[string]any{"type": "item", "value": map[string]any{"type": "event", "event": mapped}}
	frameBytes, _ := json.Marshal(frame)
	for _, subscriber := range rec.Subs {
		select {
		case subscriber <- frameBytes:
		default:
		}
	}
	rec.mu.Unlock()

	s.handleAssistantStreamEvent(rec, event)
	s.broadcastSessionControl(rec, event.Seq)
	s.emitHost(map[string]any{"type": "emit", "event": "api-session/activity", "args": []any{rec.ID.Raw(), event.Time.UnixMilli()}})
	switch event.Type {
	case session.EventTurnStart:
		s.emitHost(map[string]any{"type": "emit", "event": "api-session/status", "args": []any{rec.ID.Raw(), true}})
	case session.EventTurnEnd:
		s.emitHost(map[string]any{"type": "emit", "event": "api-session/status", "args": []any{rec.ID.Raw(), false}})
	}
}

func mapSessionEvent(event session.SessionEvent) map[string]any {
	data := map[string]any{}
	raw, _ := json.Marshal(event.Data)
	_ = json.Unmarshal(raw, &data)
	switch value := event.Data.(type) {
	case session.TurnStartData:
		data = map[string]any{"turn": int(value.Turn)}
	case session.TurnEndData:
		kind := string(value.Reason)
		if kind == string(session.ReasonFinished) {
			kind = string(session.ReasonCompleted)
		}
		data = map[string]any{"turn": int(value.Turn), "reason": map[string]any{"kind": kind}}
	case session.StepStartData:
		data = map[string]any{"turn": int(value.Turn), "step": int(value.Step)}
	case session.StepEndData:
		data = map[string]any{"turn": int(value.Turn), "step": int(value.Step)}
	case session.UserMessageData:
		source := map[string]any{"kind": "user"}
		if value.RequestID != "" {
			source["rpcId"] = value.RequestID
		}
		data = map[string]any{
			"id": fmt.Sprintf("user-%d", event.Seq), "role": "user",
			"content": []map[string]any{{"type": "text", "text": value.Content}}, "source": source,
		}
	case session.AssistantMessageData:
		content := make([]map[string]any, 0, len(value.ToolCalls)+2)
		if value.Reasoning != "" {
			content = append(content, map[string]any{"type": "reasoning", "text": value.Reasoning})
		}
		if value.Content != "" || len(value.ToolCalls) == 0 {
			content = append(content, map[string]any{"type": "text", "text": value.Content})
		}
		for _, call := range value.ToolCalls {
			arguments, _ := json.Marshal(call.Input)
			content = append(content, map[string]any{"type": "tool-call", "id": call.ID, "name": call.Name, "arguments": string(arguments)})
		}
		data = map[string]any{
			"turn": int(value.Turn), "step": int(value.Step),
			"message": map[string]any{
				"id": fmt.Sprintf("assistant-%d", event.Seq), "role": "assistant", "content": content,
				"source": map[string]any{"kind": "model", "provider": "deepseek"},
			},
			"usage": map[string]any{"inputTokens": value.PromptTokens, "outputTokens": value.OutputTokens},
		}
	case session.ToolCallData:
		data = map[string]any{
			"turn": int(value.Turn), "step": int(value.Step), "callId": value.CallID.Raw(),
			"name": value.Tool, "arguments": string(value.Input),
		}
	case session.ToolResultData:
		data = map[string]any{
			"turn": int(value.Turn), "step": int(value.Step),
			"message": map[string]any{
				"id": fmt.Sprintf("tool-%d", event.Seq), "role": "tool", "toolCallId": value.CallID.Raw(),
				"isError": value.IsError, "content": []map[string]any{{"type": "text", "text": value.Output}},
				"source": map[string]any{"kind": "tool"},
			},
		}
		if value.IsError {
			data["error"] = map[string]any{"name": "ToolError", "code": "TOOL_FAILED", "reason": value.Output}
		}
	case session.SessionTitleData:
		source := value.Source
		if source == "" {
			source = "fallback"
		}
		data = map[string]any{"title": value.Title, "source": map[string]any{"kind": source}}
	case session.ModelSelectionData:
		data = map[string]any{"provider": value.Provider, "model": value.Model}
		if value.ReasoningEffort != "" {
			data["reasoningEffort"] = value.ReasoningEffort
		}
	case session.ApprovalRequestIDData:
		data = map[string]any{"id": value.RequestID.Raw(), "toolName": value.Tool, "callId": value.CallID, "reason": value.Summary}
	case session.ApprovalDecidedData:
		outcome := value.Outcome
		if outcome == "" {
			if value.Allowed {
				outcome = "allowed-once"
			} else {
				outcome = "rejected"
			}
		}
		data = map[string]any{"id": value.RequestID.Raw(), "outcome": outcome}
	case session.TodoWriteData:
		todos := make([]map[string]any, 0, len(value.Entries))
		for index, item := range value.Entries {
			todos = append(todos, map[string]any{"id": fmt.Sprintf("todo-%d", index), "content": item.Content, "status": item.Status})
		}
		data = map[string]any{"todos": todos}
	case session.GoalChangeData:
		if value.Operation == "clear" {
			data = map[string]any{
				"operation": "clear", "ref": map[string]any{"id": value.GoalID, "revision": value.Revision},
			}
		} else {
			goalValue := map[string]any{
				"id": value.GoalID, "revision": value.Revision, "objective": value.Description,
				"phase": string(value.Phase), "maxGoalRounds": value.MaxRounds,
			}
			if value.BlockReason != nil {
				goalValue["blockedReason"] = map[string]any{"code": value.BlockReason.Code, "message": value.BlockReason.Message}
			}
			data = map[string]any{
				"operation": "put", "goal": goalValue,
				"updatedAt": event.Time.UnixMilli(), "createdAt": event.Time.UnixMilli(),
			}
		}
	}
	return map[string]any{"seq": int(event.Seq), "time": event.Time.UnixMilli(), "type": string(event.Type), "data": data}
}

func (s *Server) handleAssistantStreamEvent(rec *SessionRecord, event session.SessionEvent) {
	switch value := event.Data.(type) {
	case session.StepStartData:
		rec.mu.Lock()
		rec.attemptID = uuid.MustV4()
		rec.attemptRevision = 1
		rec.attemptIndex = 0
		rec.attemptTurn = value.Turn
		rec.attemptStep = value.Step
		rec.attemptEnded = false
		attemptID := rec.attemptID
		revision := rec.attemptRevision
		rec.mu.Unlock()
		s.broadcastAssistantFrame(rec, map[string]any{
			"type": "start", "attemptId": attemptID, "revision": revision,
			"turn": int(value.Turn), "step": int(value.Step), "startedAfterSeq": int(event.Seq),
		})
	case session.AssistantChunkData:
		s.streamAssistantChunk(rec, map[string]any{"type": "text-delta", "text": value.Text})
	case session.AssistantReasoningData:
		s.streamAssistantChunk(rec, map[string]any{"type": "reasoning-delta", "text": value.Text})
	case session.AssistantMessageData:
		s.endAssistantAttempt(rec, true, int(event.Seq), "assistant/message")
	case session.StepEndData:
		s.endAssistantAttempt(rec, false, 0, "")
	}
}

func (s *Server) streamAssistantChunk(rec *SessionRecord, chunk map[string]any) {
	rec.mu.Lock()
	if rec.attemptID == "" || rec.attemptEnded {
		rec.mu.Unlock()
		return
	}
	rec.attemptRevision++
	rec.attemptIndex++
	frame := map[string]any{
		"type": "chunk", "attemptId": rec.attemptID, "revision": rec.attemptRevision,
		"index": rec.attemptIndex, "time": time.Now().UnixMilli(), "chunk": chunk,
	}
	rec.mu.Unlock()
	s.broadcastAssistantFrame(rec, frame)
}

func (s *Server) endAssistantAttempt(rec *SessionRecord, committed bool, seq int, eventType string) {
	rec.mu.Lock()
	if rec.attemptID == "" || rec.attemptEnded {
		rec.mu.Unlock()
		return
	}
	rec.attemptEnded = true
	rec.attemptRevision++
	rec.attemptIndex++
	outcome := map[string]any{"kind": "abandoned"}
	if committed {
		outcome = map[string]any{"kind": "committed", "eventType": eventType, "seq": seq}
	}
	frame := map[string]any{
		"type": "end", "attemptId": rec.attemptID, "revision": rec.attemptRevision,
		"index": rec.attemptIndex, "outcome": outcome,
	}
	rec.mu.Unlock()
	s.broadcastAssistantFrame(rec, frame)
}

func (s *Server) requestToolApproval(ctx context.Context, rec *SessionRecord, request *tools.ToolCallRequest) bool {
	if request == nil {
		return false
	}
	eventID := uuid.MustV4()
	pending := &pendingApproval{
		id: eventID, sessionID: rec.ID.Raw(), tool: request.Tool, callID: request.CallID.Raw(), result: make(chan bool, 1),
	}
	pending.frame = map[string]any{
		"type": "waterfall", "event": "approval/request", "eventId": eventID, "agentId": rec.ID.Raw(),
		"request": map[string]any{
			"toolName": request.Tool, "callId": request.CallID.Raw(),
			"reason":        "Allow the native engine to run " + request.Tool + "?",
			"displayReason": map[string]any{"en": "Allow the native engine to run " + request.Tool + "?"},
		},
	}
	s.hostMu.Lock()
	s.pendingApprovals[eventID] = pending
	s.hostMu.Unlock()
	_, _ = rec.Log.Append(session.ApprovalRequestIDData{
		RequestID: brand.NewApprovalRequestID(eventID), Tool: request.Tool, CallID: request.CallID.Raw(),
		Summary: "Allow the native engine to run " + request.Tool + "?",
	})
	s.emitHost(pending.frame)

	waitCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	allowed := false
	select {
	case allowed = <-pending.result:
	case <-waitCtx.Done():
	}
	s.removePendingApproval(eventID, pending)
	outcome := "rejected"
	if allowed {
		outcome = "allowed-once"
	} else if ctx.Err() != nil {
		outcome = "cancelled"
	}
	_, _ = rec.Log.Append(session.ApprovalDecidedData{
		RequestID: brand.NewApprovalRequestID(eventID), Allowed: allowed, Outcome: outcome,
	})
	s.emitHost(map[string]any{"type": "cancel", "eventId": eventID})
	return allowed
}

func (s *Server) removePendingApproval(eventID string, expected *pendingApproval) {
	s.hostMu.Lock()
	if s.pendingApprovals[eventID] == expected {
		delete(s.pendingApprovals, eventID)
	}
	s.hostMu.Unlock()
}

func (s *Server) resolveHostEvent(eventID string, outcome map[string]any) {
	s.hostMu.Lock()
	pending := s.pendingApprovals[eventID]
	if pending == nil {
		s.hostMu.Unlock()
		return
	}
	kind, _ := outcome["kind"].(string)
	if kind == "next" {
		s.hostMu.Unlock()
		return
	}
	delete(s.pendingApprovals, eventID)
	s.hostMu.Unlock()
	allowed := false
	if kind == "result" {
		value, _ := outcome["value"].(string)
		allowed = value == "allowed-once"
	}
	pending.once.Do(func() { pending.result <- allowed })
}

func (s *Server) emitHost(frame map[string]any) {
	s.hostMu.Lock()
	defer s.hostMu.Unlock()
	for _, subscriber := range s.hostSubs {
		select {
		case subscriber <- frame:
		default:
		}
	}
}

func (s *Server) addHostSubscriber() (string, chan map[string]any, []map[string]any) {
	key := uuid.MustV4()
	channel := make(chan map[string]any, 64)
	s.hostMu.Lock()
	s.hostSubs[key] = channel
	pending := make([]map[string]any, 0, len(s.pendingApprovals))
	for _, approval := range s.pendingApprovals {
		pending = append(pending, approval.frame)
	}
	s.hostMu.Unlock()
	return key, channel, pending
}

func (s *Server) removeHostSubscriber(key string) {
	s.hostMu.Lock()
	channel := s.hostSubs[key]
	delete(s.hostSubs, key)
	if channel != nil {
		close(channel)
	}
	s.hostMu.Unlock()
}
