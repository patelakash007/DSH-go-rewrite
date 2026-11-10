package main

import (
	"context"
	"fmt"
	"time"

	"github.com/JopenChen/dsh-go/pkg/brand"
	"github.com/JopenChen/dsh-go/pkg/session"
	"github.com/JopenChen/dsh-go/pkg/uuid"
)

func (s *Server) forkSession(ctx context.Context, args map[string]any) (any, *RPCError) {
	request, _ := args["request"].(map[string]any)
	sourceID, _ := request["sessionId"].(string)
	source := s.sessionByID(sourceID)
	if source == nil {
		return nil, &RPCError{Code: "session/not-found", Message: "source session not found"}
	}
	events := source.Log.Events()
	boundary := uint64(numberArg(request, "atSeq", 0))
	if boundary == 0 {
		for _, event := range events {
			if event.Type == session.EventTurnEnd {
				boundary = event.Seq
			}
		}
	}
	if boundary == 0 || boundary > uint64(len(events)) {
		return nil, &RPCError{Code: "session/fork-unavailable", Message: "source has no completed fork boundary"}
	}
	prefix := append([]session.SessionEvent(nil), events[:boundary]...)
	childID := brand.NewSessionID(uuid.MustV4())
	childLog, err := session.NewSessionLogFromEvents(childID, prefix)
	if err != nil {
		return nil, &RPCError{Code: "session/fork-unavailable", Message: "fork boundary is inside an unfinished turn"}
	}
	if _, open := childLog.OpenTurn(); open {
		return nil, &RPCError{Code: "session/fork-unavailable", Message: "fork boundary is inside an unfinished turn"}
	}
	if err := s.dbStore.AppendBatch(ctx, childID, prefix); err != nil {
		return nil, &RPCError{Code: "gateway/internal", Message: fmt.Sprintf("persist fork: %v", err)}
	}

	source.mu.Lock()
	title := source.Title
	cwd := source.Cwd
	model := source.Model
	permission := source.Permission
	source.mu.Unlock()
	if title == "" {
		title = "Forked session"
	} else {
		title += " (fork)"
	}
	createdAt := time.Now()
	if len(prefix) > 0 {
		createdAt = prefix[0].Time
	}
	child := &SessionRecord{
		ID: childID, Title: title, Cwd: cwd, Model: model, Permission: permission,
		CreatedAt: createdAt, UpdatedAt: time.Now(), Log: childLog,
		Subs: map[string]chan []byte{}, Requests: map[string]struct{}{},
		Events: make([]map[string]any, 0, len(prefix)+2),
	}
	for _, event := range prefix {
		child.Events = append(child.Events, mapSessionEvent(event))
	}
	s.sessionsMu.Lock()
	s.sessions[childID.Raw()] = child
	s.sessionsMu.Unlock()
	s.attachSession(child)
	_, _ = child.Log.Append(session.EndSeedData{ParentSession: source.ID})
	_, _ = child.Log.Append(session.SessionTitleData{Title: title, Source: "fallback"})
	s.emitHost(map[string]any{"type": "emit", "event": "api-session/added", "args": []any{map[string]any{
		"sessionId": childID.Raw(), "updatedAt": child.UpdatedAt.UnixMilli(), "cwd": child.Cwd,
	}}})
	return map[string]any{"sessionId": childID.Raw()}, nil
}
