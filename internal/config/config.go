package config

type Config struct {
	LLM    LLMConfig    `json:"llm"`
	Engine EngineConfig `json:"engine"`
	Tools  ToolsConfig  `json:"tools"`
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

func Default() Config {
	return Config{
		LLM: LLMConfig{
			Model: "gpt-4o-mini",
		},
		Engine: EngineConfig{
			LoopMaxSteps:      8,
			RunTimeoutSeconds: 60,
			SystemPrompt: "You are a local coding agent. Reply with a JSON action. " +
				"Use {\"type\":\"final_answer\",\"content\":\"...\"} when you are done, " +
				"or {\"type\":\"tool_call\",\"tool_name\":\"...\",\"arguments\":{...}} to call a tool.",
		},
		Tools: ToolsConfig{
			RootDir:       ".",
			ShellEnabled:  true,
			WriteEnabled:  true,
			DeleteEnabled: false,
		},
	}
}
