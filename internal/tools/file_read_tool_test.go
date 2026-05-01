package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileReadToolReadsSmallTextFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "note.txt")
	if err := os.WriteFile(path, []byte("hello world\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	tool, err := NewFileReadTool(root)
	if err != nil {
		t.Fatalf("NewFileReadTool() error = %v", err)
	}

	result, err := tool.Execute(context.Background(), Call{
		Name:      "file_read",
		Arguments: []byte(`{"path":"note.txt"}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Output != "hello world\n" {
		t.Fatalf("unexpected output: %q", result.Output)
	}
}

func TestFileReadToolTruncatesLargeTextFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "large.txt")
	content := strings.Repeat("A", 400) + "\n" + strings.Repeat("Z", 400)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	tool, err := NewFileReadTool(root)
	if err != nil {
		t.Fatalf("NewFileReadTool() error = %v", err)
	}

	result, err := tool.Execute(context.Background(), Call{
		Name:      "file_read",
		Arguments: []byte(`{"path":"large.txt","max_bytes":256}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(result.Output, "[file_read truncated") {
		t.Fatalf("expected truncation marker, got %q", result.Output)
	}
	if !strings.Contains(result.Output, strings.Repeat("A", 32)) || !strings.Contains(result.Output, strings.Repeat("Z", 32)) {
		t.Fatalf("expected head and tail preview, got %q", result.Output)
	}
}

func TestFileReadToolOmitsBinaryFiles(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "image.bin")
	if err := os.WriteFile(path, []byte{0x00, 0x01, 0x02, 0x03}, 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	tool, err := NewFileReadTool(root)
	if err != nil {
		t.Fatalf("NewFileReadTool() error = %v", err)
	}

	result, err := tool.Execute(context.Background(), Call{
		Name:      "file_read",
		Arguments: []byte(`{"path":"image.bin"}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(result.Output, "[binary file omitted") {
		t.Fatalf("unexpected output: %q", result.Output)
	}
}

func TestFileReadToolReadsRequestedLineRange(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	content := "line1\nline2\nline3\nline4\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	tool, err := NewFileReadTool(root)
	if err != nil {
		t.Fatalf("NewFileReadTool() error = %v", err)
	}

	result, err := tool.Execute(context.Background(), Call{
		Name:      "file_read",
		Arguments: []byte(`{"path":"main.go","start_line":2,"end_line":3}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Output != "line2\nline3\n" {
		t.Fatalf("unexpected output: %q", result.Output)
	}
}

func TestFileReadToolRejectsInvalidLineRange(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	if err := os.WriteFile(path, []byte("line1\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	tool, err := NewFileReadTool(root)
	if err != nil {
		t.Fatalf("NewFileReadTool() error = %v", err)
	}

	_, err = tool.Execute(context.Background(), Call{
		Name:      "file_read",
		Arguments: []byte(`{"path":"main.go","end_line":2}`),
	})
	if err == nil || !strings.Contains(err.Error(), "start_line is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}
