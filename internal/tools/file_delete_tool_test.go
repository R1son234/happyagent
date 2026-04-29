package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFileDeleteToolDeletesFileWithConfirmation(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "notes.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	realPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("EvalSymlinks() error = %v", err)
	}

	tool, err := NewFileDeleteTool(root, true)
	if err != nil {
		t.Fatalf("NewFileDeleteTool() error = %v", err)
	}

	result, err := tool.Execute(context.Background(), Call{
		Name:      "file_delete",
		Arguments: []byte(`{"path":"notes.txt","confirm":true,"reason":"remove stale fixture"}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Output != "deleted "+realPath {
		t.Fatalf("unexpected output: %q", result.Output)
	}
}

func TestFileDeleteToolRejectsMissingConfirmation(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "notes.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	tool, err := NewFileDeleteTool(root, true)
	if err != nil {
		t.Fatalf("NewFileDeleteTool() error = %v", err)
	}

	_, err = tool.Execute(context.Background(), Call{
		Name:      "file_delete",
		Arguments: []byte(`{"path":"notes.txt","reason":"remove stale fixture"}`),
	})
	if err == nil || err.Error() != `refusing to delete "notes.txt" without confirm=true` {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFileDeleteToolRejectsEmptyReason(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "notes.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	tool, err := NewFileDeleteTool(root, true)
	if err != nil {
		t.Fatalf("NewFileDeleteTool() error = %v", err)
	}

	_, err = tool.Execute(context.Background(), Call{
		Name:      "file_delete",
		Arguments: []byte(`{"path":"notes.txt","confirm":true,"reason":"   "}`),
	})
	if err == nil || err.Error() != "file_delete requires a non-empty reason" {
		t.Fatalf("unexpected error: %v", err)
	}
}
