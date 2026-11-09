package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

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
	mux.HandleFunc("/api/", server.handleRPC)
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
