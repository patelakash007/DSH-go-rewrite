//

package tests

import (
	"context"
	"testing"
	"time"

	"github.com/JopenChen/dsh-go/pkg/telemetry"
)

func TestOTelBridgeSpansCarryBaggage(t *testing.T) {
	tm := telemetry.New()
	exporter := &telemetry.InMemoryExporter{}

	bag := telemetry.Baggage{SessionID: "s-1", TurnID: "t-1", StepID: "st-1"}
	bridge := telemetry.NewOTelBridge(exporter, func() telemetry.Baggage { return bag })
	bridge.Attach(tm)

	ctx := context.Background()
	now := time.Now()
	tm.RecordEvent(ctx, "user/message", now.Add(-time.Millisecond))
	tm.RecordChunk(ctx, 7, 42, now.Add(-time.Millisecond))
	tm.RecordTool(ctx, "bash", true, now.Add(-time.Millisecond))

	spans := exporter.Collect()
	if len(spans) != 3 {
		t.Fatalf("should export 3 spans, got %d", len(spans))
	}
	for i, sp := range spans {
		if sp.Baggage.SessionID != "s-1" || sp.Baggage.TurnID != "t-1" || sp.Baggage.StepID != "st-1" {
			t.Fatalf("span[%d] should carry session/turn/step baggage, got %+v", i, sp.Baggage)
		}
	}

	kinds := map[string]bool{}
	for _, sp := range spans {
		kinds[sp.Kind] = true
	}
	if !kinds["event"] || !kinds["chunk"] || !kinds["tool"] {
		t.Fatalf("should contain event/chunk/tool Kind types, got %v", kinds)
	}
}

func TestOTelSpanNameSuffix(t *testing.T) {
	tm := telemetry.New()
	exporter := &telemetry.InMemoryExporter{}
	bridge := telemetry.NewOTelBridge(exporter, nil)
	bridge.Attach(tm)
	tm.RecordTool(context.Background(), "todo_write", false, time.Now())

	for _, sp := range exporter.Collect() {
		if sp.Kind == "tool" && sp.Name != "session.tool.todo_write" {
			t.Fatalf("tool span name should be session.tool.todo_write, got %s", sp.Name)
		}
	}
}

func TestOTelNoBaggageProvider(t *testing.T) {
	tm := telemetry.New()
	exporter := &telemetry.InMemoryExporter{}
	bridge := telemetry.NewOTelBridge(exporter, nil)
	bridge.Attach(tm)
	tm.RecordEvent(context.Background(), "user/message", time.Now())
	if n := len(exporter.Collect()); n != 1 {
		t.Fatalf("should export 1 span, got %d", n)
	}
}
