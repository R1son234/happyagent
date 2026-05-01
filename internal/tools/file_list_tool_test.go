package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileListToolSupportsPagination(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(name), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	tool, err := NewFileListTool(root)
	if err != nil {
		t.Fatalf("NewFileListTool() error = %v", err)
	}

	result, err := tool.Execute(context.Background(), Call{
		Name:      "file_list",
		Arguments: []byte(`{"path":".","offset":1,"max_entries":1}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(result.Output, "b.txt") {
		t.Fatalf("expected paged result to contain b.txt, got %q", result.Output)
	}
	if !strings.Contains(result.Output, "[file_list truncated:") {
		t.Fatalf("expected truncation marker, got %q", result.Output)
	}
}

func TestFileListToolReturnsNoEntriesWhenOffsetExceedsSize(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	tool, err := NewFileListTool(root)
	if err != nil {
		t.Fatalf("NewFileListTool() error = %v", err)
	}

	result, err := tool.Execute(context.Background(), Call{
		Name:      "file_list",
		Arguments: []byte(`{"path":".","offset":10,"max_entries":5}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Output != "(no entries)" {
		t.Fatalf("unexpected output: %q", result.Output)
	}
}

func TestFileListToolRejectsNegativeOffset(t *testing.T) {
	root := t.TempDir()
	tool, err := NewFileListTool(root)
	if err != nil {
		t.Fatalf("NewFileListTool() error = %v", err)
	}

	_, err = tool.Execute(context.Background(), Call{
		Name:      "file_list",
		Arguments: []byte(`{"path":".","offset":-1}`),
	})
	if err == nil || !strings.Contains(err.Error(), "offset must be greater than or equal to zero") {
		t.Fatalf("unexpected error: %v", err)
	}
}
