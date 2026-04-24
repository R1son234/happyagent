package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"happyagent/internal/tools"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type ToolAdapter struct {
	client     *Client
	definition tools.Definition
	remoteName string
}

func NewToolAdapter(client *Client, tool *sdk.Tool) (*ToolAdapter, error) {
	if tool == nil {
		return nil, fmt.Errorf("mcp tool from server %q is nil", client.name)
	}

	schemaBytes, err := json.Marshal(tool.InputSchema)
	if err != nil {
		return nil, fmt.Errorf("marshal mcp tool schema %q from server %q: %w", tool.Name, client.name, err)
	}

	return &ToolAdapter{
		client:     client,
		remoteName: tool.Name,
		definition: tools.Definition{
			Name:        client.name + "__" + tool.Name,
			Description: tool.Description,
			InputSchema: string(schemaBytes),
			Dangerous:   false,
		},
	}, nil
}

func (t *ToolAdapter) Definition() tools.Definition {
	return t.definition
}

func (t *ToolAdapter) Execute(ctx context.Context, call tools.Call) (tools.Result, error) {
	var args map[string]any
	if len(call.Arguments) == 0 {
		args = map[string]any{}
	} else if err := json.Unmarshal(call.Arguments, &args); err != nil {
		return tools.Result{}, fmt.Errorf("decode mcp tool arguments for %q: %w", t.definition.Name, err)
	}

	result, err := t.client.session.CallTool(ctx, &sdk.CallToolParams{
		Name:      t.remoteName,
		Arguments: args,
	})
	if err != nil {
		return tools.Result{}, fmt.Errorf("call mcp tool %q on server %q: %w", t.remoteName, t.client.name, err)
	}
	if result.IsError {
		return tools.Result{}, fmt.Errorf("mcp tool %q on server %q returned error: %s", t.remoteName, t.client.name, formatToolResult(result))
	}

	return tools.Result{Output: formatToolResult(result)}, nil
}

func formatToolResult(result *sdk.CallToolResult) string {
	if result == nil {
		return ""
	}
	if result.StructuredContent != nil {
		if data, err := json.MarshalIndent(result.StructuredContent, "", "  "); err == nil {
			return string(data)
		}
	}

	var parts []string
	for _, content := range result.Content {
		switch c := content.(type) {
		case *sdk.TextContent:
			parts = append(parts, c.Text)
		default:
			if data, err := json.Marshal(c); err == nil {
				parts = append(parts, string(data))
			}
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}
