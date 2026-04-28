package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSearchDocsToolSearchesDocsAndRootDocsOnly(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "internal"), 0o755); err != nil {
		t.Fatalf("mkdir internal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "architecture.md"), []byte("runtime capability needle\n"), 0o644); err != nil {
		t.Fatalf("write docs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("readme needle\n"), 0o644); err != nil {
		t.Fatalf("write readme: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "internal", "code.md"), []byte("private needle\n"), 0o644); err != nil {
		t.Fatalf("write internal: %v", err)
	}

	tool, err := NewSearchDocsTool(root)
	if err != nil {
		t.Fatalf("NewSearchDocsTool() error = %v", err)
	}

	result, err := tool.Execute(context.Background(), Call{
		Name:      SearchDocsToolName,
		Arguments: []byte(`{"query":"needle","max_results":10}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(result.Output, "docs/architecture.md") {
		t.Fatalf("expected docs match, got %q", result.Output)
	}
	if !strings.Contains(result.Output, "README.md") {
		t.Fatalf("expected README match, got %q", result.Output)
	}
	if strings.Contains(result.Output, "internal/code.md") {
		t.Fatalf("expected internal docs to be excluded, got %q", result.Output)
	}
}

func TestSearchDocsToolReturnsNoMatchesMessage(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}

	tool, err := NewSearchDocsTool(root)
	if err != nil {
		t.Fatalf("NewSearchDocsTool() error = %v", err)
	}

	result, err := tool.Execute(context.Background(), Call{
		Name:      SearchDocsToolName,
		Arguments: []byte(`{"query":"absent"}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Output != "(no matching docs)" {
		t.Fatalf("unexpected output: %q", result.Output)
	}
}
