package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/JopenChen/dsh-go/pkg/compaction"
	"github.com/JopenChen/dsh-go/pkg/llm"
	"github.com/JopenChen/dsh-go/pkg/session"
)

type nativeSummarizer struct {
	server   *Server
	provider string
	model    string
}

func (n nativeSummarizer) Summarize(ctx context.Context, events []session.SessionEvent, maxRunes int) (string, error) {
	var transcript strings.Builder
	for _, event := range session.EffectiveEvents(events) {
		switch data := event.Data.(type) {
		case session.UserMessageData:
			transcript.WriteString("User: ")
			transcript.WriteString(data.Content)
			transcript.WriteString("\n\n")
		case session.AssistantMessageData:
			if strings.TrimSpace(data.Content) == "" {
				continue
			}
			transcript.WriteString("Assistant: ")
			transcript.WriteString(data.Content)
			transcript.WriteString("\n\n")
		}
	}
	if strings.TrimSpace(transcript.String()) == "" {
		return "", fmt.Errorf("nothing to summarize")
	}
	var summary strings.Builder
	_, err := n.server.router.Chat(ctx, llm.ChatRequest{
		Provider: n.provider, Model: n.model,
		System:    "Summarize the conversation so it can replace the original history. Keep decisions, facts, file paths, identifiers, and open tasks. Reply with the summary only.",
		Messages:  []llm.Message{llm.NewUserMessage(transcript.String())},
		MaxTokens: 1024,
	}, func(chunk llm.StreamChunk) {
		if chunk.Kind == llm.ChunkText {
			summary.WriteString(chunk.Text)
		}
	})
	if err != nil {
		return "", err
	}
	text := strings.TrimSpace(summary.String())
	if text == "" {
		return "", fmt.Errorf("the model returned an empty summary")
	}
	runes := []rune(text)
	if maxRunes > 0 && len(runes) > maxRunes {
		text = string(runes[:maxRunes]) + "…"
	}
	return text, nil
}

func (s *Server) compactSession(ctx context.Context, rec *SessionRecord) (string, error) {
	events := rec.Log.Events()
	if len(events) < 4 {
		return "Nothing to compact yet.", nil
	}
	rec.mu.Lock()
	provider := rec.Provider
	model := rec.Model
	rec.mu.Unlock()
	if provider == "" {
		provider = "deepseek"
	}
	if model == "" {
		model = "deepseek-chat"
	}
	compactor := compaction.New(compaction.Config{
		MaxTokens: 1, KeepTail: 4, SummaryMaxRunes: 1600,
	}, nativeSummarizer{server: s, provider: provider, model: model})
	result, err := compactor.Compact(ctx, events)
	if err != nil {
		return "", err
	}
	if _, err := rec.Log.AppendWithSurface(
		session.AssistantMessageData{Content: result.Summary},
		session.NewReplaceOp(result.Start, result.End, nil),
	); err != nil {
		return "", err
	}
	return "Compacted the conversation into a summary.", nil
}
