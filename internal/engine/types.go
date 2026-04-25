package engine

import (
	"context"

	"happyagent/internal/protocol"
	"happyagent/internal/tools"
)

type RunInput struct {
	Input         string
	SystemPrompt  string
	ToolDefs      []tools.Definition
	AfterToolCall func(ctx context.Context, toolName string, callErr error, input *RunInput) error
}

type RunResult struct {
	Output string
	Steps  []StepRecord
}

type StepRecord struct {
	Index       int
	Actions     []Action
	Observation string
}

type Action = protocol.Action

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
	Role             string
	Content          string
	ReasoningContent string
	ToolCallID       string
	ToolName         string
	Actions          []Action
}
