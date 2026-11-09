//

//

//

package provider_deepseek

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/JopenChen/dsh-go/pkg/brand"
	"github.com/JopenChen/dsh-go/pkg/credentials"
	"github.com/JopenChen/dsh-go/pkg/llm"
)

// ============================================================================

// ============================================================================

const (
	DefaultMaxIdleConns = 200

	DefaultMaxIdleConnsPerHost = 100

	DefaultIdleConnTimeout = 90 * time.Second

	DefaultTLSHandshakeTimeout = 10 * time.Second

	DefaultExpectContinueTimeout = 5 * time.Second
)

const DefaultBaseURL = "https://api.deepseek.com"

type DeepSeek struct {
	baseURL   string
	client    *http.Client
	transport *http.Transport
	creds     *credentials.Store
	apiKeyRef brand.CredentialRef
}

//

func NewDeepSeek(creds *credentials.Store, opts ...Option) *DeepSeek {

	tp := newDefaultTransport()
	d := &DeepSeek{
		baseURL:   DefaultBaseURL,
		transport: tp,
		client: &http.Client{

			Timeout:   0,
			Transport: tp,
		},
		creds:     creds,
		apiKeyRef: brand.NewCredentialRef("DEEPSEEK_API_KEY"),
	}
	for _, o := range opts {
		o(d)
	}
	return d
}

func newDefaultTransport() *http.Transport {

	def, _ := http.DefaultTransport.(*http.Transport)
	tp := def.Clone()

	tp.MaxIdleConns = DefaultMaxIdleConns
	tp.MaxIdleConnsPerHost = DefaultMaxIdleConnsPerHost
	tp.MaxConnsPerHost = 0
	tp.IdleConnTimeout = DefaultIdleConnTimeout

	if tp.DialContext == nil {
		dialer := &net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}
		tp.DialContext = dialer.DialContext
	}

	tp.TLSHandshakeTimeout = DefaultTLSHandshakeTimeout

	tp.ExpectContinueTimeout = DefaultExpectContinueTimeout

	tp.ForceAttemptHTTP2 = true

	return tp
}

// ============================================================================

// ============================================================================

type Option func(*DeepSeek)

func WithBaseURL(u string) Option {
	return func(d *DeepSeek) { d.baseURL = u }
}

func WithHTTPClient(c *http.Client) Option {
	return func(d *DeepSeek) {
		d.client = c
		if tp, ok := c.Transport.(*http.Transport); ok {
			d.transport = tp
		} else {
			d.transport = nil
		}
	}
}

func WithTransport(tp *http.Transport) Option {
	return func(d *DeepSeek) {
		d.transport = tp
		d.client.Transport = tp
	}
}

func WithAPIKeyRef(ref brand.CredentialRef) Option {
	return func(d *DeepSeek) { d.apiKeyRef = ref }
}

func WithMaxIdleConnsPerHost(n int) Option {
	return func(d *DeepSeek) {
		if d.transport != nil {
			d.transport.MaxIdleConnsPerHost = n
		}
	}
}

func WithIdleConnTimeout(d time.Duration) Option {
	return func(tp *DeepSeek) {
		if tp.transport != nil {
			tp.transport.IdleConnTimeout = d
		}
	}
}

func WithTLSHandshakeTimeout(t time.Duration) Option {
	return func(d *DeepSeek) {
		if d.transport != nil {
			d.transport.TLSHandshakeTimeout = t
		}
	}
}

func DisableHTTP2() Option {
	return func(d *DeepSeek) {
		if d.transport != nil {
			d.transport.ForceAttemptHTTP2 = false

			d.transport.TLSNextProto = map[string]func(string, *tls.Conn) http.RoundTripper{}
		}
	}
}

// ============================================================================

// ============================================================================

func (d *DeepSeek) Transport() *http.Transport { return d.transport }

func (d *DeepSeek) HTTPClient() *http.Client { return d.client }

func (d *DeepSeek) Name() string {
	return "deepseek"
}

