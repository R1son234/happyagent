package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type FileWriteTool struct {
	resolver *RootedPathResolver
}

func NewFileWriteTool(root string) (*FileWriteTool, error) {
	resolver, err := NewRootedPathResolver(root)
	if err != nil {
		return nil, err
	}
	return &FileWriteTool{resolver: resolver}, nil
}

func (t *FileWriteTool) Definition() Definition {
	return Definition{
		Name:        "file_write",
		Description: "Write content to a single file under the configured root directory.",
		InputSchema: `{"type":"object","properties":{"path":{"type":"string"},"content":{"type":"string"}},"required":["path","content"]}`,
		Dangerous:   true,
	}
}

func (t *FileWriteTool) Execute(ctx context.Context, call Call) (Result, error) {
	_ = ctx

	var input struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(call.Arguments, &input); err != nil {
		return Result{}, fmt.Errorf("decode file_write arguments: %w", err)
	}

	path, err := t.resolver.Resolve(input.Path)
	if err != nil {
		return Result{}, err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return Result{}, fmt.Errorf("create parent directory for %q: %w", path, err)
	}
	if err := os.WriteFile(path, []byte(input.Content), 0o644); err != nil {
		return Result{}, fmt.Errorf("write file %q: %w", path, err)
	}

	return Result{Output: fmt.Sprintf("wrote %d bytes to %s", len(input.Content), path)}, nil
}
