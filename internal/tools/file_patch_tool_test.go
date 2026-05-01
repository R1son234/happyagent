package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFilePatchToolReplacesExactSnippet(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	if err := os.WriteFile(path, []byte("package main\n\nconst name = \"old\"\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	tool, err := NewFilePatchTool(root)
	if err != nil {
		t.Fatalf("NewFilePatchTool() error = %v", err)
	}

	result, err := tool.Execute(context.Background(), Call{
		Name:      "file_patch",
		Arguments: []byte(`{"path":"main.go","old_text":"const name = \"old\"","new_text":"const name = \"new\""}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(result.Output, "replaced 1 occurrence") {
		t.Fatalf("unexpected output: %q", result.Output)
	}

	updated, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.Contains(string(updated), `const name = "new"`) {
		t.Fatalf("patch did not apply: %q", string(updated))
	}
}

func TestFilePatchToolRejectsUnexpectedReplacementCount(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	content := "value := old\nanother := old\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	tool, err := NewFilePatchTool(root)
	if err != nil {
		t.Fatalf("NewFilePatchTool() error = %v", err)
	}

	_, err = tool.Execute(context.Background(), Call{
		Name:      "file_patch",
		Arguments: []byte(`{"path":"main.go","old_text":"old","new_text":"new","expected_replacements":1}`),
	})
	if err == nil || !strings.Contains(err.Error(), "expected 1 replacement") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFilePatchToolRejectsMissingSnippet(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	if err := os.WriteFile(path, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	tool, err := NewFilePatchTool(root)
	if err != nil {
		t.Fatalf("NewFilePatchTool() error = %v", err)
	}

	_, err = tool.Execute(context.Background(), Call{
		Name:      "file_patch",
		Arguments: []byte(`{"path":"main.go","old_text":"missing","new_text":"new"}`),
	})
	if err == nil || !strings.Contains(err.Error(), "did not find old_text") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFilePatchToolRejectsEscapedPath(t *testing.T) {
	root := t.TempDir()
	tool, err := NewFilePatchTool(root)
	if err != nil {
		t.Fatalf("NewFilePatchTool() error = %v", err)
	}

	_, err = tool.Execute(context.Background(), Call{
		Name:      "file_patch",
		Arguments: []byte(`{"path":"../outside.txt","old_text":"a","new_text":"b"}`),
	})
	if err == nil || !strings.Contains(err.Error(), "escapes root") {
		t.Fatalf("unexpected error: %v", err)
	}
}
