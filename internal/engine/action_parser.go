package engine

import (
	"encoding/json"
	"fmt"
	"strings"

	"happyagent/internal/protocol"
)

func ParseAction(content string) (Action, error) {
	action, err := parseActionStrict(strings.TrimSpace(content))
	if err == nil {
		return action, nil
	}
	extracted, extractErr := parseTrailingActionObject(content)
	if extractErr == nil {
		return extracted, nil
	}
	return Action{}, err
}

func parseActionStrict(content string) (Action, error) {
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

func parseTrailingActionObject(content string) (Action, error) {
	content = strings.TrimSpace(content)
	for i := len(content) - 1; i >= 0; i-- {
		if content[i] != '{' {
			continue
		}
		action, err := parseActionStrict(content[i:])
		if err == nil {
			return action, nil
		}
	}
	return Action{}, fmt.Errorf("no trailing action JSON object")
}
