package config

type Config struct {
	LLM    LLMConfig    `json:"llm"`
	Engine EngineConfig `json:"engine"`
	Tools  ToolsConfig  `json:"tools"`
	MCP    MCPConfig    `json:"mcp"`
	Skills SkillsConfig `json:"skills"`
	Web    WebConfig    `json:"web"` // Web controls optional outbound web search and fetch tools.
}

type LLMConfig struct {
	Model          string `json:"model"`
	APIKey         string `json:"api_key"`
	BaseURL        string `json:"base_url"`
	TimeoutSeconds int    `json:"timeout_seconds"`
}

func (c LLMConfig) SafeString() string {
	if c.APIKey == "" {
		return "(no key)"
	}
	if len(c.APIKey) <= 8 {
		return "****"
	}
	return c.APIKey[:4] + "****" + c.APIKey[len(c.APIKey)-4:]
}

type EngineConfig struct {
	LoopMaxSteps        int    `json:"loop_max_steps"`
	MaxObservationBytes int    `json:"max_observation_bytes"`
	RunTimeoutSeconds   int    `json:"run_timeout_seconds"`
	SystemPrompt        string `json:"system_prompt"`
	OffloadEnabled      bool   `json:"offload_enabled"`
	OffloadMinBytes     int    `json:"offload_min_bytes"`
	OffloadDir          string `json:"offload_dir"`
}

type ToolsConfig struct {
	RootDir                   string   `json:"root_dir"`
	ShellEnabled              bool     `json:"shell_enabled"`
	ShellAllowedCommands      []string `json:"shell_allowed_commands"`
	ApprovedTools             []string `json:"approved_tools"`
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
	MaxPromptArgsBytes    int               `json:"max_prompt_args_bytes"`
	Servers               []MCPServerConfig `json:"servers"`
}

type MCPServerConfig struct {
	Name      string            `json:"name"`
	Command   string            `json:"command"`
	Args      []string          `json:"args"`
	Env       map[string]string `json:"env"`
	Enabled   bool              `json:"enabled"`
	SafeTools []string          `json:"safe_tools,omitempty"`
}

type SkillsConfig struct {
	Dir string `json:"dir"`
}

type WebConfig struct {
	Enabled               bool     `json:"enabled"`                 // Enabled controls whether web_search and web_fetch are registered.
	SearchBackend         string   `json:"search_backend"`          // SearchBackend selects "auto", "direct", or "searxng" for web_search.
	SearXNGURL            string   `json:"searxng_url"`             // SearXNGURL is the base URL of the SearXNG HTTP service used by web_search.
	DirectSearchURL       string   `json:"direct_search_url"`       // DirectSearchURL optionally overrides the direct search endpoint for tests or advanced use.
	RequestTimeoutSeconds int      `json:"request_timeout_seconds"` // RequestTimeoutSeconds bounds each outbound HTTP request.
	MaxFetchBytes         int      `json:"max_fetch_bytes"`         // MaxFetchBytes is the maximum extracted content preview returned by web_fetch.
	MaxSearchResults      int      `json:"max_search_results"`      // MaxSearchResults caps the number of results returned by web_search.
	AllowPrivateNetworks  bool     `json:"allow_private_networks"`  // AllowPrivateNetworks permits localhost/private-network fetches when explicitly enabled.
	BlockedDomains        []string `json:"blocked_domains"`         // BlockedDomains lists domains that web tools must not access.
}

func Default() Config {
	return Config{
		LLM: LLMConfig{
			Model:          "gpt-4o-mini",
			TimeoutSeconds: 60,
		},
		Engine: EngineConfig{
			LoopMaxSteps:        8,
			MaxObservationBytes: 8 * 1024,
			RunTimeoutSeconds:   60,
			SystemPrompt: `<agent>
  <role>You are a local coding agent.</role>
  <response_contract>
    Reply with exactly one JSON action object and no extra text.
    When you need to act, respond with {"type":"tool_call","tool_name":"...","arguments":{...}} using only tool names that appear in the provided tool list.
    When you are done, use final_answer.
  </response_contract>
</agent>`,
			OffloadEnabled:  false,
			OffloadMinBytes: 12 * 1024,
			OffloadDir:      ".happyagent/offload",
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
			MaxPromptArgsBytes:    8 * 1024,
			Servers:               nil,
		},
		Skills: SkillsConfig{
			Dir: "skills",
		},
		Web: WebConfig{
			Enabled:               false,
			SearchBackend:         "auto",
			RequestTimeoutSeconds: 15,
			MaxFetchBytes:         64 * 1024,
			MaxSearchResults:      10,
			AllowPrivateNetworks:  false,
			BlockedDomains:        nil,
		},
	}
}
