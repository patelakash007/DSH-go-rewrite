package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/JopenChen/dsh-go/pkg/brand"
	"github.com/JopenChen/dsh-go/pkg/uuid"
)

const (
	deepSeekUsageURL      = "https://platform.deepseek.com/usage"
	deepSeekTopUpURL      = "https://platform.deepseek.com/top_up"
	deepSeekAccountToken  = "DEEPSEEK_ACCOUNT_TOKEN"
	deepSeekAccountDevice = "DEEPSEEK_ACCOUNT_DEVICE_ID"
	accountAttemptTimeout = 10 * time.Minute
)

func deepSeekPlatformBase() string {
	if override := os.Getenv("DSH_DEEPSEEK_PLATFORM_ORIGIN"); override != "" {
		return strings.TrimRight(override, "/")
	}
	return "https://platform.deepseek.com"
}

type accountAttempt struct {
	ID             string
	Phase          string
	AuthorizeURL   string
	ExpiresAt      int64
	ErrorCode      string
	Verifier       string
	State          string
	AuthorizeID    string
	CallbackOrigin string
	LoginSource    string
	CreatedAt      time.Time
}

type accountState struct {
	mu       sync.Mutex
	attempt  *accountAttempt
	subs     map[string]chan map[string]any
	deviceID string
}

func newAccountState() *accountState {
	return &accountState{subs: map[string]chan map[string]any{}}
}

func (s *Server) accountToken() (string, bool) {
	token, configured := s.credStore.Resolve(context.Background(), brand.NewCredentialRef(deepSeekAccountToken))
	return token, configured && strings.TrimSpace(token) != ""
}

func (s *Server) accountView() map[string]any {
	status := "signed-out"
	if _, configured := s.accountToken(); configured {
		status = "credential-stored"
	}
	view := map[string]any{
		"status": status,
		"links":  map[string]any{"usageUrl": deepSeekUsageURL, "topUpUrl": deepSeekTopUpURL},
	}
	s.account.mu.Lock()
	if attempt := s.account.attempt; attempt != nil {
		view["attempt"] = accountAttemptView(attempt)
	}
	s.account.mu.Unlock()
	return view
}

func accountAttemptView(attempt *accountAttempt) map[string]any {
	value := map[string]any{
		"id": attempt.ID, "phase": attempt.Phase, "expiresAt": attempt.ExpiresAt,
	}
	if attempt.AuthorizeURL != "" {
		value["authorizeUrl"] = attempt.AuthorizeURL
	}
	if attempt.ErrorCode != "" {
		value["errorCode"] = attempt.ErrorCode
	}
	return value
}

func (s *Server) broadcastAccount() {
	s.account.mu.Lock()
	view := s.accountViewLocked()
	subs := make([]chan map[string]any, 0, len(s.account.subs))
	for _, channel := range s.account.subs {
		subs = append(subs, channel)
	}
	s.account.mu.Unlock()
	for _, channel := range subs {
		select {
		case channel <- view:
		default:
		}
	}
}

func (s *Server) accountViewLocked() map[string]any {
	status := "signed-out"
	if _, configured := s.accountToken(); configured {
		status = "credential-stored"
	}
	view := map[string]any{
		"status": status,
		"links":  map[string]any{"usageUrl": deepSeekUsageURL, "topUpUrl": deepSeekTopUpURL},
	}
	if s.account.attempt != nil {
		view["attempt"] = accountAttemptView(s.account.attempt)
	}
	return view
}

func (s *Server) dispatchAccountRPC(endpoint string, args map[string]any) (any, *RPCError) {
	switch endpoint {
	case "account/getState":
		return s.accountView(), nil
	case "account/startSignIn":
		return s.startAccountSignIn(args)
	case "account/cancelSignIn":
		attemptID, _ := args["attemptId"].(string)
		s.cancelAccountSignIn(attemptID)
		return s.accountView(), nil
	case "account/signOut":
		s.signOutAccount()
		return s.accountView(), nil
	case "account/getProfile":
		return s.accountProfile(), nil
	case "account/getBalance":
		return s.accountBalance(), nil
	case "account/hasRunningAccountTasks":
		return false, nil
	default:
		return nil, &RPCError{Code: "gateway/method-not-found", Message: "unknown account method"}
	}
}

