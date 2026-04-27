package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const defaultFileSearchMaxResults = 50

type FileSearchTool struct {
	resolver *RootedPathResolver
}

func NewFileSearchTool(root string) (*FileSearchTool, error) {
	resolver, err := NewRootedPathResolver(root)
	if err != nil {
		return nil, err
	}
	return &FileSearchTool{resolver: resolver}, nil
}

func (t *FileSearchTool) Definition() Definition {
	return Definition{
		Name:        "file_search",
		Description: "Search text under the configured root directory. Returns matching file paths, line numbers, and matched lines.",
		InputSchema: `{"type":"object","properties":{"query":{"type":"string"},"path":{"type":"string","description":"Optional file or directory to search under. Defaults to the configured root."},"glob":{"type":"string","description":"Optional glob filter such as *.go."},"max_results":{"type":"integer","minimum":1,"description":"Maximum number of matches to return. Defaults to 50."}},"required":["query"]}`,
	}
}

func (t *FileSearchTool) Execute(ctx context.Context, call Call) (Result, error) {
	var input struct {
		Query      string `json:"query"`
		Path       string `json:"path"`
		Glob       string `json:"glob"`
		MaxResults int    `json:"max_results"`
	}
	if err := json.Unmarshal(call.Arguments, &input); err != nil {
		return Result{}, fmt.Errorf("decode file_search arguments: %w", err)
	}
	if strings.TrimSpace(input.Query) == "" {
		return Result{}, fmt.Errorf("file_search query must not be empty")
	}

	searchRoot := "."
	if input.Path != "" {
		searchRoot = input.Path
	}

	resolvedPath, err := t.resolver.Resolve(searchRoot)
	if err != nil {
		return Result{}, err
	}

	output, err := runFileSearch(ctx, t.resolver.Root(), resolvedPath, input.Query, input.Glob, normalizeSearchLimit(input.MaxResults))
	if err != nil {
		return Result{}, err
	}
	return Result{Output: output}, nil
}

func normalizeSearchLimit(limit int) int {
	if limit <= 0 {
		return defaultFileSearchMaxResults
	}
	return limit
}

func runFileSearch(ctx context.Context, root string, searchPath string, query string, glob string, maxResults int) (string, error) {
	program, args, err := buildSearchCommand(searchPath, query, glob, maxResults)
	if err != nil {
		return "", err
	}

	cmd := exec.CommandContext(ctx, program, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if isNoMatchesExit(err) {
			return "(no matches)", nil
		}
		return "", fmt.Errorf("run %s search: %w: %s", program, err, strings.TrimSpace(stderr.String()))
	}

	return normalizeSearchOutput(root, stdout.String(), maxResults), nil
}

func buildSearchCommand(searchPath string, query string, glob string, maxResults int) (string, []string, error) {
	if _, err := exec.LookPath("rg"); err == nil {
		args := []string{"--line-number", "--no-heading", "--color", "never", "--max-count", strconv.Itoa(maxResults)}
		if glob != "" {
			args = append(args, "--glob", glob)
		}
		args = append(args, query, searchPath)
		return "rg", args, nil
	}

	if _, err := exec.LookPath("grep"); err == nil {
		args := []string{"-R", "-n", "-I", "-m", strconv.Itoa(maxResults)}
		if glob != "" {
			args = append(args, "--include", glob)
		}
		args = append(args, query, searchPath)
		return "grep", args, nil
	}

	return "", nil, fmt.Errorf("file_search requires rg or grep in PATH")
}

func isNoMatchesExit(err error) bool {
	var exitErr *exec.ExitError
	return errors.As(err, &exitErr) && exitErr.ExitCode() == 1
}

func normalizeSearchOutput(root string, output string, maxResults int) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	results := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		results = append(results, relativizeSearchLine(root, line))
	}
	if len(results) == 0 {
		return "(no matches)"
	}
	if len(results) <= maxResults {
		return strings.Join(results, "\n")
	}
	return strings.Join(results[:maxResults], "\n") + fmt.Sprintf("\n[file_search truncated to first %d matches]", maxResults)
}

func relativizeSearchLine(root string, line string) string {
	parts := strings.SplitN(line, ":", 3)
	if len(parts) < 3 {
		return line
	}

	if rel, err := filepath.Rel(root, parts[0]); err == nil && rel != ".." && !strings.HasPrefix(rel, "../") {
		parts[0] = filepath.ToSlash(rel)
	}
	return strings.Join(parts, ":")
}
