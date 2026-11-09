//

package tests

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/JopenChen/dsh-go/pkg/mcp"
	"github.com/JopenChen/dsh-go/pkg/tools"
)

type mockMCP struct {
	tools []mcp.MCPSchema
	calls int
}

func (m *mockMCP) RoundTrip(_ context.Context, req mcp.Request) (mcp.Response, error) {
	switch req.Method {
	case "initialize":
		return mcp.Response{ID: req.ID, Result: json.RawMessage(`{"protocolVersion":"2025-06-18","serverInfo":{"name":"mock"}}`)}, nil
	case "tools/list":
		b, _ := json.Marshal(map[string]any{"tools": m.tools})
		return mcp.Response{ID: req.ID, Result: b}, nil
	case "tools/call":
		m.calls++
		return mcp.Response{ID: req.ID, Result: json.RawMessage(`{"content":[{"type":"text","text":"mcp:ok"}],"isError":false}`)}, nil
	default:
		return mcp.Response{ID: req.ID, Result: json.RawMessage(`{}`)}, nil
	}
}

func TestMCPListToolsAutoRegister(t *testing.T) {
	ctx := context.Background()
	server := &mockMCP{tools: []mcp.MCPSchema{
		{Name: "read_file", Description: "Read file",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{"type": "string", "description": "File path"},
				},
				"required": []any{"path"},
			}},
		{Name: "list_dir", Description: "List directory",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}}},
	}}
	client := mcp.NewClient(server)
	if err := client.Initialize(ctx); err != nil {
		t.Fatal(err)
	}

	schemas, err := client.ListTools(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(schemas) != 2 {
		t.Fatalf("should list 2 tools, got %d", len(schemas))
	}

	bridge := mcp.NewBridge(client)
	tls, err := bridge.ToTools(schemas)
	if err != nil {
		t.Fatal(err)
	}
	if len(tls) != 2 {
		t.Fatalf("should map 2 tools, got %d", len(tls))
	}
	if tls[0].Name != "read_file" || tls[0].Schema == nil {
		t.Fatalf("read_file mapping unexpected: %+v", tls[0])
	}

	out, err := tls[0].Execute(ctx, map[string]any{"path": "/a/b"})
	if err != nil {
		t.Fatal(err)
	}
	if s, ok := out.(string); !ok || s != "mcp:ok" {
		t.Fatalf("read_file result should be mcp:ok, got %v", out)
	}
	if server.calls != 1 {
		t.Fatalf("tools/call should be called once, got %d", server.calls)
	}
}

func TestMCPUnsupportedSchemaFailClosed(t *testing.T) {
	_, err := mcp.MapSchema(map[string]any{"type": "object", "anyOf": []any{}})
	if err == nil {
		t.Fatal("schema with anyOf should fail-closed with error")
	}
}

func TestMCPToolIsError(t *testing.T) {
	ctx := context.Background()
	client := mcp.NewClient(errorTransport{})
	_, err := client.CallTool(ctx, "failing", map[string]any{})
	if err == nil {
		t.Fatal("CallTool should return error when MCP isError=true")
	}
}

type errorTransport struct{}

func (errorTransport) RoundTrip(ctx context.Context, req mcp.Request) (mcp.Response, error) {
	return mcp.Response{ID: req.ID, Result: json.RawMessage(`{"content":[],"isError":true}`)}, nil
}

func TestMCPToolMappedSchema(t *testing.T) {
	node, err := mcp.MapSchema(map[string]any{
		"type":                 "object",
		"properties":           map[string]any{"mode": map[string]any{"type": "string", "enum": []any{"a", "b"}}},
		"required":             []any{"mode"},
		"additionalProperties": false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if node.Type != tools.TypeObject {
		t.Fatalf("type should be object, got %s", node.Type)
	}
	if len(node.Required) != 1 || node.Required[0] != "mode" {
		t.Fatalf("required should contain mode, got %v", node.Required)
	}
	mode := node.Properties["mode"]
	if mode == nil || len(mode.Enum) != 2 {
		t.Fatalf("mode sub-schema enum should be [a b], got %+v", mode)
	}
	if node.AdditionalProperties == nil || *node.AdditionalProperties {
		t.Fatal("additionalProperties=false should be mapped")
	}
}