func (s *Server) startAccountSignIn(args map[string]any) (any, *RPCError) {
	callbackOrigin, _ := args["callbackOrigin"].(string)
	loginSource, _ := args["loginSource"].(string)
	if loginSource == "" {
		loginSource = "web"
	}
	origin, err := normalizeCallbackOrigin(callbackOrigin)
	if err != nil {
		return nil, &RPCError{Code: "gateway/arguments-invalid", Message: err.Error()}
	}
	verifier, err := randomBase64URL(32)
	if err != nil {
		return nil, &RPCError{Code: "gateway/internal", Message: err.Error()}
	}
	state, err := randomBase64URL(32)
	if err != nil {
		return nil, &RPCError{Code: "gateway/internal", Message: err.Error()}
	}
	challenge := sha256.Sum256([]byte(verifier))
	attempt := &accountAttempt{
		ID: uuid.MustV4(), Phase: "initializing", Verifier: verifier, State: state,
		CallbackOrigin: origin, LoginSource: loginSource, CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(accountAttemptTimeout).UnixMilli(),
	}
	s.account.mu.Lock()
	s.account.attempt = attempt
	s.account.mu.Unlock()
	s.broadcastAccount()

	init, err := s.deepSeekAuthRequest(context.Background(), "auth_init", map[string]any{
		"code_challenge":        base64.RawURLEncoding.EncodeToString(challenge[:]),
		"code_challenge_method": "S256",
		"state":                 state,
		"redirect_uri":          origin + "/oauth/callback",
		"locale":                "en",
		"login_source":          loginSource,
	})
	if err != nil {
		s.failAccountAttempt(attempt.ID, "network")
		return s.accountView(), nil
	}
	authorizeURL, _ := init["authorize_url"].(string)
	authorizeID, _ := init["authorize_id"].(string)
	expiresIn := numberArg(init, "expires_in", 0)
	if authorizeURL == "" || authorizeID == "" || expiresIn <= 0 {
		s.failAccountAttempt(attempt.ID, "protocol")
		return s.accountView(), nil
	}
	s.account.mu.Lock()
	if s.account.attempt == attempt {
		attempt.Phase = "waiting-browser"
		attempt.AuthorizeURL = authorizeURL
		attempt.AuthorizeID = authorizeID
		attempt.ExpiresAt = time.Now().Add(time.Duration(expiresIn) * time.Second).UnixMilli()
	}
	s.account.mu.Unlock()
	s.broadcastAccount()
	return s.accountView(), nil
}

func (s *Server) failAccountAttempt(attemptID, code string) {
	s.account.mu.Lock()
	if s.account.attempt != nil && s.account.attempt.ID == attemptID {
		s.account.attempt.Phase = "failed"
		s.account.attempt.ErrorCode = code
	}
	s.account.mu.Unlock()
	s.broadcastAccount()
}

func (s *Server) cancelAccountSignIn(attemptID string) {
	s.account.mu.Lock()
	attempt := s.account.attempt
	if attempt == nil || (attemptID != "" && attempt.ID != attemptID) {
		s.account.mu.Unlock()
		return
	}
	attempt.Phase = "cancelled"
	authorizeID := attempt.AuthorizeID
	verifier := attempt.Verifier
	s.account.mu.Unlock()
	s.broadcastAccount()
	if authorizeID != "" {
		go func() {
			_, _ = s.deepSeekAuthRequest(context.Background(), "auth_cancel", map[string]any{
				"authorize_id": authorizeID, "code_verifier": verifier,
			})
		}()
	}
}

func (s *Server) signOutAccount() {
	token, configured := s.accountToken()
	if configured {
		go func() {
			_ = s.deepSeekLogout(context.Background(), token)
		}()
	}
	_ = s.credStore.Unset(context.Background(), brand.NewCredentialRef(deepSeekAccountToken))
	s.account.mu.Lock()
	s.account.attempt = nil
	s.account.mu.Unlock()
	s.broadcastAccount()
}

