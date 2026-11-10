package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/JopenChen/dsh-go/pkg/brand"
	"github.com/JopenChen/dsh-go/pkg/llm"
	"github.com/JopenChen/dsh-go/pkg/llm/provider_openai"
)

const piAiSettingsNamespace = "llm-pi-ai"

var providerRoutePattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`)

type providerModel struct {
	ID            string `json:"id"`
	Name          string `json:"name,omitempty"`
	ContextWindow int    `json:"contextWindow,omitempty"`
	MaxTokens     int    `json:"maxTokens,omitempty"`
}

type providerProfile struct {
	Route       string
	DisplayName string
	API         string
	BaseURL     string
	APIKeyEnv   string
	Models      []providerModel
	Headers     map[string]string
}

type providerRouter struct {
	server *Server
}

func (r *providerRouter) Name() string { return "native-router" }

func (r *providerRouter) Chat(ctx context.Context, request llm.ChatRequest, callback func(llm.StreamChunk)) (llm.Usage, error) {
	provider := request.Provider
	if provider == "" || provider == "deepseek" {
		if r.server.adapter == nil {
			return llm.Usage{}, fmt.Errorf("native engine has no DeepSeek adapter")
		}
		return r.server.adapter.Chat(ctx, request, callback)
	}
	profile := r.server.settings.providerProfile(provider)
	if profile == nil {
		return llm.Usage{}, &llm.LlmFailure{Kind: llm.FailInvalidCredential, Message: "provider " + provider + " is not configured"}
	}
	key, _ := r.server.credStore.Resolve(ctx, brand.NewCredentialRef(profile.APIKeyEnv))
	adapter := provider_openai.New(provider_openai.Config{
		Route: profile.Route, DisplayName: profile.DisplayName, BaseURL: profile.BaseURL,
		APIKey: key, Headers: profile.Headers,
	})
	return adapter.Chat(ctx, request, callback)
}

func (s *nativeSettings) providerProfile(route string) *providerProfile {
	s.mu.Lock()
	defer s.mu.Unlock()
	section := s.namespaces[piAiSettingsNamespace]
	if section == nil {
		return nil
	}
	providers, _ := section.Value["providers"].(map[string]any)
	raw, _ := providers[route].(map[string]any)
	if raw == nil {
		return nil
	}
	return parseProviderProfile(route, raw)
}

func (s *nativeSettings) providerProfiles() []*providerProfile {
	s.mu.Lock()
	defer s.mu.Unlock()
	section := s.namespaces[piAiSettingsNamespace]
	if section == nil {
		return nil
	}
	providers, _ := section.Value["providers"].(map[string]any)
	routes := make([]string, 0, len(providers))
	for route := range providers {
		routes = append(routes, route)
	}
	sort.Strings(routes)
	result := make([]*providerProfile, 0, len(routes))
	for _, route := range routes {
		raw, _ := providers[route].(map[string]any)
		if profile := parseProviderProfile(route, raw); profile != nil {
			result = append(result, profile)
		}
	}
	return result
}

func parseProviderProfile(route string, raw map[string]any) *providerProfile {
	if raw == nil {
		return nil
	}
	profile := &providerProfile{
		Route: route, DisplayName: stringValue(raw["displayName"]),
		API: stringValue(raw["api"]), BaseURL: stringValue(raw["baseURL"]),
		APIKeyEnv: stringValue(raw["apiKeyEnv"]), Headers: map[string]string{},
	}
	if headers, ok := raw["headers"].(map[string]any); ok {
		for key, value := range headers {
			if text, ok := value.(string); ok {
				profile.Headers[key] = text
			}
		}
	}
	if models, ok := raw["models"].([]any); ok {
		for _, item := range models {
			model, _ := item.(map[string]any)
			id := stringValue(model["id"])
			if id == "" {
				continue
			}
			profile.Models = append(profile.Models, providerModel{
				ID: id, Name: stringValue(model["name"]),
				ContextWindow: numberArg(model, "contextWindow", 0),
				MaxTokens:     numberArg(model, "maxTokens", 0),
			})
		}
	}
	return profile
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}

func (s *Server) dispatchProviderRPC(endpoint string, args map[string]any) (any, *RPCError) {
	switch endpoint {
	case "llm/listConfigurableProviders":
		rows := []map[string]any{{
			"provider": "deepseek", "displayName": "DeepSeek", "settingsNs": "native-engine",
			"settingsPath": []string{}, "declared": false, "defaultBaseURL": "https://api.deepseek.com",
		}}
		for _, profile := range s.settings.providerProfiles() {
			rows = append(rows, map[string]any{
				"provider": profile.Route, "displayName": profile.DisplayName,
				"settingsNs": piAiSettingsNamespace, "settingsPath": []string{"providers", profile.Route},
				"declared": true, "defaultBaseURL": profile.BaseURL,
			})
		}
		return rows, nil
	case "llm/discoverModels":
		request, _ := args["request"].(map[string]any)
		baseURL := stringValue(request["baseURL"])
		apiKey := stringValue(request["apiKey"])
		provider := stringValue(request["provider"])
		headers := map[string]string{}
		if provider != "" {
			profile := s.settings.providerProfile(provider)
			if profile == nil {
				return nil, &RPCError{Code: "provider/not-found", Message: "provider is not configured"}
			}
			if baseURL == "" {
				baseURL = profile.BaseURL
			}
			if apiKey == "" {
				apiKey, _ = s.credStore.Resolve(context.Background(), brand.NewCredentialRef(profile.APIKeyEnv))
			}
			headers = profile.Headers
		}
		if baseURL == "" {
			return nil, &RPCError{Code: "gateway/arguments-invalid", Message: "baseURL is required"}
		}
		models, err := discoverProviderModels(baseURL, apiKey, headers)
		if err != nil {
			return nil, &RPCError{Code: "provider/discovery-failed", Message: err.Error()}
		}
		return models, nil
	default:
		return nil, &RPCError{Code: "gateway/method-not-found", Message: "unknown provider method"}
	}
}

func discoverProviderModels(baseURL, apiKey string, headers map[string]string) ([]map[string]any, error) {
	baseURL = strings.TrimRight(baseURL, "/")
	endpoint := baseURL + "/models"
	if strings.HasSuffix(baseURL, "/v1") {
		endpoint = baseURL + "/models"
	}
	request, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	if apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+apiKey)
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		detail, _ := io.ReadAll(io.LimitReader(response.Body, 16<<10))
		return nil, fmt.Errorf("HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(detail)))
	}
	var payload struct {
		Data []struct {
			ID            string `json:"id"`
			Name          string `json:"name"`
			ContextWindow int    `json:"context_window"`
			MaxTokens     int    `json:"max_tokens"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&payload); err != nil {
		return nil, err
	}
	result := make([]map[string]any, 0, len(payload.Data))
	for _, model := range payload.Data {
		if model.ID == "" {
			continue
		}
		row := map[string]any{"id": model.ID}
		if model.Name != "" {
			row["name"] = model.Name
		}
		if model.ContextWindow > 0 {
			row["contextWindow"] = model.ContextWindow
		}
		if model.MaxTokens > 0 {
			row["maxTokens"] = model.MaxTokens
		}
		result = append(result, row)
	}
	return result, nil
}

