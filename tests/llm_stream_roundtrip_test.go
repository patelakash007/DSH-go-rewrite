package tests

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/JopenChen/dsh-go/pkg/brand"
	"github.com/JopenChen/dsh-go/pkg/credentials"
	"github.com/JopenChen/dsh-go/pkg/llm"
	"github.com/JopenChen/dsh-go/pkg/llm/provider_deepseek"
)

func newDeepseekWithMock(t *testing.T, handler http.HandlerFunc) *provider_deepseek.DeepSeek {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	store := credentials.NewMemoryStore()
	_ = store.Set(context.Background(), brand.NewCredentialRef("DEEPSEEK_API_KEY"), "sk-test")

	return provider_deepseek.NewDeepSeek(store,
		provider_deepseek.WithBaseURL(srv.URL),
		provider_deepseek.WithHTTPClient(srv.Client()),
	)
}

func sseData(obj string) string {
	return "data: " + obj + "\n"
}

func TestLLMDeepseekSSERoundtrip(t *testing.T) {
	adapter := newDeepseekWithMock(t, func(w http.ResponseWriter, r *http.Request) {

		if r.Header.Get("Authorization") != "Bearer sk-test" {
			t.Errorf("Authorization header unexpected: %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "text/event-stream")

		_, _ = w.Write([]byte(sseData(`{"choices":[{"delta":{"reasoning_content":"thinking"}}]}`)))

		_, _ = w.Write([]byte(sseData(`{"choices":[{"delta":{"content":"hello"}}]}`)))

		_, _ = w.Write([]byte(sseData(`{"choices":[{"delta":{"tool_calls":[{"id":"call_1","type":"function","function":{"name":"bash","arguments":"{\"cmd\":\"ls\"}"}}]}}]}`)))

		_, _ = w.Write([]byte(sseData(`{"usage":{"prompt_tokens":10,"completion_tokens":5,"prompt_cache_hit_tokens":8,"prompt_cache_miss_tokens":2}}`)))
		_, _ = w.Write([]byte("data: [DONE]\n"))
	})

	var chunks []llm.StreamChunk
	usage, err := adapter.Chat(context.Background(), llm.ChatRequest{
		Model:  "deepseek-chat",
		System: "You are an assistant",
		Messages: []llm.Message{
			llm.NewUserMessage("hello"),
		},
	}, func(c llm.StreamChunk) {
		chunks = append(chunks, c)
	})
	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}

	if len(chunks) < 4 {
		t.Fatalf("should have at least 4 chunks (reasoning+text+tool+done): %d", len(chunks))
	}
	if chunks[0].Kind != llm.ChunkReasoning || chunks[0].Reasoning != "thinking" {
		t.Fatalf("reasoning chunk unexpected: %+v", chunks[0])
	}
	if chunks[1].Kind != llm.ChunkText || chunks[1].Text != "hello" {
		t.Fatalf("text chunk unexpected: %+v", chunks[1])
	}
	if chunks[2].Kind != llm.ChunkToolCall {
		t.Fatalf("tool call chunk unexpected: %+v", chunks[2])
	}
	if chunks[2].ToolCall == nil || chunks[2].ToolCall.Name != "bash" || chunks[2].ToolCall.Input["cmd"] != "ls" {
		t.Fatalf("tool call content unexpected: %+v", chunks[2].ToolCall)
	}
	if chunks[len(chunks)-1].Kind != llm.ChunkDone {
		t.Fatalf("final chunk should be done: %+v", chunks[len(chunks)-1])
	}

	if usage.PromptTokens != 10 || usage.CompletionTokens != 5 {
		t.Fatalf("usage token unexpected: %+v", usage)
	}
	if usage.PromptCacheHitTokens != 8 || usage.PromptCacheMissTokens != 2 {
		t.Fatalf("usage cache fields unexpected: %+v", usage)
	}
}

func TestLLMDeepseekFailureMapping(t *testing.T) {
	cases := []struct {
		name   string
		status int
		want   llm.LlmFailureKind
	}{
		{"rate-limit", http.StatusTooManyRequests, llm.FailRateLimit},
		{"overload", http.StatusInternalServerError, llm.FailOverload},
		{"context-overflow", http.StatusBadRequest, llm.FailContextOverflow},
		{"refusal", http.StatusForbidden, llm.FailResponseRefusal},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			adapter := newDeepseekWithMock(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(`{"error":"boom"}`))
			})
			_, err := adapter.Chat(context.Background(), llm.ChatRequest{Model: "m"}, func(llm.StreamChunk) {})
			if err == nil {
				t.Fatal("should return error")
			}
			failure := llm.ClassifyLlmError(err)
			if failure == nil || failure.Kind != tc.want {
				t.Fatalf("classification = %v, want %s", failure, tc.want)
			}
		})
	}
}

func TestLLMDeepseekMissingKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not initiate real request")
	}))
	defer srv.Close()

	store := credentials.NewMemoryStore()
	adapter := provider_deepseek.NewDeepSeek(store, provider_deepseek.WithBaseURL(srv.URL))

	_, err := adapter.Chat(context.Background(), llm.ChatRequest{Model: "m"}, func(llm.StreamChunk) {})
	if err == nil {
		t.Fatal("missing key should return error")
	}
	if !strings.Contains(err.Error(), "missing API key") {
		t.Fatalf("error message unexpected: %v", err)
	}
}

func TestLLMAdapterInterface(t *testing.T) {
	var _ llm.LLMAdapter = (*provider_deepseek.DeepSeek)(nil)
}
