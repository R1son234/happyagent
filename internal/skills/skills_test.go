package skills

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"happyagent/internal/tools"
)

func TestLoaderAndInject(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "demo")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "skill.yaml"), []byte("name: demo\ntools:\n  - file_list\nprompt_file: prompt.md\n"), 0o644); err != nil {
		t.Fatalf("write skill.yaml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "prompt.md"), []byte("focus on listing files"), 0o644); err != nil {
		t.Fatalf("write prompt.md: %v", err)
	}

	loader := NewLoader(dir)
	skill, err := loader.Load("demo")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	defs := []tools.Definition{
		{Name: "file_list"},
		{Name: "file_read"},
	}
	result, err := Inject(context.Background(), "base prompt", skill, defs, nil)
	if err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	if len(result.ToolDefs) != 1 || result.ToolDefs[0].Name != "file_list" {
		t.Fatalf("unexpected tool defs: %+v", result.ToolDefs)
	}
	if result.SystemPrompt == "base prompt" {
		t.Fatalf("expected skill prompt to be injected")
	}
}

func TestInjectRejectsUnknownTool(t *testing.T) {
	skill := &Skill{
		Name:  "demo",
		Tools: []string{"missing_tool"},
	}

	_, err := Inject(context.Background(), "base", skill, []tools.Definition{{Name: "file_list"}}, nil)
	if err == nil {
		t.Fatal("expected error for unknown tool")
	}
}
