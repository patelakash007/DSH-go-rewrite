package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"

	"github.com/JopenChen/dsh-go/pkg/agent"
	"github.com/JopenChen/dsh-go/pkg/brand"
	"github.com/JopenChen/dsh-go/pkg/credentials"
	"github.com/JopenChen/dsh-go/pkg/llm"
	"github.com/JopenChen/dsh-go/pkg/llm/provider_deepseek"
	"github.com/JopenChen/dsh-go/pkg/nativefs"
	"github.com/JopenChen/dsh-go/pkg/nativetools"
	"github.com/JopenChen/dsh-go/pkg/persistence/sqlite"
	"github.com/JopenChen/dsh-go/pkg/session"
	"github.com/JopenChen/dsh-go/pkg/skills"
	"github.com/JopenChen/dsh-go/pkg/storage"
	"github.com/JopenChen/dsh-go/pkg/uuid"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// SessionRecord represents the in-memory state of an active session.
type SessionRecord struct {
	ID          brand.SessionID
	Title       string
	Cwd         string
	Model       string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Log         *session.SessionLog
	Events      []map[string]any
	Subs        map[string]chan []byte
	Cancel      context.CancelFunc
	RunID       string
	Requests    map[string]struct{}
	Permission  string
	AgentPreset string
	Agent       *agent.Agent
	Tools       *nativetools.Registry
	Skills      *skills.SkillRegistry
	stopLog     func()

	attemptID       string
	attemptRevision int64
	attemptIndex    int64
	attemptTurn     uint64
	attemptStep     uint64
	attemptEnded    bool
	mu              sync.Mutex
}

// Server encapsulates the DSH HTTP/WebSocket backend.
type Server struct {
	port         int
	homeDir      string
	launchToken  string
	clientId     string
	dbStore      *sqlite.Store
	credStore    *credentials.Store
	adapter      llm.LLMAdapter
	workspaceDir string
	files        *nativefs.Service
	uploads      *uploadStore

	sessionsMu sync.RWMutex
	sessions   map[string]*SessionRecord

	workspaceMu        sync.Mutex
	workspaceSubs      map[string]chan map[string]any
	archived           map[string]bool
	workspaces         map[string]*workspaceRecord
	workspaceOrder     []string
	pinnedSessions     []string
	workspaceStatePath string

	hostMu           sync.Mutex
	hostSubs         map[string]chan map[string]any
	pendingApprovals map[string]*pendingApproval

	terminalMu        sync.Mutex
	terminals         map[string]map[string]*terminalRecord
	closedTerminalIDs map[string]bool

	goalMu         sync.Mutex
	goalActivation map[string]bool

	controlMu   sync.Mutex
	controlSubs map[string]chan map[string]any
}

func newServer(port int, homeDir string) (*Server, error) {
	workspaceDir := os.Getenv("DSH_WORKSPACE_ROOT")
	if workspaceDir == "" {
		workspaceDir = filepath.Join(homeDir, "workspace")
	}
	return newServerWithWorkspace(port, homeDir, workspaceDir)
}

func newServerWithWorkspace(port int, homeDir, workspaceDir string) (*Server, error) {
	if err := os.MkdirAll(homeDir, 0755); err != nil {
		return nil, fmt.Errorf("mkdir home: %w", err)
	}
	files, err := nativefs.New(workspaceDir)
	if err != nil {
		return nil, fmt.Errorf("initialize native workspace: %w", err)
	}
	uploads, err := newUploadStore(files.Root())
	if err != nil {
		return nil, fmt.Errorf("initialize native uploads: %w", err)
	}

	// Launch Token (32 hex characters)
	tokenBytes := make([]byte, 16)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, fmt.Errorf("generate launch token: %w", err)
	}
	launchToken := hex.EncodeToString(tokenBytes)
	clientId := uuid.MustV4()

	// Persistent SQLite session event store
	dbPath := filepath.Join(homeDir, "sessions.db")
	dbStore, err := sqlite.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite sessions db: %w", err)
	}

	// Persistent SQLite KV store for credentials
	kvPath := filepath.Join(homeDir, "kv.db")
	kvStore, err := storage.NewSQLiteKV(kvPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite kv db: %w", err)
	}
	credStore := credentials.NewStore(kvStore)

	// Pre-populate API key from environment if available
	if envKey := os.Getenv("DEEPSEEK_API_KEY"); envKey != "" {
		_ = credStore.Set(context.Background(), brand.NewCredentialRef("DEEPSEEK_API_KEY"), envKey)
	}

	dsAdapter := provider_deepseek.NewDeepSeek(credStore)

	s := &Server{
		port:               port,
		homeDir:            homeDir,
		launchToken:        launchToken,
		clientId:           clientId,
		dbStore:            dbStore,
		credStore:          credStore,
		adapter:            dsAdapter,
		workspaceDir:       files.Root(),
		files:              files,
		uploads:            uploads,
		sessions:           make(map[string]*SessionRecord),
		workspaceSubs:      make(map[string]chan map[string]any),
		archived:           make(map[string]bool),
		workspaces:         make(map[string]*workspaceRecord),
		workspaceStatePath: filepath.Join(homeDir, "workspace-state.json"),
		hostSubs:           make(map[string]chan map[string]any),
		pendingApprovals:   make(map[string]*pendingApproval),
		terminals:          make(map[string]map[string]*terminalRecord),
		closedTerminalIDs:  make(map[string]bool),
		goalActivation:     make(map[string]bool),
		controlSubs:        make(map[string]chan map[string]any),
	}
	if err := s.loadWorkspaceState(); err != nil {
		_ = dbStore.Close()
		return nil, fmt.Errorf("load native workspace state: %w", err)
	}

	// Restore known sessions from SQLite
	ctx := context.Background()
	ids, err := dbStore.ListSessions(ctx)
	if err == nil {
		for _, rawId := range ids {
			sID := brand.NewSessionID(rawId)
			evs, err := dbStore.Load(ctx, sID)
			if err == nil && len(evs) > 0 {
				logStore, restoreErr := session.NewSessionLogFromEvents(sID, evs)
				if restoreErr != nil {
					log.Printf("skip corrupt native session %s: %v", rawId, restoreErr)
					continue
				}
				rec := &SessionRecord{
					ID:         sID,
					CreatedAt:  evs[0].Time,
					UpdatedAt:  evs[len(evs)-1].Time,
					Cwd:        files.Root(),
					Model:      "deepseek-chat",
					Permission: "auto",
					Log:        logStore,
					Subs:       make(map[string]chan []byte),
					Requests:   make(map[string]struct{}),
				}
				for _, ev := range evs {
					rec.Events = append(rec.Events, mapSessionEvent(ev))
					if titleEv, ok := ev.Data.(session.SessionTitleData); ok && titleEv.Title != "" {
						rec.Title = titleEv.Title
					}
					if modelEv, ok := ev.Data.(session.ModelSelectionData); ok && modelEv.Model != "" {
						rec.Model = modelEv.Model
					}
					if presetEv, ok := ev.Data.(session.PresetChangeData); ok && presetEv.Preset != "" {
						rec.Permission = presetEv.Preset
					}
				}
				s.sessions[rawId] = rec
				s.attachSession(rec)
				s.recoverInterruptedSession(rec)
			}
		}
	}

	return s, nil
}

