package main

import (
	"sort"

	"github.com/JopenChen/dsh-go/pkg/session"
	"github.com/JopenChen/dsh-go/pkg/uuid"
)

func (s *Server) sessionProjection(rec *SessionRecord) map[string]any {
	rec.mu.Lock()
	title := rec.Title
	if title == "" {
		title = "New session"
	}
	model := rec.Model
	if model == "" {
		model = "deepseek-chat"
	}
	provider := rec.Provider
	if provider == "" {
		provider = "deepseek"
	}
	permission := rec.Permission
	if permission == "" {
		permission = "auto"
	}
	blank := len(rec.Events) == 0
	inbox := inboxProjectionLocked(rec)
	rec.mu.Unlock()
	values := map[string]any{
		"inbox":       inbox,
		"title":       title,
		"permissions": map[string]any{"currentValue": permission},
		"modelSelection": map[string]any{
			"lastUsed": map[string]any{"provider": provider, "model": model},
			"next":     map[string]any{"provider": provider, "model": model},
		},
		"sessionListMetadata": map[string]any{"blank": blank},
		"goal":                goalProjection(rec),
		"todos":               todoProjection(rec),
	}
	return map[string]any{"asOfSeq": int(rec.Log.LastSeq()), "values": values}
}

func todoProjection(rec *SessionRecord) []map[string]any {
	fold := session.FoldTodoWrite(rec.Log.Events())
	if len(fold.Entries) > 0 {
		result := make([]map[string]any, 0, len(fold.Entries))
		for index, entry := range fold.Entries {
			result = append(result, map[string]any{
				"id": index, "content": entry.Content, "status": entry.Status,
			})
		}
		return result
	}
	result := make([]map[string]any, 0, len(fold.Items))
	for index, content := range fold.Items {
		result = append(result, map[string]any{"id": index, "content": content, "status": "pending"})
	}
	return result
}

func (s *Server) sessionControlBaseline() map[string]any {
	s.sessionsMu.RLock()
	ids := make([]string, 0, len(s.sessions))
	for id := range s.sessions {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	records := make(map[string]*SessionRecord, len(ids))
	for _, id := range ids {
		records[id] = s.sessions[id]
	}
	s.sessionsMu.RUnlock()
	projections := make(map[string]any, len(ids))
	for _, id := range ids {
		projections[id] = s.sessionProjection(records[id])
	}
	return map[string]any{"type": "baseline", "value": map[string]any{"projections": projections}}
}

func (s *Server) addControlSubscriber() (string, chan map[string]any) {
	key := uuid.MustV4()
	channel := make(chan map[string]any, 32)
	s.controlMu.Lock()
	s.controlSubs[key] = channel
	s.controlMu.Unlock()
	return key, channel
}

func (s *Server) removeControlSubscriber(key string) {
	s.controlMu.Lock()
	channel := s.controlSubs[key]
	delete(s.controlSubs, key)
	if channel != nil {
		close(channel)
	}
	s.controlMu.Unlock()
}

func (s *Server) broadcastSessionControl(rec *SessionRecord, seq uint64) {
	projection := s.sessionProjection(rec)
	values, _ := projection["values"].(map[string]any)
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	changes := make([]map[string]any, 0, len(keys))
	for _, key := range keys {
		changes = append(changes, map[string]any{"key": key, "val": values[key]})
	}
	frame := map[string]any{"type": "changes", "sessionId": rec.ID.Raw(), "seq": int(seq), "changes": changes}
	s.controlMu.Lock()
	defer s.controlMu.Unlock()
	for _, channel := range s.controlSubs {
		select {
		case channel <- frame:
		default:
		}
	}
}
