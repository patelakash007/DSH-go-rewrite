//

//

//

//   Windows PowerShell：

//     go run ./examples/chat
//   Linux/macOS：

//

package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/JopenChen/dsh-go/pkg/brand"
	"github.com/JopenChen/dsh-go/pkg/credentials"
	"github.com/JopenChen/dsh-go/pkg/llm"
	"github.com/JopenChen/dsh-go/pkg/llm/provider_deepseek"
)

func main() {
	// ------------------------------------------------------------------

	// ------------------------------------------------------------------
	apiKey := os.Getenv("DEEPSEEK_API_KEY")
	if apiKey == "" {
		fmt.Println("Missing DEEPSEEK_API_KEY environment variable. Exiting.")
		fmt.Println("Please set DEEPSEEK_API_KEY before running.")
		os.Exit(1)
	}

	store := credentials.NewMemoryStore()
	if err := store.Set(context.Background(), brand.NewCredentialRef("DEEPSEEK_API_KEY"), apiKey); err != nil {
		fmt.Printf("Failed to store credentials: %v\n", err)
		os.Exit(1)
	}

	// ------------------------------------------------------------------

	// ------------------------------------------------------------------
	d := provider_deepseek.NewDeepSeek(store)
	tp := d.Transport()
	fmt.Printf("— DeepSeek connection pool: MaxIdleConnsPerHost=%d, IdleConnTimeout=%v —\n",
		tp.MaxIdleConnsPerHost, tp.IdleConnTimeout)

	// ------------------------------------------------------------------

	// ------------------------------------------------------------------
	messages := []llm.Message{
		llm.NewUserMessage("Introduce yourself in one sentence."),

		llm.NewAssistantText("Hello! I am a DSH-go assistant powered by DeepSeek."),
		llm.NewUserMessage("Can you give me a useful tip for prompt engineering?"),
	}

	model := os.Getenv("DEEPSEEK_MODEL")
	if model == "" {
		model = "deepseek-chat"
	}

	req := llm.ChatRequest{

		Model:       model,
		System:      "You are a friendly, concise assistant. Reply in at most 3 sentences.",
		Messages:    messages,
		Temperature: 0.7,
		MaxTokens:   512,
	}

	// ------------------------------------------------------------------

	// ------------------------------------------------------------------
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	fmt.Printf("\n— Current model: %s, streaming response —\n", model)
	usage, err := d.Chat(ctx, req, func(c llm.StreamChunk) {
		switch c.Kind {
		case llm.ChunkReasoning:

			fmt.Printf("🧠 %s", c.Reasoning)
		case llm.ChunkText:
			fmt.Print(c.Text)
		case llm.ChunkToolCall:
			if c.ToolCall != nil {
				fmt.Printf("\n[Tool Call] %s(%v)\n", c.ToolCall.Name, c.ToolCall.Input)
			}
		case llm.ChunkDone:
			fmt.Println("\n[Stream End]")
		}
	})
	fmt.Println()

	if err != nil {

		if f, ok := err.(*llm.LlmFailure); ok {
			fmt.Printf("\n❌ Call failed [kind=%s]: %s\n", f.Kind, f.Message)
		} else {
			fmt.Printf("\n❌ Call failed: %v\n", err)
		}
		os.Exit(1)
	}

	// ------------------------------------------------------------------

	// ------------------------------------------------------------------
	fmt.Printf("\n— Usage —\n")
	fmt.Printf("  prompt tokens      = %d\n", usage.PromptTokens)
	fmt.Printf("  completion tokens  = %d\n", usage.CompletionTokens)
	fmt.Printf("  cache hit tokens   = %d\n", usage.PromptCacheHitTokens)
	fmt.Printf("  cache miss tokens  = %d\n", usage.PromptCacheMissTokens)

	// ------------------------------------------------------------------

	// ------------------------------------------------------------------
	fmt.Println("\n— Interactive chat (enter message to continue, exit to quit) —")
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print("\nYou> ")
		line, _ := reader.ReadString('\n')
		line = strings.TrimSpace(line)
		if line == "" || strings.EqualFold(line, "exit") {
			break
		}

		messages = append(messages, llm.NewUserMessage(line))
		req.Messages = messages

		roundCtx, roundCancel := context.WithTimeout(context.Background(), 30*time.Second)

		var sb strings.Builder
		usage, err = d.Chat(roundCtx, req, func(c llm.StreamChunk) {
			if c.Kind == llm.ChunkText {
				sb.WriteString(c.Text)
				fmt.Print(c.Text)
			}
		})
		fmt.Println()
		if err != nil {
			if f, ok := err.(*llm.LlmFailure); ok {
				fmt.Printf("❌ [%s] %s\n", f.Kind, f.Message)
			} else {
				fmt.Printf("❌ %v\n", err)
			}
			break
		}

		if sb.Len() > 0 {
			messages = append(messages, llm.NewAssistantText(sb.String()))
		}

		roundCancel()
	}
}