func (s *Server) accountProfile() any {
	token, configured := s.accountToken()
	if !configured {
		return nil
	}
	value, err := s.deepSeekAccountGet(context.Background(), "/auth-api/v0/users/current", token)
	if err != nil {
		if isAccountUnauthorized(err) {
			_ = s.credStore.Unset(context.Background(), brand.NewCredentialRef(deepSeekAccountToken))
			s.broadcastAccount()
		}
		return map[string]any{"status": "failed"}
	}
	identity, _ := value["id_profile"].(map[string]any)
	profile := map[string]any{
		"id": value["id"], "name": identity["name"], "avatarUrl": identity["picture"],
	}
	contact := value["mobile"]
	if contact == nil || contact == "" {
		contact = value["mobile_number"]
	}
	if contact == nil || contact == "" {
		contact = value["email"]
	}
	profile["contact"] = contact
	return map[string]any{"status": "ready", "value": profile}
}

func (s *Server) accountBalance() any {
	token, configured := s.accountToken()
	if !configured {
		return nil
	}
	value, err := s.deepSeekAccountGet(context.Background(), "/api/v0/users/get_user_summary", token)
	if err != nil {
		if isAccountUnauthorized(err) {
			_ = s.credStore.Unset(context.Background(), brand.NewCredentialRef(deepSeekAccountToken))
			s.broadcastAccount()
		}
		return map[string]any{"status": "failed"}
	}
	return map[string]any{
		"status": "ready", "value": value["normal_wallets"], "bonusWallets": value["bonus_wallets"],
	}
}

func (s *Server) handleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	if code == "" || state == "" {
		http.Error(w, "Missing authorization code", http.StatusBadRequest)
		return
	}
	s.account.mu.Lock()
	attempt := s.account.attempt
	if attempt == nil || attempt.Phase != "waiting-browser" || attempt.State != state {
		s.account.mu.Unlock()
		http.Error(w, "Sign-in attempt is no longer waiting", http.StatusGone)
		return
	}
	attempt.Phase = "exchanging"
	verifier := attempt.Verifier
	origin := attempt.CallbackOrigin
	loginSource := attempt.LoginSource
	attemptID := attempt.ID
	s.account.mu.Unlock()
	s.broadcastAccount()

	deviceID := s.accountDeviceID()
	exchange, err := s.deepSeekAuthRequest(context.Background(), "auth_exchange", map[string]any{
		"code": code, "code_verifier": verifier,
		"redirect_uri": origin + "/oauth/callback",
		"device_id":    deviceID,
		"device_model": runtime.GOOS + "-" + runtime.GOARCH,
		"os_version":   runtime.GOOS,
	})
	if err != nil {
		s.failAccountAttempt(attemptID, "network")
		writeAccountCallbackPage(w, loginSource, false)
		return
	}
	token, _ := exchange["token"].(string)
	if token == "" {
		s.failAccountAttempt(attemptID, "protocol")
		writeAccountCallbackPage(w, loginSource, false)
		return
	}
	s.account.mu.Lock()
	if s.account.attempt != nil && s.account.attempt.ID == attemptID {
		s.account.attempt.Phase = "committing"
	}
	s.account.mu.Unlock()
	s.broadcastAccount()
	if err := s.credStore.Set(context.Background(), brand.NewCredentialRef(deepSeekAccountToken), token); err != nil {
		s.failAccountAttempt(attemptID, "storage")
		writeAccountCallbackPage(w, loginSource, false)
		return
	}
	s.account.mu.Lock()
	if s.account.attempt != nil && s.account.attempt.ID == attemptID {
		s.account.attempt.Phase = "succeeded"
	}
	s.account.mu.Unlock()
	s.broadcastAccount()
	writeAccountCallbackPage(w, loginSource, true)
}

func (s *Server) accountDeviceID() string {
	s.account.mu.Lock()
	defer s.account.mu.Unlock()
	if s.account.deviceID != "" {
		return s.account.deviceID
	}
	if stored, configured := s.credStore.Resolve(context.Background(), brand.NewCredentialRef(deepSeekAccountDevice)); configured && stored != "" {
		s.account.deviceID = stored
		return stored
	}
	generated := uuid.MustV4()
	_ = s.credStore.Set(context.Background(), brand.NewCredentialRef(deepSeekAccountDevice), generated)
	s.account.deviceID = generated
	return generated
}

