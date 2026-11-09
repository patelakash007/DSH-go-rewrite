//

//

//   go run ./examples/agent_loop
//

package main

import (
	"context"
	"fmt"
	"time"

	"github.com/JopenChen/dsh-go/pkg/agent"
	"github.com/JopenChen/dsh-go/pkg/brand"
	"github.com/JopenChen/dsh-go/pkg/llm"
	"github.com/JopenChen/dsh-go/pkg/session"
	"github.com/JopenChen/dsh-go/pkg/sysprompt"
	"github.com/JopenChen/dsh-go/pkg/tools"
)

type scriptedLLM struct {
	replies []func(cb func(llm.StreamChunk))
	callN   int
}

func (d *scriptedLLM) Name() string { return "scripted-demo" }

func (d *scriptedLLM) Chat(ctx context.Context, req llm.ChatRequest, cb func(llm.StreamChunk)) (llm.Usage, error) {
	d.callN++
	if d.callN <= len(d.replies) {
		d.replies[d.callN-1](cb)
	}

	cb(llm.StreamChunk{Kind: llm.ChunkDone})
	return llm.Usage{PromptTokens: int(len(req.Messages)), CompletionTokens: 2}, nil
}

func main() {
	ctx := context.Background()

	sl := session.NewSessionLog(brand.NewSessionID("agent-loop-demo"))

	sys := sysprompt.New()
	sys.Register("persona", 100, "You are a demo agent using DSH-go.")

	p := tools.NewPipeline().WithTool(&tools.Tool{
		Name:        "now",
		Description: "Returns current time",
		Execute: func(ctx context.Context, input map[string]any) (any, error) {
			return map[string]any{"time": time.Now().Format(time.RFC3339)}, nil
		},
	})

	adapter := &scriptedLLM{
		replies: []func(cb func(llm.StreamChunk)){
			func(cb func(llm.StreamChunk)) {
				cb(llm.StreamChunk{Kind: llm.ChunkToolCall, ToolCall: &llm.ToolCall{
					ID: "call-1", Name: "now", Input: map[string]any{},
				}})
			},
			func(cb func(llm.StreamChunk)) {
				cb(llm.StreamChunk{Kind: llm.ChunkText, Text: "Current time retrieved."})
			},
		},
	}

	a := agent.NewAgent(brand.NewSessionID("agent-loop-demo"), sl, sys, p, adapter)
	a.SetToolProvider(func(name string) *tools.Tool {

		if name == "now" {
			return &tools.Tool{
				Name: "now",
				Execute: func(ctx context.Context, input map[string]any) (any, error) {
					return map[string]any{"time": time.Now().Format(time.RFC3339)}, nil
				},
			}
		}
		return nil
	})

	a.SetRunContext(ctx, 30*time.Second)
	if err := a.Run("What time is it?"); err != nil {
		fmt.Printf("Run failed: %v\n", err)
		return
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if hasTurnEnd(sl) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	a.Dispose()

	printEvents(sl)
	fmt.Println("\nAgent Turn/Step loop example completed.")
}

func hasTurnEnd(sl *session.SessionLog) bool {
	evs := sl.Events()
	for _, ev := range evs {
		if ev.Type == session.EventTurnEnd {
			return true
		}
	}
	return false
}

func printEvents(sl *session.SessionLog) {
	seq := 0
	for _, ev := range sl.Events() {
		seq++
		_ = seq
		fmt.Printf("  #%d %s\n", ev.Seq, ev.Type)
	}
}
