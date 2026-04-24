package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

type ShellTool struct {
	resolver *RootedPathResolver
}

func NewShellTool(root string) (*ShellTool, error) {
	resolver, err := NewRootedPathResolver(root)
	if err != nil {
		return nil, err
	}
	return &ShellTool{resolver: resolver}, nil
}

func (t *ShellTool) Definition() Definition {
	return Definition{
		Name:        "shell",
		Description: "Run a simple command under the configured root directory. Complex shell features are not supported.",
		InputSchema: `{"type":"object","properties":{"command":{"type":"string"},"workdir":{"type":"string"}},"required":["command"]}`,
		Dangerous:   true,
	}
}

func (t *ShellTool) Execute(ctx context.Context, call Call) (Result, error) {
	var input struct {
		Command string `json:"command"`
		Workdir string `json:"workdir"`
	}
	if err := json.Unmarshal(call.Arguments, &input); err != nil {
		return Result{}, fmt.Errorf("decode shell arguments: %w", err)
	}

	parts := strings.Fields(input.Command)
	if len(parts) == 0 {
		return Result{}, fmt.Errorf("shell command must not be empty")
	}

	workdir := t.resolver.Root()
	if input.Workdir != "" {
		resolved, err := t.resolver.Resolve(input.Workdir)
		if err != nil {
			return Result{}, err
		}
		workdir = resolved
	}

	cmd := exec.CommandContext(ctx, parts[0], parts[1:]...)
	cmd.Dir = workdir

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return Result{}, fmt.Errorf("run command %q in %q: %w: %s", input.Command, workdir, err, stderr.String())
	}

	output := strings.TrimSpace(stdout.String())
	if output == "" {
		output = "(no output)"
	}

	return Result{Output: output}, nil
}
