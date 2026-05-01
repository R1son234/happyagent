package runtime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"happyagent/internal/skills"
	"happyagent/internal/tools"
)

func TestSkillSessionLoaderCatalogAndLoad(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "demo")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	spec := "---\nname: demo\ndescription: demo skill\n---\nfocus on listing files"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(spec), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}

	session, err := NewSkillSession(ensureSkillLoader(newTestLoader(dir)), "base prompt", []tools.Definition{{Name: "file_list"}})
	if err != nil {
		t.Fatalf("NewSkillSession() error = %v", err)
	}

	catalog := session.Catalog()
	if len(catalog) != 1 || catalog[0].Name != "demo" {
		t.Fatalf("unexpected catalog: %+v", catalog)
	}
}

func TestSkillSessionStartsWithActivateSkillToolAndBaseTools(t *testing.T) {
	dir := t.TempDir()
	writeTestSkill(t, dir, "demo", "demo skill", "focus on listing files")

	session, err := NewSkillSession(newTestLoader(dir), "base prompt", []tools.Definition{
		{Name: "file_list"},
		{Name: "shell"},
	})
	if err != nil {
		t.Fatalf("NewSkillSession() error = %v", err)
	}

	defs, err := session.ToolDefs()
	if err != nil {
		t.Fatalf("ToolDefs() error = %v", err)
	}

	if len(defs) != 4 || defs[0].Name != listCapabilitiesToolName || defs[1].Name != activateSkillToolName {
		t.Fatalf("unexpected tool defs before activation: %+v", defs)
	}

	prompt := session.SystemPrompt()
	if prompt != "base prompt" {
		t.Fatalf("unexpected system prompt before activation: %q", prompt)
	}
}

func TestSkillSessionActivateEnablesSkillPromptAndTools(t *testing.T) {
	dir := t.TempDir()
	writeTestSkill(t, dir, "demo", "demo skill", "focus on listing files")

	session, err := NewSkillSession(newTestLoader(dir), "base prompt", []tools.Definition{
		{Name: "file_list"},
		{Name: "shell"},
	})
	if err != nil {
		t.Fatalf("NewSkillSession() error = %v", err)
	}

	output, err := session.Activate(context.Background(), []string{"demo"})
	if err != nil {
		t.Fatalf("Activate() error = %v", err)
	}
	if !strings.Contains(output, "Activated skill demo.") || !strings.Contains(output, "focus on listing files") {
		t.Fatalf("unexpected activate output: %q", output)
	}

	defs, err := session.ToolDefs()
	if err != nil {
		t.Fatalf("ToolDefs() error = %v", err)
	}
	if len(defs) != 4 || defs[0].Name != listCapabilitiesToolName || defs[1].Name != activateSkillToolName || defs[2].Name != "file_list" || defs[3].Name != "shell" {
		t.Fatalf("unexpected tool defs after activation: %+v", defs)
	}

	prompt := session.SystemPrompt()
	if prompt != "base prompt" {
		t.Fatalf("unexpected system prompt after activation: %q", prompt)
	}
}

func TestCapabilitySessionJSON(t *testing.T) {
	dir := t.TempDir()
	writeTestSkill(t, dir, "demo", "demo skill", "focus on listing files")

	session, err := NewSkillSession(newTestLoader(dir), "base prompt", []tools.Definition{
		{Name: "file_list"},
		{Name: "mcp_read_resource"},
	})
	if err != nil {
		t.Fatalf("NewSkillSession() error = %v", err)
	}

	output, err := NewCapabilitySession(session, nil).CapabilitiesJSON()
	if err != nil {
		t.Fatalf("CapabilitiesJSON() error = %v", err)
	}
	if !strings.Contains(output, `"name": "demo"`) || !strings.Contains(output, `"active_skills": []`) || !strings.Contains(output, `"mcp_resources_total": 0`) {
		t.Fatalf("unexpected capabilities json: %q", output)
	}
	if !strings.Contains(output, `"available_tools": [`) || !strings.Contains(output, `"mcp_read_resource"`) || !strings.Contains(output, `"mcp_resource_read_supported": true`) {
		t.Fatalf("unexpected capabilities json: %q", output)
	}
}

func TestCapabilitySessionWithoutMCPMarksListAsNotTruncated(t *testing.T) {
	dir := t.TempDir()
	writeTestSkill(t, dir, "demo", "demo skill", "focus on listing files")

	session, err := NewSkillSession(newTestLoader(dir), "base prompt", []tools.Definition{
		{Name: "file_list"},
	})
	if err != nil {
		t.Fatalf("NewSkillSession() error = %v", err)
	}

	output, err := NewCapabilitySession(session, nil).CapabilitiesJSON()
	if err != nil {
		t.Fatalf("CapabilitiesJSON() error = %v", err)
	}
	if !strings.Contains(output, `"mcp_resources_truncated": false`) {
		t.Fatalf("unexpected capabilities json: %q", output)
	}
	if !strings.Contains(output, `"mcp_resource_read_supported": false`) {
		t.Fatalf("unexpected capabilities json: %q", output)
	}
}

func TestSkillSessionWithoutCatalogStillExposesListCapabilities(t *testing.T) {
	dir := t.TempDir()

	session, err := NewSkillSession(newTestLoader(dir), "base prompt", []tools.Definition{
		{Name: "file_list"},
	})
	if err != nil {
		t.Fatalf("NewSkillSession() error = %v", err)
	}

	defs, err := session.ToolDefs()
	if err != nil {
		t.Fatalf("ToolDefs() error = %v", err)
	}
	if len(defs) != 2 || defs[0].Name != listCapabilitiesToolName || defs[1].Name != "file_list" {
		t.Fatalf("unexpected tool defs without catalog: %+v", defs)
	}
}

func newTestLoader(dir string) *skills.Loader {
	return skills.NewLoader(dir)
}

func writeTestSkill(t *testing.T, root string, name string, description string, prompt string) {
	t.Helper()

	skillDir := filepath.Join(root, name)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	spec := "---\nname: " + name + "\ndescription: " + description + "\n---\n" + prompt

	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(spec), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
}
