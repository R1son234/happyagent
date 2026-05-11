package protocol

const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
)

const (
	ActionToolCall    = "tool_call"
	ActionFinalAnswer = "final_answer"
)

const (
	RunStatusCompleted = "completed"
	RunStatusFailed    = "failed"
)

const (
	ToolCallStatusUnavailable = "unavailable"
	ToolCallStatusBlocked     = "blocked"
	ToolCallStatusFailed      = "failed"
	ToolCallStatusSucceeded   = "succeeded"
)
