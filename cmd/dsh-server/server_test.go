package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/JopenChen/dsh-go/pkg/brand"
	"github.com/JopenChen/dsh-go/pkg/llm"
	"github.com/JopenChen/dsh-go/pkg/session"
)

type recordingAdapter struct {
	requests chan llm.ChatRequest
}

func (a *recordingAdapter) Name() string { return "recording" }

func (a *recordingAdapter) Chat(_ context.Context, req llm.ChatRequest, cb func(llm.StreamChunk)) (llm.Usage, error) {
	a.requests <- req
	cb(llm.StreamChunk{Kind: llm.ChunkText, Text: "native reply"})
	return llm.Usage{PromptTokens: 3, CompletionTokens: 2}, nil
}

func setupTestServer(t *testing.T) (*Server, *httptest.Server, func()) {
	t.Helper()
	tempDir, err := os.MkdirTemp("", "dsh-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	server, err := newServer(0, tempDir)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", server.handleRoot)
	mux.HandleFunc("/api/remote.mux", server.handleMux)
	mux.HandleFunc("/api/file", server.handleFile)
	mux.HandleFunc("/api/session/uploadFileBinary", server.handleUploadBinary)
	mux.HandleFunc("/api/", server.handleRPC)
	mux.HandleFunc("/dsh-go/v1/info", server.handleBuildInfo)
	mux.HandleFunc("/dsh-go/v1/sessions/deletion", server.handleSessionDeletionCapability)
	mux.HandleFunc("/dsh-go/v1/sessions/delete", server.handleSessionDelete)

	ts := httptest.NewServer(mux)

	cleanup := func() {
		ts.Close()
		_ = server.dbStore.Close()
		_ = os.RemoveAll(tempDir)
	}

	return server, ts, cleanup
}

type scriptedToolAdapter struct {
	mu       sync.Mutex
	requests []llm.ChatRequest
}

func (a *scriptedToolAdapter) Name() string { return "scripted-tools" }

func (a *scriptedToolAdapter) Chat(_ context.Context, request llm.ChatRequest, callback func(llm.StreamChunk)) (llm.Usage, error) {
	a.mu.Lock()
	a.requests = append(a.requests, request)
	call := len(a.requests)
	a.mu.Unlock()
	if call == 1 {
		callback(llm.StreamChunk{Kind: llm.ChunkToolCall, ToolCall: &llm.ToolCall{
			ID: "native-call-1", Name: "shell", Input: map[string]any{"command": "printf native > agent-tool.txt"},
		}})
	} else {
		callback(llm.StreamChunk{Kind: llm.ChunkText, Text: "Native tool completed."})
	}
	callback(llm.StreamChunk{Kind: llm.ChunkDone})
	return llm.Usage{PromptTokens: 5, CompletionTokens: 3}, nil
}

func TestNativeAgentToolApprovalAndHistory(t *testing.T) {
	server, _, cleanup := setupTestServer(t)
	defer cleanup()
	adapter := &scriptedToolAdapter{}
	server.adapter = adapter
	rec := server.getOrCreateSession("native-tools", "")

	_, rpcErr := server.dispatchRPC(context.Background(), "session/prompt", map[string]any{
		"request": map[string]any{
			"sessionId": "native-tools", "requestId": "tool-request",
			"content": []any{map[string]any{"type": "text", "text": "create the marker"}},
		},
	})
	if rpcErr != nil {
		t.Fatalf("prompt: %+v", rpcErr)
	}

	var eventID string
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		server.hostMu.Lock()
		for id := range server.pendingApprovals {
			eventID = id
			break
		}
		server.hostMu.Unlock()
		if eventID != "" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if eventID == "" {
		t.Fatal("shell tool did not request approval")
	}
	server.resolveHostEvent(eventID, map[string]any{"kind": "result", "value": "allowed-once"})

	marker := filepath.Join(server.workspaceDir, "agent-tool.txt")
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(marker); err == nil && string(data) == "native" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "native" {
		t.Fatalf("native shell marker = %q, %v", data, err)
	}

	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		rec.mu.Lock()
		idle := rec.Cancel == nil
		rec.mu.Unlock()
		if idle {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	adapter.mu.Lock()
	requests := append([]llm.ChatRequest(nil), adapter.requests...)
	adapter.mu.Unlock()
	if len(requests) != 2 {
		t.Fatalf("model requests = %d, want 2", len(requests))
	}
	if len(requests[0].Tools) == 0 {
		t.Fatal("native tools were not advertised to the model")
	}
	roles := make([]llm.Role, 0, len(requests[1].Messages))
	for _, message := range requests[1].Messages {
		roles = append(roles, message.Role)
	}
	if len(roles) < 3 || roles[len(roles)-2] != llm.RoleAssistant || roles[len(roles)-1] != llm.RoleTool {
		t.Fatalf("second request lost tool protocol: %v", roles)
	}

	seen := map[string]bool{}
	rec.mu.Lock()
	for _, event := range rec.Events {
		seen[event["type"].(string)] = true
	}
	rec.mu.Unlock()
	for _, eventType := range []string{"approval/asked", "approval/decided", "tool/call", "tool/result", "assistant/message", "turn/end"} {
		if !seen[eventType] {
			t.Fatalf("missing %s event; got %v", eventType, seen)
		}
	}
}

func TestWorkspaceRPCConfinesAndReadsFiles(t *testing.T) {
	server, ts, cleanup := setupTestServer(t)
	defer cleanup()
	rec := server.getOrCreateSession("workspace-rpc", "")
	if err := os.WriteFile(filepath.Join(server.workspaceDir, "hello.txt"), []byte("one\ntwo\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	value, rpcErr := server.dispatchRPC(context.Background(), "workspaceFiles/read", map[string]any{
		"workspaceFileScopeId": rec.ID.Raw(), "path": "hello.txt",
		"range": map[string]any{"offset": 2.0, "limit": 1.0},
	})
	if rpcErr != nil {
		t.Fatalf("read: %+v", rpcErr)
	}
	encoded, _ := json.Marshal(value)
	if !strings.Contains(string(encoded), `"text":"two"`) {
		t.Fatalf("read response = %s", encoded)
	}
	if _, rpcErr := server.dispatchRPC(context.Background(), "workspaceFiles/stat", map[string]any{
		"workspaceFileScopeId": rec.ID.Raw(), "path": "../outside",
	}); rpcErr == nil || rpcErr.Code != "workspace-file/not-allowed" {
		t.Fatalf("traversal response = %+v", rpcErr)
	}

	cookie := getAuthCookie(t, server, ts)
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/file?path=hello.txt", nil)
	req.AddCookie(cookie)
	response, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("file status = %d", response.StatusCode)
	}
}

