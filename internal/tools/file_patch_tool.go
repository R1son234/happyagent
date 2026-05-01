package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type FilePatchTool struct {
	resolver *RootedPathResolver
}

func NewFilePatchTool(root string) (*FilePatchTool, error) {
	resolver, err := NewRootedPathResolver(root)
	if err != nil {
		return nil, err
	}
	return &FilePatchTool{resolver: resolver}, nil
}

func (t *FilePatchTool) Definition() Definition {
	return Definition{
		Name:        "file_patch",
		Description: "Apply a precise text replacement to a single file under the configured root directory. This is safer than rewriting the whole file when only a small edit is needed.",
		InputSchema: `{"type":"object","properties":{"path":{"type":"string"},"old_text":{"type":"string","description":"Exact text to replace."},"new_text":{"type":"string","description":"Replacement text."},"expected_replacements":{"type":"integer","minimum":1,"description":"Expected non-overlapping match count. Defaults to 1."}},"required":["path","old_text","new_text"]}`,
		Dangerous:   true,
	}
}

func (t *FilePatchTool) Execute(ctx context.Context, call Call) (Result, error) {
	_ = ctx

	var input struct {
		Path                 string `json:"path"`
		OldText              string `json:"old_text"`
		NewText              string `json:"new_text"`
		ExpectedReplacements int    `json:"expected_replacements"`
	}
	if err := json.Unmarshal(call.Arguments, &input); err != nil {
		return Result{}, fmt.Errorf("decode file_patch arguments: %w", err)
	}
	if input.OldText == "" {
		return Result{}, fmt.Errorf("file_patch old_text must not be empty")
	}

	expected := input.ExpectedReplacements
	if expected <= 0 {
		expected = 1
	}

	path, err := t.resolver.Resolve(input.Path)
	if err != nil {
		return Result{}, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return Result{}, fmt.Errorf("read file %q: %w", path, err)
	}

	content := string(data)
	matches := strings.Count(content, input.OldText)
	if matches == 0 {
		return Result{}, fmt.Errorf("file_patch did not find old_text in %q", path)
	}
	if matches != expected {
		return Result{}, fmt.Errorf("file_patch expected %d replacement(s) in %q, found %d", expected, path, matches)
	}

	updated := strings.ReplaceAll(content, input.OldText, input.NewText)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return Result{}, fmt.Errorf("create parent directory for %q: %w", path, err)
	}
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		return Result{}, fmt.Errorf("write file %q: %w", path, err)
	}

	return Result{
		Output: fmt.Sprintf("patched %s: replaced %d occurrence(s)", path, matches),
	}, nil
}
