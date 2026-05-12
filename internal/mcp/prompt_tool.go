package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"happyagent/internal/tools"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type MCPPromptTool struct {
	manager *Manager
}

func NewMCPPromptTool(manager *Manager) *MCPPromptTool {
	return &MCPPromptTool{manager: manager}
}

func (t *MCPPromptTool) Definition() tools.Definition {
	return tools.Definition{
		Name:        "mcp_get_prompt",
		Description: "Call an MCP server prompt template. Arguments: name (required, the qualified prompt name like server__prompt_name), arguments (optional, a JSON object with prompt parameters).",
		InputSchema: `{
			"type": "object",
			"properties": {
				"name": {
					"type": "string",
					"description": "The qualified prompt name (format: server__prompt-name)"
				},
				"arguments": {
					"type": "object",
					"description": "Optional key-value pairs to fill prompt arguments",
					"additionalProperties": {
						"type": "string"
					}
				}
			},
			"required": ["name"]
		}`,
		Dangerous: false,
	}
}

func (t *MCPPromptTool) Execute(ctx context.Context, call tools.Call) (tools.Result, error) {
	var args struct {
		Name      string            `json:"name"`
		Arguments map[string]string `json:"arguments,omitempty"`
	}
	if len(call.Arguments) > 0 {
		if err := json.Unmarshal(call.Arguments, &args); err != nil {
			return tools.Result{}, fmt.Errorf("decode mcp_get_prompt arguments: %w", err)
		}
	}
	if args.Name == "" {
		return tools.Result{}, fmt.Errorf("mcp_get_prompt: name is required")
	}

	if !t.manager.HasPrompt(args.Name) {
		return tools.Result{}, fmt.Errorf("mcp prompt %q is not registered", args.Name)
	}

	params := &sdk.GetPromptParams{
		Name:      promptNameFromQualified(args.Name),
		Arguments: args.Arguments,
	}

	prompt := t.manager.prompts[args.Name]
	client, ok := t.manager.clients[prompt.ServerName]
	if !ok {
		return tools.Result{}, fmt.Errorf("mcp server %q is not connected", prompt.ServerName)
	}

	result, err := client.session.GetPrompt(ctx, params)
	if err != nil {
		return tools.Result{}, fmt.Errorf("get mcp prompt %q: %w", args.Name, err)
	}

	output := formatPromptResult(result)
	if output == "" {
		output = "[empty prompt result]"
	}

	return tools.Result{Output: output}, nil
}

func promptNameFromQualified(qualified string) string {
	parts := strings.SplitN(qualified, "__", 2)
	if len(parts) >= 2 {
		return parts[1]
	}
	return qualified
}