func (s *Server) providerCatalog() map[string]any {
	groups := []map[string]any{{
		"id": "deepseek", "name": "DeepSeek",
		"models": []map[string]any{
			{"id": "deepseek-chat", "name": "DeepSeek-V3", "description": "DeepSeek-V3 high-performance flagship model"},
			{"id": "deepseek-reasoner", "name": "DeepSeek-R1", "description": "DeepSeek-R1 deep reasoning model", "reasoning": map[string]any{
				"efforts":       []map[string]any{{"id": "low", "name": "Low"}, {"id": "high", "name": "High"}},
				"defaultEffort": "high",
			}},
		},
	}}
	routable := []string{"deepseek"}
	for _, profile := range s.settings.providerProfiles() {
		if len(profile.Models) == 0 {
			continue
		}
		models := make([]map[string]any, 0, len(profile.Models))
		for _, model := range profile.Models {
			row := map[string]any{"id": model.ID, "name": model.Name}
			if row["name"] == "" {
				row["name"] = model.ID
			}
			if model.ContextWindow > 0 {
				row["contextWindow"] = model.ContextWindow
			}
			if model.MaxTokens > 0 {
				row["maxTokens"] = model.MaxTokens
			}
			models = append(models, row)
		}
		name := profile.DisplayName
		if name == "" {
			name = profile.Route
		}
		groups = append(groups, map[string]any{"id": profile.Route, "name": name, "models": models})
		routable = append(routable, profile.Route)
	}
	return map[string]any{
		"default":           map[string]any{"provider": "deepseek", "model": "deepseek-chat"},
		"routableProviders": routable, "groups": groups, "failures": []any{},
	}
}

func providerSettingsSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"dict": map[string]any{
			"providers": map[string]any{
				"type":  "dict",
				"meta":  map[string]any{"description": "Custom OpenAI-compatible provider routes"},
				"inner": map[string]any{"type": "object", "meta": map[string]any{}},
			},
		},
		"meta": map[string]any{},
	}
}

func validateProviderSettings(value map[string]any) error {
	providers, _ := value["providers"].(map[string]any)
	for route, raw := range providers {
		if !providerRoutePattern.MatchString(route) {
			return fmt.Errorf("provider route %q must be lower-case words separated by single hyphens", route)
		}
		profile, _ := raw.(map[string]any)
		if profile == nil {
			return fmt.Errorf("provider %q must be an object", route)
		}
		api := stringValue(profile["api"])
		if api != "openai-completions" {
			return fmt.Errorf("provider %q uses unsupported protocol %q; only openai-completions is available", route, api)
		}
		if strings.TrimSpace(stringValue(profile["baseURL"])) == "" {
			return fmt.Errorf("provider %q requires baseURL", route)
		}
		if strings.TrimSpace(stringValue(profile["apiKeyEnv"])) == "" {
			return fmt.Errorf("provider %q requires apiKeyEnv", route)
		}
		models, _ := profile["models"].([]any)
		if len(models) == 0 {
			return fmt.Errorf("provider %q requires at least one model", route)
		}
		for _, item := range models {
			model, _ := item.(map[string]any)
			if strings.TrimSpace(stringValue(model["id"])) == "" {
				return fmt.Errorf("provider %q has a model without an id", route)
			}
		}
	}
	return nil
}