func (s *Server) getOrCreateSession(rawId string, cwd string) *SessionRecord {
	s.sessionsMu.Lock()
	defer s.sessionsMu.Unlock()

	if rec, ok := s.sessions[rawId]; ok {
		return rec
	}

	sID := brand.NewSessionID(rawId)
	now := time.Now()
	cwd = s.nativeSessionCwd(cwd)
	rec := &SessionRecord{
		ID:         sID,
		Cwd:        cwd,
		Model:      "deepseek-chat",
		CreatedAt:  now,
		UpdatedAt:  now,
		Log:        session.NewSessionLog(sID),
		Subs:       make(map[string]chan []byte),
		Requests:   make(map[string]struct{}),
		Permission: "auto",
	}
	s.sessions[rawId] = rec
	s.attachSession(rec)
	return rec
}

func (s *Server) nativeSessionCwd(requested string) string {
	if requested != "" {
		if resolved, err := s.files.Resolve(s.workspaceDir, requested); err == nil {
			if info, statErr := os.Stat(resolved); statErr == nil && info.IsDir() {
				return resolved
			}
		}
	}
	return s.workspaceDir
}

func (s *Server) archivedSessionIDs() []string {
	s.workspaceMu.Lock()
	defer s.workspaceMu.Unlock()
	ids := make([]string, 0, len(s.archived))
	for id, archived := range s.archived {
		if archived {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

func (s *Server) setSessionArchived(sessionID string, archived bool) []string {
	s.workspaceMu.Lock()
	if archived {
		s.archived[sessionID] = true
		s.pinnedSessions = removeString(s.pinnedSessions, sessionID)
	} else {
		delete(s.archived, sessionID)
	}
	ids := make([]string, 0, len(s.archived))
	for id := range s.archived {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	_ = s.persistWorkspaceStateLocked()
	s.broadcastWorkspaceLocked(map[string]any{"type": "archived", "archivedSessionIds": ids})
	if archived {
		s.broadcastWorkspaceLocked(map[string]any{
			"type": "pinned", "pinnedSessionIds": append([]string(nil), s.pinnedSessions...),
		})
	}
	s.workspaceMu.Unlock()
	return ids
}

func (s *Server) makeCookieName(authority string) string {
	sum := sha256.Sum256([]byte(authority))
	return "dsh-auth-" + base64.RawURLEncoding.EncodeToString(sum[:])
}

func (s *Server) isCookieAuthorized(r *http.Request) bool {
	expectedVal := "v1." + s.launchToken
	expectedName := s.makeCookieName(r.Host)
	cookie, err := r.Cookie(expectedName)
	return err == nil && cookie.Value == expectedVal
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		token := r.URL.Query().Get("token")
		if token != "" {
			if token == s.launchToken {
				authority := r.Host
				if authority == "" {
					authority = fmt.Sprintf("127.0.0.1:%d", s.port)
				}
				cookieName := s.makeCookieName(authority)
				cookieVal := "v1." + s.launchToken
				http.SetCookie(w, &http.Cookie{
					Name:     cookieName,
					Value:    cookieVal,
					Path:     "/",
					HttpOnly: true,
					SameSite: http.SameSiteStrictMode,
					MaxAge:   30 * 86400,
				})
				http.Redirect(w, r, "./", http.StatusSeeOther)
				return
			}
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}

		// Health Check
		if s.isCookieAuthorized(r) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
			return
		}

		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	http.NotFound(w, r)
}

// RPC Envelope formats
type ClientRequest struct {
	Type    string         `json:"type"`
	RPCID   string         `json:"rpcId"`
	Method  string         `json:"method"`
	Payload map[string]any `json:"payload"`
}

type ServerResponse struct {
	Type   string       `json:"type"`
	RPCID  string       `json:"rpcId"`
	Result ResultObject `json:"result"`
}

type ResultObject struct {
	Ok    bool      `json:"ok"`
	Value any       `json:"value,omitempty"`
	Error *RPCError `json:"error,omitempty"`
}

type RPCError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (s *Server) handleRPC(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !s.isCookieAuthorized(r) {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	endpoint := strings.TrimPrefix(r.URL.Path, "/api/")

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Read error", http.StatusBadRequest)
		return
	}

	var req ClientRequest
	_ = json.Unmarshal(bodyBytes, &req)

	rpcId := req.RPCID
	if rpcId == "" {
		rpcId = uuid.MustV4()
	}

	args, _ := req.Payload["args"].(map[string]any)
	if args == nil {
		args = make(map[string]any)
	}

	val, rpcErr := s.dispatchRPC(r.Context(), endpoint, args)

	resp := ServerResponse{
		Type:  "server-response",
		RPCID: rpcId,
		Result: ResultObject{
			Ok:    rpcErr == nil,
			Value: val,
			Error: rpcErr,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) dispatchRPC(ctx context.Context, endpoint string, args map[string]any) (any, *RPCError) {
	switch endpoint {
	case "session/list":
		s.sessionsMu.RLock()
		defer s.sessionsMu.RUnlock()

		items := make([]map[string]any, 0, len(s.sessions))
		for _, rec := range s.sessions {
			rec.mu.Lock()
			title := rec.Title
			if title == "" {
				title = "New session"
			}
			asOfSeq := len(rec.Events)
			modelName := rec.Model
			if modelName == "" {
				modelName = "deepseek-chat"
			}
			items = append(items, map[string]any{
				"sessionId":      rec.ID.Raw(),
				"updatedAt":      rec.UpdatedAt.UnixMilli(),
				"running":        rec.Cancel != nil,
				"blank":          len(rec.Events) == 0,
				"agentAvailable": true,
				"cwd":            rec.Cwd,
				"projections": map[string]any{
					"asOfSeq": asOfSeq,
					"values": map[string]any{
						"title": title,
						"permissions": map[string]any{
							"currentValue": "auto",
						},
						"modelSelection": map[string]any{
							"lastUsed": map[string]any{
								"provider": "deepseek",
								"model":    modelName,
							},
							"next": map[string]any{
								"provider": "deepseek",
								"model":    modelName,
							},
						},
						"sessionListMetadata": map[string]any{
							"blank": len(rec.Events) == 0,
						},
						"goal": goalProjection(rec),
					},
				},
			})
			rec.mu.Unlock()
		}
		sort.Slice(items, func(i, j int) bool {
			left, _ := items[i]["updatedAt"].(int64)
			right, _ := items[j]["updatedAt"].(int64)
			return left > right
		})
		return map[string]any{"items": items}, nil

	case "session/create":
		reqMap, _ := args["request"].(map[string]any)
		sIDStr, _ := reqMap["sessionId"].(string)
		if sIDStr == "" {
			sIDStr = uuid.MustV4()
		}
		cwd, _ := reqMap["cwd"].(string)
		workspaceID, _ := reqMap["workspaceId"].(string)
		agentPreset, _ := reqMap["agentPreset"].(string)
		if agentPreset != "" && agentPreset != "native" {
			return nil, &RPCError{Code: "agent-preset/not-found", Message: "agent preset not found"}
		}
		if workspaceID != "" {
			workspace := s.workspaceByID(workspaceID)
			if workspace == nil {
				return nil, &RPCError{Code: "workspace/not-found", Message: "workspace not found"}
			}
			cwd = workspace.Path
		}

		rec := s.getOrCreateSession(sIDStr, cwd)
		rec.mu.Lock()
		if agentPreset == "" {
			agentPreset = "native"
		}
		rec.AgentPreset = agentPreset
		rec.mu.Unlock()
		if workspaceID != "" {
			s.attachSessionToWorkspace(workspaceID, rec.ID.Raw())
		}
		return map[string]any{
			"sessionId":   rec.ID.Raw(),
			"agentPreset": agentPreset,
		}, nil

	case "session/projections":
		reqMap, _ := args["request"].(map[string]any)
		sIDStr, _ := reqMap["sessionId"].(string)

		s.sessionsMu.RLock()
		rec, ok := s.sessions[sIDStr]
		s.sessionsMu.RUnlock()

		if !ok {
			return nil, nil
		}

		rec.mu.Lock()
		title := rec.Title
		if title == "" {
			title = "New session"
		}
		asOfSeq := len(rec.Events)
		isBlank := len(rec.Events) == 0
		modelName := rec.Model
		if modelName == "" {
			modelName = "deepseek-chat"
		}
		rec.mu.Unlock()

		return map[string]any{
			"asOfSeq": asOfSeq,
			"values": map[string]any{
				"title": title,
				"permissions": map[string]any{
					"currentValue": "auto",
				},
				"modelSelection": map[string]any{
					"lastUsed": map[string]any{
						"provider": "deepseek",
						"model":    modelName,
					},
					"next": map[string]any{
						"provider": "deepseek",
						"model":    modelName,
					},
				},
				"sessionListMetadata": map[string]any{
					"blank": isBlank,
				},
				"goal": goalProjection(rec),
			},
		}, nil

	case "session/prompt":
		reqMap, _ := args["request"].(map[string]any)
		sIDStr, _ := reqMap["sessionId"].(string)
		reqId, _ := reqMap["requestId"].(string)
		if reqId == "" {
			reqId = uuid.MustV4()
		}

		s.sessionsMu.RLock()
		rec, ok := s.sessions[sIDStr]
		s.sessionsMu.RUnlock()
		if !ok {
			rec = s.getOrCreateSession(sIDStr, "")
		}

		// Extract prompt text and resolve staged files into app-owned workspace
		// references that the model and native tools can use.
		var promptText string
		var attached []string
		if contentList, ok := reqMap["content"].([]any); ok {
			for _, item := range contentList {
				if imap, ok := item.(map[string]any); ok {
					switch imap["type"] {
					case "text":
						if t, ok := imap["text"].(string); ok {
							promptText += t
						}
					case "file":
						receiptID, _ := imap["receiptId"].(string)
						if upload := s.uploads.get(receiptID); upload != nil && upload.SessionID == sIDStr {
							attached = append(attached, fmt.Sprintf("[Attached file: %s at %s]", upload.Name, upload.RelativePath))
						}
					}
				}
			}
		}
		if len(attached) > 0 {
			if promptText != "" {
				promptText += "\n\n"
			}
			promptText += strings.Join(attached, "\n")
		}

		if strings.TrimSpace(promptText) == "" {
			return nil, &RPCError{Code: "gateway/bad-request", Message: "prompt content is empty"}
		}

		rec.mu.Lock()
		if _, duplicate := rec.Requests[reqId]; duplicate {
			rec.mu.Unlock()
			return map[string]any{"requestId": reqId}, nil
		}
		wasRunning := rec.Cancel != nil
		rec.RunID = reqId
		rec.Requests[reqId] = struct{}{}
		rec.mu.Unlock()

		rec.mu.Lock()
		needsTitle := rec.Title == ""
		rec.mu.Unlock()
		if needsTitle {
			title := strings.TrimSpace(promptText)
			if runes := []rune(title); len(runes) > 24 {
				title = string(runes[:24]) + "..."
			}
			_, _ = rec.Log.Append(session.SessionTitleData{Title: title, Source: "fallback"})
		}
		runtimeAgent := s.ensureAgent(rec)
		rec.mu.Lock()
		rec.Cancel = func() { runtimeAgent.Cancel(agent.CancelUser) }
		rec.mu.Unlock()
		var enqueueErr error
		if wasRunning || runtimeAgent.Status() == agent.StatusRunning {
			enqueueErr = runtimeAgent.FollowupRequest(promptText, reqId)
		} else {
			enqueueErr = runtimeAgent.RunRequest(promptText, reqId)
		}
		if enqueueErr != nil {
			rec.mu.Lock()
			delete(rec.Requests, reqId)
			if !wasRunning {
				rec.Cancel = nil
				rec.RunID = ""
			}
			rec.mu.Unlock()
			return nil, &RPCError{Code: "session/queue-full", Message: enqueueErr.Error()}
		}
		return map[string]any{"requestId": reqId}, nil

	case "session/cancel":
		reqMap, _ := args["request"].(map[string]any)
		sIDStr, _ := reqMap["sessionId"].(string)

		s.sessionsMu.RLock()
		rec, ok := s.sessions[sIDStr]
		s.sessionsMu.RUnlock()

		if ok {
			rec.mu.Lock()
			cancel := rec.Cancel
			rec.mu.Unlock()
			if cancel != nil {
				cancel()
			}
		}
		return map[string]any{"accepted": true}, nil

	case "session/rename":
		reqMap, _ := args["request"].(map[string]any)
		sIDStr, _ := reqMap["sessionId"].(string)
		title, _ := reqMap["title"].(string)

		s.sessionsMu.RLock()
		rec, ok := s.sessions[sIDStr]
		s.sessionsMu.RUnlock()

		if ok {
			_, _ = rec.Log.Append(session.SessionTitleData{Title: title, Source: "user"})
		}
		return map[string]any{"title": title}, nil

	case "session/fork":
		return s.forkSession(ctx, args)

	case "session/selectModel":
		reqMap, _ := args["request"].(map[string]any)
		sIDStr, _ := reqMap["sessionId"].(string)
		provider, _ := reqMap["provider"].(string)
		model, _ := reqMap["model"].(string)
		reasoningEffort, _ := reqMap["reasoningEffort"].(string)

		s.sessionsMu.RLock()
		rec, ok := s.sessions[sIDStr]
		s.sessionsMu.RUnlock()

		if ok {
			rec.mu.Lock()
			rec.Model = model
			runtimeAgent := rec.Agent
			rec.mu.Unlock()
			if runtimeAgent != nil {
				runtimeAgent.SetModel(provider, model, reasoningEffort)
			}
			_, _ = rec.Log.Append(session.ModelSelectionData{Provider: provider, Model: model, ReasoningEffort: reasoningEffort})
		}

		selected := map[string]any{
			"provider": provider,
			"model":    model,
		}
		if reasoningEffort != "" {
			selected["reasoningEffort"] = reasoningEffort
		}
		return map[string]any{
			"selected": selected,
		}, nil

	case "session/page":
		reqMap, _ := args["request"].(map[string]any)
		var sIDStr string
		if reqMap != nil {
			sIDStr, _ = reqMap["sessionId"].(string)
			if sIDStr == "" {
				if addr, ok := reqMap["address"].(map[string]any); ok {
					sIDStr, _ = addr["sessionId"].(string)
				}
			}
		}
		if sIDStr == "" {
			sIDStr, _ = args["sessionId"].(string)
		}

		s.sessionsMu.RLock()
		rec, ok := s.sessions[sIDStr]
		s.sessionsMu.RUnlock()

		if !ok {
			return map[string]any{"records": []any{}, "hasMore": false}, nil
		}

		rec.mu.Lock()
		throughSeq := numberArg(reqMap, "throughSeq", len(rec.Events))
		if throughSeq <= 0 || throughSeq > len(rec.Events) {
			throughSeq = len(rec.Events)
		}
		maxSeq := throughSeq
		if beforeSeq := numberArg(reqMap, "beforeSeq", 0); beforeSeq > 0 && beforeSeq-1 < maxSeq {
			maxSeq = beforeSeq - 1
		}
		eligible := make([]map[string]any, 0, maxSeq)
		for _, ev := range rec.Events {
			seq, _ := ev["seq"].(int)
			if seq <= 0 || seq > maxSeq {
				continue
			}
			eligible = append(eligible, ev)
		}
		maxMessages := numberArg(reqMap, "maxMessages", 100)
		start := 0
		if maxMessages > 0 {
			messages := 0
			for i := len(eligible) - 1; i >= 0; i-- {
				if eventType, _ := eligible[i]["type"].(string); eventType == "user/message" || eventType == "assistant/message" {
					messages++
					if messages > maxMessages {
						start = i + 1
						break
					}
				}
			}
		}
		selected := eligible[start:]
		records := make([]map[string]any, len(selected))
		for i, ev := range selected {
			records[i] = map[string]any{
				"seq":   ev["seq"],
				"event": ev,
			}
		}
		rec.mu.Unlock()

		return map[string]any{
			"records": records,
			"hasMore": start > 0 || (len(selected) > 0 && selected[0]["seq"] != 1),
		}, nil

	case "session/modelCatalog":
		return map[string]any{
			"default": map[string]any{
				"provider": "deepseek",
				"model":    "deepseek-chat",
			},
			"routableProviders": []string{"deepseek"},
			"groups": []map[string]any{
				{
					"id":   "deepseek",
					"name": "DeepSeek",
					"models": []map[string]any{
						{
							"id":          "deepseek-chat",
							"name":        "DeepSeek-V3",
							"description": "DeepSeek-V3 high-performance flagship model",
						},
						{
							"id":          "deepseek-reasoner",
							"name":        "DeepSeek-R1",
							"description": "DeepSeek-R1 deep reasoning model",
							"reasoning": map[string]any{
								"efforts": []map[string]any{
									{"id": "low", "name": "Low", "description": "Fast thinking"},
									{"id": "high", "name": "High", "description": "Deep reasoning"},
								},
								"defaultEffort": "high",
							},
						},
					},
				},
			},
			"failures": []map[string]any{},
		}, nil

	case "credentials/describe":
		refs, _ := args["refs"].([]any)
		res := make(map[string]any)
		for _, r := range refs {
			if rStr, ok := r.(string); ok {
				val, ok := s.credStore.Resolve(ctx, brand.NewCredentialRef(rStr))
				res[rStr] = map[string]any{
					"configured": ok && val != "",
					"writable":   true,
					"source":     "file",
				}
			}
		}
		return res, nil

	case "credentials/set":
		refStr, _ := args["ref"].(string)
		valStr, _ := args["value"].(string)
		if refStr != "" {
			_ = s.credStore.Set(ctx, brand.NewCredentialRef(refStr), valStr)
		}
		return map[string]any{}, nil

	case "credentials/unset":
		refStr, _ := args["ref"].(string)
		if refStr != "" {
			_ = s.credStore.Unset(ctx, brand.NewCredentialRef(refStr))
		}
		return map[string]any{}, nil

	case "permissionPresets/catalog":
		return map[string]any{
			"options": []map[string]any{
				{"value": "read-only", "name": "Read Only"},
				{"value": "auto", "name": "Auto"},
				{"value": "danger-full-access", "name": "Full Access"},
			},
			"defaultOptions": []any{},
			"defaultPreset":  "auto",
		}, nil

	case "workspace/archiveSession", "workspace/unarchiveSession":
		reqMap, _ := args["request"].(map[string]any)
		sessionID, _ := reqMap["sessionId"].(string)
		if sessionID == "" {
			return nil, &RPCError{Code: "gateway/arguments-invalid", Message: "sessionId is required"}
		}
		return map[string]any{
			"archivedSessionIds": s.setSessionArchived(sessionID, endpoint == "workspace/archiveSession"),
		}, nil

	case "workspace/create", "workspace/rename", "workspace/delete", "workspace/insertBefore", "workspace/pinSession", "workspace/unpinSession", "directoryPicker/list", "directoryPicker/createDirectory":
		return s.dispatchWorkspaceStateRPC(endpoint, args)

	case "llm/listProviders":
		return []map[string]any{{"id": "deepseek", "name": "DeepSeek"}}, nil

	case "llm/listConfigurableProviders", "commands/list", "pluginManager/listBundles", "pluginManager/listPlugins":
		return []any{}, nil

	case "skills/list":
		return s.listSkills(args)

	case "agentPresets/list":
		return map[string]any{"presets": []map[string]any{{
			"id": "native", "isDefault": true, "name": "Native Agent",
			"description": "Standalone Android-native DSH agent", "order": 0,
		}}}, nil

	case "agentPresets/read":
		preset, _ := args["agentPreset"].(string)
		if preset != "native" {
			return nil, &RPCError{Code: "agent-preset/not-found", Message: "agent preset not found"}
		}
		return map[string]any{
			"agentPreset": "native", "name": "Native Agent",
			"description": "Standalone Android-native DSH agent",
			"content":     "id: native\nname: Native Agent\ndescription: Standalone Android-native DSH agent\n",
		}, nil

	case "agentPresets/select":
		sessionID, _ := args["agentId"].(string)
		preset, _ := args["agentPreset"].(string)
		if preset != "native" {
			return nil, &RPCError{Code: "agent-preset/not-found", Message: "agent preset not found"}
		}
		rec := s.sessionByID(sessionID)
		if rec == nil {
			return nil, &RPCError{Code: "session/not-found", Message: "session not found"}
		}
		for _, event := range rec.Log.Events() {
			if event.Type == session.EventTurnStart {
				return nil, &RPCError{Code: "agent-preset/session-not-blank", Message: "agent preset can only change before the first turn"}
			}
		}
		rec.mu.Lock()
		rec.AgentPreset = preset
		rec.mu.Unlock()
		_, _ = rec.Log.Append(session.AgentPresetChangeData{Preset: preset})
		return preset, nil

	case "settings/describe":
		return map[string]any{"writable": false, "namespaces": []any{}}, nil

	case "pluginInventory/list":
		return map[string]any{"managementAvailable": false, "entries": []any{}, "agentPresets": []any{}}, nil

	case "commands/execute":
		sessionID, _ := args["agentId"].(string)
		line, _ := args["line"].(string)
		rec := s.sessionByID(sessionID)
		if rec == nil {
			return nil, &RPCError{Code: "session/not-found", Message: "session not found"}
		}
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) == 2 && fields[0] == "/permission" {
			preset := fields[1]
			if preset != "read-only" && preset != "auto" && preset != "danger-full-access" {
				return nil, &RPCError{Code: "gateway/arguments-invalid", Message: "unknown permission preset"}
			}
			_, _ = rec.Log.Append(session.PresetChangeData{Preset: preset})
			return map[string]any{"commandId": uuid.MustV4(), "result": map[string]any{"kind": "success", "text": "Permission preset: " + preset}}, nil
		}
		return nil, &RPCError{Code: "commands/not-found", Message: "native engine does not implement this command"}

	case "$events/result":
		eventID, _ := args["eventId"].(string)
		outcome, _ := args["outcome"].(map[string]any)
		if eventID != "" && outcome != nil {
			s.resolveHostEvent(eventID, outcome)
		}
		return map[string]any{}, nil

	case "workspaceFiles/list", "workspaceFiles/stat", "workspaceFiles/read", "workspaceFiles/readBytes":
		return s.dispatchWorkspaceRPC(endpoint, args)

	case "fileUploads/upload":
		return s.dispatchInlineUpload(args)

	case "session/attachment":
		return s.dispatchAttachment(args)

	case "terminal/environment", "terminal/shells", "terminal/list", "terminal/create", "terminal/rename", "terminal/close", "terminal/write", "terminal/resize":
		return s.dispatchTerminalRPC(ctx, endpoint, args)

	case "goals/get", "goals/create", "goals/edit", "goals/pause", "goals/resume", "goals/complete", "goals/clear":
		return s.dispatchGoalRPC(endpoint, args)

	case "fileReferences/list", "sessionReferenceResolver/candidates":
		return s.dispatchReferences(endpoint, args)

	case "session/search":
		query, _ := args["query"].(string)
		if query == "" {
			if rMap, ok := args["request"].(map[string]any); ok {
				query, _ = rMap["query"].(string)
			}
		}
		matches, err := s.dbStore.Search(ctx, query)
		if err != nil {
			return map[string]any{"items": []any{}, "hasMore": false}, nil
		}
		items := make([]map[string]any, 0, len(matches))
		for _, m := range matches {
			items = append(items, map[string]any{
				"sessionId": m.SessionID,
				"snippet":   m.Text,
			})
		}
		return map[string]any{"items": items, "hasMore": false}, nil

	default:
		return nil, &RPCError{Code: "gateway/method-not-found", Message: "native engine does not implement " + endpoint}
	}
}

func (s *Server) sessionByID(sessionID string) *SessionRecord {
	s.sessionsMu.RLock()
	defer s.sessionsMu.RUnlock()
	return s.sessions[sessionID]
}

func (s *Server) close() {
	s.sessionsMu.RLock()
	records := make([]*SessionRecord, 0, len(s.sessions))
	for _, rec := range s.sessions {
		records = append(records, rec)
	}
	s.sessionsMu.RUnlock()
	for _, rec := range records {
		rec.mu.Lock()
		runtimeAgent := rec.Agent
		rec.mu.Unlock()
		if runtimeAgent != nil {
			runtimeAgent.Dispose()
		}
		if rec.stopLog != nil {
			rec.stopLog()
		}
	}
	s.hostMu.Lock()
	approvals := make([]*pendingApproval, 0, len(s.pendingApprovals))
	for _, pending := range s.pendingApprovals {
		approvals = append(approvals, pending)
	}
	s.pendingApprovals = map[string]*pendingApproval{}
	s.hostMu.Unlock()
	for _, pending := range approvals {
		pending.once.Do(func() { pending.result <- false })
	}
	s.closeAllTerminals()
}

// Broadcast an event to all subscribers of a session and persist it.
func (s *Server) broadcastEvent(rec *SessionRecord, evType string, evData any) map[string]any {
	rec.mu.Lock()
	defer rec.mu.Unlock()

	seq := len(rec.Events) + 1
	now := time.Now()
	rec.UpdatedAt = now

	evMap := map[string]any{
		"seq":  seq,
		"time": now.UnixMilli(),
		"type": evType,
		"data": evData,
	}
	rec.Events = append(rec.Events, evMap)

	// Persist to SQLite
	sID := rec.ID
	evJson, err := json.Marshal(evMap)
	if err == nil {
		var sEv session.SessionEvent
		if sEv.UnmarshalJSON(evJson) == nil {
			_ = s.dbStore.Append(context.Background(), sID, sEv)
		}
	}

	frame := map[string]any{
		"type":  "item",
		"value": map[string]any{"type": "event", "event": evMap},
	}
	frameBytes, _ := json.Marshal(frame)

	for _, ch := range rec.Subs {
		select {
		case ch <- frameBytes:
		default:
		}
	}

	return evMap
}

func (s *Server) broadcastAssistantFrame(rec *SessionRecord, frame map[string]any) {
	rec.mu.Lock()
	defer rec.mu.Unlock()

	msg := map[string]any{
		"type":  "item",
		"value": map[string]any{"type": "assistant-stream", "frame": frame},
	}
	frameBytes, _ := json.Marshal(msg)

	for _, ch := range rec.Subs {
		select {
		case ch <- frameBytes:
		default:
		}
	}
}

func (s *Server) executeTurn(ctx context.Context, rec *SessionRecord, reqId string, prompt string) {
	rec.mu.Lock()
	turnIdx := 0
	for _, ev := range rec.Events {
		if t, _ := ev["type"].(string); t == "turn/start" {
			turnIdx++
		}
	}
	modelToUse := rec.Model
	if modelToUse == "" {
		modelToUse = "deepseek-chat"
	}
	rec.mu.Unlock()

	defer func() {
		rec.mu.Lock()
		if rec.RunID == reqId {
			rec.Cancel = nil
			rec.RunID = ""
		}
		rec.mu.Unlock()
	}()

	// 1. turn/start
	s.broadcastEvent(rec, "turn/start", map[string]any{"turn": turnIdx})

	// 2. user/message
	userMsgId := uuid.MustV4()
	s.broadcastEvent(rec, "user/message", map[string]any{
		"id":   userMsgId,
		"role": "user",
		"content": []map[string]any{
			{"type": "text", "text": prompt},
		},
		"source": map[string]any{
			"kind":  "user",
			"rpcId": reqId,
		},
	})

	// Set title if first turn
	var generatedTitle string
	rec.mu.Lock()
	if rec.Title == "" {
		t := prompt
		if len([]rune(t)) > 24 {
			t = string([]rune(t)[:24]) + "..."
		}
		rec.Title = t
		generatedTitle = t
	}
	rec.mu.Unlock()
	if generatedTitle != "" {
		s.broadcastEvent(rec, "session/title", map[string]any{"title": generatedTitle})
	}

	// 3. step/start
	s.broadcastEvent(rec, "step/start", map[string]any{"turn": turnIdx, "step": 1})

	// 4. Stream response
	attemptId := uuid.MustV4()
	var revision int64 = 1
	rec.mu.Lock()
	startedAfterSeq := len(rec.Events)
	rec.mu.Unlock()

	s.broadcastAssistantFrame(rec, map[string]any{
		"type":            "start",
		"attemptId":       attemptId,
		"revision":        revision,
		"turn":            turnIdx,
		"step":            1,
		"startedAfterSeq": startedAfterSeq,
	})

	var fullText strings.Builder
	var fullReasoning strings.Builder
	var chunkIdx int64 = 0

	// Check if API key is configured
	apiKey, hasKey := s.credStore.Resolve(ctx, brand.NewCredentialRef("DEEPSEEK_API_KEY"))
	if !hasKey || apiKey == "" {
		notice := "Welcome to DSH Native! To chat with DeepSeek-V3 or DeepSeek-R1, please configure your DEEPSEEK_API_KEY in Settings."
		fullText.WriteString(notice)
		revision++
		chunkIdx++
		s.broadcastAssistantFrame(rec, map[string]any{
			"type":      "chunk",
			"attemptId": attemptId,
			"revision":  revision,
			"index":     chunkIdx,
			"time":      time.Now().UnixMilli(),
			"chunk": map[string]any{
				"type": "text-delta",
				"text": notice,
			},
		})
	} else {
		// Prepare history
		rec.mu.Lock()
		eventsCopy := make([]map[string]any, len(rec.Events))
		copy(eventsCopy, rec.Events)
		rec.mu.Unlock()

		messages := make([]llm.Message, 0)
		for _, ev := range eventsCopy {
			t, _ := ev["type"].(string)
			data, _ := ev["data"].(map[string]any)
			if t == "user/message" {
				if text := contentText(data["content"]); text != "" {
					messages = append(messages, llm.NewUserMessage(text))
				}
			} else if t == "assistant/message" {
				if msg, ok := data["message"].(map[string]any); ok {
					if text := contentText(msg["content"]); text != "" {
						messages = append(messages, llm.NewAssistantText(text))
					}
				}
			}
		}

		_, chatErr := s.adapter.Chat(ctx, llm.ChatRequest{
			Model:    modelToUse,
			Messages: messages,
		}, func(chunk llm.StreamChunk) {
			if ctx.Err() != nil {
				return
			}
			revision++
			chunkIdx++
			switch chunk.Kind {
			case llm.ChunkText:
				fullText.WriteString(chunk.Text)
				s.broadcastAssistantFrame(rec, map[string]any{
					"type":      "chunk",
					"attemptId": attemptId,
					"revision":  revision,
					"index":     chunkIdx,
					"time":      time.Now().UnixMilli(),
					"chunk": map[string]any{
						"type": "text-delta",
						"text": chunk.Text,
					},
				})
			case llm.ChunkReasoning:
				fullReasoning.WriteString(chunk.Reasoning)
				s.broadcastAssistantFrame(rec, map[string]any{
					"type":      "chunk",
					"attemptId": attemptId,
					"revision":  revision,
					"index":     chunkIdx,
					"time":      time.Now().UnixMilli(),
					"chunk": map[string]any{
						"type": "reasoning-delta",
						"text": chunk.Reasoning,
					},
				})
			}
		})

		if chatErr != nil && ctx.Err() == nil {
			errText := fmt.Sprintf("\n\n[DeepSeek Error: %v]", chatErr)
			fullText.WriteString(errText)
			revision++
			chunkIdx++
			s.broadcastAssistantFrame(rec, map[string]any{
				"type":      "chunk",
				"attemptId": attemptId,
				"revision":  revision,
				"index":     chunkIdx,
				"time":      time.Now().UnixMilli(),
				"chunk": map[string]any{
					"type": "text-delta",
					"text": errText,
				},
			})
		}
	}

	// 5. assistant/message committed
	asstMsgId := uuid.MustV4()
	parts := make([]map[string]any, 0)
	if fullReasoning.Len() > 0 {
		parts = append(parts, map[string]any{"type": "reasoning", "text": fullReasoning.String()})
	}
	parts = append(parts, map[string]any{"type": "text", "text": fullText.String()})

	asstEv := s.broadcastEvent(rec, "assistant/message", map[string]any{
		"turn": turnIdx,
		"step": 1,
		"message": map[string]any{
			"id":      asstMsgId,
			"role":    "assistant",
			"content": parts,
			"source": map[string]any{
				"kind":     "model",
				"provider": "deepseek",
				"model":    modelToUse,
			},
		},
	})

	// End of assistant stream
	revision++
	chunkIdx++
	s.broadcastAssistantFrame(rec, map[string]any{
		"type":      "end",
		"attemptId": attemptId,
		"revision":  revision,
		"index":     chunkIdx,
		"outcome": map[string]any{
			"kind":      "committed",
			"eventType": "assistant/message",
			"seq":       asstEv["seq"],
		},
	})

	// 6. step/end
	s.broadcastEvent(rec, "step/end", map[string]any{"turn": turnIdx, "step": 1})

	// 7. turn/end
	reasonKind := "completed"
	if ctx.Err() != nil {
		reasonKind = "aborted"
	}
	s.broadcastEvent(rec, "turn/end", map[string]any{
		"turn": turnIdx,
		"reason": map[string]any{
			"kind": reasonKind,
		},
	})
}

// contentText accepts values decoded from JSON ([]any) and values created
// inside this process ([]map[string]any). Without this normalization, a newly
// appended prompt disappears from the first model request and only works after
// the session has been reloaded from SQLite.
func contentText(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err != nil {
		return ""
	}
	var text strings.Builder
	for _, part := range parts {
		if part.Type == "text" {
			text.WriteString(part.Text)
		}
	}
	return text.String()
}

func (s *Server) handleMux(w http.ResponseWriter, r *http.Request) {
	if !s.isCookieAuthorized(r) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("websocket upgrade error: %v", err)
		return
	}
	defer conn.Close()

	var writeMu sync.Mutex
	safeWrite := func(v any) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return conn.WriteJSON(v)
	}

	type subscription struct {
		rec *SessionRecord
		key string
		ch  chan []byte
	}
	type workspaceSubscription struct {
		key string
		ch  chan map[string]any
	}
	subscriptions := make(map[string]subscription)
	workspaceSubscriptions := make(map[string]workspaceSubscription)
	hostSubscriptions := make(map[string]string)
	watchCancellations := make(map[string]context.CancelFunc)
	terminalAttachments := make(map[string]terminalAttachment)
	terminalRetains := make(map[string]context.CancelFunc)
	controlSubscriptions := make(map[string]string)
	removeSubscription := func(streamID string) {
		if sub, ok := subscriptions[streamID]; ok {
			delete(subscriptions, streamID)
			sub.rec.mu.Lock()
			delete(sub.rec.Subs, sub.key)
			close(sub.ch)
			sub.rec.mu.Unlock()
		}
		if sub, ok := workspaceSubscriptions[streamID]; ok {
			delete(workspaceSubscriptions, streamID)
			s.workspaceMu.Lock()
			delete(s.workspaceSubs, sub.key)
			close(sub.ch)
			s.workspaceMu.Unlock()
		}
		if key, ok := hostSubscriptions[streamID]; ok {
			delete(hostSubscriptions, streamID)
			s.removeHostSubscriber(key)
		}
		if cancel, ok := watchCancellations[streamID]; ok {
			delete(watchCancellations, streamID)
			cancel()
		}
		if attachment, ok := terminalAttachments[streamID]; ok {
			delete(terminalAttachments, streamID)
			attachment.record.detach(attachment.key)
		}
		if cancel, ok := terminalRetains[streamID]; ok {
			delete(terminalRetains, streamID)
			cancel()
		}
		if key, ok := controlSubscriptions[streamID]; ok {
			delete(controlSubscriptions, streamID)
			s.removeControlSubscriber(key)
		}
	}
	defer func() {
		for streamID := range subscriptions {
			removeSubscription(streamID)
		}
		for streamID := range workspaceSubscriptions {
			removeSubscription(streamID)
		}
		for streamID := range hostSubscriptions {
			removeSubscription(streamID)
		}
		for streamID := range watchCancellations {
			removeSubscription(streamID)
		}
		for streamID := range terminalAttachments {
			removeSubscription(streamID)
		}
		for streamID := range terminalRetains {
			removeSubscription(streamID)
		}
		for streamID := range controlSubscriptions {
			removeSubscription(streamID)
		}
	}()

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			break
		}

		var frame map[string]any
		if err := json.Unmarshal(msg, &frame); err != nil {
			continue
		}

		fType, _ := frame["type"].(string)
		streamId, _ := frame["streamId"].(string)

		switch fType {
		case "open":
			endpoint, _ := frame["endpoint"].(string)
			payload, _ := frame["payload"].(map[string]any)

			if endpoint == "$events" {
				removeSubscription(streamId)
				connectionClientID := uuid.MustV4()
				key, updates, pending := s.addHostSubscriber()
				hostSubscriptions[streamId] = key
				_ = safeWrite(map[string]any{
					"type":     "item",
					"streamId": streamId,
					"value": map[string]any{
						"type":     "ready",
						"clientId": connectionClientID,
						"host": map[string]any{
							"home": s.homeDir,
						},
					},
				})
				for _, request := range pending {
					_ = safeWrite(map[string]any{"type": "item", "streamId": streamId, "value": request})
				}
				go func(stream string, channel chan map[string]any) {
					for update := range channel {
						if err := safeWrite(map[string]any{"type": "item", "streamId": stream, "value": update}); err != nil {
							return
						}
					}
				}(streamId, updates)
			} else if endpoint == "session/follow" {
				removeSubscription(streamId)
				var targetSessionId string
				args, _ := payload["args"].(map[string]any)
				req, _ := args["request"].(map[string]any)
				// Accept the pre-client test shape too, while the production shape is
				// payload.args.request (DshMux wraps every logical stream's args).
				if req == nil {
					req, _ = payload["request"].(map[string]any)
				}
				if req != nil {
					targetSessionId, _ = req["sessionId"].(string)
					if targetSessionId == "" {
						if addr, ok := req["address"].(map[string]any); ok {
							targetSessionId, _ = addr["sessionId"].(string)
						} else if addrStr, ok := req["address"].(string); ok {
							targetSessionId = addrStr
						}
					}
				}
				if targetSessionId == "" && args != nil {
					targetSessionId, _ = args["sessionId"].(string)
					if targetSessionId == "" {
						if addr, ok := args["address"].(map[string]any); ok {
							targetSessionId, _ = addr["sessionId"].(string)
						} else if addrStr, ok := args["address"].(string); ok {
							targetSessionId = addrStr
						}
					}
				}
				if targetSessionId == "" {
					_ = safeWrite(map[string]any{
						"type": "error", "streamId": streamId,
						"error": map[string]any{"code": "gateway/arguments-invalid", "message": "sessionId is required"},
					})
					continue
				}

				rec := s.getOrCreateSession(targetSessionId, "")
				rec.mu.Lock()
				records := make([]map[string]any, len(rec.Events))
				for i, ev := range rec.Events {
					records[i] = map[string]any{
						"seq":   ev["seq"],
						"event": ev,
					}
				}
				title := rec.Title
				if title == "" {
					title = "New session"
				}
				createdAtMs := rec.CreatedAt.UnixMilli()
				cursor := len(rec.Events)
				isBlank := len(rec.Events) == 0
				modelName := rec.Model
				if modelName == "" {
					modelName = "deepseek-chat"
				}

				// Register sub
				ch := make(chan []byte, 100)
				subKey := uuid.MustV4()
				rec.Subs[subKey] = ch
				subscriptions[streamId] = subscription{rec: rec, key: subKey, ch: ch}
				rec.mu.Unlock()

				// Send snapshot
				_ = safeWrite(map[string]any{
					"type":     "item",
					"streamId": streamId,
					"value": map[string]any{
						"type":      "snapshot",
						"sessionId": targetSessionId,
						"header": map[string]any{
							"id":        targetSessionId,
							"version":   4,
							"createdAt": createdAtMs,
							"isSeeded":  false,
						},
						"cursor":  cursor,
						"records": records,
						"hasMore": false,
						"projections": map[string]any{
							"asOfSeq": cursor,
							"values": map[string]any{
								"title": title,
								"permissions": map[string]any{
									"currentValue": "auto",
								},
								"modelSelection": map[string]any{
									"lastUsed": map[string]any{
										"provider": "deepseek",
										"model":    modelName,
									},
									"next": map[string]any{
										"provider": "deepseek",
										"model":    modelName,
									},
								},
								"sessionListMetadata": map[string]any{
									"blank": isBlank,
								},
								"goal": goalProjection(rec),
							},
						},
					},
				})

				// Pump session events to stream
				go func(stream string, sub string, c chan []byte, r *SessionRecord) {
					for data := range c {
						var parsed map[string]any
						if json.Unmarshal(data, &parsed) == nil {
							parsed["streamId"] = stream
							if err := safeWrite(parsed); err != nil {
								break
							}
						}
					}
				}(streamId, subKey, ch, rec)
			} else if endpoint == "workspace/follow" {
				removeSubscription(streamId)
				key := uuid.MustV4()
				ch := make(chan map[string]any, 16)
				s.workspaceMu.Lock()
				s.workspaceSubs[key] = ch
				baseline := s.workspaceBaselineLocked()
				s.workspaceMu.Unlock()
				workspaceSubscriptions[streamId] = workspaceSubscription{key: key, ch: ch}
				_ = safeWrite(map[string]any{
					"type": "item", "streamId": streamId,
					"value": baseline,
				})
				go func(stream string, updates chan map[string]any) {
					for update := range updates {
						if err := safeWrite(map[string]any{"type": "item", "streamId": stream, "value": update}); err != nil {
							break
						}
					}
				}(streamId, ch)
			} else if endpoint == "session/control" {
				removeSubscription(streamId)
				key, updates := s.addControlSubscriber()
				controlSubscriptions[streamId] = key
				_ = safeWrite(map[string]any{
					"type": "item", "streamId": streamId, "value": s.sessionControlBaseline(),
				})
				go func(stream string, channel chan map[string]any) {
					for update := range channel {
						if err := safeWrite(map[string]any{"type": "item", "streamId": stream, "value": update}); err != nil {
							return
						}
					}
				}(streamId, updates)
			} else if endpoint == "workspaceFiles/changes" {
				removeSubscription(streamId)
				args, _ := payload["args"].(map[string]any)
				if args == nil {
					args = payload
				}
				sessionID, _ := args["workspaceFileScopeId"].(string)
				path, _ := args["path"].(string)
				watchCtx, cancel := context.WithCancel(r.Context())
				watchCancellations[streamId] = cancel
				go func(stream, targetSession, targetPath string) {
					err := s.watchWorkspace(watchCtx, targetSession, targetPath, func(value map[string]any) error {
						return safeWrite(map[string]any{"type": "item", "streamId": stream, "value": value})
					})
					if err != nil && watchCtx.Err() == nil {
						_ = safeWrite(map[string]any{
							"type": "error", "streamId": stream,
							"error": map[string]any{"code": string(nativefs.Code(err)), "message": err.Error()},
						})
					}
				}(streamId, sessionID, path)
			} else if endpoint == "terminal/attach" || endpoint == "terminal/follow" {
				removeSubscription(streamId)
				args, _ := payload["args"].(map[string]any)
				if args == nil {
					args = payload
				}
				sessionID, _ := args["agentId"].(string)
				id, _ := args["id"].(string)
				terminal := s.terminal(sessionID, id)
				if terminal == nil {
					_ = safeWrite(map[string]any{
						"type": "error", "streamId": streamId,
						"error": map[string]any{"code": "terminal/unavailable", "message": "terminal not found"},
					})
					continue
				}
				attachmentKey, _ := args["attachmentId"].(string)
				if attachmentKey == "" {
					attachmentKey = newTerminalAttachmentID()
				}
				attachment := terminal.attach(attachmentKey)
				terminalAttachments[streamId] = attachment
				follow := endpoint == "terminal/follow"
				go func(stream string, attached terminalAttachment, stampController bool) {
					for value := range attached.ch {
						if value["type"] == "__end" {
							_ = safeWrite(map[string]any{"type": "end", "streamId": stream})
							return
						}
						if stampController {
							value = terminalFrameWithController(value, attached.key)
						}
						if err := safeWrite(map[string]any{"type": "item", "streamId": stream, "value": value}); err != nil {
							return
						}
					}
				}(streamId, attachment, follow)
			} else if endpoint == "terminal/retain" {
				removeSubscription(streamId)
				args, _ := payload["args"].(map[string]any)
				if args == nil {
					args = payload
				}
				sessionID, _ := args["sessionId"].(string)
				id, _ := args["id"].(string)
				terminal := s.terminal(sessionID, id)
				if terminal == nil {
					_ = safeWrite(map[string]any{
						"type": "error", "streamId": streamId,
						"error": map[string]any{"code": "terminal/unavailable", "message": "terminal not found"},
					})
					continue
				}
				retainCtx, cancel := context.WithCancel(r.Context())
				terminalRetains[streamId] = cancel
				_ = safeWrite(map[string]any{"type": "item", "streamId": streamId, "value": map[string]any{"type": "retained"}})
				go func(stream string, target *terminalRecord) {
					select {
					case <-target.done:
						_ = safeWrite(map[string]any{"type": "end", "streamId": stream})
					case <-retainCtx.Done():
					}
				}(streamId, terminal)
			} else {
				_ = safeWrite(map[string]any{
					"type": "error", "streamId": streamId,
					"error": map[string]any{"code": "gateway/method-not-found", "message": "native engine does not implement " + endpoint},
				})
			}

		case "item":
			attachment, ok := terminalAttachments[streamId]
			if !ok {
				continue
			}
			value, _ := frame["value"].(map[string]any)
			sequence := numberArg(value, "sequence", 0)
			var commandErr error
			switch value["type"] {
			case "write":
				data, _ := value["data"].(string)
				commandErr = attachment.record.writeFrom(attachment.key, data)
			case "resize":
				commandErr = attachment.record.resizeFrom(attachment.key, numberArg(value, "cols", 80), numberArg(value, "rows", 24))
			default:
				commandErr = fmt.Errorf("unknown terminal input command")
			}
			ack := map[string]any{"type": "ack", "sequence": sequence}
			if commandErr != nil {
				if strings.Contains(commandErr.Error(), "read-only") {
					ack["refusal"] = "read-only"
				} else {
					ack["refusal"] = "not-running"
				}
			}
			_ = safeWrite(map[string]any{"type": "item", "streamId": streamId, "value": ack})

		case "cancel", "end":
			removeSubscription(streamId)
		}
	}
}

