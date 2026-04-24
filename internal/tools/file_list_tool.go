package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

type FileListTool struct {
	resolver *RootedPathResolver
}

func NewFileListTool(root string) (*FileListTool, error) {
	resolver, err := NewRootedPathResolver(root)
	if err != nil {
		return nil, err
	}
	return &FileListTool{resolver: resolver}, nil
}

func (t *FileListTool) Definition() Definition {
	return Definition{
		Name:        "file_list",
		Description: "List direct entries in a directory under the configured root directory.",
		InputSchema: `{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`,
	}
}

func (t *FileListTool) Execute(ctx context.Context, call Call) (Result, error) {
	_ = ctx

	var input struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(call.Arguments, &input); err != nil {
		return Result{}, fmt.Errorf("decode file_list arguments: %w", err)
	}

	if input.Path == "" {
		input.Path = "."
	}

	path, err := t.resolver.Resolve(input.Path)
	if err != nil {
		return Result{}, err
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return Result{}, fmt.Errorf("list directory %q: %w", path, err)
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			name += "/"
		}
		names = append(names, name)
	}
	sort.Strings(names)

	return Result{Output: strings.Join(names, "\n")}, nil
}