func TestNativeTerminalRPCAndDuplexMux(t *testing.T) {
	server, ts, cleanup := setupTestServer(t)
	defer cleanup()
	server.getOrCreateSession("terminal-session", "")
	created, rpcErr := server.dispatchRPC(context.Background(), "terminal/create", map[string]any{
		"agentId": "terminal-session",
		"request": map[string]any{"id": "term-1", "cols": 80.0, "rows": 24.0},
	})
	if rpcErr != nil {
		t.Fatalf("create terminal: %+v", rpcErr)
	}
	info, _ := created.(map[string]any)
	if info["state"] != "running" || info["cwd"] != server.workspaceDir {
		t.Fatalf("terminal info = %#v", info)
	}
	listed, rpcErr := server.dispatchRPC(context.Background(), "terminal/list", map[string]any{"sessionId": "terminal-session"})
	if rpcErr != nil || len(listed.([]map[string]any)) != 1 {
		t.Fatalf("terminal list = %#v, %+v", listed, rpcErr)
	}

	authCookie := getAuthCookie(t, server, ts)
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/remote.mux"
	header := make(http.Header)
	header.Add("Cookie", authCookie.String())
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	_ = ws.SetReadDeadline(time.Now().Add(5 * time.Second))
	if err := ws.WriteJSON(map[string]any{
		"type": "open", "streamId": "terminal-attach-1", "endpoint": "terminal/attach",
		"payload": map[string]any{"args": map[string]any{"agentId": "terminal-session", "id": "term-1"}},
	}); err != nil {
		t.Fatal(err)
	}
	var first map[string]any
	if err := ws.ReadJSON(&first); err != nil {
		t.Fatal(err)
	}
	firstValue, _ := first["value"].(map[string]any)
	if firstValue["type"] != "snapshot" || firstValue["writable"] != true {
		t.Fatalf("terminal first frame = %#v", first)
	}

	if err := ws.WriteJSON(map[string]any{
		"type": "item", "streamId": "terminal-attach-1",
		"value": map[string]any{"type": "write", "sequence": 1, "data": "echo NATIVE_TERMINAL_OK\n"},
	}); err != nil {
		t.Fatal(err)
	}
	seenAck, seenOutput := false, false
	for !seenAck || !seenOutput {
		var frame map[string]any
		if err := ws.ReadJSON(&frame); err != nil {
			t.Fatalf("read terminal frame: %v", err)
		}
		value, _ := frame["value"].(map[string]any)
		switch value["type"] {
		case "ack":
			if numberArg(value, "sequence", 0) == 1 && value["refusal"] == nil {
				seenAck = true
			}
		case "output":
			if text, _ := value["data"].(string); strings.Contains(text, "NATIVE_TERMINAL_OK") {
				seenOutput = true
			}
		}
	}
	if err := ws.WriteJSON(map[string]any{
		"type": "item", "streamId": "terminal-attach-1",
		"value": map[string]any{"type": "resize", "sequence": 2, "cols": 120, "rows": 40},
	}); err != nil {
		t.Fatal(err)
	}

	if _, rpcErr := server.dispatchRPC(context.Background(), "terminal/rename", map[string]any{
		"agentId": "terminal-session", "id": "term-1", "title": "Native shell",
	}); rpcErr != nil {
		t.Fatalf("rename terminal: %+v", rpcErr)
	}
	if _, rpcErr := server.dispatchRPC(context.Background(), "terminal/close", map[string]any{
		"agentId": "terminal-session", "id": "term-1",
	}); rpcErr != nil {
		t.Fatalf("close terminal: %+v", rpcErr)
	}
	if _, rpcErr := server.dispatchRPC(context.Background(), "terminal/create", map[string]any{
		"agentId": "terminal-session", "request": map[string]any{"id": "term-1", "cols": 80, "rows": 24},
	}); rpcErr == nil || rpcErr.Code != "terminal/unavailable" {
		t.Fatalf("closed id reuse = %+v", rpcErr)
	}
}

