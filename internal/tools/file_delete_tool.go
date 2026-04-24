package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
)

type FileDeleteTool struct {
	resolver *RootedPathResolver
}

func NewFileDeleteTool(root string) (*FileDeleteTool, error) {
	resolver, err := NewRootedPathResolver(root)
	if err != nil {
		return nil, err
	}
	return &FileDeleteTool{resolver: resolver}, nil
}

func (t *FileDeleteTool) Definition() Definition {
	return Definition{
		Name:        "file_delete",
		Description: "Delete a single file under the configured root directory.",
		InputSchema: `{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`,
		Dangerous:   true,
	}
}

func (t *FileDeleteTool) Execute(ctx context.Context, call Call) (Result, error) {
	_ = ctx

	var input struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(call.Arguments, &input); err != nil {
		return Result{}, fmt.Errorf("decode file_delete arguments: %w", err)
	}

	path, err := t.resolver.Resolve(input.Path)
	if err != nil {
		return Result{}, err
	}

	if err := os.Remove(path); err != nil {
		return Result{}, fmt.Errorf("delete file %q: %w", path, err)
	}

	return Result{Output: "deleted " + path}, nil
}
