package tools

import (
	"context"
	"fmt"
	"testing"
)

type stubMCPResourceReader struct {
	output string
	err    error
}

func (r stubMCPResourceReader) ReadResourcePreview(ctx context.Context, uri string, offsetBytes int, maxBytes int) (string, error) {
	_ = ctx
	if r.err != nil {
		return "", r.err
	}
	return fmt.Sprintf("%s:%s:%d:%d", r.output, uri, offsetBytes, maxBytes), nil
}

func TestMCPReadResourceTool(t *testing.T) {
	tool := NewMCPReadResourceTool(stubMCPResourceReader{output: "resource"})

	result, err := tool.Execute(context.Background(), Call{
		Name:      "mcp_read_resource",
		Arguments: []byte(`{"uri":"demo://project-summary"}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Output != "resource:demo://project-summary:0:0" {
		t.Fatalf("unexpected output: %q", result.Output)
	}
}

func TestMCPReadResourceToolSupportsPreviewArguments(t *testing.T) {
	tool := NewMCPReadResourceTool(stubMCPResourceReader{output: "resource"})

	result, err := tool.Execute(context.Background(), Call{
		Name:      "mcp_read_resource",
		Arguments: []byte(`{"uri":"demo://project-summary","offset_bytes":10,"max_bytes":32}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Output != "resource:demo://project-summary:10:32" {
		t.Fatalf("unexpected output: %q", result.Output)
	}
}

func TestMCPReadResourceToolRejectsEmptyURI(t *testing.T) {
	tool := NewMCPReadResourceTool(stubMCPResourceReader{output: "resource"})

	_, err := tool.Execute(context.Background(), Call{
		Name:      "mcp_read_resource",
		Arguments: []byte(`{"uri":""}`),
	})
	if err == nil || err.Error() != "mcp_read_resource uri must not be empty" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMCPReadResourceToolPropagatesReaderError(t *testing.T) {
	tool := NewMCPReadResourceTool(stubMCPResourceReader{err: fmt.Errorf("boom")})

	_, err := tool.Execute(context.Background(), Call{
		Name:      "mcp_read_resource",
		Arguments: []byte(`{"uri":"demo://project-summary"}`),
	})
	if err == nil || err.Error() != "boom" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMCPReadResourceToolRejectsNegativeOffset(t *testing.T) {
	tool := NewMCPReadResourceTool(stubMCPResourceReader{output: "resource"})

	_, err := tool.Execute(context.Background(), Call{
		Name:      "mcp_read_resource",
		Arguments: []byte(`{"uri":"demo://project-summary","offset_bytes":-1}`),
	})
	if err == nil || err.Error() != "mcp_read_resource offset_bytes must be greater than or equal to zero" {
		t.Fatalf("unexpected error: %v", err)
	}
}
