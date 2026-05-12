package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func (m *Manager) CallPrompt(ctx context.Context, name string, rawArgs json.RawMessage) (string, error) {
	prompt, ok := m.prompts[name]
	if !ok {
		return "", fmt.Errorf("mcp prompt %q is not registered", name)
	}
	client, ok := m.clients[prompt.ServerName]
	if !ok {
		return "", fmt.Errorf("mcp server %q is not connected", prompt.ServerName)
	}

	var arguments map[string]string
	if len(rawArgs) > 0 {
		if err := json.Unmarshal(rawArgs, &arguments); err != nil {
			return "", fmt.Errorf("decode prompt arguments for %q: %w", name, err)
		}
	}

	params := &sdk.GetPromptParams{Name: prompt.Name, Arguments: arguments}
	result, err := client.session.GetPrompt(ctx, params)
	if err != nil {
		return "", fmt.Errorf("get mcp prompt %q from server %q: %w", prompt.Name, prompt.ServerName, err)
	}

	return formatPromptResult(result), nil
}

func formatPromptResult(result *sdk.GetPromptResult) string {
	if result == nil {
		return ""
	}

	var parts []string
	for _, msg := range result.Messages {
		role := msg.Role
		content := extractPromptContent(msg.Content)
		parts = append(parts, fmt.Sprintf("[%s]\n%s", role, content))
	}
	return strings.Join(parts, "\n\n")
}

func extractPromptContent(content any) string {
	if content == nil {
		return ""
	}
	switch c := content.(type) {
	case *sdk.TextContent:
		return c.Text
	case *sdk.ImageContent:
		return fmt.Sprintf("[image: %s, %d bytes]", c.MIMEType, len(c.Data))
	default:
		if data, err := json.Marshal(c); err == nil {
			return string(data)
		}
	}
	return ""
}