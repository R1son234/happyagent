package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type FileDeleteTool struct {
	resolver            *RootedPathResolver
	requireConfirmation bool
}

func NewFileDeleteTool(root string, requireConfirmation bool) (*FileDeleteTool, error) {
	resolver, err := NewRootedPathResolver(root)
	if err != nil {
		return nil, err
	}
	return &FileDeleteTool{resolver: resolver, requireConfirmation: requireConfirmation}, nil
}

func (t *FileDeleteTool) Definition() Definition {
	return Definition{
		Name:        "file_delete",
		Description: "Delete a single file under the configured root directory. Requires explicit confirmation when delete confirmation is enabled.",
		InputSchema: `{"type":"object","properties":{"path":{"type":"string"},"confirm":{"type":"boolean","description":"Set to true to confirm deletion when confirmation is required."},"reason":{"type":"string","description":"Short explanation for why the file should be deleted."}},"required":["path"]}`,
		Dangerous:   true,
	}
}

func (t *FileDeleteTool) Execute(ctx context.Context, call Call) (Result, error) {
	_ = ctx

	var input struct {
		Path    string `json:"path"`
		Confirm bool   `json:"confirm"`
		Reason  string `json:"reason"`
	}
	if err := json.Unmarshal(call.Arguments, &input); err != nil {
		return Result{}, fmt.Errorf("decode file_delete arguments: %w", err)
	}
	if t.requireConfirmation && !input.Confirm {
		return Result{}, fmt.Errorf("refusing to delete %q without confirm=true", input.Path)
	}
	if strings.TrimSpace(input.Reason) == "" {
		return Result{}, fmt.Errorf("file_delete requires a non-empty reason")
	}

	path, err := t.resolver.Resolve(input.Path)
	if err != nil {
		return Result{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return Result{}, fmt.Errorf("stat file %q: %w", path, err)
	}
	if info.IsDir() {
		return Result{}, fmt.Errorf("path %q is a directory; file_delete only removes files", path)
	}

	if err := os.Remove(path); err != nil {
		return Result{}, fmt.Errorf("delete file %q: %w", path, err)
	}

	return Result{Output: "deleted " + path}, nil
}
