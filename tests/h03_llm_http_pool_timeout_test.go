//

package tests

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/JopenChen/dsh-go/pkg/brand"
	"github.com/JopenChen/dsh-go/pkg/credentials"
	"github.com/JopenChen/dsh-go/pkg/llm"
	"github.com/JopenChen/dsh-go/pkg/llm/provider_deepseek"
)

func newCredsH03() *credentials.Store {
	s := credentials.NewMemoryStore()
	_ = s.Set(context.Background(), brand.NewCredentialRef("DEEPSEEK_API_KEY"), "sk-test-123")
	return s
}

// ============================================================================

// ============================================================================

func TestH03DefaultPoolTuning(t *testing.T) {
	d := provider_deepseek.NewDeepSeek(newCredsH03())
	tp := d.Transport()
	if tp == nil {
		t.Fatal("default Transport() should be non-nil")
	}
	if tp.MaxIdleConns != provider_deepseek.DefaultMaxIdleConns {
		t.Fatalf("MaxIdleConns = %d, want %d", tp.MaxIdleConns, provider_deepseek.DefaultMaxIdleConns)
	}
	if tp.MaxIdleConnsPerHost != provider_deepseek.DefaultMaxIdleConnsPerHost {
		t.Fatalf("MaxIdleConnsPerHost = %d, want %d", tp.MaxIdleConnsPerHost, provider_deepseek.DefaultMaxIdleConnsPerHost)
	}
	if tp.IdleConnTimeout != provider_deepseek.DefaultIdleConnTimeout {
		t.Fatalf("IdleConnTimeout = %v, want %v", tp.IdleConnTimeout, provider_deepseek.DefaultIdleConnTimeout)
	}
	if tp.TLSHandshakeTimeout != provider_deepseek.DefaultTLSHandshakeTimeout {
		t.Fatalf("TLSHandshakeTimeout = %v, want %v", tp.TLSHandshakeTimeout, provider_deepseek.DefaultTLSHandshakeTimeout)
	}
	if !tp.ForceAttemptHTTP2 {
		t.Fatal("ForceAttemptHTTP2 should be true (H2 enabled by default)")
	}

	if d.HTTPClient().Timeout != 0 {
		t.Fatalf("http.Client.Timeout = %v, want 0 (delegated to H01 ctx budget)", d.HTTPClient().Timeout)
	}
}

// ============================================================================

// ============================================================================

func TestH03OptionsOverride(t *testing.T) {
	d := provider_deepseek.NewDeepSeek(newCredsH03(),
		provider_deepseek.WithMaxIdleConnsPerHost(42),
		provider_deepseek.WithIdleConnTimeout(7*time.Second),
		provider_deepseek.WithTLSHandshakeTimeout(3*time.Second),
		provider_deepseek.DisableHTTP2(),
	)
	tp := d.Transport()
	if tp.MaxIdleConnsPerHost != 42 {
		t.Fatalf("MaxIdleConnsPerHost after WithMaxIdleConnsPerHost(42) = %d", tp.MaxIdleConnsPerHost)
	}
	if tp.IdleConnTimeout != 7*time.Second {
		t.Fatalf("IdleConnTimeout after WithIdleConnTimeout(7s) = %v", tp.IdleConnTimeout)
	}
	if tp.TLSHandshakeTimeout != 3*time.Second {
		t.Fatalf("TLSHandshakeTimeout after WithTLSHandshakeTimeout(3s) = %v", tp.TLSHandshakeTimeout)
	}
	if tp.ForceAttemptHTTP2 {
		t.Fatal("ForceAttemptHTTP2 after DisableHTTP2 should be false")
	}
	if tp.TLSNextProto == nil {
		t.Fatal("TLSNextProto after DisableHTTP2 should be non-nil empty map")
	}
}

// ============================================================================

// ============================================================================

//

