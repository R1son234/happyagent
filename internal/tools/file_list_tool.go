package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

const defaultFileListMaxEntries = 200

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
		Description: "List direct entries in a directory under the configured root directory. Supports pagination to avoid oversized directory observations.",
		InputSchema: `{"type":"object","properties":{"path":{"type":"string"},"offset":{"type":"integer","minimum":0,"description":"Optional zero-based start offset."},"max_entries":{"type":"integer","minimum":1,"description":"Optional maximum number of entries to return. Defaults to 200."}},"required":["path"]}`,
	}
}

func (t *FileListTool) Execute(ctx context.Context, call Call) (Result, error) {
	_ = ctx

	var input struct {
		Path       string `json:"path"`
		Offset     int    `json:"offset"`
		MaxEntries int    `json:"max_entries"`
	}
	if err := json.Unmarshal(call.Arguments, &input); err != nil {
		return Result{}, fmt.Errorf("decode file_list arguments: %w", err)
	}
	if input.Offset < 0 {
		return Result{}, fmt.Errorf("file_list offset must be greater than or equal to zero")
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

	maxEntries := input.MaxEntries
	if maxEntries <= 0 {
		maxEntries = defaultFileListMaxEntries
	}
	if input.Offset >= len(names) {
		return Result{Output: "(no entries)"}, nil
	}

	end := input.Offset + maxEntries
	if end > len(names) {
		end = len(names)
	}

	selected := names[input.Offset:end]
	output := strings.Join(selected, "\n")
	if end < len(names) {
		output += fmt.Sprintf("\n[file_list truncated: showing entries %d-%d of %d, use offset to continue]", input.Offset+1, end, len(names))
	}

	return Result{Output: output}, nil
}
