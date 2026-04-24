package engine

import "happyagent/internal/tools"

type RunInput struct {
	Input        string
	SystemPrompt string
	ToolDefs     []tools.Definition
}

type RunResult struct {
	Output string
	Steps  []StepRecord
}

type StepRecord struct {
	Index       int
	Action      Action
	Observation string
}

type Action struct {
	Type      string `json:"type"`
	ToolName  string `json:"tool_name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
	Content   string `json:"content,omitempty"`
}

type LoopState struct {
	Messages []MessageEnvelope
	Steps    []StepRecord
}

type StepResult struct {
	Observation string
	Done        bool
	Output      string
}

type MessageEnvelope struct {
	Role    string
	Content string
}
