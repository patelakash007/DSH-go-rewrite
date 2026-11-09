//

package tests

import (
	"testing"

	"github.com/JopenChen/dsh-go/pkg/agent"
	"github.com/JopenChen/dsh-go/pkg/llm"
	"github.com/JopenChen/dsh-go/pkg/waterfall"
)

func TestRequestErrorRetryWithinMax(t *testing.T) {
	err := &llm.LlmFailure{Kind: llm.FailOverload, Message: "overload"}
	p := agent.NewRequestErrorPayload(err, 1, 3)
	act := agent.ResolveRequestError(agent.NewRequestErrorWaterfall(), p)
	if act.Kind != agent.ActionRetry {
		t.Fatalf("should retry on 1st attempt, got %s (%s)", act.Kind, act.Reason)
	}
}

func TestRequestErrorAbortAfterMax(t *testing.T) {
	err := &llm.LlmFailure{Kind: llm.FailRateLimit, Message: "rate limited"}

	maxRetries := 3
	var last agent.RequestErrorAction
	for count := 0; count <= maxRetries; count++ {
		p := agent.NewRequestErrorPayload(err, count, maxRetries)
		last = agent.ResolveRequestError(agent.NewRequestErrorWaterfall(), p)
		if last.Kind == agent.ActionAbort {
			break
		}
	}
	if last.Kind != agent.ActionAbort {
		t.Fatalf("should abort after exceeding max attempts, got %s", last.Kind)
	}
	if last.Reason == "" {
		t.Fatal("abort should include close reason")
	}
}

func TestRequestErrorCustomWaterfall(t *testing.T) {
	err := &llm.LlmFailure{Kind: llm.FailOverload, Message: "overload"}

	chain := agent.NewRequestErrorWaterfall(
		func(p *agent.RequestErrorPayload, next waterfall.NextFunc) error {
			a := agent.RequestErrorAction{Kind: agent.ActionAbort, Reason: "policy: never retry this route"}
			p.Action = &a
			return next()
		},
	)
	p := agent.NewRequestErrorPayload(err, 0, 5)
	if act := agent.ResolveRequestError(chain, p); act.Kind != agent.ActionAbort {
		t.Fatalf("custom middleware should abort consistently, got %s", act.Kind)
	}
}

func TestRequestErrorNonRetryableAbort(t *testing.T) {
	err := &llm.LlmFailure{Kind: llm.FailResponseRefusal, Message: "refused"}
	p := agent.NewRequestErrorPayload(err, 0, 10)
	act := agent.ResolveRequestError(agent.NewRequestErrorWaterfall(), p)
	if act.Kind != agent.ActionAbort {
		t.Fatalf("refusal should abort directly, got %s", act.Kind)
	}
	if p.RetryableError {
		t.Fatal("refusal should not be marked retryable")
	}
}

func TestRequestErrorRecordEvent(t *testing.T) {

	err := &llm.LlmFailure{Kind: llm.FailOverload, Message: "overload"}

	agent.RecordRequestError(nil, err)
	agent.RecordRequestError(nil, nil)
}
