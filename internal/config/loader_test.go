package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFromPathSucceedsWithValidConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	os.WriteFile(path, []byte(`{"llm":{"api_key":"test-key"}}`), 0o644)
	cfg, err := LoadFromPath(path)
	if err != nil {
		t.Fatalf("LoadFromPath() error = %v", err)
	}
	if cfg.LLM.APIKey != "test-key" {
		t.Fatalf("unexpected api_key: %q", cfg.LLM.APIKey)
	}
	if cfg.LLM.Model != "gpt-4o-mini" {
		t.Fatalf("expected default model, got %q", cfg.LLM.Model)
	}
}

func TestLoadFromPathRejectsMissingFile(t *testing.T) {
	_, err := LoadFromPath("/nonexistent/path/config.json")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadFromPathRejectsInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	os.WriteFile(path, []byte(`{bad json`), 0o644)
	_, err := LoadFromPath(path)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestLoadFromPathRejectsMissingAPIKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	os.WriteFile(path, []byte(`{"llm":{"model":"test"}}`), 0o644)
	_, err := LoadFromPath(path)
	if err == nil {
		t.Fatal("expected error for missing API key")
	}
}

func TestOverrideCSV(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"a,b,c", []string{"a", "b", "c"}},
		{"a, b , c", []string{"a", "b", "c"}},
		{"", nil},
		{",,,", nil},
	}
	for _, tt := range tests {
		got := OverrideCSV(tt.input)
		if len(got) != len(tt.want) {
			t.Fatalf("OverrideCSV(%q) = %v, want %v", tt.input, got, tt.want)
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Fatalf("OverrideCSV(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
			}
		}
	}
}

func TestLLMConfigSafeString(t *testing.T) {
	tests := []struct {
		key  string
		want string
	}{
		{"", "(no key)"},
		{"short", "****"},
		{"abcdefghijklmnop", "abcd****mnop"},
	}
	for _, tt := range tests {
		c := LLMConfig{APIKey: tt.key}
		got := c.SafeString()
		if got != tt.want {
			t.Fatalf("SafeString(%q) = %q, want %q", tt.key, got, tt.want)
		}
	}
}

func TestValidateRejectsEmptyShellAllowlistWhenEnabled(t *testing.T) {
	cfg := Default()
	cfg.LLM.APIKey = "test-key"
	cfg.Tools.ShellAllowedCommands = nil

	err := validate(cfg)
	if err == nil || err.Error() != "tools.shell_allowed_commands must not be empty when shell is enabled" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateRejectsNonPositiveWriteLimit(t *testing.T) {
	cfg := Default()
	cfg.LLM.APIKey = "test-key"
	cfg.Tools.WriteMaxBytes = 0

	err := validate(cfg)
	if err == nil || err.Error() != "tools.write_max_bytes must be greater than zero" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateRejectsInvalidOffloadConfigWhenEnabled(t *testing.T) {
	cfg := Default()
	cfg.LLM.APIKey = "test-key"
	cfg.Engine.OffloadEnabled = true
	cfg.Engine.OffloadMinBytes = 0

	err := validate(cfg)
	if err == nil || err.Error() != "engine.offload_min_bytes must be greater than zero when offload is enabled" {
		t.Fatalf("unexpected error: %v", err)
	}

	cfg.Engine.OffloadMinBytes = 1024
	cfg.Engine.OffloadDir = " "
	err = validate(cfg)
	if err == nil || err.Error() != "engine.offload_dir must not be empty when offload is enabled" {
		t.Fatalf("unexpected error: %v", err)
	}
}