func TestNativeStreamingAndInlineUploads(t *testing.T) {
	server, ts, cleanup := setupTestServer(t)
	defer cleanup()
	server.getOrCreateSession("upload-session", "")
	cookie := getAuthCookie(t, server, ts)

	req, _ := http.NewRequest(
		http.MethodPost,
		ts.URL+"/api/session/uploadFileBinary?sessionId=upload-session&name=notes.txt",
		strings.NewReader("native upload"),
	)
	req.AddCookie(cookie)
	response, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("upload status = %d", response.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	value, _ := body["value"].(map[string]any)
	receiptID, _ := value["receiptId"].(string)
	file, _ := value["file"].(map[string]any)
	attachmentID, _ := file["attachmentId"].(string)
	if receiptID == "" || attachmentID == "" {
		t.Fatalf("upload response = %#v", body)
	}

	attachment, rpcErr := server.dispatchRPC(context.Background(), "session/attachment", map[string]any{
		"request": map[string]any{"sessionId": "upload-session", "attachmentId": attachmentID},
	})
	if rpcErr != nil {
		t.Fatalf("attachment: %+v", rpcErr)
	}
	encoded, _ := attachment.(map[string]any)["data"].(string)
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || string(decoded) != "native upload" {
		t.Fatalf("attachment data = %q, %v", decoded, err)
	}

	inline, rpcErr := server.dispatchRPC(context.Background(), "fileUploads/upload", map[string]any{
		"agentId": "upload-session",
		"request": map[string]any{"name": "inline.bin", "data": base64.StdEncoding.EncodeToString([]byte{1, 2, 3})},
	})
	if rpcErr != nil {
		t.Fatalf("inline upload: %+v", rpcErr)
	}
	if inline.(map[string]any)["receiptId"] == "" {
		t.Fatalf("inline response = %#v", inline)
	}

	// Metadata is durable: a new store over the same workspace resolves the receipt.
	reloaded, err := newUploadStore(server.workspaceDir)
	if err != nil {
		t.Fatal(err)
	}
	if upload := reloaded.get(receiptID); upload == nil || upload.AttachmentID != attachmentID {
		t.Fatalf("reloaded upload = %#v", upload)
	}
}

func TestNativeGoalLifecycleAndProjection(t *testing.T) {
	server, _, cleanup := setupTestServer(t)
	defer cleanup()
	rec := server.getOrCreateSession("goal-session", "")
	created, rpcErr := server.dispatchRPC(context.Background(), "goals/create", map[string]any{
		"agentId": "goal-session", "request": map[string]any{"objective": "Ship native DSH", "maxGoalRounds": 5},
	})
	if rpcErr != nil {
		t.Fatalf("create goal: %+v", rpcErr)
	}
	ref := created.(map[string]any)["ref"].(map[string]any)
	if ref["id"] == "" || numberArg(ref, "revision", 0) != 1 {
		t.Fatalf("goal ref = %#v", ref)
	}
	view, rpcErr := server.dispatchRPC(context.Background(), "goals/get", map[string]any{"agentId": "goal-session"})
	if rpcErr != nil || view.(map[string]any)["activation"] != "armed" {
		t.Fatalf("goal view = %#v, %+v", view, rpcErr)
	}
	if _, stale := server.dispatchRPC(context.Background(), "goals/edit", map[string]any{
		"agentId": "goal-session", "ref": map[string]any{"id": ref["id"], "revision": 0},
		"request": map[string]any{"objective": "wrong"},
	}); stale == nil || stale.Code != "GOAL_STALE_REVISION" {
		t.Fatalf("stale edit = %+v", stale)
	}
	edited, rpcErr := server.dispatchRPC(context.Background(), "goals/edit", map[string]any{
		"agentId": "goal-session", "ref": ref,
		"request": map[string]any{"objective": "Ship standalone native DSH", "maxGoalRounds": 8},
	})
	if rpcErr != nil {
		t.Fatal(rpcErr.Message)
	}
	editedView := edited.(map[string]any)
	if editedView["objective"] != "Ship standalone native DSH" || numberArg(editedView, "revision", 0) != 2 {
		t.Fatalf("edited view = %#v", editedView)
	}
	ref = map[string]any{"id": editedView["id"], "revision": editedView["revision"]}
	paused, rpcErr := server.dispatchRPC(context.Background(), "goals/pause", map[string]any{"agentId": "goal-session", "ref": ref})
	if rpcErr != nil || paused.(map[string]any)["phase"] != "paused" {
		t.Fatalf("paused = %#v, %+v", paused, rpcErr)
	}
	pausedView := paused.(map[string]any)
	ref = map[string]any{"id": pausedView["id"], "revision": pausedView["revision"]}
	resumed, rpcErr := server.dispatchRPC(context.Background(), "goals/resume", map[string]any{"agentId": "goal-session", "ref": ref})
	if rpcErr != nil || resumed.(map[string]any)["activation"] != "armed" {
		t.Fatalf("resumed = %#v, %+v", resumed, rpcErr)
	}
	resumedView := resumed.(map[string]any)
	ref = map[string]any{"id": resumedView["id"], "revision": resumedView["revision"]}
	cleared, rpcErr := server.dispatchRPC(context.Background(), "goals/clear", map[string]any{"agentId": "goal-session", "ref": ref})
	if rpcErr != nil || cleared.(map[string]any)["id"] != ref["id"] {
		t.Fatalf("cleared = %#v, %+v", cleared, rpcErr)
	}
	missing, rpcErr := server.dispatchRPC(context.Background(), "goals/get", map[string]any{"agentId": "goal-session"})
	if rpcErr != nil || missing != nil {
		t.Fatalf("goal after clear = %#v, %+v", missing, rpcErr)
	}

	projection := goalProjection(rec)
	if projection != nil {
		t.Fatalf("projection after clear = %#v", projection)
	}
	seenPut, seenClear := false, false
	rec.mu.Lock()
	for _, event := range rec.Events {
		if event["type"] != "goal/change" {
			continue
		}
		data, _ := event["data"].(map[string]any)
		seenPut = seenPut || data["operation"] == "put"
		seenClear = seenClear || data["operation"] == "clear"
	}
	rec.mu.Unlock()
	if !seenPut || !seenClear {
		t.Fatalf("goal event operations: put=%v clear=%v", seenPut, seenClear)
	}
}

func TestSessionControlBaselineAndChanges(t *testing.T) {
	server, _, cleanup := setupTestServer(t)
	defer cleanup()
	rec := server.getOrCreateSession("control-session", "")
	key, updates := server.addControlSubscriber()
	defer server.removeControlSubscriber(key)
	baseline := server.sessionControlBaseline()
	value, _ := baseline["value"].(map[string]any)
	projections, _ := value["projections"].(map[string]any)
	if projections["control-session"] == nil {
		t.Fatalf("control baseline = %#v", baseline)
	}
	_, _ = rec.Log.Append(session.SessionTitleData{Title: "Controlled", Source: "user"})
	select {
	case update := <-updates:
		if update["sessionId"] != "control-session" || update["changes"] == nil {
			t.Fatalf("control update = %#v", update)
		}
	case <-time.After(time.Second):
		t.Fatal("session/control did not publish a projection change")
	}
}

func TestNativeSessionForkPersistsClosedPrefix(t *testing.T) {
	server, _, cleanup := setupTestServer(t)
	defer cleanup()
	rec := server.getOrCreateSession("fork-source", "")
	adapter := &recordingAdapter{requests: make(chan llm.ChatRequest, 1)}
	server.adapter = adapter
	if err := server.credStore.Set(context.Background(), brand.NewCredentialRef("DEEPSEEK_API_KEY"), "test-key"); err != nil {
		t.Fatal(err)
	}
	if _, rpcErr := server.dispatchRPC(context.Background(), "session/prompt", map[string]any{
		"request": map[string]any{
			"sessionId": "fork-source", "requestId": "fork-turn",
			"content": []any{map[string]any{"type": "text", "text": "fork this session"}},
		},
	}); rpcErr != nil {
		t.Fatal(rpcErr.Message)
	}
	select {
	case <-adapter.requests:
	case <-time.After(time.Second):
		t.Fatal("source turn did not start")
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, open := rec.Log.OpenTurn(); !open && rec.Log.LastSeq() > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	forked, rpcErr := server.dispatchRPC(context.Background(), "session/fork", map[string]any{
		"request": map[string]any{"sessionId": "fork-source"},
	})
	if rpcErr != nil {
		t.Fatalf("fork: %+v", rpcErr)
	}
	childID, _ := forked.(map[string]any)["sessionId"].(string)
	child := server.sessionByID(childID)
	if child == nil || childID == "fork-source" {
		t.Fatalf("fork child = %#v", forked)
	}
	if _, open := child.Log.OpenTurn(); open {
		t.Fatal("fork restored an open turn")
	}
	if problems := child.Log.Check(); len(problems) != 0 {
		t.Fatalf("fork invariants = %v", problems)
	}
	count, err := server.dbStore.Count(context.Background(), child.ID)
	if err != nil || count != child.Log.Len() {
		t.Fatalf("fork persisted events = %d/%d, %v", count, child.Log.Len(), err)
	}
	events := child.Log.Events()
	if len(events) < 2 || events[len(events)-2].Type != session.EventEndSeed || events[len(events)-1].Type != session.EventSessionTitle {
		t.Fatalf("fork tail = %#v", events)
	}
}

func TestNativeWorkspaceDirectoryAndPinLifecycle(t *testing.T) {
	server, _, cleanup := setupTestServer(t)
	defer cleanup()
	baseline := server.workspaceBaselineLocked()
	value := baseline["value"].(map[string]any)
	if len(value["items"].([]map[string]any)) != 1 {
		t.Fatalf("default workspace baseline = %#v", baseline)
	}

	createdPath, rpcErr := server.dispatchRPC(context.Background(), "directoryPicker/createDirectory", map[string]any{
		"path": server.workspaceDir, "name": "project-one",
	})
	if rpcErr != nil {
		t.Fatalf("create directory: %+v", rpcErr)
	}
	level, rpcErr := server.dispatchRPC(context.Background(), "directoryPicker/list", map[string]any{"path": server.workspaceDir})
	if rpcErr != nil {
		t.Fatal(rpcErr.Message)
	}
	entries := level.(map[string]any)["entries"].([]map[string]any)
	foundProject := false
	for _, entry := range entries {
		foundProject = foundProject || entry["path"] == createdPath
	}
	if !foundProject {
		t.Fatalf("directory level = %#v", level)
	}

	created, rpcErr := server.dispatchRPC(context.Background(), "workspace/create", map[string]any{
		"request": map[string]any{"path": createdPath},
	})
	if rpcErr != nil {
		t.Fatalf("create workspace: %+v", rpcErr)
	}
	workspace := created.(map[string]any)["workspace"].(map[string]any)
	workspaceID := workspace["workspaceId"].(string)
	if created.(map[string]any)["created"] != true || workspaceID == "" {
		t.Fatalf("workspace create = %#v", created)
	}
	renamed, rpcErr := server.dispatchRPC(context.Background(), "workspace/rename", map[string]any{
		"request": map[string]any{"workspaceId": workspaceID, "title": "Native Project"},
	})
	if rpcErr != nil || renamed.(map[string]any)["workspace"].(map[string]any)["title"] != "Native Project" {
		t.Fatalf("workspace rename = %#v, %+v", renamed, rpcErr)
	}

	sessionValue, rpcErr := server.dispatchRPC(context.Background(), "session/create", map[string]any{
		"request": map[string]any{"workspaceId": workspaceID, "sessionId": "workspace-session"},
	})
	if rpcErr != nil || sessionValue.(map[string]any)["sessionId"] != "workspace-session" {
		t.Fatalf("workspace session = %#v, %+v", sessionValue, rpcErr)
	}
	rec := server.sessionByID("workspace-session")
	if rec.Cwd != createdPath {
		t.Fatalf("workspace cwd = %q, want %q", rec.Cwd, createdPath)
	}
	if attached := server.workspaceByID(workspaceID); attached == nil || !containsString(attached.SessionIDs, "workspace-session") {
		t.Fatalf("attached workspace = %#v", attached)
	}

	pinned, rpcErr := server.dispatchRPC(context.Background(), "workspace/pinSession", map[string]any{
		"request": map[string]any{"sessionId": "workspace-session"},
	})
	if rpcErr != nil || !containsString(pinned.(map[string]any)["pinnedSessionIds"].([]string), "workspace-session") {
		t.Fatalf("pin = %#v, %+v", pinned, rpcErr)
	}
	server.setSessionArchived("workspace-session", true)
	server.workspaceMu.Lock()
	stillPinned := containsString(server.pinnedSessions, "workspace-session")
	server.workspaceMu.Unlock()
	if stillPinned {
		t.Fatal("archiving did not clear the pin")
	}
	if _, err := os.Stat(server.workspaceStatePath); err != nil {
		t.Fatalf("workspace state not persisted: %v", err)
	}
	if _, rpcErr := server.dispatchRPC(context.Background(), "workspace/delete", map[string]any{
		"request": map[string]any{"workspaceId": workspaceID},
	}); rpcErr != nil {
		t.Fatalf("delete workspace: %+v", rpcErr)
	}
	if _, err := os.Stat(createdPath.(string)); err != nil {
		t.Fatalf("workspace delete removed folder: %v", err)
	}
}

func TestNativeSkillDiscoveryAndLoading(t *testing.T) {
	server, _, cleanup := setupTestServer(t)
	defer cleanup()
	rec := server.getOrCreateSession("skill-session", "")
	skillDir := filepath.Join(rec.Cwd, ".dsh", "skills", "native-review")
	if err := os.MkdirAll(skillDir, 0o700); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: native-review\ndescription: Review native Android changes\nmodel-invocable: true\nuser-invocable: true\n---\nAlways inspect the native boundary.\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	value, rpcErr := server.dispatchRPC(context.Background(), "skills/list", map[string]any{
		"request": map[string]any{"sessionId": "skill-session"},
	})
	if rpcErr != nil {
		t.Fatal(rpcErr.Message)
	}
	listed := value.(map[string]any)["skills"].([]map[string]any)
	if len(listed) != 1 || listed[0]["name"] != "native-review" || listed[0]["description"] != "Review native Android changes" {
		t.Fatalf("skills = %#v", listed)
	}
	registry := server.ensureSkillRegistry(rec)
	loaded, err := nativeSkillTool(registry).Execute(context.Background(), map[string]any{"name": "native-review"})
	if err != nil || !strings.Contains(loaded.(string), "Always inspect") {
		t.Fatalf("loaded skill = %#v, %v", loaded, err)
	}
}

func TestNativeAgentPresetCatalogueAndSelection(t *testing.T) {
	server, _, cleanup := setupTestServer(t)
	defer cleanup()
	rec := server.getOrCreateSession("preset-session", "")
	listed, rpcErr := server.dispatchRPC(context.Background(), "agentPresets/list", map[string]any{})
	if rpcErr != nil || len(listed.(map[string]any)["presets"].([]map[string]any)) != 1 {
		t.Fatalf("preset list = %#v, %+v", listed, rpcErr)
	}
	document, rpcErr := server.dispatchRPC(context.Background(), "agentPresets/read", map[string]any{"agentPreset": "native"})
	if rpcErr != nil || !strings.Contains(document.(map[string]any)["content"].(string), "id: native") {
		t.Fatalf("preset document = %#v, %+v", document, rpcErr)
	}
	selected, rpcErr := server.dispatchRPC(context.Background(), "agentPresets/select", map[string]any{
		"agentId": "preset-session", "agentPreset": "native",
	})
	if rpcErr != nil || selected != "native" {
		t.Fatalf("preset selection = %#v, %+v", selected, rpcErr)
	}
	if rec.AgentPreset != "native" {
		t.Fatalf("record preset = %q", rec.AgentPreset)
	}
}

func TestNativeFileAndSessionReferenceCandidates(t *testing.T) {
	server, _, cleanup := setupTestServer(t)
	defer cleanup()
	source := server.getOrCreateSession("reference-source", "")
	other := server.getOrCreateSession("reference-other", "")
	_, _ = other.Log.Append(session.SessionTitleData{Title: "Other conversation", Source: "user"})
	if err := os.MkdirAll(filepath.Join(source.Cwd, "src"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source.Cwd, "src", "native.go"), []byte("package native"), 0o600); err != nil {
		t.Fatal(err)
	}
	files, rpcErr := server.dispatchRPC(context.Background(), "fileReferences/list", map[string]any{
		"agentId": source.ID.Raw(), "query": "native",
	})
	if rpcErr != nil {
		t.Fatal(rpcErr.Message)
	}
	fileRows := files.([]map[string]any)
	if len(fileRows) != 1 || fileRows[0]["path"] != "src/native.go" {
		t.Fatalf("file references = %#v", fileRows)
	}
	sessions, rpcErr := server.dispatchRPC(context.Background(), "sessionReferenceResolver/candidates", map[string]any{
		"agentId": source.ID.Raw(), "query": "Other",
	})
	if rpcErr != nil {
		t.Fatal(rpcErr.Message)
	}
	sessionRows := sessions.([]map[string]any)
	if len(sessionRows) != 1 || sessionRows[0]["sessionId"] != other.ID.Raw() || !strings.HasPrefix(sessionRows[0]["mention"].(string), "@[Other conversation](dsh-session:") {
		t.Fatalf("session references = %#v", sessionRows)
	}
}

func TestNativeSettingsAndCommandCatalogue(t *testing.T) {
	server, _, cleanup := setupTestServer(t)
	defer cleanup()
	description, rpcErr := server.dispatchRPC(context.Background(), "settings/describe", map[string]any{})
	if rpcErr != nil || len(description.(map[string]any)["namespaces"].([]map[string]any)) != 2 {
		t.Fatalf("settings description = %#v, %+v", description, rpcErr)
	}
	updated, rpcErr := server.dispatchRPC(context.Background(), "settings/mutate", map[string]any{
		"ns": "native-engine", "expectedRevision": 0,
		"ops": []any{map[string]any{"op": "set", "path": []any{"defaultPermission"}, "value": "read-only"}},
	})
	if rpcErr != nil || updated.(map[string]any)["revision"] != 1 {
		t.Fatalf("settings update = %#v, %+v", updated, rpcErr)
	}
	if _, conflict := server.dispatchRPC(context.Background(), "settings/mutate", map[string]any{
		"ns": "native-engine", "expectedRevision": 0,
		"ops": []any{map[string]any{"op": "set", "path": []any{"defaultPermission"}, "value": "auto"}},
	}); conflict == nil || conflict.Code != "settings/conflict" {
		t.Fatalf("settings conflict = %+v", conflict)
	}
	rec := server.getOrCreateSession("settings-session", "")
	if rec.Permission != "read-only" {
		t.Fatalf("new session permission = %q", rec.Permission)
	}
	commands, rpcErr := server.dispatchRPC(context.Background(), "commands/list", map[string]any{"agentId": rec.ID.Raw()})
	if rpcErr != nil || len(commands.([]map[string]any)) != 1 || commands.([]map[string]any)[0]["name"] != "permission" {
		t.Fatalf("commands = %#v, %+v", commands, rpcErr)
	}
	if _, err := os.Stat(server.settings.path); err != nil {
		t.Fatalf("settings document not persisted: %v", err)
	}
}

func TestNativeBackgroundJobs(t *testing.T) {
	server, _, cleanup := setupTestServer(t)
	defer cleanup()
	rec := server.getOrCreateSession("job-session", "")
	job, err := server.startShellJob(rec.ID.Raw(), "printf 'job-output\\n'; sleep 0.2; printf 'job-done\\n'", rec.Cwd, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if job.ID == "" || job.Status != "running" {
		t.Fatalf("job = %#v", job)
	}
	listed := server.jobs.list(rec.ID.Raw())
	if len(listed) != 1 || listed[0].ID != job.ID {
		t.Fatalf("job list = %#v", listed)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		job.mu.Lock()
		status := job.Status
		job.mu.Unlock()
		if status != "running" && status != "stopping" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	job.mu.Lock()
	status := job.Status
	output := ""
	for _, chunk := range job.chunks {
		output += chunk.Text
	}
	job.mu.Unlock()
	if status != "completed" || !strings.Contains(output, "job-output") || !strings.Contains(output, "job-done") {
		t.Fatalf("job status=%s output=%q", status, output)
	}
	if len(server.jobs.list(rec.ID.Raw())) != 0 {
		t.Fatal("finished job was not removed from the live list")
	}

	slow, err := server.startShellJob(rec.ID.Raw(), "sleep 30", rec.Cwd, 0)
	if err != nil {
		t.Fatal(err)
	}
	killed, rpcErr := server.dispatchRPC(context.Background(), "job/kill", map[string]any{
		"request": map[string]any{"sessionId": rec.ID.Raw(), "jobId": slow.ID},
	})
	if rpcErr != nil || killed.(map[string]any)["outcome"] != "requested" {
		t.Fatalf("job kill = %#v, %+v", killed, rpcErr)
	}
	select {
	case <-slow.done:
	case <-time.After(5 * time.Second):
		t.Fatal("killed job did not finish")
	}
	slow.mu.Lock()
	slowStatus := slow.Status
	slow.mu.Unlock()
	if slowStatus != "killed" {
		t.Fatalf("killed job status = %q", slowStatus)
	}
	if _, rpcErr := server.dispatchRPC(context.Background(), "job/kill", map[string]any{
		"request": map[string]any{"sessionId": rec.ID.Raw(), "jobId": "missing-job"},
	}); rpcErr == nil || rpcErr.Code != "job/not-found" {
		t.Fatalf("missing job kill = %+v", rpcErr)
	}
}

func TestNativeMessageFeedbackLifecycle(t *testing.T) {
	server, _, cleanup := setupTestServer(t)
	defer cleanup()
	adapter := &recordingAdapter{requests: make(chan llm.ChatRequest, 1)}
	server.adapter = adapter
	if err := server.credStore.Set(context.Background(), brand.NewCredentialRef("DEEPSEEK_API_KEY"), "test-key"); err != nil {
		t.Fatal(err)
	}
	rec := server.getOrCreateSession("feedback-session", "")
	if _, rpcErr := server.dispatchRPC(context.Background(), "session/prompt", map[string]any{
		"request": map[string]any{
			"sessionId": rec.ID.Raw(), "requestId": "feedback-turn",
			"content": []any{map[string]any{"type": "text", "text": "reply for feedback"}},
		},
	}); rpcErr != nil {
		t.Fatal(rpcErr.Message)
	}
	select {
	case <-adapter.requests:
	case <-time.After(time.Second):
		t.Fatal("model request did not start")
	}
	var messageID string
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && messageID == "" {
		rec.mu.Lock()
		for _, event := range rec.Events {
			if event["type"] == "assistant/message" {
				data, _ := event["data"].(map[string]any)
				message, _ := data["message"].(map[string]any)
				messageID, _ = message["id"].(string)
			}
		}
		rec.mu.Unlock()
		if messageID == "" {
			time.Sleep(10 * time.Millisecond)
		}
	}
	if messageID == "" {
		t.Fatal("assistant message id not found")
	}
	put, rpcErr := server.dispatchRPC(context.Background(), "messageFeedback/put", map[string]any{
		"request": map[string]any{
			"sessionId": rec.ID.Raw(), "messageId": messageID, "rating": "positive",
			"note": "Useful", "category": "task-result", "ifVersion": nil,
		},
	})
	if rpcErr != nil || put.(map[string]any)["ok"] != true {
		t.Fatalf("feedback put = %#v, %+v", put, rpcErr)
	}
	item := put.(map[string]any)["value"].(*feedbackItem)
	if item.Version == "" || item.MessageID != messageID {
		t.Fatalf("feedback item = %#v", item)
	}
	conflict, rpcErr := server.dispatchRPC(context.Background(), "messageFeedback/put", map[string]any{
		"request": map[string]any{
			"sessionId": rec.ID.Raw(), "messageId": messageID, "rating": "negative", "ifVersion": "stale",
		},
	})
	if rpcErr != nil || conflict.(map[string]any)["ok"] != false {
		t.Fatalf("feedback conflict = %#v, %+v", conflict, rpcErr)
	}
	listed, _ := server.dispatchRPC(context.Background(), "messageFeedback/list", map[string]any{
		"request": map[string]any{"sessionId": rec.ID.Raw()},
	})
	items := listed.(map[string]any)["value"].(map[string]any)["items"].([]*feedbackItem)
	if len(items) != 1 || items[0].Version != item.Version {
		t.Fatalf("feedback list = %#v", listed)
	}
	deleted, rpcErr := server.dispatchRPC(context.Background(), "messageFeedback/delete", map[string]any{
		"request": map[string]any{"sessionId": rec.ID.Raw(), "messageId": messageID, "ifVersion": item.Version},
	})
	if rpcErr != nil || deleted.(map[string]any)["ok"] != true {
		t.Fatalf("feedback delete = %#v, %+v", deleted, rpcErr)
	}
}

func getAuthCookie(t *testing.T, server *Server, ts *httptest.Server) *http.Cookie {
	t.Helper()
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Get(ts.URL + "/?token=" + server.launchToken)
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}
	defer resp.Body.Close()
	for _, c := range resp.Cookies() {
		if strings.HasPrefix(c.Name, "dsh-auth-") {
			return c
		}
	}
	t.Fatalf("no auth cookie returned")
	return nil
}

func TestHandshakeAndAuthentication(t *testing.T) {
	server, ts, cleanup := setupTestServer(t)
	defer cleanup()

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // Do not follow redirects
		},
	}

	// 1. Unauthenticated GET / should be 401
	resp, err := client.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET / failed: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected status 401 for unauthenticated GET /, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// 2. GET / with wrong token should be 401
	resp, err = client.Get(ts.URL + "/?token=wrong_token")
	if err != nil {
		t.Fatalf("GET /?token=wrong failed: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected status 401 for wrong token, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// 3. GET / with valid token should set cookie and redirect (303)
	resp, err = client.Get(ts.URL + "/?token=" + server.launchToken)
	if err != nil {
		t.Fatalf("GET /?token=<valid> failed: %v", err)
	}
	if resp.StatusCode != http.StatusSeeOther {
		t.Errorf("expected status 303 See Other, got %d", resp.StatusCode)
	}

	cookies := resp.Cookies()
	var authCookie *http.Cookie
	for _, c := range cookies {
		if strings.HasPrefix(c.Name, "dsh-auth-") {
			authCookie = c
			break
		}
	}
	if authCookie == nil {
		t.Fatalf("expected dsh-auth-* cookie in response, found none")
	}
	if !strings.HasPrefix(authCookie.Value, "v1.") {
		t.Errorf("expected cookie value to start with v1., got %s", authCookie.Value)
	}
	resp.Body.Close()

	// 4. Authenticated health check: GET / with cookie should return 200 OK
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/", nil)
	req.AddCookie(authCookie)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("authenticated GET / failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK for authenticated GET /, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestUnauthenticatedAccessRejected(t *testing.T) {
	_, ts, cleanup := setupTestServer(t)
	defer cleanup()

	client := ts.Client()

	// 1. Unauthenticated RPC POST /api/session/list -> 401
	resp, err := client.Post(ts.URL+"/api/session/list", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("POST /api/session/list failed: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized for unauthenticated RPC, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// 2. Unauthenticated WebSocket upgrade -> 401
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/remote.mux"
	_, wsResp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err == nil {
		t.Errorf("expected websocket dial without cookie to fail")
	} else if wsResp != nil && wsResp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized for unauthenticated websocket, got %d", wsResp.StatusCode)
	}

	// 3. Unauthenticated session deletion capability -> 401
	resp, err = client.Get(ts.URL + "/dsh-go/v1/sessions/deletion")
	if err != nil {
		t.Fatalf("GET deletion capability failed: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 for unauthenticated deletion capability, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// 4. Unauthenticated session delete -> 401
	resp, err = client.Post(ts.URL+"/dsh-go/v1/sessions/delete", "application/json", strings.NewReader(`{"sessionId":"s1"}`))
	if err != nil {
		t.Fatalf("POST delete failed: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 for unauthenticated session delete, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestRPCEndpoints(t *testing.T) {
	server, ts, cleanup := setupTestServer(t)
	defer cleanup()

	client := ts.Client()
	authCookie := getAuthCookie(t, server, ts)

	callRPC := func(method string, args map[string]any) map[string]any {
		body := map[string]any{
			"type":   "client-request",
			"rpcId":  "test-rpc-id",
			"method": method,
			"payload": map[string]any{
				"args": args,
			},
		}
		raw, _ := json.Marshal(body)
		req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/"+method, strings.NewReader(string(raw)))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(authCookie)

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("rpc %s failed: %v", method, err)
		}
		defer resp.Body.Close()

		var res ServerResponse
		if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
			t.Fatalf("decode rpc response for %s: %v", method, err)
		}
		if !res.Result.Ok {
			t.Fatalf("rpc %s returned not ok: %v", method, res.Result.Error)
		}
		val, _ := res.Result.Value.(map[string]any)
		return val
	}

	// 1. session/create
	created := callRPC("session/create", map[string]any{
		"request": map[string]any{
			"cwd": "/test/workspace",
		},
	})
	sID, _ := created["sessionId"].(string)
	if sID == "" {
		t.Fatalf("expected non-empty sessionId")
	}

	// 2. session/selectModel
	modelRes := callRPC("session/selectModel", map[string]any{
		"request": map[string]any{
			"sessionId":       sID,
			"provider":        "deepseek",
			"model":           "deepseek-reasoner",
			"reasoningEffort": "high",
		},
	})
	selected, ok := modelRes["selected"].(map[string]any)
	if !ok || selected["model"] != "deepseek-reasoner" {
		t.Errorf("expected selected model deepseek-reasoner, got %v", modelRes)
	}

	// 3. session/projections: verify selected model reflected
	proj := callRPC("session/projections", map[string]any{
		"request": map[string]any{
			"sessionId": sID,
		},
	})
	values, ok := proj["values"].(map[string]any)
	if !ok {
		t.Fatalf("expected projections values")
	}
	modelSel, _ := values["modelSelection"].(map[string]any)
	lastUsed, _ := modelSel["lastUsed"].(map[string]any)
	if lastUsed["model"] != "deepseek-reasoner" {
		t.Errorf("expected projections model deepseek-reasoner, got %v", lastUsed["model"])
	}

	// 4. session/rename: verify title update
	renameRes := callRPC("session/rename", map[string]any{
		"request": map[string]any{
			"sessionId": sID,
			"title":     "New Title Test",
		},
	})
	if renameRes["title"] != "New Title Test" {
		t.Errorf("expected title 'New Title Test', got %v", renameRes["title"])
	}

	// 5. session/page: verify pagination returns records map
	pageRes := callRPC("session/page", map[string]any{
		"request": map[string]any{
			"address": map[string]any{
				"kind":      "session",
				"sessionId": sID,
			},
			"throughSeq": 10,
		},
	})
	records, ok := pageRes["records"].([]any)
	if !ok {
		t.Fatalf("expected records array from session/page, got %v", pageRes)
	}
	// At least the session/title event from rename was recorded
	if len(records) == 0 {
		t.Errorf("expected at least 1 record from session/page")
	}

	// 6. session/list
	list := callRPC("session/list", map[string]any{
		"_request": map[string]any{},
	})
	items, ok := list["items"].([]any)
	if !ok || len(items) == 0 {
		t.Fatalf("expected at least 1 session in list, got %v", list)
	}

	// 7. session/modelCatalog: verify deepseek-reasoner reasoning efforts
	catalog := callRPC("session/modelCatalog", map[string]any{})
	groups, _ := catalog["groups"].([]any)
	if len(groups) == 0 {
		t.Fatalf("expected model catalog groups")
	}
	group0 := groups[0].(map[string]any)
	models := group0["models"].([]any)
	var reasonerModel map[string]any
	for _, m := range models {
		mMap := m.(map[string]any)
		if mMap["id"] == "deepseek-reasoner" {
			reasonerModel = mMap
			break
		}
	}
	if reasonerModel == nil {
		t.Fatalf("expected deepseek-reasoner model in catalog")
	}
	reasoning, ok := reasonerModel["reasoning"].(map[string]any)
	if !ok || reasoning["defaultEffort"] != "high" {
		t.Errorf("expected reasoning.defaultEffort = high, got %v", reasonerModel)
	}

	// 8. credentials/set and credentials/describe
	callRPC("credentials/set", map[string]any{
		"ref":   "DEEPSEEK_API_KEY",
		"value": "sk-test-key-123",
	})

	desc := callRPC("credentials/describe", map[string]any{
		"refs": []any{"DEEPSEEK_API_KEY"},
	})
	credStatus, ok := desc["DEEPSEEK_API_KEY"].(map[string]any)
	if !ok || credStatus["configured"] != true {
		t.Errorf("expected DEEPSEEK_API_KEY to be configured, got %v", desc)
	}
}

func TestSessionDeletion(t *testing.T) {
	server, ts, cleanup := setupTestServer(t)
	defer cleanup()

	client := ts.Client()
	authCookie := getAuthCookie(t, server, ts)

	// 1. GET /dsh-go/v1/sessions/deletion
	capReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/dsh-go/v1/sessions/deletion", nil)
	capReq.AddCookie(authCookie)
	capResp, err := client.Do(capReq)
	if err != nil {
		t.Fatalf("GET /dsh-go/v1/sessions/deletion failed: %v", err)
	}
	if capResp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK for deletion capability, got %d", capResp.StatusCode)
	}
	var capBody map[string]any
	_ = json.NewDecoder(capResp.Body).Decode(&capBody)
	capResp.Body.Close()
	if capBody["supported"] != true || capBody["version"] != float64(1) {
		t.Errorf("unexpected deletion capability body: %v", capBody)
	}

	// Create a session to delete
	rec := server.getOrCreateSession("session-to-delete", "/tmp")
	rec.Title = "Delete Me"

	// 2. POST /dsh-go/v1/sessions/delete
	delReqBody, _ := json.Marshal(map[string]any{"sessionId": "session-to-delete"})
	delReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/dsh-go/v1/sessions/delete", strings.NewReader(string(delReqBody)))
	delReq.Header.Set("Content-Type", "application/json")
	delReq.AddCookie(authCookie)

	delResp, err := client.Do(delReq)
	if err != nil {
		t.Fatalf("POST /dsh-go/v1/sessions/delete failed: %v", err)
	}
	if delResp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK for session delete, got %d", delResp.StatusCode)
	}
	var delBody map[string]any
	_ = json.NewDecoder(delResp.Body).Decode(&delBody)
	delResp.Body.Close()
	if delBody["deleted"] != true || delBody["sessionId"] != "session-to-delete" {
		t.Errorf("unexpected delete response body: %v", delBody)
	}

	// Verify session is gone from memory
	server.sessionsMu.RLock()
	_, exists := server.sessions["session-to-delete"]
	server.sessionsMu.RUnlock()
	if exists {
		t.Errorf("expected session-to-delete to be removed from server.sessions")
	}
}

func TestWebSocketMux(t *testing.T) {
	server, ts, cleanup := setupTestServer(t)
	defer cleanup()

	authCookie := getAuthCookie(t, server, ts)

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/remote.mux"
	header := make(http.Header)
	header.Add("Cookie", authCookie.String())

	ws, _, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		t.Fatalf("dial websocket failed: %v", err)
	}
	defer ws.Close()

	// 1. Open $events stream
	err = ws.WriteJSON(map[string]any{
		"type":     "open",
		"streamId": "events-stream-1",
		"endpoint": "$events",
		"payload":  map[string]any{"args": map[string]any{}},
	})
	if err != nil {
		t.Fatalf("open $events failed: %v", err)
	}

	var respFrame map[string]any
	err = ws.ReadJSON(&respFrame)
	if err != nil {
		t.Fatalf("read $events frame failed: %v", err)
	}

	if respFrame["streamId"] != "events-stream-1" {
		t.Errorf("expected streamId events-stream-1, got %v", respFrame["streamId"])
	}
	val, _ := respFrame["value"].(map[string]any)
	if val["type"] != "ready" {
		t.Errorf("expected ready type, got %v", val["type"])
	}

	// 2. Open session/follow using Flutter's address structure
	err = ws.WriteJSON(map[string]any{
		"type":     "open",
		"streamId": "follow-stream-1",
		"endpoint": "session/follow",
		"payload": map[string]any{
			"request": map[string]any{
				"address": map[string]any{
					"kind":      "session",
					"sessionId": "test-session-123",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("open session/follow failed: %v", err)
	}

	var followResp map[string]any
	err = ws.ReadJSON(&followResp)
	if err != nil {
		t.Fatalf("read session/follow snapshot failed: %v", err)
	}
	if followResp["streamId"] != "follow-stream-1" {
		t.Errorf("expected streamId follow-stream-1, got %v", followResp["streamId"])
	}
	followVal, _ := followResp["value"].(map[string]any)
	if followVal["type"] != "snapshot" {
		t.Errorf("expected snapshot type, got %v", followVal["type"])
	}
	if followVal["sessionId"] != "test-session-123" {
		t.Errorf("expected sessionId test-session-123 from address, got %v", followVal["sessionId"])
	}

	// 3. Open session/follow using the actual DshMux envelope. It wraps the
	// endpoint arguments inside payload.args, with request nested below that.
	err = ws.WriteJSON(map[string]any{
		"type":     "open",
		"streamId": "follow-stream-production",
		"endpoint": "session/follow",
		"payload": map[string]any{
			"args": map[string]any{
				"request": map[string]any{
					"address": map[string]any{
						"kind":      "session",
						"sessionId": "production-envelope-session",
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("open production session/follow failed: %v", err)
	}
	var productionFollow map[string]any
	if err := ws.ReadJSON(&productionFollow); err != nil {
		t.Fatalf("read production session/follow snapshot failed: %v", err)
	}
	productionValue, _ := productionFollow["value"].(map[string]any)
	if productionValue["sessionId"] != "production-envelope-session" {
		t.Fatalf("production DshMux envelope resolved the wrong session: %v", productionValue)
	}

	// 4. Workspace state is a real stream: baseline first, then complete-set
	// archive updates used by the Flutter session drawer.
	if err := ws.WriteJSON(map[string]any{
		"type": "open", "streamId": "workspace-stream", "endpoint": "workspace/follow",
		"payload": map[string]any{"args": map[string]any{}},
	}); err != nil {
		t.Fatalf("open workspace/follow failed: %v", err)
	}
	var workspaceBaseline map[string]any
	if err := ws.ReadJSON(&workspaceBaseline); err != nil {
		t.Fatalf("read workspace baseline: %v", err)
	}
	baselineValue, _ := workspaceBaseline["value"].(map[string]any)
	if baselineValue["type"] != "baseline" {
		t.Fatalf("expected workspace baseline, got %v", workspaceBaseline)
	}
	server.setSessionArchived("production-envelope-session", true)
	var workspaceUpdate map[string]any
	if err := ws.ReadJSON(&workspaceUpdate); err != nil {
		t.Fatalf("read workspace archive update: %v", err)
	}
	updateValue, _ := workspaceUpdate["value"].(map[string]any)
	if updateValue["type"] != "archived" {
		t.Fatalf("expected workspace archive update, got %v", workspaceUpdate)
	}
}

func TestPromptPassesFreshAndHistoricalMessagesToAdapter(t *testing.T) {
	server, _, cleanup := setupTestServer(t)
	defer cleanup()

	adapter := &recordingAdapter{requests: make(chan llm.ChatRequest, 2)}
	server.adapter = adapter
	if err := server.credStore.Set(context.Background(), brand.NewCredentialRef("DEEPSEEK_API_KEY"), "test-key"); err != nil {
		t.Fatalf("set test credential: %v", err)
	}

	rec := server.getOrCreateSession("adapter-history", "/tmp")
	prompt := func(requestID, text string) llm.ChatRequest {
		_, rpcErr := server.dispatchRPC(context.Background(), "session/prompt", map[string]any{
			"request": map[string]any{
				"sessionId": "adapter-history",
				"requestId": requestID,
				"content":   []any{map[string]any{"type": "text", "text": text}},
			},
		})
		if rpcErr != nil {
			t.Fatalf("prompt %s rejected: %+v", requestID, rpcErr)
		}
		select {
		case request := <-adapter.requests:
			return request
		case <-time.After(3 * time.Second):
			t.Fatalf("prompt %s did not reach adapter", requestID)
			return llm.ChatRequest{}
		}
	}
	waitIdle := func() {
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			rec.mu.Lock()
			idle := rec.Cancel == nil
			rec.mu.Unlock()
			if idle {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatal("turn did not become idle")
	}

	first := prompt("request-1", "first prompt")
	if len(first.Messages) != 1 || first.Messages[0].Role != llm.RoleUser || first.Messages[0].Content[0].Text != "first prompt" {
		t.Fatalf("fresh in-memory prompt missing from adapter request: %#v", first.Messages)
	}
	waitIdle()

	second := prompt("request-2", "second prompt")
	if len(second.Messages) != 3 {
		t.Fatalf("expected user/assistant/user history, got %#v", second.Messages)
	}
	if second.Messages[0].Content[0].Text != "first prompt" ||
		second.Messages[1].Content[0].Text != "native reply" ||
		second.Messages[2].Content[0].Text != "second prompt" {
		t.Fatalf("adapter history has wrong content: %#v", second.Messages)
	}
}

func TestTurnExecution(t *testing.T) {
	server, ts, cleanup := setupTestServer(t)
	defer cleanup()

	authCookie := getAuthCookie(t, server, ts)

	// Create session
	rec := server.getOrCreateSession("turn-test-session", "/tmp")

	// Trigger prompt
	body := map[string]any{
		"type":   "client-request",
		"rpcId":  "prompt-rpc-1",
		"method": "session/prompt",
		"payload": map[string]any{
			"args": map[string]any{
				"request": map[string]any{
					"sessionId": "turn-test-session",
					"content": []map[string]any{
						{"type": "text", "text": "Hello, world!"},
					},
				},
			},
		},
	}
	raw, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/session/prompt", strings.NewReader(string(raw)))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(authCookie)

	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("prompt failed: %v", err)
	}
	resp.Body.Close()

	// Wait for turn to complete
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		rec.mu.Lock()
		isDone := false
		for _, ev := range rec.Events {
			if t, _ := ev["type"].(string); t == "turn/end" {
				isDone = true
				break
			}
		}
		rec.mu.Unlock()
		if isDone {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	rec.mu.Lock()
	defer rec.mu.Unlock()

	var turnStartFound, userMsgFound, asstMsgFound, turnEndFound bool
	var turn0Idx int = -1
	for _, ev := range rec.Events {
		switch ev["type"] {
		case "turn/start":
			turnStartFound = true
			if data, ok := ev["data"].(map[string]any); ok {
				if tNum, ok := data["turn"].(int); ok {
					turn0Idx = tNum
				}
			}
		case "user/message":
			userMsgFound = true
		case "assistant/message":
			asstMsgFound = true
		case "turn/end":
			turnEndFound = true
		}
	}

	if !turnStartFound || !userMsgFound || !asstMsgFound || !turnEndFound {
		t.Fatalf("expected all turn events, got events: %v", rec.Events)
	}
	if turn0Idx != 0 {
		t.Errorf("expected first turn index to be 0, got %d", turn0Idx)
	}
	if rec.Title == "" {
		t.Errorf("expected session title to be set from prompt")
	}
}
