package main

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func (s *Server) dispatchReferences(endpoint string, args map[string]any) (any, *RPCError) {
	sessionID, _ := args["agentId"].(string)
	rec := s.sessionByID(sessionID)
	if rec == nil {
		return nil, &RPCError{Code: "session/not-found", Message: "session not found"}
	}
	query, _ := args["query"].(string)
	if endpoint == "fileReferences/list" {
		return fileReferenceCandidates(rec.Cwd, query), nil
	}
	return s.sessionReferenceCandidates(rec, query), nil
}

func fileReferenceCandidates(root, query string) []map[string]any {
	query = strings.TrimSpace(filepath.ToSlash(query))
	lowerQuery := strings.ToLower(query)
	result := make([]map[string]any, 0)
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if path == root {
			return nil
		}
		if entry.IsDir() && (entry.Name() == ".dsh" || entry.Name() == ".git" || entry.Name() == "node_modules") {
			return filepath.SkipDir
		}
		relative, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil
		}
		relative = filepath.ToSlash(relative)
		if lowerQuery != "" && !strings.Contains(strings.ToLower(relative), lowerQuery) {
			return nil
		}
		kind := "file"
		if entry.IsDir() {
			kind = "directory"
		}
		result = append(result, map[string]any{"path": relative, "kind": kind})
		if len(result) >= 500 {
			return filepath.SkipAll
		}
		return nil
	})
	sort.SliceStable(result, func(i, j int) bool {
		left := result[i]["path"].(string)
		right := result[j]["path"].(string)
		leftBase := strings.ToLower(filepath.Base(left))
		rightBase := strings.ToLower(filepath.Base(right))
		leftPrefix := lowerQuery != "" && strings.HasPrefix(leftBase, lowerQuery)
		rightPrefix := lowerQuery != "" && strings.HasPrefix(rightBase, lowerQuery)
		if leftPrefix != rightPrefix {
			return leftPrefix
		}
		return left < right
	})
	if len(result) > 100 {
		result = result[:100]
	}
	return result
}

func (s *Server) sessionReferenceCandidates(source *SessionRecord, query string) []map[string]any {
	query = strings.ToLower(strings.TrimSpace(query))
	s.sessionsMu.RLock()
	records := make([]*SessionRecord, 0, len(s.sessions))
	for _, rec := range s.sessions {
		if rec.ID.Raw() != source.ID.Raw() {
			records = append(records, rec)
		}
	}
	s.sessionsMu.RUnlock()
	type candidate struct {
		row       map[string]any
		same      bool
		createdAt int64
	}
	items := make([]candidate, 0, len(records))
	for _, rec := range records {
		rec.mu.Lock()
		title := rec.Title
		if title == "" {
			title = rec.ID.Raw()
		}
		cwd := rec.Cwd
		createdAt := rec.CreatedAt.UnixMilli()
		rec.mu.Unlock()
		if query != "" && !strings.Contains(strings.ToLower(rec.ID.Raw()+" "+title+" "+cwd), query) {
			continue
		}
		same := cwd == source.Cwd
		items = append(items, candidate{
			row: map[string]any{
				"sessionId": rec.ID.Raw(), "label": title,
				"mention":       sessionMention(rec.ID.Raw(), title),
				"sameWorkspace": same, "createdAt": createdAt, "cwd": cwd,
			},
			same: same, createdAt: createdAt,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].same != items[j].same {
			return items[i].same
		}
		return items[i].createdAt > items[j].createdAt
	})
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		result = append(result, item.row)
	}
	if len(result) > 100 {
		result = result[:100]
	}
	return result
}

func sessionMention(sessionID, label string) string {
	raw, _ := json.Marshal(sessionID)
	encoded := base64.RawURLEncoding.EncodeToString(raw)
	label = strings.NewReplacer(`\`, `\\`, `]`, `\]`).Replace(label)
	return "@[" + label + "](dsh-session:" + encoded + ")"
}
