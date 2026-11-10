// Package provider_anthropic implements the Anthropic Messages streaming
// protocol used by DeepSeek's Anthropic-compatible endpoint. It supports both
// API-key authentication and the DeepSeek account token header.
package provider_anthropic

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/JopenChen/dsh-go/pkg/llm"
)

type Config struct {
	Route      string
	BaseURL    string
	APIKey     string
	AuthHeader string
	Headers    map[string]string
	WebSearch  bool
	SearchUses int
}

type Provider struct {
	config Config
	client *http.Client
}

func New(config Config) *Provider {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConnsPerHost = 16
	transport.IdleConnTimeout = 90 * time.Second
	transport.TLSHandshakeTimeout = 15 * time.Second
	return &Provider{config: config, client: &http.Client{Transport: transport}}
}

func (p *Provider) Name() string { return p.config.Route }

type messagesRequest struct {
	Model     string          `json:"model"`
	MaxTokens int             `json:"max_tokens"`
	System    string          `json:"system,omitempty"`
	Messages  []anthropicMsg  `json:"messages"`
	Tools     []anthropicTool `json:"tools,omitempty"`
	Stream    bool            `json:"stream"`
}

type anthropicMsg struct {
	Role    string `json:"role"`
	Content []any  `json:"content"`
}

type anthropicTool struct {
	Type        string         `json:"type,omitempty"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"input_schema,omitempty"`
	MaxUses     int            `json:"max_uses,omitempty"`
}

type textBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type toolUseBlock struct {
	Type  string         `json:"type"`
	ID    string         `json:"id"`
	Name  string         `json:"name"`
	Input map[string]any `json:"input"`
}

type toolResultBlock struct {
	Type      string `json:"type"`
	ToolUseID string `json:"tool_use_id"`
	Content   string `json:"content"`
	IsError   bool   `json:"is_error,omitempty"`
}

