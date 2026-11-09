//

package tests

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/JopenChen/dsh-go/pkg/brand"
	"github.com/JopenChen/dsh-go/pkg/credentials"
	"github.com/JopenChen/dsh-go/pkg/llm"
	"github.com/JopenChen/dsh-go/pkg/llm/provider_deepseek"
	"github.com/JopenChen/dsh-go/pkg/storage"
)

func deepseekSSEWithUsage() string {
	return "data: " + `{"choices":[{"delta":{"content":"hi"}}]}` + "\n\n" +
		"data: " + `{"choices":[],"usage":{"prompt_tokens":1000,"completion_tokens":50,"prompt_cache_hit_tokens":900,"prompt_cache_miss_tokens":100}}` + "\n\n" +
		"data: [DONE]\n\n"
}

func TestDeepSeekParseUsageFromMockSSE(t *testing.T) {

	store := credentials.NewStore(storage.NewMemoryKV())
	if err := store.Set(context.Background(), brand.NewCredentialRef("DEEPSEEK_API_KEY"), "sk-mock"); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte(deepseekSSEWithUsage()))
	}))
	defer srv.Close()

	adapter := provider_deepseek.NewDeepSeek(store, provider_deepseek.WithBaseURL(srv.URL))

	text := ""
	usage, err := adapter.Chat(context.Background(), llm.ChatRequest{Model: "deepseek-chat"}, func(c llm.StreamChunk) {
		if c.Kind == llm.ChunkText {
			text += c.Text
		}
	})
	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}
	if text != "hi" {
		t.Fatalf("should receive streaming text hi, got %q", text)
	}
	if usage.PromptCacheHitTokens != 900 || usage.PromptCacheMissTokens != 100 {
		t.Fatalf("cache field parsing error: %+v", usage)
	}
	if usage.PromptTokens != 1000 || usage.CompletionTokens != 50 {
		t.Fatalf("normal token parsing error: %+v", usage)
	}

	if r := llm.HitRatioOf(usage); r != 0.9 {
		t.Fatalf("hit rate should be 0.9, got %v", r)
	}
}
