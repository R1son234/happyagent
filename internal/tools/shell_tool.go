package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

const maxShellOutputBytes = 16 * 1024

type ShellTool struct {
	resolver        *RootedPathResolver
	allowedCommands map[string]struct{}
}

func NewShellTool(root string, allowedCommands []string) (*ShellTool, error) {
	resolver, err := NewRootedPathResolver(root)
	if err != nil {
		return nil, err
	}
	allowed := make(map[string]struct{}, len(allowedCommands))
	for _, command := range allowedCommands {
		command = strings.TrimSpace(command)
		if command == "" {
			continue
		}
		allowed[command] = struct{}{}
	}
	return &ShellTool{resolver: resolver, allowedCommands: allowed}, nil
}

func (t *ShellTool) Definition() Definition {
	return Definition{
		Name:        "shell",
		Description: "Run an allowlisted command under the configured root directory. Prefer argv for exact arguments; command remains available for simple whitespace-split commands.",
		InputSchema: `{"type":"object","properties":{"command":{"type":"string","description":"Legacy shorthand for simple commands split on whitespace."},"argv":{"type":"array","items":{"type":"string"},"description":"Preferred exact argv form. Example: [\"git\",\"status\",\"--short\"]"},"workdir":{"type":"string"},"run_in_background":{"type":"boolean","description":"When true, start the command as a background job and report completion later."}},"additionalProperties":false}`,
		Dangerous:   true,
	}
}

func (t *ShellTool) Execute(ctx context.Context, call Call) (Result, error) {
	var input struct {
		Command    string   `json:"command"`
		Argv       []string `json:"argv"`
		Workdir    string   `json:"workdir"`
		Background bool     `json:"run_in_background"`
	}
	if err := json.Unmarshal(call.Arguments, &input); err != nil {
		return Result{}, fmt.Errorf("decode shell arguments: %w", err)
	}

	parts, err := resolveShellArgs(input.Command, input.Argv)
	if err != nil {
		return Result{}, err
	}
	if err := t.validateCommand(parts[0]); err != nil {
		return Result{}, err
	}

	workdir := t.resolver.Root()
	if input.Workdir != "" {
		resolved, err := t.resolver.Resolve(input.Workdir)
		if err != nil {
			return Result{}, err
		}
		workdir = resolved
	} else if override := ShellWorkdirFromContext(ctx); override != "" {
		resolved, err := t.resolver.Resolve(override)
		if err != nil {
			return Result{}, err
		}
		workdir = resolved
	}
	if input.Background {
		store := BackgroundStoreFromContext(ctx)
		if store == nil {
			return Result{}, fmt.Errorf("background shell execution is unavailable outside an active runtime session")
		}
		job := store.Start("shell")
		go func() {
			output, runErr := runShellCommand(context.Background(), parts, workdir)
			store.Complete(job.ID, output, runErr)
		}()
		data, _ := json.MarshalIndent(map[string]string{
			"job_id":   job.ID,
			"status":   string(job.Status),
			"log_path": job.LogPath,
		}, "", "  ")
		return Result{Output: string(data)}, nil
	}

	output, err := runShellCommand(ctx, parts, workdir)
	if err != nil {
		return Result{}, err
	}
	return Result{Output: output}, nil
}

type shellWorkdirContextKey struct{}

func WithShellWorkdir(ctx context.Context, workdir string) context.Context {
	return context.WithValue(ctx, shellWorkdirContextKey{}, workdir)
}

func ShellWorkdirFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	workdir, _ := ctx.Value(shellWorkdirContextKey{}).(string)
	return workdir
}

func runShellCommand(ctx context.Context, parts []string, workdir string) (string, error) {
	cmd := exec.CommandContext(ctx, parts[0], parts[1:]...)
	cmd.Dir = workdir

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		output := strings.TrimSpace(stdout.String() + "\n" + stderr.String())
		return output, fmt.Errorf("run command %q in %q: %w: %s", strings.Join(parts, " "), workdir, err, truncateToolOutput(stderr.String(), maxShellOutputBytes))
	}

	output := strings.TrimSpace(truncateToolOutput(stdout.String(), maxShellOutputBytes))
	if output == "" {
		output = "(no output)"
	}

	return output, nil
}

func (t *ShellTool) validateCommand(command string) error {
	if len(t.allowedCommands) == 0 {
		return fmt.Errorf("shell command %q is not allowed", command)
	}

	if _, ok := t.allowedCommands[command]; ok {
		return nil
	}

	base := filepath.Base(command)
	if _, ok := t.allowedCommands[base]; ok {
		return nil
	}

	return fmt.Errorf("shell command %q is not allowed; allowed commands: %s", command, strings.Join(sortedKeys(t.allowedCommands), ", "))
}

func resolveShellArgs(command string, argv []string) ([]string, error) {
	if len(argv) > 0 {
		if strings.TrimSpace(command) != "" {
			return nil, fmt.Errorf("shell expects either command or argv, not both")
		}
		for _, arg := range argv {
			if arg == "" {
				return nil, fmt.Errorf("shell argv must not contain empty arguments")
			}
		}
		return append([]string(nil), argv...), nil
	}

	parts := strings.Fields(command)
	if len(parts) == 0 {
		return nil, fmt.Errorf("shell command must not be empty")
	}
	return parts, nil
}

func truncateToolOutput(output string, limit int) string {
	if limit <= 0 || len(output) <= limit {
		return output
	}
	return output[:limit] + "\n...[output truncated]"
}

func sortedKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
