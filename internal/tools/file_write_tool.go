package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type FileWriteTool struct {
	resolver         *RootedPathResolver
	maxBytes         int
	requireOverwrite bool
}

func NewFileWriteTool(root string, maxBytes int, requireOverwrite bool) (*FileWriteTool, error) {
	resolver, err := NewRootedPathResolver(root)
	if err != nil {
		return nil, err
	}
	return &FileWriteTool{
		resolver:         resolver,
		maxBytes:         maxBytes,
		requireOverwrite: requireOverwrite,
	}, nil
}

func (t *FileWriteTool) Definition() Definition {
	return Definition{
		Name:        "file_write",
		Description: "Write content to a single file under the configured root directory. Existing files require explicit overwrite when overwrite protection is enabled.",
		InputSchema: `{"type":"object","properties":{"path":{"type":"string"},"content":{"type":"string"},"overwrite":{"type":"boolean","description":"Required when replacing an existing file if overwrite protection is enabled."}},"required":["path","content"]}`,
		Dangerous:   true,
	}
}

func (t *FileWriteTool) Execute(ctx context.Context, call Call) (Result, error) {
	_ = ctx

	var input struct {
		Path      string `json:"path"`
		Content   string `json:"content"`
		Overwrite bool   `json:"overwrite"`
	}
	if err := json.Unmarshal(call.Arguments, &input); err != nil {
		return Result{}, fmt.Errorf("decode file_write arguments: %w", err)
	}
	if len(input.Content) > t.maxBytes {
		return Result{}, fmt.Errorf("write content exceeds limit: %d bytes > %d bytes", len(input.Content), t.maxBytes)
	}

	path, err := t.resolver.Resolve(input.Path)
	if err != nil {
		return Result{}, err
	}
	if err := t.validateWrite(path, input.Content, input.Overwrite); err != nil {
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

func (t *FileWriteTool) validateWrite(path string, content string, overwrite bool) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat file %q: %w", path, err)
	}
	if info.IsDir() {
		return fmt.Errorf("path %q is a directory", path)
	}

	existing, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read existing file %q: %w", path, err)
	}
	if string(existing) == content {
		return nil
	}
	if t.requireOverwrite && !overwrite {
		return fmt.Errorf("refusing to overwrite existing file %q without overwrite=true", path)
	}
	return nil
}
