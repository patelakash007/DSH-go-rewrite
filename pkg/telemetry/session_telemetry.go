//

//

package telemetry

import (
	"context"
	"sync"
	"time"
)

// ============================================================================

// ============================================================================

type EventHookData struct {
	EventType string
	LatencyMS time.Duration
}

type ChunkHookData struct {
	Seq       uint64
	TextLen   int
	LatencyMS time.Duration
}

type ToolHookData struct {
	ToolName  string
	Ok        bool
	LatencyMS time.Duration
}

type EventHook func(ctx context.Context, d EventHookData)
type ChunkHook func(ctx context.Context, d ChunkHookData)
type ToolHook func(ctx context.Context, d ToolHookData)

// ============================================================================

// ============================================================================

type SessionTelemetry struct {
	mu         sync.RWMutex
	eventHooks []EventHook
	chunkHooks []ChunkHook
	toolHooks  []ToolHook
	now        func() time.Time
}

func New() *SessionTelemetry {
	return &SessionTelemetry{
		eventHooks: []EventHook{},
		chunkHooks: []ChunkHook{},
		toolHooks:  []ToolHook{},
		now:        time.Now,
	}
}

func (t *SessionTelemetry) nowFn() func() time.Time {
	if t.now == nil {
		return time.Now
	}
	return t.now
}

// ============================================================================

// ============================================================================

func (t *SessionTelemetry) RegisterEventHook(h EventHook) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.eventHooks = append(t.eventHooks, h)
}

func (t *SessionTelemetry) RegisterChunkHook(h ChunkHook) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.chunkHooks = append(t.chunkHooks, h)
}

func (t *SessionTelemetry) RegisterToolHook(h ToolHook) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.toolHooks = append(t.toolHooks, h)
}

// ============================================================================

// ============================================================================

func (t *SessionTelemetry) RecordEvent(ctx context.Context, eventType string, startedAt time.Time) {
	d := EventHookData{EventType: eventType, LatencyMS: t.since(startedAt)}
	t.dispatchEvent(ctx, d)
}

func (t *SessionTelemetry) RecordChunk(ctx context.Context, seq uint64, textLen int, startedAt time.Time) {
	d := ChunkHookData{Seq: seq, TextLen: textLen, LatencyMS: t.since(startedAt)}
	t.dispatchChunk(ctx, d)
}

func (t *SessionTelemetry) RecordTool(ctx context.Context, toolName string, ok bool, startedAt time.Time) {
	d := ToolHookData{ToolName: toolName, Ok: ok, LatencyMS: t.since(startedAt)}
	t.dispatchTool(ctx, d)
}

func (t *SessionTelemetry) since(startedAt time.Time) time.Duration {
	if startedAt.IsZero() {
		return 0
	}
	return t.nowFn()().Sub(startedAt)
}

// ============================================================================

// ============================================================================

func (t *SessionTelemetry) dispatchEvent(ctx context.Context, d EventHookData) {
	t.mu.RLock()
	hooks := append([]EventHook(nil), t.eventHooks...)
	t.mu.RUnlock()
	for _, h := range hooks {
		callSafe(func() { h(ctx, d) })
	}
}

func (t *SessionTelemetry) dispatchChunk(ctx context.Context, d ChunkHookData) {
	t.mu.RLock()
	hooks := append([]ChunkHook(nil), t.chunkHooks...)
	t.mu.RUnlock()
	for _, h := range hooks {
		callSafe(func() { h(ctx, d) })
	}
}

func (t *SessionTelemetry) dispatchTool(ctx context.Context, d ToolHookData) {
	t.mu.RLock()
	hooks := append([]ToolHook(nil), t.toolHooks...)
	t.mu.RUnlock()
	for _, h := range hooks {
		callSafe(func() { h(ctx, d) })
	}
}

func callSafe(fn func()) {
	defer func() { _ = recover() }()
	fn()
}
