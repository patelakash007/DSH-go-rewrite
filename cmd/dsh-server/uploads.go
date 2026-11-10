package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/JopenChen/dsh-go/pkg/uuid"
)

const maxUploadBytes = 30 << 20

var uploadNameUnsafe = regexp.MustCompile(`[^A-Za-z0-9._ -]+`)

type uploadedFile struct {
	ReceiptID    string `json:"receiptId"`
	AttachmentID string `json:"attachmentId"`
	SessionID    string `json:"sessionId"`
	Name         string `json:"name"`
	RelativePath string `json:"relativePath"`
	Bytes        int64  `json:"bytes"`
	MediaType    string `json:"mediaType"`
	CreatedAt    int64  `json:"createdAt"`
}

type uploadStore struct {
	root string
	dir  string
	mu   sync.RWMutex
	byID map[string]*uploadedFile
}

func newUploadStore(workspaceRoot string) (*uploadStore, error) {
	dir := filepath.Join(workspaceRoot, ".dsh", "uploads")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	store := &uploadStore{root: workspaceRoot, dir: dir, byID: map[string]*uploadedFile{}}
	entries, _ := os.ReadDir(dir)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		var upload uploadedFile
		if json.Unmarshal(raw, &upload) != nil || upload.ReceiptID == "" || upload.SessionID == "" {
			continue
		}
		absolute := filepath.Join(workspaceRoot, filepath.FromSlash(upload.RelativePath))
		if info, err := os.Stat(absolute); err == nil && info.Mode().IsRegular() && info.Size() == upload.Bytes {
			copy := upload
			store.byID[upload.ReceiptID] = &copy
		}
	}
	return store, nil
}

func (s *uploadStore) get(id string) *uploadedFile {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value := s.byID[id]
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func (s *uploadStore) put(sessionID, name string, reader io.Reader) (*uploadedFile, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("sessionId is required")
	}
	name = safeUploadName(name)
	receiptID := uuid.MustV4()
	attachmentID := uuid.MustV4()
	relative := filepath.ToSlash(filepath.Join(".dsh", "uploads", receiptID+"-"+name))
	absolute := filepath.Join(s.root, filepath.FromSlash(relative))
	tmp, err := os.CreateTemp(s.dir, ".upload-*")
	if err != nil {
		return nil, err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	written, copyErr := io.Copy(tmp, io.LimitReader(reader, maxUploadBytes+1))
	if copyErr != nil {
		_ = tmp.Close()
		return nil, copyErr
	}
	if written > maxUploadBytes {
		_ = tmp.Close()
		return nil, fmt.Errorf("upload exceeds %d bytes", maxUploadBytes)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return nil, err
	}
	if err := tmp.Close(); err != nil {
		return nil, err
	}
	if err := os.Rename(tmpName, absolute); err != nil {
		return nil, err
	}
	upload := &uploadedFile{
		ReceiptID: receiptID, AttachmentID: attachmentID, SessionID: sessionID,
		Name: name, RelativePath: relative, Bytes: written,
		MediaType: detectUploadType(absolute, name), CreatedAt: time.Now().UnixMilli(),
	}
	metadata, _ := json.MarshalIndent(upload, "", "  ")
	metadata = append(metadata, '\n')
	if err := atomicUploadMetadata(filepath.Join(s.dir, receiptID+".json"), metadata); err != nil {
		_ = os.Remove(absolute)
		return nil, err
	}
	s.mu.Lock()
	s.byID[receiptID] = upload
	s.mu.Unlock()
	copy := *upload
	return &copy, nil
}

func (s *uploadStore) cloneSession(sourceSessionID, targetSessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	clones := make([]*uploadedFile, 0)
	for _, upload := range s.byID {
		if upload.SessionID != sourceSessionID {
			continue
		}
		copy := *upload
		copy.ReceiptID = uuid.MustV4()
		copy.AttachmentID = uuid.MustV4()
		copy.SessionID = targetSessionID
		copy.CreatedAt = time.Now().UnixMilli()
		clones = append(clones, &copy)
	}
	for _, clone := range clones {
		metadata, _ := json.MarshalIndent(clone, "", "  ")
		metadata = append(metadata, '\n')
		if err := atomicUploadMetadata(filepath.Join(s.dir, clone.ReceiptID+".json"), metadata); err != nil {
			return err
		}
		s.byID[clone.ReceiptID] = clone
	}
	return nil
}

func (s *uploadStore) findAttachment(sessionID, attachmentID string) *uploadedFile {
	s.mu.RLock()
	defer s.mu.RUnlock()
	keys := make([]string, 0, len(s.byID))
	for key := range s.byID {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		upload := s.byID[key]
		if upload.SessionID == sessionID && upload.AttachmentID == attachmentID {
			copy := *upload
			return &copy
		}
	}
	return nil
}

func (s *Server) handleUploadBinary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.isCookieAuthorized(r) {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	sessionID := r.URL.Query().Get("sessionId")
	if s.sessionByID(sessionID) == nil {
		writeUploadHTTPError(w, http.StatusNotFound, "session/not-found", "session not found")
		return
	}
	if r.ContentLength > maxUploadBytes {
		writeUploadHTTPError(w, http.StatusRequestEntityTooLarge, "file-upload/too-large", "upload is too large")
		return
	}
	upload, err := s.uploads.put(sessionID, r.URL.Query().Get("name"), r.Body)
	if err != nil {
		status := http.StatusInternalServerError
		code := "file-upload/failed"
		if strings.Contains(err.Error(), "exceeds") {
			status = http.StatusRequestEntityTooLarge
			code = "file-upload/too-large"
		}
		writeUploadHTTPError(w, status, code, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "value": uploadReceipt(upload)})
}

func (s *Server) dispatchInlineUpload(args map[string]any) (any, *RPCError) {
	sessionID, _ := args["agentId"].(string)
	if s.sessionByID(sessionID) == nil {
		return nil, &RPCError{Code: "session/not-found", Message: "session not found"}
	}
	request, _ := args["request"].(map[string]any)
	encoded, _ := request["data"].(string)
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, &RPCError{Code: "gateway/arguments-invalid", Message: "upload data is not valid base64"}
	}
	if len(data) > maxUploadBytes {
		return nil, &RPCError{Code: "file-upload/too-large", Message: "upload is too large"}
	}
	name, _ := request["name"].(string)
	upload, err := s.uploads.put(sessionID, name, bytes.NewReader(data))
	if err != nil {
		return nil, &RPCError{Code: "file-upload/failed", Message: err.Error()}
	}
	return uploadReceipt(upload), nil
}