type sseEvent struct {
	Type    string `json:"type"`
	Index   int    `json:"index"`
	Message struct {
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	} `json:"message"`
	ContentBlock struct {
		Type  string         `json:"type"`
		ID    string         `json:"id"`
		Name  string         `json:"name"`
		Input map[string]any `json:"input"`
	} `json:"content_block"`
	Delta struct {
		Type        string `json:"type"`
		Text        string `json:"text"`
		Thinking    string `json:"thinking"`
		PartialJSON string `json:"partial_json"`
		StopReason  string `json:"stop_reason"`
	} `json:"delta"`
	Usage struct {
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

func (p *Provider) Chat(ctx context.Context, request llm.ChatRequest, callback func(llm.StreamChunk)) (llm.Usage, error) {
	if strings.TrimSpace(p.config.BaseURL) == "" {
		return llm.Usage{}, fmt.Errorf("provider %s has no base URL", p.config.Route)
	}
	if strings.TrimSpace(p.config.APIKey) == "" {
		return llm.Usage{}, &llm.LlmFailure{Kind: llm.FailInvalidCredential, Message: "provider credential is not configured"}
	}
	payload := p.payload(request)
	body, err := json.Marshal(payload)
	if err != nil {
		return llm.Usage{}, err
	}
	endpoint := strings.TrimRight(p.config.BaseURL, "/") + "/messages"
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return llm.Usage{}, err
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Accept", "text/event-stream")
	httpRequest.Header.Set("anthropic-version", "2023-06-01")
	authHeader := p.config.AuthHeader
	if authHeader == "" {
		authHeader = "x-api-key"
	}
	httpRequest.Header.Set(authHeader, p.config.APIKey)
	if authHeader == "x-api-key" {
		httpRequest.Header.Set("Authorization", "Bearer "+p.config.APIKey)
	}
	for key, value := range p.config.Headers {
		httpRequest.Header.Set(key, value)
	}
	response, err := p.client.Do(httpRequest)
	if err != nil {
		if ctx.Err() != nil {
			return llm.Usage{}, &llm.LlmFailure{Kind: llm.FailOverload, Message: ctx.Err().Error(), Cause: ctx.Err()}
		}
		return llm.Usage{}, &llm.LlmFailure{Kind: llm.FailOverload, Message: err.Error(), Cause: err}
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		detail, _ := io.ReadAll(io.LimitReader(response.Body, 64<<10))
		failure := llm.NewProviderFailure(string(detail))
		failure.Message = fmt.Sprintf("provider %s returned HTTP %d: %s", p.config.Route, response.StatusCode, strings.TrimSpace(string(detail)))
		return llm.Usage{}, failure
	}
	return p.readSSE(ctx, response.Body, callback)
}

func (p *Provider) payload(request llm.ChatRequest) messagesRequest {
	messages := make([]anthropicMsg, 0, len(request.Messages))
	for _, message := range request.Messages {
		blocks := make([]any, 0, len(message.Content))
		for _, block := range message.Content {
			switch block.Kind {
			case llm.BlockText:
				if block.Text != "" {
					blocks = append(blocks, textBlock{Type: "text", Text: block.Text})
				}
			case llm.BlockReasoning:
				// Reasoning is not replayed to the model.
			case llm.BlockToolUse:
				if block.ToolCall != nil {
					blocks = append(blocks, toolUseBlock{
						Type: "tool_use", ID: block.ToolCall.ID, Name: block.ToolCall.Name, Input: block.ToolCall.Input,
					})
				}
			case llm.BlockToolResult:
				if block.ToolResult != nil {
					blocks = append(blocks, toolResultBlock{
						Type: "tool_result", ToolUseID: block.ToolResult.ToolUseID,
						Content: block.ToolResult.Content, IsError: block.ToolResult.IsError,
					})
				}
			}
		}
		if len(blocks) == 0 {
			continue
		}
		role := "user"
		if message.Role == llm.RoleAssistant {
			role = "assistant"
		}
		if role == "user" && len(messages) > 0 && messages[len(messages)-1].Role == "user" {
			messages[len(messages)-1].Content = append(messages[len(messages)-1].Content, blocks...)
			continue
		}
		messages = append(messages, anthropicMsg{Role: role, Content: blocks})
	}
	tools := make([]anthropicTool, 0, len(request.Tools)+1)
	for _, tool := range request.Tools {
		tools = append(tools, anthropicTool{
			Name: tool.Name, Description: tool.Description, InputSchema: tool.Parameters,
		})
	}
	if p.config.WebSearch {
		uses := p.config.SearchUses
		if uses <= 0 {
			uses = 5
		}
		tools = append(tools, anthropicTool{Type: "web_search_20250305", Name: "web_search", MaxUses: uses})
	}
	maxTokens := request.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 4096
	}
	return messagesRequest{
		Model: request.Model, MaxTokens: maxTokens, System: request.System,
		Messages: messages, Tools: tools, Stream: true,
	}
}

func (p *Provider) readSSE(ctx context.Context, reader io.Reader, callback func(llm.StreamChunk)) (llm.Usage, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	type pendingTool struct {
		id        string
		name      string
		arguments strings.Builder
	}
	pending := map[int]*pendingTool{}
	var usage llm.Usage
	for scanner.Scan() {
		if ctx.Err() != nil {
			return usage, &llm.LlmFailure{Kind: llm.FailOverload, Message: ctx.Err().Error(), Cause: ctx.Err()}
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}
		var event sseEvent
		if json.Unmarshal([]byte(data), &event) != nil {
			continue
		}
		switch event.Type {
		case "message_start":
			usage.PromptTokens = event.Message.Usage.InputTokens
			usage.CompletionTokens = event.Message.Usage.OutputTokens
		case "content_block_start":
			if event.ContentBlock.Type == "tool_use" {
				pending[event.Index] = &pendingTool{id: event.ContentBlock.ID, name: event.ContentBlock.Name}
			}
		case "content_block_delta":
			if callback == nil {
				continue
			}
			switch event.Delta.Type {
			case "text_delta":
				if event.Delta.Text != "" {
					callback(llm.StreamChunk{Kind: llm.ChunkText, Text: event.Delta.Text})
				}
			case "thinking_delta":
				if event.Delta.Thinking != "" {
					callback(llm.StreamChunk{Kind: llm.ChunkReasoning, Reasoning: event.Delta.Thinking})
				}
			case "input_json_delta":
				if tool := pending[event.Index]; tool != nil {
					tool.arguments.WriteString(event.Delta.PartialJSON)
				}
			}
		case "content_block_stop":
			tool := pending[event.Index]
			if tool == nil {
				continue
			}
			delete(pending, event.Index)
			input := map[string]any{}
			if tool.arguments.Len() > 0 {
				_ = json.Unmarshal([]byte(tool.arguments.String()), &input)
			}
			if callback != nil {
				callback(llm.StreamChunk{Kind: llm.ChunkToolCall, ToolCall: &llm.ToolCall{
					ID: tool.id, Name: tool.name, Input: input,
				}})
			}
		case "message_delta":
			if event.Usage.OutputTokens > 0 {
				usage.CompletionTokens = event.Usage.OutputTokens
			}
		case "message_stop":
			if callback != nil {
				callback(llm.StreamChunk{Kind: llm.ChunkDone})
			}
		case "error":
			return usage, &llm.LlmFailure{Kind: llm.FailOverload, Message: event.Error.Message}
		}
	}
	if err := scanner.Err(); err != nil {
		return usage, &llm.LlmFailure{Kind: llm.FailOverload, Message: err.Error(), Cause: err}
	}
	return usage, nil
}
