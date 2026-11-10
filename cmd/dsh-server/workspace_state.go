package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/JopenChen/dsh-go/pkg/uuid"
)

type workspaceRecord struct {
	WorkspaceID string   `json:"workspaceId"`
	Path        string   `json:"path"`
	Title       string   `json:"title"`
	SessionIDs  []string `json:"sessionIds"`
	CreatedAt   string   `json:"createdAt"`
	UpdatedAt   string   `json:"updatedAt"`
}

type persistedWorkspaceState struct {
	Workspaces       []*workspaceRecord `json:"workspaces"`
	Order            []string           `json:"order"`
	ArchivedSessions []string           `json:"archivedSessionIds"`
	PinnedSessions   []string           `json:"pinnedSessionIds"`
}

func (s *Server) loadWorkspaceState() error {
	raw, err := os.ReadFile(s.workspaceStatePath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	var state persistedWorkspaceState
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &state); err != nil {
			return fmt.Errorf("decode workspace state: %w", err)
		}
	}
	s.workspaceMu.Lock()
	defer s.workspaceMu.Unlock()
	for _, workspace := range state.Workspaces {
		if workspace == nil || workspace.WorkspaceID == "" {
			continue
		}
		resolved, resolveErr := s.files.Resolve(s.workspaceDir, workspace.Path)
		if resolveErr != nil {
			continue
		}
		if info, statErr := os.Stat(resolved); statErr != nil || !info.IsDir() {
			continue
		}
		copy := *workspace
		copy.Path = resolved
		copy.SessionIDs = uniqueStrings(copy.SessionIDs)
		s.workspaces[copy.WorkspaceID] = &copy
	}
	for _, id := range state.Order {
		if s.workspaces[id] != nil && !containsString(s.workspaceOrder, id) {
			s.workspaceOrder = append(s.workspaceOrder, id)
		}
	}
	for id := range s.workspaces {
		if !containsString(s.workspaceOrder, id) {
			s.workspaceOrder = append(s.workspaceOrder, id)
		}
	}
	if len(s.workspaces) == 0 {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		workspace := &workspaceRecord{
			WorkspaceID: "native-default", Path: s.workspaceDir, Title: "Workspace",
			CreatedAt: now, UpdatedAt: now,
		}
		s.workspaces[workspace.WorkspaceID] = workspace
		s.workspaceOrder = []string{workspace.WorkspaceID}
	}
	for _, id := range state.ArchivedSessions {
		s.archived[id] = true
	}
	s.pinnedSessions = uniqueStrings(state.PinnedSessions)
	return s.persistWorkspaceStateLocked()
}

func (s *Server) persistWorkspaceStateLocked() error {
	state := persistedWorkspaceState{
		Order:          append([]string(nil), s.workspaceOrder...),
		PinnedSessions: append([]string(nil), s.pinnedSessions...),
	}
	for _, id := range s.workspaceOrder {
		if workspace := s.workspaces[id]; workspace != nil {
			copy := *workspace
			copy.SessionIDs = append([]string(nil), workspace.SessionIDs...)
			state.Workspaces = append(state.Workspaces, &copy)
		}
	}
	for id, archived := range s.archived {
		if archived {
			state.ArchivedSessions = append(state.ArchivedSessions, id)
		}
	}
	sort.Strings(state.ArchivedSessions)
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(s.workspaceStatePath), ".workspace-state-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
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
	return os.Rename(tmpName, s.workspaceStatePath)
}

func (s *Server) workspaceByID(id string) *workspaceRecord {
	s.workspaceMu.Lock()
	defer s.workspaceMu.Unlock()
	workspace := s.workspaces[id]
	if workspace == nil {
		return nil
	}
	copy := *workspace
	copy.SessionIDs = append([]string(nil), workspace.SessionIDs...)
	return &copy
}