func (s *Server) dispatchAttachment(args map[string]any) (any, *RPCError) {
	request, _ := args["request"].(map[string]any)
	sessionID, _ := request["sessionId"].(string)
	attachmentID, _ := request["attachmentId"].(string)
	if attachment, ok := request["attachment"].(map[string]any); ok {
		if fullID, _ := attachment["attachmentId"].(string); fullID != "" {
			attachmentID = fullID
		}
	}
	upload := s.uploads.findAttachment(sessionID, attachmentID)
	if upload == nil {
		return nil, &RPCError{Code: "session/attachment-invalid", Message: "attachment not found"}
	}
	absolute := filepath.Join(s.uploads.root, filepath.FromSlash(upload.RelativePath))
	data, err := os.ReadFile(absolute)
	if err != nil || int64(len(data)) != upload.Bytes {
		return nil, &RPCError{Code: "session/attachment-invalid", Message: "attachment data is unavailable"}
	}
	return map[string]any{"attachment": uploadAttachment(upload), "data": base64.StdEncoding.EncodeToString(data)}, nil
}

func uploadReceipt(upload *uploadedFile) map[string]any {
	return map[string]any{"receiptId": upload.ReceiptID, "file": uploadAttachment(upload)}
}

func uploadAttachment(upload *uploadedFile) map[string]any {
	return map[string]any{
		"attachmentId": upload.AttachmentID, "name": upload.Name,
		"bytes": upload.Bytes, "mediaType": upload.MediaType,
	}
}

func writeUploadHTTPError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok": false, "error": map[string]any{"code": code, "message": message},
	})
}

func safeUploadName(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	name = uploadNameUnsafe.ReplaceAllString(name, "_")
	name = strings.Trim(name, " .")
	if name == "" {
		name = "attachment.bin"
	}
	runes := []rune(name)
	if len(runes) > 120 {
		name = string(runes[:120])
	}
	return name
}

func detectUploadType(path, name string) string {
	if byExtension := mime.TypeByExtension(strings.ToLower(filepath.Ext(name))); byExtension != "" {
		return strings.Split(byExtension, ";")[0]
	}
	file, err := os.Open(path)
	if err != nil {
		return "application/octet-stream"
	}
	defer file.Close()
	header := make([]byte, 512)
	read, _ := file.Read(header)
	return http.DetectContentType(header[:read])
}

func atomicUploadMetadata(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".metadata-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err := tmp.Write(data); err != nil {
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
	return os.Rename(name, path)
}
