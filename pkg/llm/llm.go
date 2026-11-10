//

//

package llm

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// ============================================================================

// ============================================================================

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

type ContentBlockKind string

const (
	BlockText       ContentBlockKind = "text"
	BlockReasoning  ContentBlockKind = "reasoning"
	BlockToolUse    ContentBlockKind = "tool_use"
	BlockToolResult ContentBlockKind = "tool_result"
	BlockImage      ContentBlockKind = "image"
)

type ToolCall struct {
	ID    string         `json:"id"`
	Name  string         `json:"name"`
	Input map[string]any `json:"input"`
}

type ToolResultContent struct {
	ToolUseID string `json:"toolUseId"`
	Content   string `json:"content"`
	IsError   bool   `json:"isError,omitempty"`
}

type ImageContent struct {
	URL        string `json:"url,omitempty"`
	Attachment string `json:"attachment,omitempty"`
}

type ContentBlock struct {
	Kind       ContentBlockKind   `json:"kind"`
	Text       string             `json:"text,omitempty"`
	ToolCall   *ToolCall          `json:"toolCall,omitempty"`
	ToolResult *ToolResultContent `json:"toolResult,omitempty"`
	Image      *ImageContent      `json:"image,omitempty"`
}

func Text(s string) ContentBlock {
	return ContentBlock{Kind: BlockText, Text: s}
}

func Reasoning(s string) ContentBlock {
	return ContentBlock{Kind: BlockReasoning, Text: s}
}

func ToolUse(tc *ToolCall) ContentBlock {
	return ContentBlock{Kind: BlockToolUse, ToolCall: tc}
}

func ToolResult(tr *ToolResultContent) ContentBlock {
	return ContentBlock{Kind: BlockToolResult, ToolResult: tr}
}

func Image(img *ImageContent) ContentBlock {
	return ContentBlock{Kind: BlockImage, Image: img}
}

type Message struct {
	Role    Role           `json:"role"`
	Content []ContentBlock `json:"content"`
}

func NewUserMessage(text string) Message {
	return Message{Role: RoleUser, Content: []ContentBlock{Text(text)}}
}

func NewAssistantText(text string) Message {
	return Message{Role: RoleAssistant, Content: []ContentBlock{Text(text)}}
}

func NewAssistantMessage(text string, calls ...*ToolCall) Message {
	content := make([]ContentBlock, 0, len(calls)+1)
	if text != "" {
		content = append(content, Text(text))
	}
	for _, call := range calls {
		if call != nil {
			content = append(content, ToolUse(call))
		}
	}
	return Message{Role: RoleAssistant, Content: content}
}

func NewToolResultMessage(callID, content string, isError bool) Message {
	return Message{Role: RoleTool, Content: []ContentBlock{ToolResult(&ToolResultContent{
		ToolUseID: callID,
		Content:   content,
		IsError:   isError,
	})}}
}

// ============================================================================

// ============================================================================

type ToolSchema struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// ============================================================================

// ============================================================================

type StreamChunkKind string

const (
	ChunkText      StreamChunkKind = "text"
	ChunkReasoning StreamChunkKind = "reasoning"
	ChunkToolCall  StreamChunkKind = "tool-call"
	ChunkDone      StreamChunkKind = "done"
)

type StreamChunk struct {
	Kind      StreamChunkKind `json:"kind"`
	Text      string          `json:"text,omitempty"`
	Reasoning string          `json:"reasoning,omitempty"`
	ToolCall  *ToolCall       `json:"toolCall,omitempty"`
}

type Usage struct {
	PromptTokens          int `json:"promptTokens"`
	CompletionTokens      int `json:"completionTokens"`
	PromptCacheHitTokens  int `json:"promptCacheHitTokens,omitempty"`
	PromptCacheMissTokens int `json:"promptCacheMissTokens,omitempty"`
}

// ============================================================================

// ============================================================================

type LlmFailureKind string

//

// CONTEXT_WINDOW_EXCEEDED / QUOTA / EMPTY_RESPONSE / INVALID_CREDENTIAL /

const (
	FailOverload LlmFailureKind = "overload"

	FailRateLimit LlmFailureKind = "rate-limit"

	FailResponseRefusal LlmFailureKind = "response-refusal"

	FailContextOverflow LlmFailureKind = "context-overflow"

	FailQuota LlmFailureKind = "quota"

	FailEmptyResponse LlmFailureKind = "empty-response"

	FailInvalidCredential LlmFailureKind = "invalid-credential"
)

func (f LlmFailureKind) Retryable() bool {
	switch f {
	case FailOverload, FailRateLimit, FailContextOverflow, FailEmptyResponse:
		return true
	default:
		return false
	}
}

