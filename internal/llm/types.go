package llm

import "happyagent/internal/protocol"

type Message struct {
	Role             string           `json:"role"`
	Content          string           `json:"content"`
	ReasoningContent string           `json:"reasoning_content,omitempty"`
	ToolCallID       string           `json:"tool_call_id,omitempty"`
	ToolName         string           `json:"tool_name,omitempty"`
	Actions          []protocol.Action `json:"actions,omitempty"`
}

type ToolSpec struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	InputSchema string `json:"input_schema"`
}

type ChatRequest struct {
	Messages []Message  `json:"messages"`
	Tools    []ToolSpec `json:"tools,omitempty"`
}

type ChatResponse struct {
	Message Message          `json:"message"`
	Actions []protocol.Action `json:"actions,omitempty"`
	Usage   TokenUsage       `json:"usage,omitempty"`
}

type TokenUsage struct {
	PromptTokens     int `json:"prompt_tokens,omitempty"`
	CompletionTokens int `json:"completion_tokens,omitempty"`
	TotalTokens      int `json:"total_tokens,omitempty"`
}
