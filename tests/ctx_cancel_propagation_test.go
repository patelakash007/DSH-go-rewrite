//

package tests

import (
	"context"
	"testing"
	"time"

	"github.com/JopenChen/dsh-go/pkg/agent"
	"github.com/JopenChen/dsh-go/pkg/brand"
	"github.com/JopenChen/dsh-go/pkg/llm"
	"github.com/JopenChen/dsh-go/pkg/session"
	"github.com/JopenChen/dsh-go/pkg/tools"
)

func TestH01PipelinePropagatesCancel(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())

	var observed bool
	tool := &tools.Tool{
		Name: "probe", Description: "probe",
		Execute: func(ctx context.Context, input map[string]any) (any, error) {
			select {
			case <-ctx.Done():
				observed = true
			case <-time.After(2 * time.Second):
			}
			return "ok", nil
		},
	}
	pipeline := tools.NewPipeline().WithTool(tool)

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	res := pipeline.Run(parent, &tools.ToolCallRequest{
		CallID: brand.NewToolCallID("h1-1"), Tool: "probe", Input: map[string]any{},
	}, tool)
	if !observed {
		t.Fatal("tool ctx should observe Done after parent cancellation (cancellation not propagated to tool)")
	}
	if res.IsError {
		t.Fatalf("tool should not error, only transparently pass cancellation signal: %s", res.Error)
	}
}

func TestH01AgentPathCancellation(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())

	sid := brand.NewSessionID("h1-agent")
	pipeline := tools.NewPipeline()
	var canceled bool
	tool := &tools.Tool{
		Name: "probe", Description: "probe",
		Execute: func(ctx context.Context, _ map[string]any) (any, error) {
			select {
			case <-ctx.Done():
				canceled = true
			case <-time.After(2 * time.Second):
			}
			return "ok", nil
		},
	}
	pipeline.WithTool(tool)

	adapter := &toolCallOnceLLM{toolName: "probe"}
	a := agent.NewAgent(sid, session.NewSessionLog(sid), nil, pipeline, adapter)
	a.SetToolProvider(func(name string) *tools.Tool { return tool })
	a.SetRunContext(parent, 0)
	_ = a.Run("do it")

	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && !canceled {
		time.Sleep(20 * time.Millisecond)
	}
	a.Dispose()
	if !canceled {
		t.Fatal("tools executed via pipeline should observe Done after upstream ctx bound to Agent is cancelled")
	}
}

type toolCallOnceLLM struct {
	toolName string
}

func (m *toolCallOnceLLM) Name() string { return "toolonce" }

func (m *toolCallOnceLLM) Chat(ctx context.Context, req llm.ChatRequest, cb func(llm.StreamChunk)) (llm.Usage, error) {
	if cb != nil {
		cb(llm.StreamChunk{Kind: llm.ChunkToolCall, ToolCall: &llm.ToolCall{ID: "call_1", Name: m.toolName, Input: map[string]any{}}})
		cb(llm.StreamChunk{Kind: llm.ChunkDone})
	}
	return llm.Usage{}, nil
}
