package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/JopenChen/dsh-go/pkg/agent"
	"github.com/JopenChen/dsh-go/pkg/brand"
	"github.com/JopenChen/dsh-go/pkg/session"
	"github.com/JopenChen/dsh-go/pkg/tools"
	"github.com/JopenChen/dsh-go/pkg/uuid"
)

const (
	subagentModeOneShot     = "one-shot"
	subagentModeContinuable = "continuable"
)

// sessionAddressID resolves the session id of a `session/follow` or
// `session/page` request. Subagent addresses carry the child id.
func sessionAddressID(request map[string]any) string {
	if request == nil {
		return ""
	}
	if address, ok := request["address"].(map[string]any); ok {
		if kind, _ := address["kind"].(string); kind == "subagent" {
			if child, _ := address["childSessionId"].(string); child != "" {
				return child
			}
		}
		if id, _ := address["sessionId"].(string); id != "" {
			return id
		}
	}
	if id, _ := request["sessionId"].(string); id != "" {
		return id
	}
	return ""
}

// plainSubagentAddress reports whether a request addresses a subagent child as
// an ordinary session. The official Host refuses those reads so the client can
// adopt the parent route first.
func plainSubagentAddress(request map[string]any, rec *SessionRecord) bool {
	if rec == nil || rec.Origin != "subagent" {
		return false
	}
	if address, ok := request["address"].(map[string]any); ok {
		kind, _ := address["kind"].(string)
		return kind != "subagent"
	}
	return true
}

func (s *Server) createSubagentSession(parent *SessionRecord, label, mode string) (*SessionRecord, error) {
	if parent == nil {
		return nil, fmt.Errorf("parent session is unavailable")
	}
	if mode != subagentModeOneShot && mode != subagentModeContinuable {
		return nil, fmt.Errorf("unknown subagent mode %q", mode)
	}
	childID := brand.NewSessionID(uuid.MustV4())
	now := time.Now()
	parent.mu.Lock()
	cwd := parent.Cwd
	model := parent.Model
	provider := parent.Provider
	permission := parent.Permission
	parent.mu.Unlock()
	child := &SessionRecord{
		ID: childID, Title: label, Cwd: cwd, Model: model, Provider: provider,
		Permission: permission, CreatedAt: now, UpdatedAt: now,
		Log: session.NewSessionLog(childID), Subs: map[string]chan []byte{},
		Requests: map[string]struct{}{}, ParentSessionID: parent.ID.Raw(),
		Origin: "subagent", SubagentMode: mode, SubagentLabel: label,
		SubagentCreated: now,
	}
	s.sessionsMu.Lock()
	s.sessions[childID.Raw()] = child
	s.sessionsMu.Unlock()
	s.attachSession(child)
	_, _ = child.Log.Append(session.SubagentDescriptorData{Mode: mode, Label: label, ParentSessionID: parent.ID.Raw()})
	_, _ = child.Log.Append(session.SubagentModelPolicyData{Mode: mode})
	_, _ = child.Log.Append(session.SessionTitleData{Title: label, Source: "fallback"})

	parent.mu.Lock()
	parent.SubagentCatalog = append(parent.SubagentCatalog, session.SubagentCatalogEntry{
		ID: childID.Raw(), Mode: mode, Label: label, CreatedAt: now.UnixMilli(),
	})
	catalog := append([]session.SubagentCatalogEntry(nil), parent.SubagentCatalog...)
	parent.mu.Unlock()
	_, _ = parent.Log.Append(session.SubagentCatalogData{Entries: catalog})
	s.emitHost(map[string]any{"type": "emit", "event": "api-session/added", "args": []any{map[string]any{
		"sessionId": childID.Raw(), "updatedAt": child.UpdatedAt.UnixMilli(), "cwd": child.Cwd,
		"parentSessionId": parent.ID.Raw(), "origin": "subagent",
	}}})
	return child, nil
}

