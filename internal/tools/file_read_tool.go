package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
)

type FileReadTool struct {
	resolver *RootedPathResolver
}

func NewFileReadTool(root string) (*FileReadTool, error) {
	resolver, err := NewRootedPathResolver(root)
	if err != nil {
		return nil, err
	}
	return &FileReadTool{resolver: resolver}, nil
}

func (t *FileReadTool) Definition() Definition {
	return Definition{
		Name:        "file_read",
		Description: "Read the content of a single file under the configured root directory.",
		InputSchema: `{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`,
	}
}

func (t *FileReadTool) Execute(ctx context.Context, call Call) (Result, error) {
	_ = ctx

	var input struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(call.Arguments, &input); err != nil {
		return Result{}, fmt.Errorf("decode file_read arguments: %w", err)
	}

	path, err := t.resolver.Resolve(input.Path)
	if err != nil {
		return Result{}, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return Result{}, fmt.Errorf("read file %q: %w", path, err)
	}

	return Result{Output: string(data)}, nil
}
