package tools

import (
	"context"
	"testing"
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