func (s *Server) spawnSubagentTool(sessionID string) *tools.Tool {
	allow := false
	return &tools.Tool{
		Name:        "spawn_subagent",
		Description: "Start a child agent with its own conversation and return its final answer. Use it for focused, independent work.",
		Schema: &tools.JsonSchemaNode{
			Type: tools.TypeObject,
			Properties: map[string]*tools.JsonSchemaNode{
				"prompt": {Type: tools.TypeString, Description: "Complete task for the child agent"},
				"label":  {Type: tools.TypeString, Description: "Short durable label shown in the subagent list"},
				"mode":   {Type: tools.TypeString, Description: "one-shot (default) or continuable"},
			},
			Required: []string{"prompt"}, AdditionalProperties: &allow,
		},
		Execute: func(ctx context.Context, input map[string]any) (any, error) {
			return s.spawnSubagentFromTool(ctx, sessionID, input)
		},
	}
}

func (s *Server) spawnSubagentFromTool(ctx context.Context, parentID string, input map[string]any) (any, error) {
	parent := s.sessionByID(parentID)
	if parent == nil {
		return nil, fmt.Errorf("parent session is unavailable")
	}
	if parent.ParentSessionID != "" {
		return nil, fmt.Errorf("subagents cannot spawn nested subagents")
	}
	prompt, _ := input["prompt"].(string)
	if strings.TrimSpace(prompt) == "" {
		return nil, fmt.Errorf("spawn_subagent requires a prompt")
	}
	label, _ := input["label"].(string)
	label = strings.TrimSpace(label)
	if label == "" {
		label = strings.TrimSpace(prompt)
		if runes := []rune(label); len(runes) > 48 {
			label = string(runes[:48]) + "..."
		}
	}
	mode, _ := input["mode"].(string)
	if mode == "" {
		mode = subagentModeOneShot
	}
	child, err := s.createSubagentSession(parent, label, mode)
	if err != nil {
		return nil, err
	}
	runtimeAgent := s.ensureAgent(child)
	requestID := uuid.MustV4()
	child.mu.Lock()
	child.Cancel = func() { runtimeAgent.Cancel(agent.CancelUser) }
	child.RunID = requestID
	child.mu.Unlock()
	runErr := runtimeAgent.RunRequestWait(ctx, prompt, requestID)
	child.mu.Lock()
	child.Cancel = nil
	child.RunID = ""
	child.mu.Unlock()
	if runErr != nil {
		return map[string]any{
			"sessionId": child.ID.Raw(), "mode": mode, "label": label,
			"status": "failed", "error": runErr.Error(),
		}, nil
	}
	return map[string]any{
		"sessionId": child.ID.Raw(), "mode": mode, "label": label,
		"status": "completed", "output": lastAssistantText(child),
	}, nil
}

func lastAssistantText(rec *SessionRecord) string {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	for index := len(rec.Events) - 1; index >= 0; index-- {
		event := rec.Events[index]
		if event["type"] != string(session.EventAssistantMessage) {
			continue
		}
		data, _ := event["data"].(map[string]any)
		message, _ := data["message"].(map[string]any)
		content, _ := message["content"].([]map[string]any)
		var builder strings.Builder
		for _, block := range content {
			if block["type"] == "text" {
				text, _ := block["text"].(string)
				builder.WriteString(text)
			}
		}
		if builder.Len() > 0 {
			return builder.String()
		}
	}
	return ""
}

