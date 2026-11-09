//

//

package telemetry

import (
	"context"
	"sync"
	"time"
)

// ============================================================================
// Baggage & Span
// ============================================================================

type Baggage struct {
	SessionID string `json:"sessionId"`
	TurnID    string `json:"turnId"`
	StepID    string `json:"stepId"`
}

type Span struct {
	Name    string            `json:"name"`
	Baggage Baggage           `json:"baggage"`
	Started time.Time         `json:"started"`
	Ended   time.Time         `json:"ended"`
	Kind    string            `json:"kind,omitempty"` // event/chunk/tool
	Attrs   map[string]string `json:"attrs,omitempty"`
}

// ============================================================================
// Exporter
// ============================================================================

type Exporter interface {
	ExportSpan(ctx context.Context, span Span) error
}

// ============================================================================

// ============================================================================

type InMemoryExporter struct {
	mu    sync.Mutex
	Spans []Span
}

func (m *InMemoryExporter) ExportSpan(_ context.Context, span Span) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Spans = append(m.Spans, span)
	return nil
}

func (m *InMemoryExporter) Collect() []Span {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Span, len(m.Spans))
	copy(out, m.Spans)
	return out
}

// ============================================================================
// BaggageProvider & OTelBridge
// ============================================================================

type BaggageProvider func() Baggage

type OTelBridge struct {
	exporter Exporter
	baggage  BaggageProvider
}

func NewOTelBridge(exporter Exporter, baggage BaggageProvider) *OTelBridge {
	return &OTelBridge{exporter: exporter, baggage: baggage}
}

func (b *OTelBridge) Attach(tm *SessionTelemetry) {
	bag := b.currentBaggage()
	tm.RegisterEventHook(func(ctx context.Context, d EventHookData) {
		_ = b.exporter.ExportSpan(ctx, Span{
			Name:    "session.event." + d.EventType,
			Baggage: bag,
			Kind:    "event",
			Started: time.Now(), Ended: time.Now(),
			Attrs: map[string]string{"latency_ms": d.LatencyMS.String()},
		})
	})
	tm.RegisterChunkHook(func(ctx context.Context, d ChunkHookData) {
		_ = b.exporter.ExportSpan(ctx, Span{
			Name:    "session.chunk",
			Baggage: bag,
			Kind:    "chunk",
			Started: time.Now(), Ended: time.Now(),
			Attrs: map[string]string{"seq": u64str(d.Seq), "len": itoa(d.TextLen)},
		})
	})
	tm.RegisterToolHook(func(ctx context.Context, d ToolHookData) {
		_ = b.exporter.ExportSpan(ctx, Span{
			Name:    "session.tool." + d.ToolName,
			Baggage: bag,
			Kind:    "tool",
			Started: time.Now(), Ended: time.Now(),
			Attrs: map[string]string{"ok": boolstr(d.Ok), "latency_ms": d.LatencyMS.String()},
		})
	})
}

func (b *OTelBridge) currentBaggage() Baggage {
	if b.baggage == nil {
		return Baggage{}
	}
	return b.baggage()
}

// ============================================================================

// ============================================================================

func u64str(v uint64) string {
	return itoa(int(v))
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	var buf [20]byte
	i := len(buf)
	for v != 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func boolstr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
