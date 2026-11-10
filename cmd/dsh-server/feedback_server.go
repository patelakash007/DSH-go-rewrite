package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/JopenChen/dsh-go/pkg/uuid"
)

type feedbackItem struct {
	MessageID string `json:"messageId"`
	Rating    string `json:"rating"`
	Version   string `json:"version"`
	Note      string `json:"note,omitempty"`
	Category  string `json:"category,omitempty"`
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt"`
}

type feedbackStore struct {
	path  string
	mu    sync.Mutex
	items map[string]map[string]*feedbackItem
}

func newFeedbackStore(path string) (*feedbackStore, error) {
	store := &feedbackStore{path: path, items: map[string]map[string]*feedbackItem{}}
	raw, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &store.items); err != nil {
			return nil, err
		}
	}
	return store, nil
}

func (s *Server) dispatchMessageFeedback(endpoint string, args map[string]any) (any, *RPCError) {
	request, _ := args["request"].(map[string]any)
	sessionID, _ := request["sessionId"].(string)
	rec := s.sessionByID(sessionID)
	if rec == nil {
		return feedbackFailure("session-not-found", nil), nil
	}
	messageID, _ := request["messageId"].(string)

	s.feedback.mu.Lock()
	defer s.feedback.mu.Unlock()
	if s.feedback.items[sessionID] == nil {
		s.feedback.items[sessionID] = map[string]*feedbackItem{}
	}
	items := s.feedback.items[sessionID]
	switch endpoint {
	case "messageFeedback/list":
		rows := make([]*feedbackItem, 0, len(items))
		for _, item := range items {
			copy := *item
			rows = append(rows, &copy)
		}
		sort.Slice(rows, func(i, j int) bool { return rows[i].CreatedAt < rows[j].CreatedAt })
		return map[string]any{"ok": true, "value": map[string]any{"items": rows}}, nil
	case "messageFeedback/put":
		if !validAssistantMessage(rec, messageID) {
			return feedbackFailure("target-not-found", nil), nil
		}
		rating, _ := request["rating"].(string)
		if rating != "positive" && rating != "negative" {
			return feedbackFailure("rating-invalid", nil), nil
		}
		note, _ := request["note"].(string)
		if request["note"] != nil && strings.TrimSpace(note) == "" {
			return feedbackFailure("note-blank", nil), nil
		}
		if len([]rune(note)) > 2000 {
			return feedbackFailure("note-too-large", nil), nil
		}
		category, _ := request["category"].(string)
		existing := items[messageID]
		ifVersion, hasVersion := request["ifVersion"].(string)
		if existing == nil {
			if hasVersion && ifVersion != "" {
				return feedbackFailure("version-conflict", nil), nil
			}
		} else if !hasVersion || ifVersion != existing.Version {
			return feedbackFailure("version-conflict", existing), nil
		}
		now := time.Now().UnixMilli()
		createdAt := now
		if existing != nil {
			createdAt = existing.CreatedAt
		}
		item := &feedbackItem{
			MessageID: messageID, Rating: rating, Version: uuid.MustV4(),
			Note: note, Category: category, CreatedAt: createdAt, UpdatedAt: now,
		}
		items[messageID] = item
		if err := s.feedback.persistLocked(); err != nil {
			return nil, &RPCError{Code: "gateway/internal", Message: err.Error()}
		}
		return map[string]any{"ok": true, "value": item}, nil
	case "messageFeedback/delete":
		existing := items[messageID]
		if existing == nil {
			return map[string]any{"ok": true, "value": map[string]any{}}, nil
		}
		ifVersion, _ := request["ifVersion"].(string)
		if ifVersion != existing.Version {
			return feedbackFailure("version-conflict", existing), nil
		}
		delete(items, messageID)
		if err := s.feedback.persistLocked(); err != nil {
			return nil, &RPCError{Code: "gateway/internal", Message: err.Error()}
		}
		return map[string]any{"ok": true, "value": map[string]any{}}, nil
	default:
		return nil, &RPCError{Code: "gateway/method-not-found", Message: "unknown feedback method"}
	}
}

func validAssistantMessage(rec *SessionRecord, messageID string) bool {
	var seq int
	if _, err := fmt.Sscanf(messageID, "assistant-%d", &seq); err != nil || seq <= 0 {
		return false
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	for _, event := range rec.Events {
		if event["seq"] == seq && event["type"] == "assistant/message" {
			return true
		}
	}
	return false
}

func feedbackFailure(code string, current *feedbackItem) map[string]any {
	errorValue := map[string]any{"code": code}
	if current != nil {
		copy := *current
		errorValue["current"] = &copy
	}
	return map[string]any{"ok": false, "error": errorValue}
}

func (s *feedbackStore) persistLocked() error {
	raw, err := json.MarshalIndent(s.items, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".feedback-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, s.path)
}

func (s *feedbackStore) removeSession(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.items[sessionID] == nil {
		return
	}
	delete(s.items, sessionID)
	_ = s.persistLocked()
}
