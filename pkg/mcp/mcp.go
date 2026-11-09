//

//

package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/JopenChen/dsh-go/pkg/tools"
)

// ============================================================================

// ============================================================================

type MCPSchema struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"inputSchema"`
}

// ============================================================================
// Transport
// ============================================================================

type Request struct {
	ID     uint64         `json:"id"`
	Method string         `json:"method"`
	Params map[string]any `json:"params,omitempty"`
}

type Response struct {
	ID     uint64          `json:"id"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  json.RawMessage `json:"error,omitempty"`
}

type Transport interface {
	RoundTrip(ctx context.Context, req Request) (Response, error)
}

// ============================================================================
// MCPClient
// ============================================================================

type Client struct {
	transport Transport
	sessionID string
}

func NewClient(t Transport) *Client {
	return &Client{transport: t}
}

func (c *Client) Initialize(ctx context.Context) error {
	resp, err := c.transport.RoundTrip(ctx, Request{ID: 1, Method: "initialize", Params: map[string]any{
		"protocolVersion": "2025-06-18",
		"clientInfo":      map[string]any{"name": "dsh-go", "version": "0.1"},
	}})
	if err != nil {
		return err
	}
	if len(resp.Error) > 0 {
		return fmt.Errorf("mcp: initialize error: %s", resp.Error)
	}

	var initRes struct {
		SessionID string `json:"sessionId,omitempty"`
	}
	_ = json.Unmarshal(resp.Result, &initRes)
	c.sessionID = initRes.SessionID
	return nil
}

func (c *Client) ListTools(ctx context.Context) ([]MCPSchema, error) {
	resp, err := c.transport.RoundTrip(ctx, Request{ID: 2, Method: "tools/list"})
	if err != nil {
		return nil, err
	}
	if len(resp.Error) > 0 {
		return nil, fmt.Errorf("mcp: list tools error: %s", resp.Error)
	}
	var out struct {
		Tools []MCPSchema `json:"tools"`
	}
	if err := json.Unmarshal(resp.Result, &out); err != nil {
		return nil, err
	}
	return out.Tools, nil
}

func (c *Client) CallTool(ctx context.Context, name string, input map[string]any) (string, error) {
	resp, err := c.transport.RoundTrip(ctx, Request{ID: 3, Method: "tools/call", Params: map[string]any{
		"name": name, "arguments": input,
	}})
	if err != nil {
		return "", err
	}
	if len(resp.Error) > 0 {
		return "", fmt.Errorf("mcp: call %q error: %s", name, resp.Error)
	}
	var out struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError,omitempty"`
	}
	if err := json.Unmarshal(resp.Result, &out); err != nil {
		return "", err
	}
	var sb strings.Builder
	for _, c := range out.Content {
		if c.Type == "text" {
			sb.WriteString(c.Text)
		}
	}
	if out.IsError {
		return sb.String(), fmt.Errorf("mcp: tool %q returned isError", name)
	}
	return sb.String(), nil
}

// ============================================================================
// ToolBridge：MCP → pkg/tools.Tool
// ============================================================================

type Bridge struct {
	client *Client
}

func NewBridge(client *Client) *Bridge {
	return &Bridge{client: client}
}

func (b *Bridge) ToTool(schema MCPSchema) (*tools.Tool, error) {
	node, err := MapSchema(schema.InputSchema)
	if err != nil {
		return nil, err
	}
	client := b.client
	name := schema.Name
	return &tools.Tool{
		Name:        name,
		Description: schema.Description,
		Schema:      node,
		Execute: func(ctx context.Context, input map[string]any) (any, error) {
			text, cerr := client.CallTool(ctx, name, input)
			if cerr != nil {
				return nil, cerr
			}
			return text, nil
		},
	}, nil
}

func (b *Bridge) ToTools(schemas []MCPSchema) ([]*tools.Tool, error) {
	out := make([]*tools.Tool, 0, len(schemas))
	for _, s := range schemas {
		tl, err := b.ToTool(s)
		if err != nil {
			return nil, err
		}
		out = append(out, tl)
	}
	return out, nil
}

func MapSchema(m map[string]any) (*tools.JsonSchemaNode, error) {
	if m == nil {
		return nil, nil
	}
	node := &tools.JsonSchemaNode{}
	if desc, ok := m["description"].(string); ok {
		node.Description = desc
	}
	if tv, ok := m["type"].(string); ok {
		st, err := mapType(tv)
		if err != nil {
			return nil, err
		}
		node.Type = st
	}
	if err := checkUnsupported(m); err != nil {
		return nil, err
	}
	// enum / const / default
	if e, ok := m["enum"].([]any); ok {
		node.Enum = e
	}
	if c, ok := m["const"]; ok {
		node.Const = c
	}
	if d, ok := m["default"]; ok {
		node.Default = d
	}
	// object properties
	if props, ok := m["properties"].(map[string]any); ok {
		node.Properties = map[string]*tools.JsonSchemaNode{}
		for k, v := range props {
			if sub, ok := v.(map[string]any); ok {
				child, err := MapSchema(sub)
				if err != nil {
					return nil, err
				}
				node.Properties[k] = child
			}
		}
	}
	if req, ok := m["required"].([]any); ok {
		for _, r := range req {
			if s, ok := r.(string); ok {
				node.Required = append(node.Required, s)
			}
		}
	}
	if add, ok := m["additionalProperties"].(bool); ok && !add {
		f := false
		node.AdditionalProperties = &f
	}
	// array items
	if items, ok := m["items"].(map[string]any); ok {
		child, err := MapSchema(items)
		if err != nil {
			return nil, err
		}
		node.Items = child
	}
	return node, nil
}

func mapType(t string) (tools.SchemaType, error) {
	switch t {
	case "string":
		return tools.TypeString, nil
	case "number":
		return tools.TypeNumber, nil
	case "integer":
		return tools.TypeInteger, nil
	case "boolean":
		return tools.TypeBoolean, nil
	case "array":
		return tools.TypeArray, nil
	case "object":
		return tools.TypeObject, nil
	case "null":
		return tools.TypeNull, nil
	default:
		return "", fmt.Errorf("mcp: unsupported type %q", t)
	}
}

func checkUnsupported(m map[string]any) error {
	for k := range m {
		switch k {
		case "type", "description", "properties", "required", "items", "enum",
			"const", "default", "additionalProperties":
			continue
		case "anyOf", "oneOf", "allOf", "not", "ref", "$ref":
			return fmt.Errorf("mcp: unsupported schema keyword %q (fail-closed)", k)
		}
	}
	return nil
}
