package engine

import (
	"encoding/json"
	"fmt"
)

func ParseAction(content string) (Action, error) {
	var raw struct {
		Type      string          `json:"type"`
		ToolName  string          `json:"tool_name"`
		Arguments json.RawMessage `json:"arguments"`
		Content   string          `json:"content"`
	}

	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return Action{}, fmt.Errorf("parse model action as JSON: %w", err)
	}

	action := Action{
		Type:     raw.Type,
		ToolName: raw.ToolName,
		Content:  raw.Content,
	}
	if len(raw.Arguments) > 0 {
		action.Arguments = string(raw.Arguments)
	}

	switch action.Type {
	case "tool_call":
		if action.ToolName == "" {
			return Action{}, fmt.Errorf("tool_call action requires tool_name")
		}
		if action.Arguments == "" {
			action.Arguments = "{}"
		}
	case "final_answer":
		if action.Content == "" {
			return Action{}, fmt.Errorf("final_answer action requires content")
		}
	default:
		return Action{}, fmt.Errorf("unsupported action type %q", action.Type)
	}

	return action, nil
}