type LlmFailure struct {
	Kind    LlmFailureKind `json:"kind"`
	Message string         `json:"message"`

	Cause error `json:"-"`
}

func (e *LlmFailure) Error() string {
	return fmt.Sprintf("llm failure [%s]: %s", e.Kind, e.Message)
}

func (e *LlmFailure) Unwrap() error { return e.Cause }

func ClassifyLlmError(err error) *LlmFailure {
	if err == nil {
		return nil
	}
	if f, ok := err.(*LlmFailure); ok {
		return f
	}

	return &LlmFailure{Kind: "unknown", Message: err.Error()}
}

// ============================================================================

// ============================================================================

var (
	reContextWindow = regexp.MustCompile(
		`(?:^|[^a-z0-9])context[\s_-](?:length|window)[\s_-](?:exceed(?:ed|s)?|overflow(?:ed)?|limit[\s_-]exceeded)(?:$|[^a-z0-9])` +
			`|(?:maximum|max)[\s_-]*(?:allowed|supported)?[\s_-]*context[\s_-]+(?:length|window)` +
			`|(?:^|[^a-z0-9])(?:prompt|input|request|messages?)[\s_-](?:too[\s_-](?:long|large)|exceed(?:ed)?)[\s_-]for[\s_-](?:the[\s_-])?(?:model)?(?:[^a-z0-9]|$)` +
			`|(?:^|[^a-z0-9])(?:input|prompt|request)[\s_-]+(?:is[\s_-]+)?too[\s_-](?:long|large)[\s_-]+for[\s_-]+(?:this|the)[\s_-]+model(?:[^a-z0-9]|$)`,
	)

	reQuota = regexp.MustCompile(
		`insufficient[\s_-]+(?:quota|balance|credits?)` +
			`|(?:quota|usage[\s_-]+limit)[\s_-]+(?:exceeded|exhausted|reached)` +
			`|(?:out[\s_-]+of|not[\s_-]+enough)[\s_-]+(?:credits?|budget|quota|balance)` +
			`|don.t[\s_-]+have[\s_-]+enough[\s_-]*(?:api[\s_-]+)?credits?` +
			`|(?:balance|credits?)[\s_-]+(?:exhausted|depleted)`,
	)

	reInvalidCred = regexp.MustCompile(
		`(?:invalid|wrong|incorrect|bad|unauthorized|auth)[\s_-]+(?:api[\s_-]?key|credential|token|secret)` +
			`|(?:api[\s_-]?key|credential)[\s_-]+(?:invalid|revoked|mismatch)` +
			`|401[\s_-]*(?:invalid|authentication)`,
	)

	reRateLimit = regexp.MustCompile(`rate[\s_-]*limit|too[\s_-]+many[\s_-]+requests|\b429\b`)

	reEmptyResponse = regexp.MustCompile(`empty[\s_-]+response|no[\s_-]+content|empty[\s_-]+completion|zero[\s_-]+output`)

	reRefusal = regexp.MustCompile(`(?:refus|declin)[e\w]*|cannot[\s_-]+(?:answer|comply)|policy[\s_-]+(?:refus|declin)`)
)

func ClassifyProviderDetail(detail string) LlmFailureKind {
	d := strings.ToLower(strings.TrimSpace(detail))
	if d == "" {
		return ""
	}
	switch {
	case reContextWindow.MatchString(d):
		return FailContextOverflow
	case reQuota.MatchString(d):
		return FailQuota
	case reInvalidCred.MatchString(d):
		return FailInvalidCredential
	case reRateLimit.MatchString(d):
		return FailRateLimit
	case reEmptyResponse.MatchString(d):
		return FailEmptyResponse
	case reRefusal.MatchString(d):
		return FailResponseRefusal
	default:
		return ""
	}
}

func NewProviderFailure(detail string) *LlmFailure {
	kind := ClassifyProviderDetail(detail)
	if kind == "" {
		kind = "unknown"
	}
	return &LlmFailure{Kind: kind, Message: detail}
}

// ============================================================================

// ============================================================================

type ChatRequest struct {
	Model string `json:"model"`

	Messages []Message `json:"messages"`

	System string `json:"system,omitempty"`

	Tools []ToolSchema `json:"tools,omitempty"`

	Temperature float64 `json:"temperature,omitempty"`

	MaxTokens int `json:"maxTokens,omitempty"`
}

type LLMAdapter interface {
	Name() string

	Chat(ctx context.Context, req ChatRequest, cb func(StreamChunk)) (Usage, error)
}
