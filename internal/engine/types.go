package engine

import (
	"context"
	"time"

	"happyagent/internal/llm"
	"happyagent/internal/protocol"
	"happyagent/internal/tools"
)

type RunConfig struct {
	MaxObservationBytes int
	Offload             OffloadConfig
}

type RunHooks struct {
	BeforeToolCall      func(ctx context.Context, action Action, input *RunInput) (string, bool, error)
	AfterToolCall       func(ctx context.Context, toolName string, callErr error, input *RunInput) error
	ValidateFinalAnswer func(content string) error
	OnStepStart         func(stepIndex int)
	OnToolCallStart     func(toolName string)
	OnToolCallEnd       func(toolName string, succeeded bool)
}

type RunInput struct {
	Input          string
	SystemPrompt   string
	RuntimeContext string
	ToolDefs       []tools.Definition
	Config         RunConfig
	Hooks          RunHooks
}

type OffloadConfig struct {
	Enabled  bool
	MinBytes int
	Dir      string
	RootDir  string
	RunID    string
}

type RunResult struct {
	Output string
	Steps  []StepRecord
	Trace  RunTrace
}

type StepRecord struct {
	Index                   int              `json:"index"`
	Actions                 []Action         `json:"actions"`
	ToolCalls               []ToolCallRecord `json:"tool_calls,omitempty"`
	Observation             string           `json:"observation"`
	ModelUsage              llm.TokenUsage   `json:"model_usage"`
	PlanningDurationMillis  int64            `json:"planning_duration_millis"`
	ExecutionDurationMillis int64            `json:"execution_duration_millis"`
}

type ToolCallRecord struct {
	ToolName       string `json:"tool_name"`
	Status         string `json:"status"`
	Offloaded      bool   `json:"offloaded,omitempty"`
	OffloadPath    string `json:"offload_path,omitempty"`
	OffloadedBytes int    `json:"offloaded_bytes,omitempty"`
	OffloadError   string `json:"offload_error,omitempty"`
}

type Action = protocol.Action

type LoopState struct {
	Messages             []MessageEnvelope
	Steps                []StepRecord
	Todos                []tools.TodoItem
	DeliveryToolFailures map[string]string
}

type StepResult struct {
	Observation string
	ToolCalls   []ToolCallRecord
	Done        bool
	Output      string
}

type PlanStepResult struct {
	Actions  []Action
	Usage    llm.TokenUsage
	Duration time.Duration
}

type RunTrace struct {
	StartedAt                  time.Time      `json:"started_at"`
	FinishedAt                 time.Time      `json:"finished_at"`
	DurationMillis             int64          `json:"duration_millis"`
	TerminationReason          string         `json:"termination_reason,omitempty"`
	ErrorCategory              string         `json:"error_category,omitempty"`
	StepCount                  int            `json:"step_count"`
	ToolCallCount              int            `json:"tool_call_count"`
	ToolCallsByName            map[string]int `json:"tool_calls_by_name"`
	ExecutedToolCallCount      int            `json:"executed_tool_call_count"`
	ExecutedToolCallsByName    map[string]int `json:"executed_tool_calls_by_name"`
	SuccessfulToolCallCount    int            `json:"successful_tool_call_count"`
	SuccessfulToolCallsByName  map[string]int `json:"successful_tool_calls_by_name"`
	FailedToolCallCount        int            `json:"failed_tool_call_count,omitempty"`
	FailedToolCallsByName      map[string]int `json:"failed_tool_calls_by_name,omitempty"`
	UnavailableToolCallCount   int            `json:"unavailable_tool_call_count,omitempty"`
	UnavailableToolCallsByName map[string]int `json:"unavailable_tool_calls_by_name,omitempty"`
	BlockedToolCallCount       int            `json:"blocked_tool_call_count,omitempty"`
	BlockedToolCallsByName     map[string]int `json:"blocked_tool_calls_by_name,omitempty"`
	OffloadedToolResultCount   int            `json:"offloaded_tool_result_count,omitempty"`
	OffloadedToolResultBytes   int            `json:"offloaded_tool_result_bytes,omitempty"`
	OffloadedToolResultsByName map[string]int `json:"offloaded_tool_results_by_name,omitempty"`
	PromptTokens               int            `json:"prompt_tokens"`
	CompletionTokens           int            `json:"completion_tokens"`
	TotalTokens                int            `json:"total_tokens"`
}

type MessageEnvelope struct {
	Role             string
	Content          string
	ReasoningContent string
	ToolCallID       string
	ToolName         string
	Actions          []Action
}
