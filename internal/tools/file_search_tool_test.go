package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileSearchToolFindsMatchesWithLineNumbers(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "alpha.txt"), []byte("hello\nneedle here\nbye\n"), 0o644); err != nil {
		t.Fatalf("write alpha.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "beta.go"), []byte("package demo\n// needle comment\n"), 0o644); err != nil {
		t.Fatalf("write beta.go: %v", err)
	}

	tool, err := NewFileSearchTool(root)
	if err != nil {
		t.Fatalf("NewFileSearchTool() error = %v", err)
	}

	result, err := tool.Execute(context.Background(), Call{
		Name:      "file_search",
		Arguments: []byte(`{"query":"needle","max_results":10}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(result.Output, "alpha.txt:2:needle here") {
		t.Fatalf("expected alpha.txt match, got %q", result.Output)
	}
	if !strings.Contains(result.Output, "beta.go:2:// needle comment") {
		t.Fatalf("expected beta.go match, got %q", result.Output)
	}
}

func TestFileSearchToolAppliesGlobAndLimit(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "alpha.txt"), []byte("needle one\nneedle two\n"), 0o644); err != nil {
		t.Fatalf("write alpha.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "beta.go"), []byte("needle three\n"), 0o644); err != nil {
		t.Fatalf("write beta.go: %v", err)
	}

	tool, err := NewFileSearchTool(root)
	if err != nil {
		t.Fatalf("NewFileSearchTool() error = %v", err)
	}

	result, err := tool.Execute(context.Background(), Call{
		Name:      "file_search",
		Arguments: []byte(`{"query":"needle","glob":"*.txt","max_results":1}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if strings.Contains(result.Output, "beta.go") {
		t.Fatalf("expected glob to exclude beta.go, got %q", result.Output)
	}
	if !strings.Contains(result.Output, "alpha.txt:1:needle one") {
		t.Fatalf("expected limited txt match, got %q", result.Output)
	}
}

func TestFileSearchToolRejectsEscapedPath(t *testing.T) {
	root := t.TempDir()
	tool, err := NewFileSearchTool(root)
	if err != nil {
		t.Fatalf("NewFileSearchTool() error = %v", err)
	}

	_, err = tool.Execute(context.Background(), Call{
		Name:      "file_search",
		Arguments: []byte(`{"query":"needle","path":"../outside"}`),
	})
	if err == nil || !strings.Contains(err.Error(), "escapes root") {
		t.Fatalf("unexpected error: %v", err)
	}
}
