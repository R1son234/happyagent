package config

type Config struct {
	LLM    LLMConfig    `json:"llm"`
	Engine EngineConfig `json:"engine"`
	Tools  ToolsConfig  `json:"tools"`
	MCP    MCPConfig    `json:"mcp"`
	Skills SkillsConfig `json:"skills"`
}

type LLMConfig struct {
	Model   string `json:"model"`
	APIKey  string `json:"api_key"`
	BaseURL string `json:"base_url"`
}

type EngineConfig struct {
	LoopMaxSteps        int    `json:"loop_max_steps"`
	MaxObservationBytes int    `json:"max_observation_bytes"`
	RunTimeoutSeconds   int    `json:"run_timeout_seconds"`
	SystemPrompt        string `json:"system_prompt"`
}

type ToolsConfig struct {
	RootDir                   string   `json:"root_dir"`
	ShellEnabled              bool     `json:"shell_enabled"`
	ShellAllowedCommands      []string `json:"shell_allowed_commands"`
	WriteEnabled              bool     `json:"write_enabled"`
	WriteMaxBytes             int      `json:"write_max_bytes"`
	WriteRequireOverwrite     bool     `json:"write_require_overwrite"`
	DeleteEnabled             bool     `json:"delete_enabled"`
	DeleteRequireConfirmation bool     `json:"delete_require_confirmation"`
}

type MCPConfig struct {
	ConnectTimeoutSeconds int               `json:"connect_timeout_seconds"`
	MaxListedResources    int               `json:"max_listed_resources"`
	MaxResourceBytes      int               `json:"max_resource_bytes"`
	Servers               []MCPServerConfig `json:"servers"`
}

type MCPServerConfig struct {
	Name    string            `json:"name"`
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
	Enabled bool              `json:"enabled"`
}

type SkillsConfig struct {
	Dir string `json:"dir"`
}

func Default() Config {
	return Config{
		LLM: LLMConfig{
			Model: "gpt-4o-mini",
		},
		Engine: EngineConfig{
			LoopMaxSteps:        8,
			MaxObservationBytes: 8 * 1024,
			RunTimeoutSeconds:   60,
			SystemPrompt: "You are a local coding agent. Reply with exactly one JSON action object and no extra text. " +
				"When you need to act, respond with " +
				"{\"type\":\"tool_call\",\"tool_name\":\"...\",\"arguments\":{...}} " +
				"using only tool names that appear in the provided tool list.",
		},
		Tools: ToolsConfig{
			RootDir:                   ".",
			ShellEnabled:              true,
			ShellAllowedCommands:      []string{"cat", "echo", "find", "git", "go", "grep", "head", "ls", "make", "pwd", "printf", "rg", "sed", "tail", "wc"},
			WriteEnabled:              true,
			WriteMaxBytes:             32 * 1024,
			WriteRequireOverwrite:     true,
			DeleteEnabled:             false,
			DeleteRequireConfirmation: true,
		},
		MCP: MCPConfig{
			ConnectTimeoutSeconds: 15,
			MaxListedResources:    100,
			MaxResourceBytes:      8 * 1024,
			Servers:               nil,
		},
		Skills: SkillsConfig{
			Dir: "skills",
		},
	}
}
