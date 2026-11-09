//

//

package tests

import (
	"testing"

	"github.com/JopenChen/dsh-go/pkg/llm"
)

// ============================================================================

// ============================================================================

func TestR04KindEnumComplete(t *testing.T) {
	want := []struct {
		kind llm.LlmFailureKind
		name string
	}{
		{llm.FailOverload, "overload"},
		{llm.FailRateLimit, "rate-limit"},
		{llm.FailResponseRefusal, "response-refusal"},
		{llm.FailContextOverflow, "context-overflow"},
		{llm.FailQuota, "quota"},
		{llm.FailEmptyResponse, "empty-response"},
		{llm.FailInvalidCredential, "invalid-credential"},
	}
	for _, w := range want {
		if string(w.kind) != w.name {
			t.Fatalf("Kind constant unexpected: %q != %q", w.kind, w.name)
		}
	}
}

// ============================================================================

// ============================================================================

func TestR04RetryableSemantics(t *testing.T) {
	cases := []struct {
		kind llm.LlmFailureKind
		want bool
	}{
		{llm.FailOverload, true},
		{llm.FailRateLimit, true},
		{llm.FailContextOverflow, true},
		{llm.FailEmptyResponse, true},
		{llm.FailResponseRefusal, false},
		{llm.FailQuota, false},
		{llm.FailInvalidCredential, false},
		{llm.LlmFailureKind("unknown"), false},
	}
	for _, c := range cases {
		if got := c.kind.Retryable(); got != c.want {
			t.Fatalf("Retryable(%q) = %v, want %v", c.kind, got, c.want)
		}
	}
}

// ============================================================================

// ============================================================================

func TestR04ClassifyProviderDetail(t *testing.T) {
	cases := []struct {
		detail string
		want   llm.LlmFailureKind
	}{

		{"This model's maximum context length is 128K tokens. However, your messages resulted in 200K tokens", llm.FailContextOverflow},
		{"context_length_exceeded", llm.FailContextOverflow},
		{"prompt is too long for this model's context window", llm.FailContextOverflow},

		{"Insufficient_quota", llm.FailQuota},
		{"quota exceeded for the current billing cycle", llm.FailQuota},
		{"You don't have enough API credits", llm.FailQuota},
		{"out of budget", llm.FailQuota},

		{"invalid API key", llm.FailInvalidCredential},
		{"api_key_mismatch", llm.FailInvalidCredential},
		{"incorrect credential provided", llm.FailInvalidCredential},

		{"Rate limit reached", llm.FailRateLimit},
		{"too many requests", llm.FailRateLimit},
		{"HTTP 429 Too Many Requests", llm.FailRateLimit},

		{"empty response from model, no content blocks", llm.FailEmptyResponse},
		{"finish_reason stop with no content", llm.FailEmptyResponse},

		{"The model refused to answer the request due to safety policy", llm.FailResponseRefusal},

		{"Some unrelated network error: connection reset", llm.LlmFailureKind("unknown")},
	}
	for _, c := range cases {
		got := llm.ClassifyProviderDetail(c.detail)

		gotStr := string(got)
		if gotStr == "" {
			gotStr = "unknown"
		}
		if gotStr != string(c.want) {
			t.Fatalf("ClassifyProviderDetail(%q) = %q, want %q", c.detail, gotStr, c.want)
		}
	}
}

func TestR04EmptyDetail(t *testing.T) {
	if got := llm.NewProviderFailure(""); got.Kind != "unknown" {
		t.Fatalf("empty detail should classify as unknown, got %q", got.Kind)
	}
}

func TestR04NewProviderFailure(t *testing.T) {
	f := llm.NewProviderFailure("quota exhausted for the workspace")
	if f.Kind != llm.FailQuota {
		t.Fatalf("NewProviderFailure quota = %q", f.Kind)
	}
	if f.Message != "quota exhausted for the workspace" {
		t.Fatalf("Message should retain original text, got %q", f.Message)
	}
	u := llm.NewProviderFailure("gibberish unknown backend hiccup")
	if u.Kind != "unknown" {
		t.Fatalf("unrecognized should classify as unknown, got %q", u.Kind)
	}
}
