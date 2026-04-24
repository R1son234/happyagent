package protocol

import "encoding/json"

type Action struct {
	Type       string          `json:"type"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
	ToolName   string          `json:"tool_name,omitempty"`
	Arguments  json.RawMessage `json:"arguments,omitempty"`
	Content    string          `json:"content,omitempty"`
}