func (s *Server) dispatchSubagentRPC(endpoint string, args map[string]any) (any, *RPCError) {
	request, _ := args["request"].(map[string]any)
	parentID, _ := request["parentSessionId"].(string)
	childID, _ := request["childSessionId"].(string)
	parent := s.sessionByID(parentID)
	if parent == nil {
		return nil, &RPCError{Code: "subagent/parent-unavailable", Message: "parent session is not running"}
	}
	child := s.sessionByID(childID)
	if child == nil {
		return nil, &RPCError{Code: "subagent/not-found", Message: "subagent session not found"}
	}
	if child.ParentSessionID != parentID {
		return nil, &RPCError{Code: "subagent/parent-unavailable", Message: "subagent does not belong to this parent"}
	}
	switch endpoint {
	case "subagents/prompt":
		if child.SubagentMode != subagentModeContinuable {
			return nil, &RPCError{Code: "subagent/not-resumable", Message: "this subagent is read-only"}
		}
		text := contentText(request["content"])
		if strings.TrimSpace(text) == "" {
			return nil, &RPCError{Code: "gateway/bad-request", Message: "prompt content is empty"}
		}
		requestID, _ := request["requestId"].(string)
		if requestID == "" {
			requestID = uuid.MustV4()
		}
		runtimeAgent := s.ensureAgent(child)
		child.mu.Lock()
		running := child.Cancel != nil
		child.Cancel = func() { runtimeAgent.Cancel(agent.CancelUser) }
		child.RunID = requestID
		child.mu.Unlock()
		var err error
		if running || runtimeAgent.Status() == agent.StatusRunning {
			err = runtimeAgent.FollowupRequest(text, requestID)
		} else {
			err = runtimeAgent.RunRequest(text, requestID)
		}
		if err != nil {
			child.mu.Lock()
			child.Cancel = nil
			child.RunID = ""
			child.mu.Unlock()
			return nil, &RPCError{Code: "session/queue-full", Message: err.Error()}
		}
		return map[string]any{"requestId": requestID}, nil
	case "subagents/interruptByParent":
		if child.SubagentMode != subagentModeContinuable {
			return map[string]any{"accepted": false}, nil
		}
		child.mu.Lock()
		cancel := child.Cancel
		child.mu.Unlock()
		if cancel != nil {
			cancel()
		}
		return map[string]any{"accepted": true}, nil
	default:
		return nil, &RPCError{Code: "gateway/method-not-found", Message: "unknown subagent method"}
	}
}

func subagentCatalogProjection(rec *SessionRecord) []map[string]any {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	return subagentCatalogLocked(rec)
}

func subagentCatalogLocked(rec *SessionRecord) []map[string]any {
	rows := make([]map[string]any, 0, len(rec.SubagentCatalog))
	for _, entry := range rec.SubagentCatalog {
		row := map[string]any{"id": entry.ID, "mode": entry.Mode}
		if entry.Label != "" {
			row["label"] = entry.Label
		}
		if entry.CreatedAt > 0 {
			row["createdAt"] = entry.CreatedAt
		}
		rows = append(rows, row)
	}
	return rows
}

func subagentIdentityProjection(rec *SessionRecord) map[string]any {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	return subagentIdentityLocked(rec)
}

func subagentIdentityLocked(rec *SessionRecord) map[string]any {
	if rec.Origin != "subagent" || rec.SubagentMode == "" {
		return nil
	}
	value := map[string]any{"mode": rec.SubagentMode}
	if rec.SubagentLabel != "" {
		value["label"] = rec.SubagentLabel
	}
	return value
}

func subagentTimingProjection(rec *SessionRecord) map[string]any {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	return subagentTimingLocked(rec)
}

func subagentTimingLocked(rec *SessionRecord) map[string]any {
	if rec.Origin != "subagent" {
		return nil
	}
	value := map[string]any{"settledMs": rec.subagentSettledMs}
	if !rec.subagentActiveSince.IsZero() {
		active := map[string]any{"since": rec.subagentActiveSince.UnixMilli()}
		if !rec.subagentActiveUntil.IsZero() {
			active["through"] = rec.subagentActiveUntil.UnixMilli()
		}
		value["active"] = active
	}
	if rec.subagentLastDone != nil {
		value["lastTurnCompleted"] = *rec.subagentLastDone
	}
	return value
}

func (s *Server) addSubagentProjections(rec *SessionRecord, values map[string]any) {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	addSubagentProjectionsLocked(rec, values)
}

func addSubagentProjectionsLocked(rec *SessionRecord, values map[string]any) {
	if catalog := subagentCatalogLocked(rec); len(catalog) > 0 {
		values["subagentCatalog"] = catalog
	}
	if identity := subagentIdentityLocked(rec); identity != nil {
		values["subagent"] = identity
	}
	if timing := subagentTimingLocked(rec); timing != nil {
		values["subagentTiming"] = timing
	}
}
