package provider_deepseek

import (
	"context"
	"strings"
	"testing"

	"github.com/JopenChen/dsh-go/pkg/llm"
)

func TestReadSSEAssemblesFragmentedToolCalls(t *testing.T) {
	stream := strings.Join([]string{
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call-1","type":"function","function":{"name":"write_file","arguments":"{\"path\":"}}]}}]}`,
		``,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"note.txt\",\"content\":\"ok\"}"}}]}}]}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")
	adapter := &DeepSeek{}
	var chunks []llm.StreamChunk
	if _, err := adapter.readSSE(context.Background(), strings.NewReader(stream), func(chunk llm.StreamChunk) {
		chunks = append(chunks, chunk)
	}); err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 2 || chunks[0].Kind != llm.ChunkToolCall || chunks[1].Kind != llm.ChunkDone {
		t.Fatalf("chunks = %#v", chunks)
	}
	call := chunks[0].ToolCall
	if call == nil || call.ID != "call-1" || call.Name != "write_file" || call.Input["path"] != "note.txt" || call.Input["content"] != "ok" {
		t.Fatalf("tool call = %#v", call)
	}
}

func TestBuildPayloadPreservesToolProtocol(t *testing.T) {
	adapter := &DeepSeek{}
	payload := adapter.buildPayload(llm.ChatRequest{
		Model: "deepseek-chat",
		Messages: []llm.Message{
			llm.NewUserMessage("write it"),
			llm.NewAssistantMessage("", &llm.ToolCall{ID: "call-1", Name: "write_file", Input: map[string]any{"path": "x"}}),
			llm.NewToolResultMessage("call-1", "done", false),
		},
	})
	if len(payload.Messages) != 3 {
		t.Fatalf("messages = %#v", payload.Messages)
	}
	assistant := payload.Messages[1]
	if assistant.Role != "assistant" || len(assistant.ToolCalls) != 1 || assistant.ToolCalls[0].Function.Name != "write_file" {
		t.Fatalf("assistant = %#v", assistant)
	}
	tool := payload.Messages[2]
	if tool.Role != "tool" || tool.ToolCallID != "call-1" || tool.Content != "done" {
		t.Fatalf("tool = %#v", tool)
	}
}