func (s *Server) attachSessionToWorkspace(workspaceID, sessionID string) {
	s.workspaceMu.Lock()
	workspace := s.workspaces[workspaceID]
	if workspace == nil {
		s.workspaceMu.Unlock()
		return
	}
	if !containsString(workspace.SessionIDs, sessionID) {
		workspace.SessionIDs = append(workspace.SessionIDs, sessionID)
		workspace.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		_ = s.persistWorkspaceStateLocked()
	}
	frame := map[string]any{"type": "upsert", "workspace": workspaceWire(workspace)}
	s.broadcastWorkspaceLocked(frame)
	s.workspaceMu.Unlock()
}

func (s *Server) removeSessionFromWorkspaceState(sessionID string) {
	s.workspaceMu.Lock()
	delete(s.archived, sessionID)
	s.pinnedSessions = removeString(s.pinnedSessions, sessionID)
	for _, workspace := range s.workspaces {
		workspace.SessionIDs = removeString(workspace.SessionIDs, sessionID)
	}
	_ = s.persistWorkspaceStateLocked()
	s.broadcastWorkspaceLocked(s.workspaceBaselineLocked())
	s.workspaceMu.Unlock()
}

func (s *Server) dispatchWorkspaceStateRPC(endpoint string, args map[string]any) (any, *RPCError) {
	switch endpoint {
	case "directoryPicker/list":
		path, _ := args["path"].(string)
		return s.listDirectories(path)
	case "directoryPicker/createDirectory":
		parent, _ := args["path"].(string)
		name, _ := args["name"].(string)
		if name == "" || name == "." || name == ".." || filepath.Base(name) != name || strings.ContainsAny(name, `/\\`) {
			return nil, &RPCError{Code: "directory-picker/create-failed", Message: "invalid directory name"}
		}
		resolvedParent, err := s.files.Resolve(s.workspaceDir, parent)
		if err != nil {
			return nil, &RPCError{Code: "directory-picker/unreadable", Message: err.Error()}
		}
		target := filepath.Join(resolvedParent, name)
		if _, err := os.Stat(target); err == nil {
			return nil, &RPCError{Code: "directory-picker/exists", Message: "directory already exists"}
		}
		if err := os.Mkdir(target, 0o700); err != nil {
			return nil, &RPCError{Code: "directory-picker/create-failed", Message: err.Error()}
		}
		return target, nil
	}

	request, _ := args["request"].(map[string]any)
	switch endpoint {
	case "workspace/create":
		path, _ := request["path"].(string)
		resolved, err := s.files.Resolve(s.workspaceDir, path)
		if err != nil {
			return nil, &RPCError{Code: "workspace/invalid-path", Message: err.Error()}
		}
		if err := os.MkdirAll(resolved, 0o700); err != nil {
			return nil, &RPCError{Code: "workspace/invalid-path", Message: err.Error()}
		}
		s.workspaceMu.Lock()
		for _, existing := range s.workspaces {
			if existing.Path == resolved {
				wire := workspaceWire(existing)
				s.workspaceMu.Unlock()
				return map[string]any{"workspace": wire, "created": false}, nil
			}
		}
		title := filepath.Base(resolved)
		if resolved == s.workspaceDir {
			title = "Workspace"
		}
		title = s.uniqueWorkspaceTitleLocked(title, "")
		now := time.Now().UTC().Format(time.RFC3339Nano)
		workspace := &workspaceRecord{WorkspaceID: uuid.MustV4(), Path: resolved, Title: title, CreatedAt: now, UpdatedAt: now}
		s.workspaces[workspace.WorkspaceID] = workspace
		s.workspaceOrder = append([]string{workspace.WorkspaceID}, s.workspaceOrder...)
		_ = s.persistWorkspaceStateLocked()
		s.broadcastWorkspaceLocked(map[string]any{"type": "upsert", "workspace": workspaceWire(workspace)})
		s.broadcastWorkspaceLocked(map[string]any{"type": "order", "workspaceIds": append([]string(nil), s.workspaceOrder...)})
		wire := workspaceWire(workspace)
		s.workspaceMu.Unlock()
		return map[string]any{"workspace": wire, "created": true}, nil
	case "workspace/rename":
		id, _ := request["workspaceId"].(string)
		title, _ := request["title"].(string)
		title = strings.TrimSpace(title)
		if title == "" {
			return nil, &RPCError{Code: "gateway/arguments-invalid", Message: "workspace title is empty"}
		}
		s.workspaceMu.Lock()
		workspace := s.workspaces[id]
		if workspace == nil {
			s.workspaceMu.Unlock()
			return nil, &RPCError{Code: "workspace/not-found", Message: "workspace not found"}
		}
		for otherID, other := range s.workspaces {
			if otherID != id && strings.EqualFold(other.Title, title) {
				s.workspaceMu.Unlock()
				return nil, &RPCError{Code: "workspace/name-conflict", Message: "workspace title already exists"}
			}
		}
		workspace.Title = title
		workspace.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		_ = s.persistWorkspaceStateLocked()
		s.broadcastWorkspaceLocked(map[string]any{"type": "upsert", "workspace": workspaceWire(workspace)})
		wire := workspaceWire(workspace)
		s.workspaceMu.Unlock()
		return map[string]any{"workspace": wire}, nil
	case "workspace/delete":
		id, _ := request["workspaceId"].(string)
		s.workspaceMu.Lock()
		if s.workspaces[id] == nil {
			s.workspaceMu.Unlock()
			return nil, &RPCError{Code: "workspace/not-found", Message: "workspace not found"}
		}
		delete(s.workspaces, id)
		s.workspaceOrder = removeString(s.workspaceOrder, id)
		_ = s.persistWorkspaceStateLocked()
		s.broadcastWorkspaceLocked(map[string]any{"type": "remove", "workspaceId": id})
		s.broadcastWorkspaceLocked(map[string]any{"type": "order", "workspaceIds": append([]string(nil), s.workspaceOrder...)})
		s.workspaceMu.Unlock()
		return map[string]any{}, nil
	case "workspace/insertBefore":
		id, _ := request["workspaceId"].(string)
		before, _ := request["beforeWorkspaceId"].(string)
		s.workspaceMu.Lock()
		if s.workspaces[id] == nil || (before != "" && s.workspaces[before] == nil) {
			s.workspaceMu.Unlock()
			return nil, &RPCError{Code: "workspace/not-found", Message: "workspace not found"}
		}
		order := removeString(s.workspaceOrder, id)
		index := len(order)
		if before != "" {
			for i, candidate := range order {
				if candidate == before {
					index = i
					break
				}
			}
		}
		order = append(order, "")
		copy(order[index+1:], order[index:])
		order[index] = id
		s.workspaceOrder = order
		_ = s.persistWorkspaceStateLocked()
		s.broadcastWorkspaceLocked(map[string]any{"type": "order", "workspaceIds": append([]string(nil), order...)})
		s.workspaceMu.Unlock()
		return map[string]any{"workspaceIds": order}, nil
	case "workspace/pinSession", "workspace/unpinSession":
		sessionID, _ := request["sessionId"].(string)
		if s.sessionByID(sessionID) == nil {
			return nil, &RPCError{Code: "session/not-found", Message: "session not found"}
		}
		s.workspaceMu.Lock()
		if endpoint == "workspace/pinSession" {
			if s.archived[sessionID] {
				s.workspaceMu.Unlock()
				return nil, &RPCError{Code: "workspace/archived", Message: "archived sessions cannot be pinned"}
			}
			s.pinnedSessions = append([]string{sessionID}, removeString(s.pinnedSessions, sessionID)...)
		} else {
			s.pinnedSessions = removeString(s.pinnedSessions, sessionID)
		}
		_ = s.persistWorkspaceStateLocked()
		pinned := append([]string(nil), s.pinnedSessions...)
		s.broadcastWorkspaceLocked(map[string]any{"type": "pinned", "pinnedSessionIds": pinned})
		s.workspaceMu.Unlock()
		return map[string]any{"pinnedSessionIds": pinned}, nil
	default:
		return nil, &RPCError{Code: "gateway/method-not-found", Message: "unknown workspace method"}
	}
}

