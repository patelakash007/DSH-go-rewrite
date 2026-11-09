//

//

package llm

import (
	"context"
	"math"
	"math/rand"
	"time"

	"github.com/JopenChen/dsh-go/pkg/session"
)

type RetryConfig struct {
	MaxAttempts int

	BaseBackoff time.Duration

	MaxBackoff time.Duration

	Jitter float64
}

func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts: 3,
		BaseBackoff: 100 * time.Millisecond,
		MaxBackoff:  4 * time.Second,
		Jitter:      0.1,
	}
}

type RetryWriter func(data session.LLMRetryData)

type RetryMiddleware struct {
	inner LLMAdapter
	cfg   RetryConfig
	write RetryWriter
}

func NewRetryMiddleware(inner LLMAdapter, cfg RetryConfig, write RetryWriter) *RetryMiddleware {
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 3
	}
	if cfg.BaseBackoff <= 0 {
		cfg.BaseBackoff = 100 * time.Millisecond
	}
	if cfg.MaxBackoff <= 0 {
		cfg.MaxBackoff = 4 * time.Second
	}
	return &RetryMiddleware{inner: inner, cfg: cfg, write: write}
}

func (m *RetryMiddleware) Name() string { return m.inner.Name() }

func (m *RetryMiddleware) Chat(ctx context.Context, req ChatRequest, cb func(StreamChunk)) (Usage, error) {
	var lastErr error
	for attempt := 1; attempt <= m.cfg.MaxAttempts; attempt++ {
		usage, err := m.inner.Chat(ctx, req, cb)
		if err == nil {
			return usage, nil
		}
		lastErr = err
		f := ClassifyLlmError(err)

		if f.Kind != FailOverload && f.Kind != FailRateLimit {
			return usage, err
		}
		if attempt == m.cfg.MaxAttempts {

			return usage, err
		}
		backoff := m.computeBackoff(attempt)
		if m.write != nil {
			m.write(session.LLMRetryData{
				Attempt:   attempt,
				BackoffMs: backoff.Milliseconds(),
				Error:     err.Error(),
			})
		}
		select {
		case <-ctx.Done():
			return usage, ctx.Err()
		case <-time.After(backoff):
		}
	}
	return Usage{}, lastErr
}

func (m *RetryMiddleware) computeBackoff(attempt int) time.Duration {
	base := float64(m.cfg.BaseBackoff) * math.Pow(2, float64(attempt-1))
	if base > float64(m.cfg.MaxBackoff) {
		base = float64(m.cfg.MaxBackoff)
	}
	ms := base
	if m.cfg.Jitter > 0 {
		span := m.cfg.Jitter * base
		ms = base - span + rand.Float64()*(2*span)
	}
	if ms < 0 {
		ms = 0
	}
	return time.Duration(ms)
}
