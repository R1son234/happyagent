package engine

import (
	"context"
	"time"

	"happyagent/internal/llm"
	"happyagent/internal/protocol"
	"happyagent/internal/tools"
)

type RunInput struct {
	Input               string
	SystemPrompt        string
	ToolDefs            []tools.Definition
	MaxObservationBytes int
	AfterToolCall       func(ctx context.Context, toolName string, callErr error, input *RunInput) error
}

type RunResult struct {
	Output string
	Steps  []StepRecord
	Trace  RunTrace
}

type StepRecord struct {
	Index                   int            `json:"index"`
	Actions                 []Action       `json:"actions"`
	Observation             string         `json:"observation"`
	ModelUsage              llm.TokenUsage `json:"model_usage"`
	PlanningDurationMillis  int64          `json:"planning_duration_millis"`
	ExecutionDurationMillis int64          `json:"execution_duration_millis"`
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

type PlanStepResult struct {
	Actions  []Action
	Usage    llm.TokenUsage
	Duration time.Duration
}

type RunTrace struct {
	StartedAt        time.Time      `json:"started_at"`
	FinishedAt       time.Time      `json:"finished_at"`
	DurationMillis   int64          `json:"duration_millis"`
	StepCount        int            `json:"step_count"`
	ToolCallCount    int            `json:"tool_call_count"`
	ToolCallsByName  map[string]int `json:"tool_calls_by_name"`
	PromptTokens     int            `json:"prompt_tokens"`
	CompletionTokens int            `json:"completion_tokens"`
	TotalTokens      int            `json:"total_tokens"`
}

type MessageEnvelope struct {
	Role             string
	Content          string
	ReasoningContent string
	ToolCallID       string
	ToolName         string
	Actions          []Action
}
