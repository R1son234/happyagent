package protocol

import (
	"bytes"
	"encoding/json"
)

type Action struct {
	Type       string          `json:"type"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
	ToolName   string          `json:"tool_name,omitempty"`
	Arguments  json.RawMessage `json:"arguments,omitempty"`
	Content    string          `json:"content,omitempty"`
}

func (a Action) MarshalJSON() ([]byte, error) {
	type actionJSON struct {
		Type       string          `json:"type"`
		ToolCallID string          `json:"tool_call_id,omitempty"`
		ToolName   string          `json:"tool_name,omitempty"`
		Arguments  json.RawMessage `json:"arguments,omitempty"`
		Content    string          `json:"content,omitempty"`
	}

	arguments := a.Arguments
	if len(bytes.TrimSpace(arguments)) > 0 && !json.Valid(arguments) {
		sanitized, err := json.Marshal(map[string]any{
			"_invalid_json": true,
			"_raw":          string(arguments),
		})
		if err != nil {
			return nil, err
		}
		arguments = sanitized
	}

	return json.Marshal(actionJSON{
		Type:       a.Type,
		ToolCallID: a.ToolCallID,
		ToolName:   a.ToolName,
		Arguments:  arguments,
		Content:    a.Content,
	})
}
