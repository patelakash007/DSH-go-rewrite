//

package tests

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/JopenChen/dsh-go/pkg/llm"
	"github.com/JopenChen/dsh-go/pkg/session"
)

type flakyAdapter struct {
	mu        sync.Mutex
	failTimes int
	calls     int
	failKind  llm.LlmFailureKind
}

func (f *flakyAdapter) Name() string { return "flaky" }

func (f *flakyAdapter) Chat(ctx context.Context, req llm.ChatRequest, cb func(llm.StreamChunk)) (llm.Usage, error) {
	f.mu.Lock()
	f.calls++
	r := f.calls
	f.mu.Unlock()
	if r <= f.failTimes {
		return llm.Usage{}, &llm.LlmFailure{Kind: f.failKind, Message: "overload"}
	}
	return llm.Usage{PromptTokens: 1, CompletionTokens: 1}, nil
}

type retryEventSink struct {
	mu     sync.Mutex
	events []session.LLMRetryData
}

func (s *retryEventSink) write(d session.LLMRetryData) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, d)
}

func (s *retryEventSink) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.events)
}

func fastCfg(maxAttempts int) llm.RetryConfig {
	return llm.RetryConfig{
		MaxAttempts: maxAttempts,
		BaseBackoff: 1 * time.Millisecond,
		MaxBackoff:  5 * time.Millisecond,
		Jitter:      0,
	}
}

func TestLLMRetryWritesEventsAndSucceeds(t *testing.T) {
	inner := &flakyAdapter{failTimes: 3, failKind: llm.FailOverload}
	sink := &retryEventSink{}
	mw := llm.NewRetryMiddleware(inner, fastCfg(4), sink.write)

	usage, err := mw.Chat(context.Background(), llm.ChatRequest{}, func(llm.StreamChunk) {})
	if err != nil {
		t.Fatalf("attempt 4 should succeed, got %v", err)
	}
	if usage.PromptTokens+usage.CompletionTokens <= 0 {
		t.Fatalf("should return usage, got %+v", usage)
	}
	if sink.count() != 3 {
		t.Fatalf("should write 3 retry events, got %d", sink.count())
	}
	if inner.calls != 4 {
		t.Fatalf("should call 4 times in total, got %d", inner.calls)
	}
}

func TestLLMRetryNonRetryableNoRetry(t *testing.T) {
	inner := &flakyAdapter{failTimes: 100, failKind: llm.FailResponseRefusal}
	sink := &retryEventSink{}
	mw := llm.NewRetryMiddleware(inner, fastCfg(5), sink.write)

	_, err := mw.Chat(context.Background(), llm.ChatRequest{}, func(llm.StreamChunk) {})
	if err == nil {
		t.Fatal("refusal should bubble error")
	}

	if inner.calls != 1 {
		t.Fatalf("refusal should not retry, called %d times", inner.calls)
	}
	if sink.count() != 0 {
		t.Fatalf("refusal should not write retry events, got %d", sink.count())
	}
}

func TestLLMRetryExceedMaxReturnsError(t *testing.T) {
	inner := &flakyAdapter{failTimes: 100, failKind: llm.FailOverload}
	sink := &retryEventSink{}
	mw := llm.NewRetryMiddleware(inner, fastCfg(2), sink.write)

	_, err := mw.Chat(context.Background(), llm.ChatRequest{}, func(llm.StreamChunk) {})
	if err == nil {
		t.Fatal("exceeding max attempts should bubble error")
	}
	if inner.calls != 2 {
		t.Fatalf("max=2 should only call 2 times, got %d", inner.calls)
	}
}
