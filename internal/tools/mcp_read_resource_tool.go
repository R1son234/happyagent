package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type MCPResourceReader interface {
	ReadResourcePreview(ctx context.Context, uri string, offsetBytes int, maxBytes int) (string, error)
}

type MCPReadResourceTool struct {
	reader MCPResourceReader
}

func NewMCPReadResourceTool(reader MCPResourceReader) *MCPReadResourceTool {
	return &MCPReadResourceTool{reader: reader}
}

func (t *MCPReadResourceTool) Definition() Definition {
	return Definition{
		Name:        "mcp_read_resource",
		Description: "Read the content of a registered MCP resource by URI. Supports byte offsets and optional per-call limits within the configured MCP resource output bound.",
		InputSchema: `{"type":"object","properties":{"uri":{"type":"string","description":"Exact MCP resource URI returned by list_capabilities."},"offset_bytes":{"type":"integer","minimum":0,"description":"Optional zero-based byte offset for reading a later window of the resource."},"max_bytes":{"type":"integer","minimum":1,"description":"Optional maximum bytes to return for this call. The configured MCP resource limit still applies."}},"required":["uri"]}`,
	}
}

func (t *MCPReadResourceTool) Execute(ctx context.Context, call Call) (Result, error) {
	if t.reader == nil {
		return Result{}, fmt.Errorf("mcp_read_resource is unavailable without an MCP manager")
	}

	var input struct {
		URI         string `json:"uri"`
		OffsetBytes int    `json:"offset_bytes"`
		MaxBytes    int    `json:"max_bytes"`
	}
	if err := json.Unmarshal(call.Arguments, &input); err != nil {
		return Result{}, fmt.Errorf("decode mcp_read_resource arguments: %w", err)
	}
	if strings.TrimSpace(input.URI) == "" {
		return Result{}, fmt.Errorf("mcp_read_resource uri must not be empty")
	}
	if input.OffsetBytes < 0 {
		return Result{}, fmt.Errorf("mcp_read_resource offset_bytes must be greater than or equal to zero")
	}
	if input.MaxBytes < 0 {
		return Result{}, fmt.Errorf("mcp_read_resource max_bytes must be greater than zero when set")
	}

	output, err := t.reader.ReadResourcePreview(ctx, input.URI, input.OffsetBytes, input.MaxBytes)
	if err != nil {
		return Result{}, err
	}
	return Result{Output: output}, nil
}