func (s *Server) handleSessionDeletionCapability(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.isCookieAuthorized(r) {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"version":1,"supported":true}`))
}

func (s *Server) handleSessionDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.isCookieAuthorized(r) {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	var req struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.SessionID == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"code":"invalid-argument","message":"sessionId is required"}}`))
		return
	}

	sIDStr := req.SessionID
	s.sessionsMu.Lock()
	if rec, ok := s.sessions[sIDStr]; ok {
		rec.mu.Lock()
		if rec.Cancel != nil {
			rec.Cancel()
			rec.Cancel = nil
		}
		rec.mu.Unlock()
		delete(s.sessions, sIDStr)
	}
	s.sessionsMu.Unlock()
	s.closeSessionTerminals(sIDStr)
	s.removeSessionFromWorkspaceState(sIDStr)

	_ = s.dbStore.Delete(r.Context(), brand.NewSessionID(sIDStr))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"deleted":   true,
		"sessionId": sIDStr,
	})
}

func main() {
	portFlag := flag.Int("port", 3080, "server port")
	homeFlag := flag.String("home", "", "dsh home directory")
	workspaceFlag := flag.String("workspace", "", "native workspace root")
	versionFlag := flag.Bool("version", false, "print engine build information")
	flag.Parse()
	if *versionFlag {
		encoded, _ := json.Marshal(currentBuildInfo())
		fmt.Println(string(encoded))
		return
	}

	homeDir := *homeFlag
	if homeDir == "" {
		if envHome := os.Getenv("DSH_HOME"); envHome != "" {
			homeDir = envHome
		} else {
			userHome, _ := os.UserHomeDir()
			homeDir = filepath.Join(userHome, ".dsh")
		}
	}

	port := *portFlag
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		// Ephemeral fallback
		ln, err = net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			log.Fatalf("failed to listen on port %d: %v", port, err)
		}
	}
	actualPort := ln.Addr().(*net.TCPAddr).Port

	workspaceDir := *workspaceFlag
	if workspaceDir == "" {
		workspaceDir = os.Getenv("DSH_WORKSPACE_ROOT")
	}
	if workspaceDir == "" {
		workspaceDir = filepath.Join(homeDir, "workspace")
	}
	server, err := newServerWithWorkspace(actualPort, homeDir, workspaceDir)
	if err != nil {
		log.Fatalf("failed to initialize server: %v", err)
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

	httpServer := &http.Server{
		Handler: mux,
	}

	// Notify Kotlin host on stdout
	fmt.Printf("dsh web: http://127.0.0.1:%d/?token=%s\n", actualPort, server.launchToken)
	_ = os.Stdout.Sync()

	go func() {
		if err := httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	server.close()
	_ = httpServer.Shutdown(ctx)
	_ = server.dbStore.Close()
}
