package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/JopenChen/dsh-go/pkg/goal"
	"github.com/JopenChen/dsh-go/pkg/session"
	"github.com/JopenChen/dsh-go/pkg/uuid"
)

func nativeCommandCatalog() []map[string]any {
	return []map[string]any{
		{"name": "compact", "description": "Compact the conversation into a summary", "definitionId": "@deepseek-ai/dsh-command-compact"},
		{"name": "permission", "description": "Change the native tool permission preset", "definitionId": "@deepseek-ai/dsh-permission-presets", "input": map[string]any{"hint": "read-only | auto | danger-full-access"}},
		{"name": "plan", "description": "Turn plan mode on or off", "definitionId": "@deepseek-ai/dsh-plan-mode", "input": map[string]any{"hint": "off"}},
		{"name": "goal", "description": "Create or manage the session goal", "definitionId": "@deepseek-ai/dsh-command-goal", "input": map[string]any{"hint": "<objective> | edit <objective> | pause | resume | clear"}},
	}
}

func (s *Server) executeNativeCommand(ctx context.Context, rec *SessionRecord, line string) (any, *RPCError) {
	fields := strings.Fields(strings.TrimSpace(line))
	if len(fields) == 0 {
		return nil, &RPCError{Code: "gateway/arguments-invalid", Message: "command line is empty"}
	}
	success := func(text string) (any, *RPCError) {
		return map[string]any{
			"commandId": uuid.MustV4(),
			"result":    map[string]any{"kind": "success", "text": text},
		}, nil
	}
	failure := func(text string) (any, *RPCError) {
		return map[string]any{
			"commandId": uuid.MustV4(),
			"result":    map[string]any{"kind": "error", "text": text},
		}, nil
	}
	switch fields[0] {
	case "/permission":
		if len(fields) != 2 {
			return failure("Usage: /permission read-only | auto | danger-full-access")
		}
		preset := fields[1]
		if preset != "read-only" && preset != "auto" && preset != "danger-full-access" {
			return nil, &RPCError{Code: "gateway/arguments-invalid", Message: "unknown permission preset"}
		}
		_, _ = rec.Log.Append(session.PresetChangeData{Preset: preset})
		return success("Permission preset: " + preset)
	case "/compact":
		if len(fields) != 1 {
			return failure("Usage: /compact (no arguments)")
		}
		text, err := s.compactSession(ctx, rec)
		if err != nil {
			return failure(err.Error())
		}
		return success(text)
	case "/plan":
		mode := "on"
		if len(fields) == 2 && fields[1] == "off" {
			mode = "off"
		} else if len(fields) > 2 || (len(fields) == 2 && fields[1] != "off") {
			return failure("Usage: /plan or /plan off")
		}
		_, _ = rec.Log.Append(session.PlanModeData{Mode: mode})
		if mode == "on" {
			return success("Plan mode on.")
		}
		return success("Plan mode off.")
	case "/goal":
		return s.executeGoalCommand(rec, fields[1:])
	default:
		return nil, &RPCError{Code: "commands/not-found", Message: "native engine does not implement this command"}
	}
}

func (s *Server) executeGoalCommand(rec *SessionRecord, args []string) (any, *RPCError) {
	success := func(text string) (any, *RPCError) {
		return map[string]any{
			"commandId": uuid.MustV4(),
			"result":    map[string]any{"kind": "success", "text": text},
		}, nil
	}
	current, present := goal.FromLog(rec.Log)
	if len(args) == 0 {
		if !present {
			return success("No goal is currently set. Usage: /goal <objective> | edit <objective> | pause | resume | clear")
		}
		return success(fmt.Sprintf("Goal (%s): %s\n\nCommands: /goal edit <objective>, /goal pause, /goal resume, /goal clear", current.Phase, current.Description))
	}
	action := args[0]
	ref := map[string]any{"id": current.ID, "revision": current.Revision}
	switch action {
	case "clear":
		if !present {
			return success("No goal is currently set.")
		}
		if _, rpcErr := s.dispatchGoalRPC("goals/clear", map[string]any{
			"agentId": rec.ID.Raw(), "ref": ref,
		}); rpcErr != nil {
			return nil, rpcErr
		}
		return success("Goal cleared.")
	case "pause":
		if !present {
			return success("No goal is currently set; /goal pause requires one.")
		}
		if _, rpcErr := s.dispatchGoalRPC("goals/pause", map[string]any{
			"agentId": rec.ID.Raw(), "ref": ref,
		}); rpcErr != nil {
			return nil, rpcErr
		}
		return success("Goal paused.")
	case "resume":
		if !present {
			return success("No goal is currently set; /goal resume requires one.")
		}
		if _, rpcErr := s.dispatchGoalRPC("goals/resume", map[string]any{
			"agentId": rec.ID.Raw(), "ref": ref,
		}); rpcErr != nil {
			return nil, rpcErr
		}
		return success("Goal resumed.")
	case "complete":
		if !present {
			return success("No goal is currently set; /goal complete requires one.")
		}
		if _, rpcErr := s.dispatchGoalRPC("goals/complete", map[string]any{
			"agentId": rec.ID.Raw(), "ref": ref,
		}); rpcErr != nil {
			return nil, rpcErr
		}
		return success("Goal completed.")
	case "edit":
		if !present {
			return success("No goal is currently set; /goal edit requires one.")
		}
		objective := strings.TrimSpace(strings.Join(args[1:], " "))
		if objective == "" {
			return success("Usage: /goal edit <objective>")
		}
		if _, rpcErr := s.dispatchGoalRPC("goals/edit", map[string]any{
			"agentId": rec.ID.Raw(), "ref": ref,
			"request": map[string]any{"objective": objective},
		}); rpcErr != nil {
			return nil, rpcErr
		}
		return success("Goal updated.")
	default:
		objective := strings.TrimSpace(strings.Join(args, " "))
		if objective == "" {
			return success("Usage: /goal <objective>")
		}
		if _, rpcErr := s.dispatchGoalRPC("goals/create", map[string]any{
			"agentId": rec.ID.Raw(),
			"request": map[string]any{"objective": objective},
		}); rpcErr != nil {
			return nil, rpcErr
		}
		return success("Goal created: " + objective)
	}
}