// ============================================================================

// ============================================================================

type chatRequest struct {
	Model         string         `json:"model"`
	Messages      []chatMessage  `json:"messages"`
	Tools         []chatTool     `json:"tools,omitempty"`
	Stream        bool           `json:"stream"`
	Temperature   float64        `json:"temperature,omitempty"`
	MaxTokens     int            `json:"max_tokens,omitempty"`
	StreamOptions *streamOptions `json:"stream_options,omitempty"`
}

type streamOptions struct {
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
		PromptTokens          int `json:"prompt_tokens"`
		CompletionTokens      int `json:"completion_tokens"`
		PromptCacheHitTokens  int `json:"prompt_cache_hit_tokens"`
		PromptCacheMissTokens int `json:"prompt_cache_miss_tokens"`
	} `json:"usage"`
}

// ============================================================================

// ============================================================================

//

func (d *DeepSeek) Chat(ctx context.Context, req llm.ChatRequest, cb func(llm.StreamChunk)) (llm.Usage, error) {

	apiKey, ok := d.creds.Resolve(ctx, d.apiKeyRef)
	if !ok || apiKey == "" {
		return llm.Usage{}, &llm.LlmFailure{Kind: llm.FailResponseRefusal, Message: "missing API key credential"}
	}

	payload := d.buildPayload(req)
	body, err := json.Marshal(payload)
	if err != nil {
		return llm.Usage{}, fmt.Errorf("deepseek: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, d.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return llm.Usage{}, fmt.Errorf("deepseek: new request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := d.client.Do(httpReq)
	if err != nil {

		if ctx.Err() != nil {
			return llm.Usage{}, &llm.LlmFailure{Kind: llm.FailOverload, Message: ctx.Err().Error(), Cause: ctx.Err()}
		}
		return llm.Usage{}, &llm.LlmFailure{Kind: llm.FailOverload, Message: err.Error(), Cause: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {

		body, rerr := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
		if rerr == nil && len(body) > 0 {
			return llm.Usage{}, d.mapHTTPErrorBody(resp.StatusCode, string(body))
		}
		return llm.Usage{}, d.mapHTTPError(resp.StatusCode)
	}

	return d.readSSE(ctx, resp.Body, cb)
}

func (d *DeepSeek) buildPayload(req llm.ChatRequest) chatRequest {
	messages := make([]chatMessage, 0, len(req.Messages)+1)
	if req.System != "" {
		messages = append(messages, chatMessage{Role: "system", Content: req.System})
	}
	for _, m := range req.Messages {
		cm := chatMessage{Role: string(m.Role)}
		var sb strings.Builder
		for _, blk := range m.Content {
			switch blk.Kind {
			case llm.BlockText:
				sb.WriteString(blk.Text)
			case llm.BlockToolResult:
				if blk.ToolResult != nil {
					sb.WriteString("[tool_result:" + blk.ToolResult.Content + "]")
				}
			}
		}
		cm.Content = sb.String()
		messages = append(messages, cm)
	}

	tools := make([]chatTool, 0, len(req.Tools))
	for _, t := range req.Tools {
		tools = append(tools, chatTool{
			Type: "function",
			Function: chatFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
			},
		})
	}

	return chatRequest{
		Model:         req.Model,
		Messages:      messages,
		Tools:         tools,
		Stream:        true,
		Temperature:   req.Temperature,
		MaxTokens:     req.MaxTokens,
		StreamOptions: &streamOptions{IncludeUsage: true},
	}
}

// ============================================================================

// ============================================================================

type sseScanResult struct {
	line string
	err  error
	done bool
}

//

func (d *DeepSeek) readSSE(ctx context.Context, r io.Reader, cb func(llm.StreamChunk)) (llm.Usage, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	ch := make(chan sseScanResult, 16)
	go func() {
		defer close(ch)
		for {
			if !scanner.Scan() {
				if err := scanner.Err(); err != nil {
					ch <- sseScanResult{err: err, done: true}
				} else {
					ch <- sseScanResult{done: true}
				}
				return
			}
			ch <- sseScanResult{line: scanner.Text()}
		}
	}()

	var usage llm.Usage
	for {
		select {
		case <-ctx.Done():

			return usage, &llm.LlmFailure{
				Kind:    llm.FailOverload,
				Message: "deepseek: sse read interrupted: " + ctx.Err().Error(),
				Cause:   ctx.Err(),
			}
		case res, ok := <-ch:
			if !ok {

				return usage, nil
			}
			if res.done {
				if res.err != nil {
					return usage, &llm.LlmFailure{Kind: llm.FailOverload, Message: res.err.Error(), Cause: res.err}
				}
				return usage, nil
			}

			line := strings.TrimSpace(res.line)
			if line == "" {
				continue
			}
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "[DONE]" {
				if cb != nil {
					cb(llm.StreamChunk{Kind: llm.ChunkDone})
				}

				continue
			}
			var chunk sseChunk
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue
			}

			if chunk.Usage != nil {
				usage = llm.Usage{
					PromptTokens:          chunk.Usage.PromptTokens,
					CompletionTokens:      chunk.Usage.CompletionTokens,
					PromptCacheHitTokens:  chunk.Usage.PromptCacheHitTokens,
					PromptCacheMissTokens: chunk.Usage.PromptCacheMissTokens,
				}
			}
			if len(chunk.Choices) == 0 {
				continue
			}
			delta := chunk.Choices[0].Delta
			if cb != nil {
				if delta.Reasoning != "" {
					cb(llm.StreamChunk{Kind: llm.ChunkReasoning, Reasoning: delta.Reasoning})
				}
				if delta.Content != "" {
					cb(llm.StreamChunk{Kind: llm.ChunkText, Text: delta.Content})
				}
				for _, tc := range delta.ToolCalls {
					cb(llm.StreamChunk{Kind: llm.ChunkToolCall, ToolCall: &llm.ToolCall{
						ID:    tc.ID,
						Name:  tc.Function.Name,
						Input: parseArgs(tc.Function.Arguments),
					}})
				}
			}
		}
	}
}

func parseArgs(args string) map[string]any {
	out := map[string]any{}
	if args == "" {
		return out
	}
	_ = json.Unmarshal([]byte(args), &out)
	return out
}

// ============================================================================

// ============================================================================

func (d *DeepSeek) mapHTTPError(status int) *llm.LlmFailure {
	switch {
	case status == 429:
		return &llm.LlmFailure{Kind: llm.FailRateLimit, Message: fmt.Sprintf("http %d rate limited", status)}
	case status == 401:
		return &llm.LlmFailure{Kind: llm.FailInvalidCredential, Message: fmt.Sprintf("http %d invalid credential / unauthorized", status)}
	case status >= 500:
		return &llm.LlmFailure{Kind: llm.FailOverload, Message: fmt.Sprintf("http %d server overload", status)}
	case status == 400 || status == 422:
		return &llm.LlmFailure{Kind: llm.FailContextOverflow, Message: fmt.Sprintf("http %d bad request (possible context overflow)", status)}
	default:
		return &llm.LlmFailure{Kind: llm.FailResponseRefusal, Message: fmt.Sprintf("http %d", status)}
	}
}

func (d *DeepSeek) mapHTTPErrorBody(status int, body string) *llm.LlmFailure {
	const maxBody = 200
	snippet := body
	if len(snippet) > maxBody {
		snippet = snippet[:maxBody] + "..."
	}
	if f := llm.NewProviderFailure(body); f.Kind != "unknown" && f.Kind != "" {

		f.Message = fmt.Sprintf("http %d: %s", status, snippet)
		return f
	}

	f := d.mapHTTPError(status)
	f.Message = fmt.Sprintf("%s (body: %s)", f.Message, snippet)
	return f
}

var _ = errors.New
