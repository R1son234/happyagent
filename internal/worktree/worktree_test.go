package worktree

import (
	"path/filepath"
	"testing"
)

func TestManagerRejectsWorktreePathOutsideRoot(t *testing.T) {
	manager, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	if _, err := manager.Enter(filepath.Join("..", "outside")); err == nil {
		t.Fatalf("expected outside worktree path to fail")
	}
}

func TestManagerKeepValidatesPath(t *testing.T) {
	root := t.TempDir()
	manager, err := NewManager(root)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	path := filepath.Join(root, ".happyagent", "worktrees", "feature-a")
	got, err := manager.Keep(path)
	if err != nil {
		t.Fatalf("Keep() error = %v", err)
	}
	if got != filepath.Clean(path) {
		t.Fatalf("unexpected kept path: %q", got)
	}
}

func TestCleanSlug(t *testing.T) {
	if got := cleanSlug(" Feature/Test 01 "); got != "feature-test-01" {
		t.Fatalf("unexpected slug: %q", got)
	}
}
