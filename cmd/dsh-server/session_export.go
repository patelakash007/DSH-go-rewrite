package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/JopenChen/dsh-go/pkg/session"
)

func (s *Server) handleSessionExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.isCookieAuthorized(r) {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	sessionID := r.URL.Query().Get("sessionId")
	rec := s.sessionByID(sessionID)
	if rec == nil {
		http.Error(w, `{"error":"session not found"}`, http.StatusNotFound)
		return
	}
	includeDescendants := r.URL.Query().Get("includeDescendants") == "true"
	records := []*SessionRecord{rec}
	if includeDescendants {
		records = append(records, s.descendantSessions(sessionID)...)
	}
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	for _, record := range records {
		if err := writeSessionArchive(archive, record, s.uploads); err != nil {
			_ = archive.Close()
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	if err := archive.Close(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	filename := "dsh-session-" + sanitizeArchiveName(sessionID) + ".zip"
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", buffer.Len()))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buffer.Bytes())
}

func (s *Server) descendantSessions(parentID string) []*SessionRecord {
	s.sessionsMu.RLock()
	all := make([]*SessionRecord, 0, len(s.sessions))
	for _, rec := range s.sessions {
		all = append(all, rec)
	}
	s.sessionsMu.RUnlock()
	result := make([]*SessionRecord, 0)
	seen := map[string]bool{parentID: true}
	changed := true
	for changed {
		changed = false
		for _, rec := range all {
			if seen[rec.ID.Raw()] {
				continue
			}
			for _, event := range rec.Log.Events() {
				seed, ok := event.Data.(session.EndSeedData)
				if !ok || seed.ParentSession.Raw() == "" {
					continue
				}
				if seen[seed.ParentSession.Raw()] {
					seen[rec.ID.Raw()] = true
					result = append(result, rec)
					changed = true
					break
				}
			}
		}
	}
	return result
}

func writeSessionArchive(archive *zip.Writer, rec *SessionRecord, uploads *uploadStore) error {
	rec.mu.Lock()
	title := rec.Title
	cwd := rec.Cwd
	model := rec.Model
	provider := rec.Provider
	createdAt := rec.CreatedAt
	updatedAt := rec.UpdatedAt
	events := make([]map[string]any, len(rec.Events))
	copy(events, rec.Events)
	rec.mu.Unlock()
	payload := map[string]any{
		"sessionId": rec.ID.Raw(), "title": title, "cwd": cwd,
		"model": model, "provider": provider,
		"createdAt": createdAt.UTC().Format("2006-01-02T15:04:05.000Z07:00"),
		"updatedAt": updatedAt.UTC().Format("2006-01-02T15:04:05.000Z07:00"),
		"events":    events,
	}
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	entry, err := archive.Create("sessions/" + sanitizeArchiveName(rec.ID.Raw()) + "/session.json")
	if err != nil {
		return err
	}
	if _, err := entry.Write(raw); err != nil {
		return err
	}
	if uploads == nil {
		return nil
	}
	uploads.mu.RLock()
	defer uploads.mu.RUnlock()
	for _, upload := range uploads.byID {
		if upload.SessionID != rec.ID.Raw() {
			continue
		}
		absolute := filepath.Join(uploads.root, filepath.FromSlash(upload.RelativePath))
		data, err := readFileBounded(absolute, maxUploadBytes)
		if err != nil {
			continue
		}
		entry, err := archive.Create("sessions/" + sanitizeArchiveName(rec.ID.Raw()) + "/attachments/" + upload.Name)
		if err != nil {
			return err
		}
		if _, err := entry.Write(data); err != nil {
			return err
		}
	}
	return nil
}

func readFileBounded(path string, limit int64) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Size() > limit {
		return nil, fmt.Errorf("file exceeds export limit")
	}
	return os.ReadFile(path)
}

func sanitizeArchiveName(value string) string {
	var builder strings.Builder
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || char == '-' || char == '_' {
			builder.WriteRune(char)
		} else {
			builder.WriteByte('_')
		}
	}
	if builder.Len() == 0 {
		return "session"
	}
	return builder.String()
}
