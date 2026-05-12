package config

import "testing"

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