func writeAccountCallbackPage(w http.ResponseWriter, loginSource string, success bool) {
	if loginSource == "desktop" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	message := "Sign-in complete. You can close this tab and return to DSH."
	if !success {
		message = "Sign-in failed. Close this tab and try again in DSH."
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, "<!doctype html><html><meta charset=\"utf-8\"><title>"+message+"</title><body><p>"+message+"</p></body></html>")
}

func (s *Server) openAccountWatch(ctx context.Context, send func(map[string]any) error) {
	key := uuid.MustV4()
	channel := make(chan map[string]any, 16)
	s.account.mu.Lock()
	s.account.subs[key] = channel
	initial := s.accountViewLocked()
	s.account.mu.Unlock()
	defer func() {
		s.account.mu.Lock()
		delete(s.account.subs, key)
		s.account.mu.Unlock()
	}()
	if err := send(initial); err != nil {
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case value := <-channel:
			if err := send(value); err != nil {
				return
			}
		}
	}
}

func normalizeCallbackOrigin(value string) (string, error) {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "http" {
		return "", fmt.Errorf("callbackOrigin must be a loopback HTTP origin")
	}
	host := parsed.Hostname()
	if host != "127.0.0.1" && host != "localhost" && host != "::1" {
		return "", fmt.Errorf("callbackOrigin must be a loopback HTTP origin")
	}
	if parsed.Port() == "" {
		return "", fmt.Errorf("callbackOrigin must include a port")
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return "", fmt.Errorf("callbackOrigin must not include a path")
	}
	return parsed.Scheme + "://" + parsed.Host, nil
}

func randomBase64URL(size int) (string, error) {
	buffer := make([]byte, size)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

type accountHTTPError struct {
	status int
	code   int
	body   string
}

func (e *accountHTTPError) Error() string {
	return fmt.Sprintf("account platform HTTP %d", e.status)
}

func isAccountUnauthorized(err error) bool {
	failure, ok := err.(*accountHTTPError)
	return ok && (failure.status == http.StatusUnauthorized || failure.code == 40003)
}

func (s *Server) deepSeekAuthRequest(ctx context.Context, method string, body map[string]any) (map[string]any, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost,
		deepSeekPlatformBase()+"/auth-api/v0/dsh/"+method, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	return s.deepSeekPlatformDo(request, "")
}

func (s *Server) deepSeekAccountGet(ctx context.Context, path, token string) (map[string]any, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, deepSeekPlatformBase()+path, nil)
	if err != nil {
		return nil, err
	}
	return s.deepSeekPlatformDo(request, token)
}

func (s *Server) deepSeekLogout(ctx context.Context, token string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, deepSeekPlatformBase()+"/auth-api/v0/users/logout", nil)
	if err != nil {
		return err
	}
	_, err = s.deepSeekPlatformDo(request, token)
	return err
}

func (s *Server) deepSeekPlatformDo(request *http.Request, token string) (map[string]any, error) {
	if token != "" {
		request.Header.Set("x-dsh-auth-token", token)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 64<<10))
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		failure := &accountHTTPError{status: response.StatusCode, body: string(raw)}
		var envelope struct {
			Code int `json:"code"`
		}
		if json.Unmarshal(raw, &envelope) == nil {
			failure.code = envelope.Code
		}
		return nil, failure
	}
	var envelope struct {
		Code int `json:"code"`
		Data struct {
			BizCode int             `json:"biz_code"`
			BizData json.RawMessage `json:"biz_data"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, err
	}
	if envelope.Code != 0 || envelope.Data.BizCode != 0 {
		return nil, &accountHTTPError{status: response.StatusCode, code: envelope.Code}
	}
	result := map[string]any{}
	if len(envelope.Data.BizData) > 0 {
		if err := json.Unmarshal(envelope.Data.BizData, &result); err != nil {
			return nil, err
		}
	}
	return result, nil
}
