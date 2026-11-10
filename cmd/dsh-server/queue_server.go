package main

import (
	"strings"
	"time"

	"github.com/JopenChen/dsh-go/pkg/agent"
)

func (s *Server) dispatchUpdateQueue(args map[string]any) (any, *RPCError) {
	request, _ := args["request"].(map[string]any)
	sessionID, _ := request["sessionId"].(string)
	rec := s.sessionByID(sessionID)
	if rec == nil {
		return nil, &RPCError{Code: "session/not-found", Message: "session not found"}
	}
	itemID, _ := request["itemId"].(string)
	action, _ := request["action"].(map[string]any)
	kind, _ := action["kind"].(string)
	rec.mu.Lock()
	index := -1
	for i, item := range rec.Queue {
		if item.ID == itemID {
			index = i
			break
		}
	}
	if index < 0 {
		rec.mu.Unlock()
		return nil, &RPCError{Code: "session/queue-item-not-found", Message: "queued prompt not found"}
	}
	switch kind {
	case "remove":
		rec.Queue = append(rec.Queue[:index], rec.Queue[index+1:]...)
	case "edit":
		text := contentText(action["content"])
		if strings.TrimSpace(text) == "" {
			rec.mu.Unlock()
			return nil, &RPCError{Code: "gateway/bad-request", Message: "queued prompt content is empty"}
		}
		rec.Queue[index].Text = text
	case "steer":
		item := rec.Queue[index]
		rec.Queue = append(rec.Queue[:index], rec.Queue[index+1:]...)
		rec.Queue = append([]queuedPrompt{item}, rec.Queue...)
	default:
		rec.mu.Unlock()
		return nil, &RPCError{Code: "gateway/arguments-invalid", Message: "unknown queue action"}
	}
	rec.mu.Unlock()
	s.broadcastInbox(rec)
	return map[string]any{"accepted": true}, nil
}

func (s *Server) broadcastInbox(rec *SessionRecord) {
	rec.mu.Lock()
	inbox := inboxProjectionLocked(rec)
	rec.mu.Unlock()
	s.broadcastSessionControl(rec, rec.Log.LastSeq())
	s.emitHost(map[string]any{
		"type": "emit", "event": "api-session/activity",
		"args": []any{rec.ID.Raw(), time.Now().UnixMilli()},
	})
	_ = inbox
}

func inboxProjectionLocked(rec *SessionRecord) map[string]any {
	nextTurn := make([]map[string]any, 0, len(rec.Queue))
	for _, item := range rec.Queue {
		nextTurn = append(nextTurn, map[string]any{
			"id": item.ID, "role": "user",
			"content": []map[string]any{{"type": "text", "text": item.Text}},
			"source":  map[string]any{"kind": "user", "rpcId": item.RequestID},
		})
	}
	return map[string]any{"next-turn": nextTurn, "next-step": []any{}}
}

func (s *Server) startNextQueued(rec *SessionRecord) {
	rec.mu.Lock()
	if len(rec.Queue) == 0 || rec.Cancel != nil {
		rec.mu.Unlock()
		return
	}
	item := rec.Queue[0]
	rec.Queue = rec.Queue[1:]
	rec.RunID = item.RequestID
	rec.mu.Unlock()
	runtimeAgent := s.ensureAgent(rec)
	rec.mu.Lock()
	rec.Cancel = func() { runtimeAgent.Cancel(agent.CancelUser) }
	rec.mu.Unlock()
	if err := runtimeAgent.RunRequest(item.Text, item.RequestID); err != nil {
		rec.mu.Lock()
		rec.Cancel = nil
		rec.RunID = ""
		rec.mu.Unlock()
		return
	}
	s.broadcastInbox(rec)
}