func TestH03CtxCancelDuringDo(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started <- struct{}{}

		<-release
	}))
	defer func() {

		select {
		case <-release:
		default:
			close(release)
		}
		srv.Close()
	}()

	d := provider_deepseek.NewDeepSeek(newCredsH03(), provider_deepseek.WithBaseURL(srv.URL))

	parent, cancel := context.WithCancel(context.Background())
	go func() {
		<-started
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	_, err := d.Chat(parent, llm.ChatRequest{Model: "deepseek-chat"}, nil)

	close(release)

	if err == nil {
		t.Fatal("Chat after ctx cancellation should return error, got nil")
	}
	var f *llm.LlmFailure
	if !errors.As(err, &f) {
		t.Fatalf("Chat returned error type should be *LlmFailure, got %T: %v", err, err)
	}

	if !errors.Is(f, context.Canceled) {
		t.Fatalf("LlmFailure error chain errors.Is(f, context.Canceled) = false; err=%v cause=%v", f, f.Cause)
	}
}

// ============================================================================

// ============================================================================

func TestH03CtxCancelDuringSSEBody(t *testing.T) {
	var sentCount atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		for i := 0; i < 10000; i++ {
			select {
			case <-r.Context().Done():
				return
			default:
			}

			line := fmt.Sprintf("data: {\"choices\":[{\"delta\":{\"content\":\"%d\"}}]}\n\n", i)
			_, werr := fmt.Fprint(w, line)
			if werr != nil {
				return
			}
			flusher.Flush()
			sentCount.Add(1)
			time.Sleep(20 * time.Millisecond)
		}
	}))
	defer srv.Close()

	d := provider_deepseek.NewDeepSeek(newCredsH03(), provider_deepseek.WithBaseURL(srv.URL))

	start := time.Now()

	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()

	var gotChunks int
	_, err := d.Chat(ctx, llm.ChatRequest{Model: "deepseek-chat"}, func(c llm.StreamChunk) {
		if c.Kind == llm.ChunkText {
			gotChunks++
		}
	})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("Chat after ctx timeout in SSE should return error, got nil")
	}
	var f *llm.LlmFailure
	if !errors.As(err, &f) {
		t.Fatalf("returned error should be *LlmFailure, got %T", err)
	}

	if !errors.Is(f, context.DeadlineExceeded) {
		t.Fatalf("should errors.Is to DeadlineExceeded; err=%v cause=%v", f, f.Cause)
	}

	if elapsed > 500*time.Millisecond {
		t.Fatalf("ctx timeout duration = %v, should be < 500ms (H03 SSE ctx interrupt failed)", elapsed)
	}

	if gotChunks < 1 {
		t.Fatalf("chunks read = %d, should be >= 1 (otherwise mock server may not have sent anything)", gotChunks)
	}
}

// ============================================================================

// ============================================================================

func TestH03NormalSSEUsage(t *testing.T) {
	payload := strings.Join([]string{
		"data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}",
		"data: {\"choices\":[{\"delta\":{\"content\":\" World\"}}]}",
		"data: {\"choices\":[],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":2,\"prompt_cache_hit_tokens\":7,\"prompt_cache_miss_tokens\":3}}",
		"data: [DONE]",
		"",
	}, "\n\n")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(payload))
	}))
	defer srv.Close()

	d := provider_deepseek.NewDeepSeek(newCredsH03(), provider_deepseek.WithBaseURL(srv.URL))

	var text strings.Builder
	usage, err := d.Chat(context.Background(), llm.ChatRequest{Model: "deepseek-chat"}, func(c llm.StreamChunk) {
		if c.Kind == llm.ChunkText {
			text.WriteString(c.Text)
		}
	})
	if err != nil {
		t.Fatalf("Chat returned error: %v", err)
	}
	if text.String() != "Hello World" {
		t.Fatalf("concatenated content = %q, want \"Hello World\"", text.String())
	}
	if usage.PromptTokens != 10 || usage.CompletionTokens != 2 {
		t.Fatalf("usage = %+v, want prompt=10 completion=2", usage)
	}
	if usage.PromptCacheHitTokens != 7 || usage.PromptCacheMissTokens != 3 {
		t.Fatalf("cache usage hit=%d miss=%d, want hit=7 miss=3",
			usage.PromptCacheHitTokens, usage.PromptCacheMissTokens)
	}
}
