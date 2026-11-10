package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"time"

	"github.com/JopenChen/dsh-go/pkg/nativefs"
)

func (s *Server) dispatchWorkspaceRPC(endpoint string, args map[string]any) (any, *RPCError) {
	sessionID, _ := args["workspaceFileScopeId"].(string)
	rec := s.sessionByID(sessionID)
	if rec == nil {
		return nil, &RPCError{Code: "session/not-found", Message: "session not found"}
	}
	rec.mu.Lock()
	cwd := rec.Cwd
	rec.mu.Unlock()
	path, _ := args["path"].(string)
	if path == "" {
		path = "."
	}

	switch endpoint {
	case "workspaceFiles/list":
		listing, err := s.files.List(cwd, path, nativefs.DefaultMaxEntries)
		if err != nil {
			return nil, workspaceRPCError(err)
		}
		return listing, nil
	case "workspaceFiles/stat":
		stat, err := s.files.Stat(cwd, path)
		if err != nil {
			return nil, workspaceRPCError(err)
		}
		return stat, nil
	case "workspaceFiles/read":
		rangeArgs, _ := args["range"].(map[string]any)
		offset := numberArg(rangeArgs, "offset", 1)
		limit := numberArg(rangeArgs, "limit", 500)
		page, err := s.files.ReadText(cwd, path, offset, limit, nativefs.DefaultMaxTextBytes)
		if err != nil {
			return nil, workspaceRPCError(err)
		}
		return page, nil
	case "workspaceFiles/readBytes":
		options, _ := args["options"].(map[string]any)
		rangeArgs, _ := options["range"].(map[string]any)
		offset := int64(numberArg(rangeArgs, "offset", 0))
		length := numberArg(rangeArgs, "length", 1<<20)
		window, err := s.files.ReadWindow(cwd, path, offset, length)
		if err != nil {
			return nil, workspaceRPCError(err)
		}
		return map[string]any{
			"data": base64.StdEncoding.EncodeToString(window.Data), "eof": window.EOF,
			"offset": window.Offset, "absolutePath": window.AbsolutePath,
			"version": window.Version, "bytes": window.Bytes,
		}, nil
	default:
		return nil, &RPCError{Code: "gateway/method-not-found", Message: "unknown workspace method"}
	}
}

func workspaceRPCError(err error) *RPCError {
	code := string(nativefs.Code(err))
	if code == "" {
		code = string(nativefs.ErrIO)
	}
	return &RPCError{Code: code, Message: err.Error()}
}

func numberArg(values map[string]any, key string, fallback int) int {
	if values == nil {
		return fallback
	}
	switch value := values[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	case json.Number:
		parsed, err := value.Int64()
		if err == nil {
			return int(parsed)
		}
	}
	return fallback
}

func (s *Server) handleFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.isCookieAuthorized(r) {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	path := r.URL.Query().Get("path")
	data, _, err := s.files.ReadWhole(s.workspaceDir, path, nativefs.DefaultMaxWholeBytes)
	if err != nil {
		status := http.StatusInternalServerError
		switch nativefs.Code(err) {
		case nativefs.ErrNotFound:
			status = http.StatusNotFound
		case nativefs.ErrNotAllowed:
			status = http.StatusForbidden
		case nativefs.ErrTooLarge:
			status = http.StatusRequestEntityTooLarge
		case nativefs.ErrInvalid, nativefs.ErrNotFile:
			status = http.StatusBadRequest
		}
		http.Error(w, err.Error(), status)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (s *Server) watchWorkspace(ctx context.Context, sessionID, path string, send func(map[string]any) error) error {
	rec := s.sessionByID(sessionID)
	if rec == nil {
		return errors.New("session not found")
	}
	rec.mu.Lock()
	cwd := rec.Cwd
	rec.mu.Unlock()
	if path == "" {
		path = "."
	}
	resolved, err := s.files.Resolve(cwd, path)
	if err != nil {
		return err
	}
	version, present, err := s.files.Signature(cwd, path)
	if err != nil {
		return err
	}
	if !present {
		return &nativefs.Error{Code: nativefs.ErrNotFound, Path: path}
	}
	if err := send(map[string]any{"kind": "ready"}); err != nil {
		return err
	}
	ticker := time.NewTicker(750 * time.Millisecond)
	defer ticker.Stop()
	lastVersion := version
	lastPresent := present
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			version, present, signatureErr := s.files.Signature(cwd, path)
			if signatureErr != nil && nativefs.Code(signatureErr) != nativefs.ErrNotFound {
				return signatureErr
			}
			if version == lastVersion && present == lastPresent {
				continue
			}
			change := map[string]any{"absolutePath": resolved, "absent": !present}
			if present {
				change["version"] = version
			}
			if err := send(map[string]any{"kind": "change", "change": change}); err != nil {
				return err
			}
			lastVersion, lastPresent = version, present
		}
	}
}

func workspaceRelative(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(rel)
}
