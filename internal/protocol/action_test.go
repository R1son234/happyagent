package protocol

import (
	"encoding/json"
	"testing"
)

func TestActionMarshalJSONSanitizesInvalidArguments(t *testing.T) {
	data, err := json.Marshal(Action{
		Type:      ActionToolCall,
		ToolName:  "demo",
		Arguments: json.RawMessage(`{"unterminated"`),
	})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if !json.Valid(data) {
		t.Fatalf("expected valid JSON, got %s", data)
	}

	var got struct {
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if got.Arguments["_invalid_json"] != true {
		t.Fatalf("expected invalid JSON marker, got %+v", got.Arguments)
	}
	if got.Arguments["_raw"] != `{"unterminated"` {
		t.Fatalf("expected raw payload, got %+v", got.Arguments)
	}
}
