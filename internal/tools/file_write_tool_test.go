package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFileWriteToolWritesNewFile(t *testing.T) {
	root := t.TempDir()
	tool, err := NewFileWriteTool(root, 1024, true)
	if err != nil {
		t.Fatalf("NewFileWriteTool() error = %v", err)
	}

	result, err := tool.Execute(context.Background(), Call{
		Name:      "file_write",
		Arguments: []byte(`{"path":"notes.txt","content":"hello"}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Output != "wrote 5 bytes to "+filepath.Join(root, "notes.txt") {
		t.Fatalf("unexpected output: %q", result.Output)
	}
}

func TestFileWriteToolRejectsOverwriteWithoutFlag(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "notes.txt")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	tool, err := NewFileWriteTool(root, 1024, true)
	if err != nil {
		t.Fatalf("NewFileWriteTool() error = %v", err)
	}

	_, err = tool.Execute(context.Background(), Call{
		Name:      "file_write",
		Arguments: []byte(`{"path":"notes.txt","content":"new"}`),
	})
	if err == nil || err.Error() != `refusing to overwrite existing file "`+path+`" without overwrite=true` {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFileWriteToolRejectsOversizedContent(t *testing.T) {
	root := t.TempDir()
	tool, err := NewFileWriteTool(root, 4, true)
	if err != nil {
		t.Fatalf("NewFileWriteTool() error = %v", err)
	}

	_, err = tool.Execute(context.Background(), Call{
		Name:      "file_write",
		Arguments: []byte(`{"path":"notes.txt","content":"hello"}`),
	})
	if err == nil || err.Error() != "write content exceeds limit: 5 bytes > 4 bytes" {
		t.Fatalf("unexpected error: %v", err)
	}
}
