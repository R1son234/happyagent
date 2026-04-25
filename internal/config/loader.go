package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
)

const configPath = "happyagent.local.json"

func Load() (Config, error) {
	cfg := Default()

	if err := loadFromFile(configPath, &cfg); err != nil {
		return Config{}, err
	}

	applyEnv(&cfg)

	if err := validate(cfg); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func ConfigPath() string {
	return configPath
}

func loadFromFile(path string, cfg *Config) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read config file %q: %w", path, err)
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return fmt.Errorf("parse config file %q as JSON: %w", path, err)
	}

	return nil
}

func applyEnv(cfg *Config) {
	overrideString("HAPPYAGENT_LLM_MODEL", &cfg.LLM.Model)
	overrideString("HAPPYAGENT_LLM_API_KEY", &cfg.LLM.APIKey)
	overrideString("HAPPYAGENT_LLM_BASE_URL", &cfg.LLM.BaseURL)
	overrideString("HAPPYAGENT_SYSTEM_PROMPT", &cfg.Engine.SystemPrompt)
	overrideString("HAPPYAGENT_ROOT_DIR", &cfg.Tools.RootDir)
	overrideString("HAPPYAGENT_SKILLS_DIR", &cfg.Skills.Dir)

	overrideInt("HAPPYAGENT_LOOP_MAX_STEPS", &cfg.Engine.LoopMaxSteps)
	overrideInt("HAPPYAGENT_RUN_TIMEOUT_SECONDS", &cfg.Engine.RunTimeoutSeconds)
	overrideInt("HAPPYAGENT_MCP_CONNECT_TIMEOUT_SECONDS", &cfg.MCP.ConnectTimeoutSeconds)

	overrideBool("HAPPYAGENT_SHELL_ENABLED", &cfg.Tools.ShellEnabled)
	overrideBool("HAPPYAGENT_WRITE_ENABLED", &cfg.Tools.WriteEnabled)
	overrideBool("HAPPYAGENT_DELETE_ENABLED", &cfg.Tools.DeleteEnabled)
}

func validate(cfg Config) error {
	if cfg.Engine.LoopMaxSteps <= 0 {
		return fmt.Errorf("engine.loop_max_steps must be greater than zero")
	}
	if cfg.Engine.RunTimeoutSeconds <= 0 {
		return fmt.Errorf("engine.run_timeout_seconds must be greater than zero")
	}
	if cfg.LLM.Model == "" {
		return fmt.Errorf("llm.model must not be empty")
	}
	if cfg.LLM.APIKey == "" {
		return fmt.Errorf("llm.api_key must not be empty")
	}
	if cfg.Tools.RootDir == "" {
		return fmt.Errorf("tools.root_dir must not be empty")
	}
	if cfg.MCP.ConnectTimeoutSeconds <= 0 {
		return fmt.Errorf("mcp.connect_timeout_seconds must be greater than zero")
	}
	for i, server := range cfg.MCP.Servers {
		if !server.Enabled {
			continue
		}
		if server.Name == "" {
			return fmt.Errorf("mcp.servers[%d].name must not be empty", i)
		}
		if server.Command == "" {
			return fmt.Errorf("mcp.servers[%d].command must not be empty", i)
		}
	}
	return nil
}

func overrideString(key string, dest *string) {
	if value, ok := os.LookupEnv(key); ok {
		*dest = value
	}
}

func overrideInt(key string, dest *int) {
	value, ok := os.LookupEnv(key)
	if !ok {
		return
	}

	parsed, err := strconv.Atoi(value)
	if err == nil {
		*dest = parsed
	}
}

func overrideBool(key string, dest *bool) {
	value, ok := os.LookupEnv(key)
	if !ok {
		return
	}

	parsed, err := strconv.ParseBool(value)
	if err == nil {
		*dest = parsed
	}
}
