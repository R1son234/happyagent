package profile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadAllLoadsProfiles(t *testing.T) {
	dir := t.TempDir()
	writeProfile(t, dir, "general-assistant", `{
  "name": "general-assistant",
  "system_prompt": "general prompt",
  "enabled_tools": ["final_answer", "file_read"],
  "enabled_skills": ["file-inspector"]
}`)
	writeProfile(t, dir, "career-copilot", `{
  "name": "career-copilot",
  "system_prompt": "career prompt",
  "enabled_tools": ["final_answer", "file_read"],
  "enabled_skills": []
}`)

	profiles, err := LoadAll(dir)
	if err != nil {
		t.Fatalf("LoadAll() error = %v", err)
	}
	if len(profiles) != 2 {
		t.Fatalf("expected 2 profiles, got %d", len(profiles))
	}
	if profiles[0].Name != "career-copilot" || profiles[1].Name != "general-assistant" {
		t.Fatalf("unexpected profiles: %+v", profiles)
	}
}

func TestLoadByNameRejectsDuplicateTools(t *testing.T) {
	dir := t.TempDir()
	writeProfile(t, dir, "general-assistant", `{
  "name": "general-assistant",
  "system_prompt": "general prompt",
  "enabled_tools": ["file_read", "file_read"],
  "enabled_skills": []
}`)

	_, err := LoadByName(dir, "general-assistant")
	if err == nil || !strings.Contains(err.Error(), "enabled_tools: must not contain duplicates") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadByNameRejectsMissingSystemPrompt(t *testing.T) {
	dir := t.TempDir()
	writeProfile(t, dir, "general-assistant", `{
  "name": "general-assistant",
  "system_prompt": "",
  "enabled_tools": ["file_read"],
  "enabled_skills": []
}`)

	_, err := LoadByName(dir, "general-assistant")
	if err == nil || !strings.Contains(err.Error(), "system_prompt: must not be empty") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadByNameRejectsMissingProfile(t *testing.T) {
	dir := t.TempDir()

	_, err := LoadByName(dir, "missing")
	if err == nil || !strings.Contains(err.Error(), `read profile "missing"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func writeProfile(t *testing.T, root string, name string, content string) {
	t.Helper()

	profileDir := filepath.Join(root, name)
	if err := os.MkdirAll(profileDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(profileDir, fileName), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}
