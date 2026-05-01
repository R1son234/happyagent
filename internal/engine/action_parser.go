package engine

import (
	"encoding/json"
	"fmt"

	"happyagent/internal/protocol"
)

func ParseAction(content string) (Action, error) {
	var action Action

	if err := json.Unmarshal([]byte(content), &action); err != nil {
		return Action{}, fmt.Errorf("parse model action as JSON: %w", err)
	}

	switch action.Type {
	case protocol.ActionToolCall:
		if action.ToolName == "" {
			return Action{}, fmt.Errorf("tool_call action requires tool_name")
		}
		if action.ToolCallID == "" {
			return Action{}, fmt.Errorf("tool_call action requires tool_call_id")
		}
		if len(action.Arguments) == 0 {
			action.Arguments = json.RawMessage("{}")
		}
	case protocol.ActionFinalAnswer:
		if action.Content == "" {
			return Action{}, fmt.Errorf("final_answer action requires content")
		}
	default:
		return Action{}, fmt.Errorf("unsupported action type %q", action.Type)
	}

	return action, nil
}
