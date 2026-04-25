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
	LoopMaxSteps      int    `json:"loop_max_steps"`
	RunTimeoutSeconds int    `json:"run_timeout_seconds"`
	SystemPrompt      string `json:"system_prompt"`
}

type ToolsConfig struct {
	RootDir       string `json:"root_dir"`
	ShellEnabled  bool   `json:"shell_enabled"`
	WriteEnabled  bool   `json:"write_enabled"`
	DeleteEnabled bool   `json:"delete_enabled"`
}

type MCPConfig struct {
	ConnectTimeoutSeconds int               `json:"connect_timeout_seconds"`
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
			LoopMaxSteps:      8,
			RunTimeoutSeconds: 60,
			SystemPrompt: "You are a local coding agent. Reply with exactly one JSON action object and no extra text. " +
				"When you need to act, respond with " +
				"{\"type\":\"tool_call\",\"tool_name\":\"...\",\"arguments\":{...}} " +
				"using only tool names that appear in the provided tool list.",
		},
		Tools: ToolsConfig{
			RootDir:       ".",
			ShellEnabled:  true,
			WriteEnabled:  true,
			DeleteEnabled: false,
		},
		MCP: MCPConfig{
			ConnectTimeoutSeconds: 15,
			Servers:               nil,
		},
		Skills: SkillsConfig{
			Dir: "skills",
		},
	}
}