func (s *Server) listDirectories(path string) (any, *RPCError) {
	if path == "" {
		path = s.workspaceDir
	}
	if !filepath.IsAbs(path) {
		return nil, &RPCError{Code: "directory-picker/unreadable", Message: "directory path must be absolute"}
	}
	resolved, err := s.files.Resolve(s.workspaceDir, path)
	if err != nil {
		return nil, &RPCError{Code: "directory-picker/unreadable", Message: err.Error()}
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return nil, &RPCError{Code: "directory-picker/unreadable", Message: "directory is not readable"}
	}
	entries, err := os.ReadDir(resolved)
	if err != nil {
		return nil, &RPCError{Code: "directory-picker/unreadable", Message: err.Error()}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	rows := make([]map[string]any, 0)
	truncated := false
	for _, entry := range entries {
		child := filepath.Join(resolved, entry.Name())
		childInfo, statErr := os.Stat(child)
		if statErr != nil || !childInfo.IsDir() {
			continue
		}
		if len(rows) >= 1000 {
			truncated = true
			break
		}
		rows = append(rows, map[string]any{"name": entry.Name(), "path": child, "hidden": strings.HasPrefix(entry.Name(), ".")})
	}
	crumbs := []map[string]any{{"name": filepath.Base(s.workspaceDir), "path": s.workspaceDir, "hidden": false}}
	if resolved != s.workspaceDir {
		rel, _ := filepath.Rel(s.workspaceDir, resolved)
		current := s.workspaceDir
		for _, segment := range strings.Split(filepath.ToSlash(rel), "/") {
			current = filepath.Join(current, segment)
			crumbs = append(crumbs, map[string]any{"name": segment, "path": current, "hidden": strings.HasPrefix(segment, ".")})
		}
	}
	return map[string]any{
		"path": resolved, "home": s.workspaceDir, "crumbs": crumbs, "entries": rows, "truncated": truncated,
	}, nil
}

func (s *Server) workspaceBaselineLocked() map[string]any {
	items := make([]map[string]any, 0, len(s.workspaceOrder))
	for _, id := range s.workspaceOrder {
		if workspace := s.workspaces[id]; workspace != nil {
			items = append(items, workspaceWire(workspace))
		}
	}
	archived := make([]string, 0, len(s.archived))
	for id, active := range s.archived {
		if active {
			archived = append(archived, id)
		}
	}
	sort.Strings(archived)
	return map[string]any{
		"type": "baseline", "value": map[string]any{
			"items": items, "archivedSessionIds": archived,
			"pinnedSessionIds": append([]string(nil), s.pinnedSessions...),
		},
	}
}

func (s *Server) broadcastWorkspaceLocked(frame map[string]any) {
	for _, channel := range s.workspaceSubs {
		select {
		case channel <- frame:
		default:
		}
	}
}

func workspaceWire(workspace *workspaceRecord) map[string]any {
	return map[string]any{
		"workspaceId": workspace.WorkspaceID, "path": workspace.Path, "title": workspace.Title,
		"sessionIds": append([]string(nil), workspace.SessionIDs...),
		"createdAt":  workspace.CreatedAt, "updatedAt": workspace.UpdatedAt,
	}
}

func (s *Server) uniqueWorkspaceTitleLocked(base, exceptID string) string {
	if strings.TrimSpace(base) == "" {
		base = "Workspace"
	}
	title := base
	for suffix := 2; ; suffix++ {
		conflict := false
		for id, workspace := range s.workspaces {
			if id != exceptID && strings.EqualFold(workspace.Title, title) {
				conflict = true
				break
			}
		}
		if !conflict {
			return title
		}
		title = fmt.Sprintf("%s %d", base, suffix)
	}
}

func uniqueStrings(values []string) []string {
	result := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func removeString(values []string, target string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != target {
			result = append(result, value)
		}
	}
	return result
}
