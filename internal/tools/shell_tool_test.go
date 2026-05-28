package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"happyagent/internal/background"
)

func TestShellToolSupportsArgvInput(t *testing.T) {
	tool, err := NewShellTool(t.TempDir(), []string{"printf"})
	if err != nil {
		t.Fatalf("NewShellTool() error = %v", err)
	}

	result, err := tool.Execute(context.Background(), Call{
		Name:      "shell",
		Arguments: []byte(`{"argv":["/usr/bin/printf","hello world"]}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Output != "hello world" {
		t.Fatalf("unexpected output: %q", result.Output)
	}
}

func TestShellToolRunsInBackground(t *testing.T) {
	root := t.TempDir()
	store := background.NewStore(root)
	tool, err := NewShellTool(root, []string{"printf"})
	if err != nil {
		t.Fatalf("NewShellTool() error = %v", err)
	}
	ctx := WithBackgroundStore(context.Background(), store)
	result, err := tool.Execute(ctx, Call{
		Name:      "shell",
		Arguments: []byte(`{"argv":["/usr/bin/printf","hello"],"run_in_background":true}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(result.Output, `"status": "running"`) {
		t.Fatalf("expected background job output, got %q", result.Output)
	}
	var jobs []background.Job
	for i := 0; i < 20; i++ {
		jobs = store.UnconsumedFinished()
		if len(jobs) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(jobs) != 1 || jobs[0].Status != background.StatusComplete || jobs[0].Output != "hello" {
		t.Fatalf("unexpected finished jobs: %+v", jobs)
	}
}

func TestShellToolUsesContextWorkdirOverride(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "child"), 0o755); err != nil {
		t.Fatalf("mkdir child: %v", err)
	}
	tool, err := NewShellTool(root, []string{"pwd"})
	if err != nil {
		t.Fatalf("NewShellTool() error = %v", err)
	}
	ctx := WithShellWorkdir(context.Background(), "child")
	result, err := tool.Execute(ctx, Call{
		Name:      "shell",
		Arguments: []byte(`{"argv":["pwd"]}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	expected, err := filepath.EvalSymlinks(filepath.Join(root, "child"))
	if err != nil {
		t.Fatalf("EvalSymlinks() error = %v", err)
	}
	if result.Output != expected {
		t.Fatalf("unexpected pwd: %q", result.Output)
	}
}

func TestShellToolRejectsAmbiguousCommandInput(t *testing.T) {
	tool, err := NewShellTool(t.TempDir(), []string{"echo"})
	if err != nil {
		t.Fatalf("NewShellTool() error = %v", err)
	}

	_, err = tool.Execute(context.Background(), Call{
		Name:      "shell",
		Arguments: []byte(`{"command":"echo hi","argv":["echo","hi"]}`),
	})
	if err == nil || err.Error() != "shell expects either command or argv, not both" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestShellToolRejectsCommandOutsideAllowlist(t *testing.T) {
	tool, err := NewShellTool(t.TempDir(), []string{"printf"})
	if err != nil {
		t.Fatalf("NewShellTool() error = %v", err)
	}

	_, err = tool.Execute(context.Background(), Call{
		Name:      "shell",
		Arguments: []byte(`{"argv":["/bin/ls"]}`),
	})
	if err == nil || err.Error() != `shell command "/bin/ls" is not allowed; allowed commands: printf` {
		t.Fatalf("unexpected error: %v", err)
	}
}
