package main

import (
	"os"
	"path/filepath"
	"testing"

	"happyagent/internal/career"
	"happyagent/internal/config"
)

func TestPrepareDesktopConfigUsesWorkspaceAsToolRoot(t *testing.T) {
	cfg := config.Default()
	cfg.Tools.RootDir = "."

	workspaceRoot := prepareDesktopConfig(&cfg, "custom-workspace")

	if cfg.Tools.RootDir != "custom-workspace" {
		t.Fatalf("Tools.RootDir = %q, want custom-workspace", cfg.Tools.RootDir)
	}
	if workspaceRoot != "custom-workspace" {
		t.Fatalf("workspaceRoot = %q, want custom-workspace", workspaceRoot)
	}
	if cfg.Engine.RunTimeoutSeconds < career.MinAnalyzeTimeoutSeconds {
		t.Fatalf("expected career timeout preparation, got %d", cfg.Engine.RunTimeoutSeconds)
	}
}

func TestPrepareDesktopConfigUsesDefaultWorkspaceWhenBlank(t *testing.T) {
	cfg := config.Default()

	workspaceRoot := prepareDesktopConfig(&cfg, "")

	if cfg.Tools.RootDir != career.DefaultWorkspaceRoot {
		t.Fatalf("Tools.RootDir = %q, want %q", cfg.Tools.RootDir, career.DefaultWorkspaceRoot)
	}
	if workspaceRoot != career.DefaultWorkspaceRoot {
		t.Fatalf("workspaceRoot = %q, want %q", workspaceRoot, career.DefaultWorkspaceRoot)
	}
}

func TestEnsureDesktopWorkspaceCreatesMissingWorkspace(t *testing.T) {
	workspaceRoot := filepath.Join(t.TempDir(), "new-workspace")

	if _, err := os.Stat(workspaceRoot); !os.IsNotExist(err) {
		t.Fatalf("expected workspace to be absent before test, stat error = %v", err)
	}

	if err := ensureDesktopWorkspace(workspaceRoot); err != nil {
		t.Fatalf("ensureDesktopWorkspace() error = %v", err)
	}

	for _, rel := range []string{
		"inbox",
		filepath.Join(".happyagent", "workspace", "workspace.json"),
		filepath.Join(".happyagent", "workspace", "index.json"),
	} {
		if _, err := os.Stat(filepath.Join(workspaceRoot, rel)); err != nil {
			t.Fatalf("expected %s to exist: %v", rel, err)
		}
	}
}
