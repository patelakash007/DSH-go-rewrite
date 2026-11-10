package main

import (
	"strings"
	"time"

	"github.com/JopenChen/dsh-go/pkg/goal"
	"github.com/JopenChen/dsh-go/pkg/session"
	"github.com/JopenChen/dsh-go/pkg/uuid"
)

func (s *Server) dispatchGoalRPC(endpoint string, args map[string]any) (any, *RPCError) {
	sessionID, _ := args["agentId"].(string)
	rec := s.sessionByID(sessionID)
	if rec == nil {
		return nil, &RPCError{Code: "session/not-found", Message: "session not found"}
	}
	current, present := goal.FromLog(rec.Log)

	switch endpoint {
	case "goals/get":
		if !present {
			return nil, nil
		}
		return s.goalView(sessionID, current), nil
	case "goals/create":
		if present && current.Phase != goal.PhaseComplete {
			return nil, &RPCError{Code: "GOAL_ALREADY_EXISTS", Message: "an unfinished goal already exists"}
		}
		request, _ := args["request"].(map[string]any)
		objective, _ := request["objective"].(string)
		objective = strings.TrimSpace(objective)
		if objective == "" {
			return nil, &RPCError{Code: "gateway/arguments-invalid", Message: "goal objective is required"}
		}
		maxRounds := numberArg(request, "maxGoalRounds", 8)
		if maxRounds < 1 || maxRounds > 100 {
			return nil, &RPCError{Code: "gateway/arguments-invalid", Message: "maxGoalRounds must be between 1 and 100"}
		}
		created := goal.NewGoal(uuid.MustV4(), objective, maxRounds)
		if present {
			created.Revision = current.Revision + 1
		}
		if err := appendGoal(rec, created); err != nil {
			return nil, &RPCError{Code: "gateway/internal", Message: err.Error()}
		}
		s.setGoalActivation(sessionID, true, &created)
		return map[string]any{"ref": goalRef(created)}, nil
	}

	if !present {
		return nil, &RPCError{Code: "GOAL_NOT_FOUND", Message: "session has no current goal"}
	}
	if stale := validateGoalRef(args["ref"], current); stale != nil {
		return nil, stale
	}

	switch endpoint {
	case "goals/edit":
		request, _ := args["request"].(map[string]any)
		if objective, ok := request["objective"].(string); ok {
			objective = strings.TrimSpace(objective)
			if objective == "" {
				return nil, &RPCError{Code: "gateway/arguments-invalid", Message: "goal objective is empty"}
			}
			current.Description = objective
		}
		if _, ok := request["maxGoalRounds"]; ok {
			maxRounds := numberArg(request, "maxGoalRounds", current.MaxRounds)
			if maxRounds < 1 || maxRounds > 100 {
				return nil, &RPCError{Code: "gateway/arguments-invalid", Message: "maxGoalRounds must be between 1 and 100"}
			}
			current.MaxRounds = maxRounds
		}
		current.Revision++
		if err := appendGoal(rec, current); err != nil {
			return nil, &RPCError{Code: "gateway/internal", Message: err.Error()}
		}
		return s.goalView(sessionID, current), nil
	case "goals/pause":
		if current.Phase != goal.PhaseActive {
			return nil, &RPCError{Code: "GOAL_INVALID_TRANSITION", Message: "only an active goal can be paused"}
		}
		current.Phase = goal.PhasePaused
		current.Revision++
		if err := appendGoal(rec, current); err != nil {
			return nil, &RPCError{Code: "gateway/internal", Message: err.Error()}
		}
		s.setGoalActivation(sessionID, false, &current)
		return s.goalView(sessionID, current), nil
	case "goals/resume":
		if current.Phase == goal.PhaseComplete {
			return nil, &RPCError{Code: "GOAL_INVALID_TRANSITION", Message: "a completed goal cannot resume"}
		}
		if current.Phase != goal.PhaseActive {
			current.Phase = goal.PhaseActive
			current.Revision++
			if err := appendGoal(rec, current); err != nil {
				return nil, &RPCError{Code: "gateway/internal", Message: err.Error()}
			}
		}
		s.setGoalActivation(sessionID, true, &current)
		return s.goalView(sessionID, current), nil
	case "goals/complete":
		if current.Phase == goal.PhaseComplete {
			return s.goalView(sessionID, current), nil
		}
		current.Phase = goal.PhaseComplete
		current.Revision++
		if err := appendGoal(rec, current); err != nil {
			return nil, &RPCError{Code: "gateway/internal", Message: err.Error()}
		}
		s.setGoalActivation(sessionID, false, &current)
		return s.goalView(sessionID, current), nil
	case "goals/clear":
		tombstone := current.Revision + 1
		_, err := rec.Log.Append(session.GoalChangeData{
			Operation: "clear", GoalID: current.ID, Revision: tombstone,
		})
		if err != nil {
			return nil, &RPCError{Code: "gateway/internal", Message: err.Error()}
		}
		s.setGoalActivation(sessionID, false, nil)
		return map[string]any{"id": current.ID, "revision": tombstone}, nil
	default:
		return nil, &RPCError{Code: "gateway/method-not-found", Message: "unknown goal method"}
	}
}

