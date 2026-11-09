package tests

import (
	"encoding/json"
	"testing"

	"github.com/JopenChen/dsh-go/pkg/brand"
	"github.com/JopenChen/dsh-go/pkg/session"
)

func TestRequestHeaderRebuild(t *testing.T) {
	sl := session.NewSessionLog(brand.NewSessionID("rh_1"))

	_, _ = sl.Append(session.RequestHeaderData{ConfigEpoch: 1, SystemHash: "old", ToolCount: 2})
	_, _ = sl.Append(session.RequestHeaderData{ConfigEpoch: 2, SystemHash: "new", ToolCount: 3})
	_, _ = sl.Append(session.RequestContextData{Provider: "deepseek", Model: "deepseek-chat", Window: 64000, Reason: "initial"})

	header, ok := session.FoldEpochHeader(sl.Events())
	if !ok {
		t.Fatal("should fold to header")
	}
	if header.ConfigEpoch != 2 || header.SystemHash != "new" || header.ToolCount != 3 {
		t.Fatalf("header snapshot should be latest: %+v", header)
	}

	ctx, ok := session.FoldRequestContext(sl.Events())
	if !ok {
		t.Fatal("should fold to context")
	}
	if ctx.Provider != "deepseek" || ctx.Model != "deepseek-chat" || ctx.Reason != session.ReasonInitial {
		t.Fatalf("context routing unexpected: %+v", ctx)
	}

	snap := &session.RequestHeaderSnapshot{
		Header:       header,
		Context:      ctx,
		SystemPrompt: "system text",
		ToolSchemas:  json.RawMessage(`[{"name":"bash"}]`),
	}
	system, schemas, err := session.RebuildFromHeader(snap)
	if err != nil {
		t.Fatalf("RebuildFromHeader failed: %v", err)
	}
	if system != "system text" {
		t.Fatalf("rebuild system mismatch: %q", system)
	}
	if string(schemas) != `[{"name":"bash"}]` {
		t.Fatalf("rebuild schemas mismatch: %s", schemas)
	}
}

func TestRequestHeaderCompactionRebuild(t *testing.T) {
	sl := session.NewSessionLog(brand.NewSessionID("rh_2"))
	_, _ = sl.Append(session.RequestHeaderData{ConfigEpoch: 5, SystemHash: "h5", ToolCount: 4})

	events := sl.Events()

	header, ok := session.FoldEpochHeader(events)
	if !ok {
		t.Fatal("should still fold header after compaction")
	}
	if header.SystemHash != "h5" {
		t.Fatalf("rebuild should be based on snapshot: %+v", header)
	}
}
