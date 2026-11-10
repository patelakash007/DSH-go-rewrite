package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/JopenChen/dsh-go/pkg/brand"
	"github.com/JopenChen/dsh-go/pkg/llm"
)

const (
	cotTranslationProvider = "deepseek-official"
	cotTranslationMaxChars = 4000
)

func (s *Server) cotTranslationAvailable() bool {
	key, configured := s.credStore.Resolve(context.Background(), brand.NewCredentialRef("DEEPSEEK_API_KEY"))
	return configured && strings.TrimSpace(key) != ""
}

func (s *Server) dispatchCotRPC(endpoint string, args map[string]any) (any, *RPCError) {
	switch endpoint {
	case "cotTranslation/limits":
		providers := []string{}
		if s.cotTranslationAvailable() {
			providers = append(providers, cotTranslationProvider)
		}
		return map[string]any{
			"maxTextChars": cotTranslationMaxChars,
			"preferences": map[string]any{
				"provider": cotTranslationProvider, "targetLanguage": "auto",
			},
			"availableProviders": providers,
		}, nil
	case "cotTranslation/translate":
		request, _ := args["request"].(map[string]any)
		text, _ := request["text"].(string)
		target, _ := request["targetLanguage"].(string)
		provider, _ := request["provider"].(string)
		if strings.TrimSpace(text) == "" {
			return nil, &RPCError{Code: "gateway/arguments-invalid", Message: "text is required"}
		}
		if len([]rune(text)) > cotTranslationMaxChars {
			return nil, &RPCError{Code: "cotTranslation/failed", Message: "text exceeds the translation limit"}
		}
		if provider != cotTranslationProvider {
			return nil, &RPCError{Code: "cotTranslation/failed", Message: "the requested translation provider is not available"}
		}
		if strings.TrimSpace(target) == "" {
			target = "English"
		}
		if !s.cotTranslationAvailable() {
			return nil, &RPCError{Code: "cotTranslation/failed", Message: "no DeepSeek API key is configured"}
		}
		translated, err := s.translateReasoning(context.Background(), text, target)
		if err != nil {
			return nil, &RPCError{Code: "cotTranslation/failed", Message: err.Error()}
		}
		return translated, nil
	default:
		return nil, &RPCError{Code: "gateway/method-not-found", Message: "unknown translation method"}
	}
}

func (s *Server) translateReasoning(ctx context.Context, text, target string) (string, error) {
	var builder strings.Builder
	_, err := s.router.Chat(ctx, llm.ChatRequest{
		Provider: "deepseek", Model: "deepseek-chat",
		System: "You translate an AI assistant's internal reasoning for a reader. Preserve code, identifiers, numbers and markdown. Reply with the translation only.",
		Messages: []llm.Message{llm.NewUserMessage(fmt.Sprintf(
			"Translate the following reasoning into %s. Return only the translated text.\n\n%s", target, text,
		))},
		MaxTokens: 2048,
	}, func(chunk llm.StreamChunk) {
		if chunk.Kind == llm.ChunkText {
			builder.WriteString(chunk.Text)
		}
	})
	if err != nil {
		return "", err
	}
	translated := strings.TrimSpace(builder.String())
	if translated == "" {
		return "", fmt.Errorf("the translation provider returned no text")
	}
	return translated, nil
}
