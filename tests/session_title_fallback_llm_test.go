//

package tests

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/JopenChen/dsh-go/pkg/brand"
	"github.com/JopenChen/dsh-go/pkg/llm"
	"github.com/JopenChen/dsh-go/pkg/session"
	"github.com/JopenChen/dsh-go/pkg/sessiontitle"
)

// ============================================================================
// Mock LLMAdapter
// ============================================================================

type mockLLM struct {
	text string
	err  error
}

func (m *mockLLM) Name() string { return "mock-title" }

func (m *mockLLM) Chat(ctx context.Context, req llm.ChatRequest, cb func(llm.StreamChunk)) (llm.Usage, error) {
	if m.err != nil {
		return llm.Usage{}, m.err
	}

	for _, r := range m.text {
		cb(llm.StreamChunk{Kind: llm.ChunkText, Text: string(r)})
	}
	cb(llm.StreamChunk{Kind: llm.ChunkDone})
	return llm.Usage{}, nil
}

// ============================================================================

// ============================================================================

func TestFallbackPrefix(t *testing.T) {
	long := "This is a very long user message used to verify that title prefixes are truncated to around thirty runes, with subsequent content omitted from the title."
	got := sessiontitle.Fallback(long)
	if len([]rune(got)) != 30 && len([]rune(got)) != 31 {
		t.Fatalf("fallback should truncate to ~30 runes (excluding ellipsis), got %d runes (%q)", len([]rune(got)), got)
	}
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("overly long input should end with ellipsis, got %q", got)
	}

	if !isValidUTF8(got) {
		t.Fatalf("fallback output contains invalid UTF-8: %q", got)
	}
}

func TestFallbackShort(t *testing.T) {
	got := sessiontitle.Fallback("Fix login error")
	if got != "Fix login error" {
		t.Fatalf("short message should return as-is, got %q", got)
	}
}

func TestFallbackEmpty(t *testing.T) {
	if got := sessiontitle.Fallback("   "); got != "" {
		t.Fatalf("blank content should return empty title, got %q", got)
	}
}

func TestNoLLMFallsBack(t *testing.T) {
	gen := &sessiontitle.Generator{}
	res := gen.Generate(context.Background(), "Please help deploy a go service")
	if res.Source != sessiontitle.SourceFallback {
		t.Fatalf("Source should be fallback when no LLM, got %v", res.Source)
	}
	if res.Title == "" {
		t.Fatal("fallback title cannot be empty")
	}
}

func TestLLMGenerated(t *testing.T) {
	gen := &sessiontitle.Generator{LLM: &mockLLM{text: "【Deploy Go Service】"}}
	res := gen.Generate(context.Background(), "Please help deploy a go service")
	if res.Source != sessiontitle.SourceLLM {
		t.Fatalf("Source should be llm on LLM success, got %v", res.Source)
	}

	if trimmed := sessiontitle.TrimTitle(res.Title); sessiontitle.TrimTitle(trimmed) == "" {
		t.Fatalf("LLM title is empty: %q", res.Title)
	}
	if strings.Contains(res.Title, "【") || strings.Contains(res.Title, "】") {
		t.Fatalf("LLM title decoration symbols not trimmed: %q", res.Title)
	}
	if res.Title != "Deploy Go Service" {
		t.Fatalf("LLM title after trimming should be 'Deploy Go Service', got %q", res.Title)
	}
}

func TestLLMFailureFallsBack(t *testing.T) {
	gen := &sessiontitle.Generator{LLM: &mockLLM{err: &llm.LlmFailure{Kind: llm.FailOverload, Message: "overloaded"}}}
	res := gen.Generate(context.Background(), "Deploy a go service")
	if res.Source != sessiontitle.SourceFallback {
		t.Fatalf("should fallback when LLM fails, got %v", res.Source)
	}
	if res.Title != "Deploy a go service" {
		t.Fatalf("fallback title should be first message prefix, got %q", res.Title)
	}
}

func TestLLMEmptyOutputFallsBack(t *testing.T) {
	gen := &sessiontitle.Generator{LLM: &mockLLM{text: "  \n  "}}
	res := gen.Generate(context.Background(), "Help me write test cases")
	if res.Source != sessiontitle.SourceFallback {
		t.Fatalf("empty LLM output should fallback, got %v", res.Source)
	}
	if res.Title != "Help me write test cases" {
		t.Fatalf("fallback title should be first message prefix, got %q", res.Title)
	}
}

func TestTitleEventFold(t *testing.T) {
	sl := session.NewSessionLog(brand.NewSessionID("title-s1"))

	if _, err := sl.Append(session.SessionTitleData{Title: "Old Title"}); err != nil {
		t.Fatalf("append Old Title event failed: %v", err)
	}
	if _, err := sl.Append(session.SessionTitleData{Title: "New Title"}); err != nil {
		t.Fatalf("append New Title event failed: %v", err)
	}
	proj := session.FoldAllFromLog(sl)
	if !proj.Title.Present || proj.Title.Title != "New Title" {
		t.Fatalf("latest-wins fold should be 'New Title', got %+v", proj.Title)
	}
}

func isValidUTF8(s string) bool {
	return utf8.ValidString(s)
}
