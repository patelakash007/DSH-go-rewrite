//

//

//

//

//   go run ./examples/mcp
//

package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/JopenChen/dsh-go/pkg/mcp"
	"github.com/JopenChen/dsh-go/pkg/tools"
)

func main() {
	ctx := context.Background()

	// ------------------------------------------------------------------

	// ------------------------------------------------------------------
	srv := newMemoryServer()
	client := mcp.NewClient(srv)

	// ------------------------------------------------------------------

	// ------------------------------------------------------------------
	if err := client.Initialize(ctx); err != nil {
		panic(err)
	}
	fmt.Println("— MCP initialization handshake succeeded —")

	// ------------------------------------------------------------------

	// ------------------------------------------------------------------
	schemas, err := client.ListTools(ctx)
	if err != nil {
		panic(err)
	}
	fmt.Printf("— MCP server exposed tools count: %d —\n", len(schemas))
	for _, s := range schemas {
		fmt.Printf("  · %s — %s\n", s.Name, s.Description)
	}

	// ------------------------------------------------------------------

	// ------------------------------------------------------------------
	bridge := mcp.NewBridge(client)
	toolList, err := bridge.ToTools(schemas)
	if err != nil {
		panic(err)
	}

	fmt.Println("\n— Unified call via bridged Tool —")
	for _, t := range toolList {

		out, err := t.Execute(ctx, demoInputFor(t.Name))
		if err != nil {
			fmt.Printf("  [%s] ❌ %v\n", t.Name, err)
			continue
		}
		fmt.Printf("  [%s] => %v\n", t.Name, out)
	}

	// ------------------------------------------------------------------

	// ------------------------------------------------------------------
	fmt.Println("\n— Added to tools pipeline —")
	p := tools.NewPipeline()
	for _, t := range toolList {
		p.WithTool(t)
	}

	fmt.Printf("  · Registered %d MCP tools to pipeline\n", len(toolList))
}

func demoInputFor(name string) map[string]any {
	switch name {
	case "echo":
		return map[string]any{"text": "Hello, MCP"}
	case "add":
		return map[string]any{"a": float64(2), "b": float64(3)}
	default:
		return map[string]any{}
	}
}

// ---------------------------------------------------------------------------

//

// ---------------------------------------------------------------------------

type memoryServer struct {
}

func newMemoryServer() *memoryServer {
	return &memoryServer{}
}

func (s *memoryServer) RoundTrip(ctx context.Context, req mcp.Request) (mcp.Response, error) {
	switch req.Method {
	case "initialize":

		return ok(req.ID, map[string]any{
			"protocolVersion": "2025-06-18",
			"serverInfo":      map[string]any{"name": "demo-memory-server", "version": "0.0.1"},
		}), nil

	case "tools/list":

		return ok(req.ID, map[string]any{"tools": []map[string]any{
			{
				"name":        "echo",
				"description": "Echo input text",
				"inputSchema": map[string]any{
					"type":     "object",
					"required": []any{"text"},
					"properties": map[string]any{
						"text": map[string]any{"type": "string", "description": "Text to echo"},
					},
				},
			},
			{
				"name":        "add",
				"description": "Add two numbers",
				"inputSchema": map[string]any{
					"type":     "object",
					"required": []any{"a", "b"},
					"properties": map[string]any{
						"a": map[string]any{"type": "number"},
						"b": map[string]any{"type": "number"},
					},
				},
			},
		}}), nil

	case "tools/call":

		name, _ := req.Params["name"].(string)
		args, _ := req.Params["arguments"].(map[string]any)
		switch name {
		case "echo":
			text, _ := args["text"].(string)
			return ok(req.ID, map[string]any{"content": []map[string]any{{"type": "text", "text": "echo: " + text}}}), nil
		case "add":
			a, _ := args["a"].(float64)
			b, _ := args["b"].(float64)
			return ok(req.ID, map[string]any{"content": []map[string]any{{"type": "text", "text": fmt.Sprintf("%.0f", a+b)}}}), nil
		default:
			return errResp(req.ID, fmt.Sprintf("unknown tool %q", name)), nil
		}
	}
	return errResp(req.ID, "method not found: "+req.Method), nil
}

func ok(id uint64, result map[string]any) mcp.Response {
	raw, _ := json.Marshal(result)
	return mcp.Response{ID: id, Result: raw}
}

func errResp(id uint64, msg string) mcp.Response {
	raw, _ := json.Marshal(map[string]any{"message": msg})
	return mcp.Response{ID: id, Error: raw}
}
