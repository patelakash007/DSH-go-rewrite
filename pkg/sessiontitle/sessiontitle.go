//

//

//

package sessiontitle

import (
	"context"
	"strings"

	"github.com/JopenChen/dsh-go/pkg/llm"
)

// ============================================================================

// ============================================================================

const defaultMaxRunes = 30

type Source string

const (
	SourceLLM      Source = "llm"
	SourceFallback Source = "fallback"
)

// ============================================================================

// ============================================================================

func truncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(strings.TrimSpace(s))
	if len(runes) <= max {
		return string(runes)
	}
	return string(runes[:max]) + "…"
}

func Fallback(firstUserMessage string) string {
	return FallbackMax(firstUserMessage, defaultMaxRunes)
}

func FallbackMax(firstUserMessage string, maxRunes int) string {
	return truncateRunes(firstUserMessage, maxRunes)
}

// ============================================================================

// ============================================================================

type Generator struct {
	LLM llm.LLMAdapter

	Model string

	MaxRunes int

	System string

	MaxTokens int
}

type Result struct {
	Title  string `json:"title"`
	Source Source `json:"source"`
}

var FallbackSystemPrompt = "You are a title extraction assistant. Based on user input, summarize their intent in a short phrase of at most 10 words. Output only the title itself, no quotes, no explanation."

func (g *Generator) maxRunes() int {
	if g == nil || g.MaxRunes <= 0 {
		return defaultMaxRunes
	}
	return g.MaxRunes
}

func (g *Generator) fallbackSystem() string {
	if g == nil || g.System == "" {
		return FallbackSystemPrompt
	}
	return g.System
}

func (g *Generator) Generate(ctx context.Context, firstUserMessage string) Result {

	fallback := FallbackMax(firstUserMessage, g.maxRunes())

	if g == nil || g.LLM == nil || strings.TrimSpace(firstUserMessage) == "" {
		return Result{Title: fallback, Source: SourceFallback}
	}

	title, err := g.generateFromLLM(ctx, firstUserMessage)
	if err != nil || strings.TrimSpace(title) == "" {

		return Result{Title: fallback, Source: SourceFallback}
	}
	return Result{Title: truncateRunes(title, g.maxRunes()), Source: SourceLLM}
}

func (g *Generator) generateFromLLM(ctx context.Context, firstUserMessage string) (string, error) {
	req := llm.ChatRequest{
		Model:       g.Model,
		System:      g.fallbackSystem(),
		Messages:    []llm.Message{llm.NewUserMessage(firstUserMessage)},
		MaxTokens:   g.MaxTokens,
		Temperature: 0.2,
	}
	var sb strings.Builder
	_, err := g.LLM.Chat(ctx, req, func(c llm.StreamChunk) {
		if c.Kind == llm.ChunkText {
			sb.WriteString(c.Text)
		}
	})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(TrimTitle(sb.String())), nil
}

func TrimTitle(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "\"'「」『』《》【】。，、；：！？. ")
	return s
}