func appendGoal(rec *SessionRecord, value goal.Goal) error {
	_, err := rec.Log.Append(session.GoalChangeData{
		Operation: "put", GoalID: value.ID, Phase: session.GoalPhase(value.Phase),
		Description: value.Description, MaxRounds: value.MaxRounds, Revision: value.Revision,
		BlockReason: func() *session.GoalBlockReason {
			if value.BlockReason == nil {
				return nil
			}
			return &session.GoalBlockReason{Code: value.BlockReason.Code, Message: value.BlockReason.Message}
		}(),
	})
	return err
}

func validateGoalRef(raw any, current goal.Goal) *RPCError {
	ref, _ := raw.(map[string]any)
	id, _ := ref["id"].(string)
	revision := uint64(numberArg(ref, "revision", 0))
	if id != current.ID || revision != current.Revision {
		return &RPCError{Code: "GOAL_STALE_REVISION", Message: "goal revision is stale"}
	}
	return nil
}

func (s *Server) goalView(sessionID string, value goal.Goal) map[string]any {
	s.goalMu.Lock()
	armed := s.goalActivation[sessionID]
	s.goalMu.Unlock()
	view := goalSnapshot(value)
	view["roundsStarted"] = goalRounds(s.sessionByID(sessionID))
	view["activation"] = map[bool]string{true: "armed", false: "disarmed"}[armed]
	return view
}

func goalSnapshot(value goal.Goal) map[string]any {
	result := map[string]any{
		"id": value.ID, "revision": value.Revision, "objective": value.Description,
		"phase": string(value.Phase), "maxGoalRounds": value.MaxRounds,
	}
	if value.BlockReason != nil {
		result["blockedReason"] = map[string]any{"code": value.BlockReason.Code, "message": value.BlockReason.Message}
	}
	return result
}

func goalRef(value goal.Goal) map[string]any {
	return map[string]any{"id": value.ID, "revision": value.Revision}
}

func goalRounds(rec *SessionRecord) int {
	if rec == nil {
		return 0
	}
	max := 0
	for _, event := range rec.Log.Events() {
		if round, ok := event.Data.(session.GoalRoundData); ok && round.Round > max {
			max = round.Round
		}
	}
	return max
}

func (s *Server) setGoalActivation(sessionID string, armed bool, value *goal.Goal) {
	s.goalMu.Lock()
	s.goalActivation[sessionID] = armed
	s.goalMu.Unlock()
	var current any
	if value != nil {
		current = map[string]any{
			"id": value.ID, "revision": value.Revision,
			"activation": map[bool]string{true: "armed", false: "disarmed"}[armed],
		}
	}
	s.emitHost(map[string]any{
		"type": "emit", "event": "goal/activation-changed",
		"args": []any{map[string]any{"sessionId": sessionID, "goal": current, "at": time.Now().UnixMilli()}},
	})
}

func goalProjection(rec *SessionRecord) any {
	if rec == nil {
		return nil
	}
	value, ok := goal.FromLog(rec.Log)
	if !ok {
		return nil
	}
	events := rec.Log.Events()
	createdAt, updatedAt := int64(0), int64(0)
	if len(events) > 0 {
		createdAt = events[0].Time.UnixMilli()
		updatedAt = events[len(events)-1].Time.UnixMilli()
	}
	return map[string]any{
		"goal": goalSnapshot(value), "roundsStarted": goalRounds(rec),
		"createdAt": createdAt, "updatedAt": updatedAt,
	}
}
