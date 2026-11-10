// Package provider_openai implements a configurable OpenAI-compatible chat
// completions provider for the native DSH engine. It intentionally supports
// only the streaming chat-completions protocol; other declared protocols are
// rejected explicitly instead of being silently treated as compatible.
package provider_openai

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
	Route       string
	DisplayName string
	BaseURL     string
	APIKey      string
	Headers     map[string]string
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
	return &Provider{
		config: config,
		client: &http.Client{Transport: transport},
	}
}

func (p *Provider) Name() string { return p.config.Route }

type chatRequest struct {
	Model         string        `json:"model"`
	Messages      []chatMessage `json:"messages"`
	Tools         []chatTool    `json:"tools,omitempty"`
	Stream        bool          `json:"stream"`
	StreamOptions *streamOption `json:"stream_options,omitempty"`
}

type streamOption struct {
	IncludeUsage bool `json:"include_usage"`
}

type chatMessage struct {
	Role       string         `json:"role"`
	Content    string         `json:"content"`
	ToolCalls  []chatToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
}

type chatTool struct {
	Type     string       `json:"type"`
	Function chatFunction `json:"function"`
}

type chatFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type chatToolCall struct {
	Index    int                  `json:"index,omitempty"`
	ID       string               `json:"id"`
	Type     string               `json:"type"`
	Function chatToolCallFunction `json:"function"`
}

type chatToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type sseChunk struct {
	Choices []struct {
		Delta struct {
			Content   string         `json:"content"`
			Reasoning string         `json:"reasoning_content"`
			ToolCalls []chatToolCall `json:"tool_calls"`
		} `json:"delta"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

func (p *Provider) Chat(ctx context.Context, request llm.ChatRequest, callback func(llm.StreamChunk)) (llm.Usage, error) {
	if strings.TrimSpace(p.config.BaseURL) == "" {
		return llm.Usage{}, fmt.Errorf("provider %s has no base URL", p.config.Route)
	}
	if strings.TrimSpace(p.config.APIKey) == "" {
		return llm.Usage{}, &llm.LlmFailure{Kind: llm.FailInvalidCredential, Message: "provider API key is not configured"}
	}
	payload := p.payload(request)
	body, err := json.Marshal(payload)
	if err != nil {
		return llm.Usage{}, err
	}
	endpoint := strings.TrimRight(p.config.BaseURL, "/") + "/chat/completions"
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return llm.Usage{}, err
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Accept", "text/event-stream")
	httpRequest.Header.Set("Authorization", "Bearer "+p.config.APIKey)
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

func (p *Provider) payload(request llm.ChatRequest) chatRequest {
	messages := make([]chatMessage, 0, len(request.Messages)+1)
	if request.System != "" {
		messages = append(messages, chatMessage{Role: "system", Content: request.System})
	}
	for _, message := range request.Messages {
		entry := chatMessage{Role: string(message.Role)}
		var text strings.Builder
		for _, block := range message.Content {
			switch block.Kind {
			case llm.BlockText, llm.BlockReasoning:
				text.WriteString(block.Text)
			case llm.BlockToolUse:
				if block.ToolCall != nil {
					arguments, _ := json.Marshal(block.ToolCall.Input)
					entry.ToolCalls = append(entry.ToolCalls, chatToolCall{
						ID: block.ToolCall.ID, Type: "function",
						Function: chatToolCallFunction{Name: block.ToolCall.Name, Arguments: string(arguments)},
					})
				}
			case llm.BlockToolResult:
				if block.ToolResult != nil {
					entry.ToolCallID = block.ToolResult.ToolUseID
					text.WriteString(block.ToolResult.Content)
				}
			}
		}
		entry.Content = text.String()
		messages = append(messages, entry)
	}
	tools := make([]chatTool, 0, len(request.Tools))
	for _, tool := range request.Tools {
		tools = append(tools, chatTool{
			Type: "function",
			Function: chatFunction{
				Name: tool.Name, Description: tool.Description, Parameters: tool.Parameters,
			},
		})
	}
	return chatRequest{
		Model: request.Model, Messages: messages, Tools: tools, Stream: true,
		StreamOptions: &streamOption{IncludeUsage: true},
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
	order := make([]int, 0)
	flush := func() {
		if callback == nil {
			return
		}
		for _, index := range order {
			call := pending[index]
			if call == nil {
				continue
			}
			input := map[string]any{}
			if call.arguments.Len() > 0 {
				_ = json.Unmarshal([]byte(call.arguments.String()), &input)
			}
			callback(llm.StreamChunk{Kind: llm.ChunkToolCall, ToolCall: &llm.ToolCall{
				ID: call.id, Name: call.name, Input: input,
			}})
		}
		pending = map[int]*pendingTool{}
		order = order[:0]
	}
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
		if data == "[DONE]" {
			flush()
			if callback != nil {
				callback(llm.StreamChunk{Kind: llm.ChunkDone})
			}
			continue
		}
		var chunk sseChunk
		if json.Unmarshal([]byte(data), &chunk) != nil {
			continue
		}
		if chunk.Usage != nil {
			usage = llm.Usage{PromptTokens: chunk.Usage.PromptTokens, CompletionTokens: chunk.Usage.CompletionTokens}
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		delta := chunk.Choices[0].Delta
		if callback == nil {
			continue
		}
		if delta.Reasoning != "" {
			callback(llm.StreamChunk{Kind: llm.ChunkReasoning, Reasoning: delta.Reasoning})
		}
		if delta.Content != "" {
			callback(llm.StreamChunk{Kind: llm.ChunkText, Text: delta.Content})
		}
		for _, call := range delta.ToolCalls {
			entry := pending[call.Index]
			if entry == nil {
				entry = &pendingTool{}
				pending[call.Index] = entry
				order = append(order, call.Index)
			}
			if call.ID != "" {
				entry.id = call.ID
			}
			if call.Function.Name != "" {
				entry.name = call.Function.Name
			}
			entry.arguments.WriteString(call.Function.Arguments)
		}
	}
	if err := scanner.Err(); err != nil {
		return usage, &llm.LlmFailure{Kind: llm.FailOverload, Message: err.Error(), Cause: err}
	}
	flush()
	return usage, nil
}
