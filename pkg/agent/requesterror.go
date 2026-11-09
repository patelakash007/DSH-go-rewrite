//

//

package agent

import (
	"context"

	"github.com/JopenChen/dsh-go/pkg/llm"
	"github.com/JopenChen/dsh-go/pkg/session"
	"github.com/JopenChen/dsh-go/pkg/waterfall"
)

type RequestErrorActionKind string

const (
	ActionRetry RequestErrorActionKind = "retry"
	ActionAbort RequestErrorActionKind = "abort"
)

type RequestErrorAction struct {
	Kind RequestErrorActionKind `json:"kind"`

	Reason string `json:"reason,omitempty"`
}

type RequestErrorPayload struct {
	Err error

	RetryCount int

	MaxRetries int

	RetryableError bool

	Action *RequestErrorAction
}

func ShouldRetryError(err error) bool {
	if err == nil {
		return false
	}
	f := llm.ClassifyLlmError(err)
	return f.Kind == llm.FailOverload || f.Kind == llm.FailRateLimit
}

func NewRequestErrorPayload(err error, retryCount, maxRetries int) *RequestErrorPayload {
	return &RequestErrorPayload{
		Err:            err,
		RetryCount:     retryCount,
		MaxRetries:     maxRetries,
		RetryableError: ShouldRetryError(err),
	}
}

func TerminalRetryDecision(p *RequestErrorPayload) RequestErrorAction {
	if p.RetryableError && p.RetryCount < p.MaxRetries {
		return RequestErrorAction{Kind: ActionRetry, Reason: "retryable llm failure within max retries"}
	}
	if p.RetryableError {
		return RequestErrorAction{Kind: ActionAbort, Reason: "max request-error retries exceeded"}
	}
	return RequestErrorAction{Kind: ActionAbort, Reason: "non-retryable request error"}
}

type RequestErrorWaterfall = waterfall.Chain[RequestErrorPayload]

func NewRequestErrorWaterfall(handlers ...waterfall.Handler[RequestErrorPayload]) *RequestErrorWaterfall {
	return waterfall.New(handlers...)
}

//

func ResolveRequestError(chain *RequestErrorWaterfall, p *RequestErrorPayload) RequestErrorAction {
	if chain == nil {
		return TerminalRetryDecision(p)
	}
	_ = chain.Run(context.Background(), p)
	if p.Action != nil {
		return *p.Action
	}
	return TerminalRetryDecision(p)
}

func RecordRequestError(log *session.SessionLog, err error) {
	if log == nil || err == nil {
		return
	}
	_, _ = log.Append(session.AgentErrorData{
		Message: err.Error(),
		Pkg:     "pkg/agent",
	})
}
