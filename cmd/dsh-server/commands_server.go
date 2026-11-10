package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/JopenChen/dsh-go/pkg/goal"
	"github.com/JopenChen/dsh-go/pkg/session"
	"github.com/JopenChen/dsh-go/pkg/uuid"
)

const goalCommandUsage = "Usage: /goal [<objective>|clear|edit <objective>|pause|resume]"

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
			return success("Plan mode on. Use /plan off to leave.")
		}
		return success("Plan mode off.")
	case "/goal":
		return s.executeGoalCommand(rec, fields[1:])
	default:
		return nil, &RPCError{Code: "commands/not-found", Message: "native engine does not implement this command"}
	}
}

func goalCommandSummary(title string, view map[string]any) string {
	phase, _ := view["phase"].(string)
	objective, _ := view["objective"].(string)
	maxRounds := numberArg(view, "maxGoalRounds", 0)
	rounds := numberArg(view, "roundsStarted", 0)
	activation, _ := view["activation"].(string)
	return fmt.Sprintf(
		"%s\nStatus: %s\nObjective: %s\nRounds: %d/%d\nActivation: %s\n\nCommands: /goal edit <objective>, /goal pause, /goal resume, /goal clear",
		title, phase, objective, rounds, maxRounds, activation,
	)
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
			return success("No goal is currently set.\n" + goalCommandUsage)
		}
		return success(goalCommandSummary("Goal", s.goalView(rec.ID.Raw(), current)))
	}
	action := args[0]
	ref := map[string]any{"id": current.ID, "revision": current.Revision}
	runGoal := func(endpoint string, request map[string]any) (map[string]any, *RPCError) {
		payload := map[string]any{"agentId": rec.ID.Raw(), "ref": ref}
		if request != nil {
			payload["request"] = request
		}
		value, rpcErr := s.dispatchGoalRPC(endpoint, payload)
		if rpcErr != nil {
			return nil, rpcErr
		}
		view, _ := value.(map[string]any)
		return view, nil
	}
	switch action {
	case "clear":
		if !present {
			return success("No goal to clear.")
		}
		if _, rpcErr := s.dispatchGoalRPC("goals/clear", map[string]any{
			"agentId": rec.ID.Raw(), "ref": ref,
		}); rpcErr != nil {
			return nil, rpcErr
		}
		return success("Goal cleared.")
	case "pause":
		if !present {
			return success("No goal is currently set; /goal pause requires one. " + goalCommandUsage)
		}
		view, rpcErr := runGoal("goals/pause", nil)
		if rpcErr != nil {
			return nil, rpcErr
		}
		return success(goalCommandSummary("Goal paused", view))
	case "resume":
		if !present {
			return success("No goal is currently set; /goal resume requires one. " + goalCommandUsage)
		}
		view, rpcErr := runGoal("goals/resume", nil)
		if rpcErr != nil {
			return nil, rpcErr
		}
		return success(goalCommandSummary("Goal resumed", view))
	case "complete":
		if !present {
			return success("No goal is currently set; /goal complete requires one. " + goalCommandUsage)
		}
		view, rpcErr := runGoal("goals/complete", nil)
		if rpcErr != nil {
			return nil, rpcErr
		}
		return success(goalCommandSummary("Goal", view))
	case "edit":
		if !present {
			return success("No goal is currently set; /goal edit requires one. " + goalCommandUsage)
		}
		objective := strings.TrimSpace(strings.Join(args[1:], " "))
		if objective == "" {
			return success("Goal editing requires a replacement objective.\n" + goalCommandUsage)
		}
		view, rpcErr := runGoal("goals/edit", map[string]any{"objective": objective})
		if rpcErr != nil {
			return nil, rpcErr
		}
		return success(goalCommandSummary("Goal updated", view))
	default:
		objective := strings.TrimSpace(strings.Join(args, " "))
		if objective == "" {
			return success("No goal is currently set.\n" + goalCommandUsage)
		}
		value, rpcErr := s.dispatchGoalRPC("goals/create", map[string]any{
			"agentId": rec.ID.Raw(),
			"request": map[string]any{"objective": objective},
		})
		if rpcErr != nil {
			return nil, rpcErr
		}
		_ = value
		created, _ := goal.FromLog(rec.Log)
		return success(goalCommandSummary("Goal created", s.goalView(rec.ID.Raw(), created)))
	}
}
