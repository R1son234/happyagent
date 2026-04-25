package llm

import (
	"testing"

	"happyagent/internal/protocol"

	"github.com/cloudwego/eino/schema"
)

func TestFromEinoMessageUsesSingleToolCall(t *testing.T) {
	message, actions, err := fromEinoMessage(&schema.Message{
		Role: schema.Assistant,
		ToolCalls: []schema.ToolCall{
			{
				ID:   "call_1",
				Type: "function",
				Function: schema.FunctionCall{
					Name:      "file_list",
					Arguments: `{"path":"."}`,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("fromEinoMessage() error = %v", err)
	}
	if len(actions) != 1 {
		t.Fatalf("unexpected action count: %d", len(actions))
	}
	action := actions[0]
	if action.Type != protocol.ActionToolCall || action.ToolCallID != "call_1" || action.ToolName != "file_list" || string(action.Arguments) != `{"path":"."}` {
		t.Fatalf("unexpected action: %+v", action)
	}
	if len(message.Actions) != 1 || message.Actions[0].ToolCallID != action.ToolCallID || message.Actions[0].ToolName != action.ToolName || string(message.Actions[0].Arguments) != string(action.Arguments) {
		t.Fatalf("message actions mismatch: %+v vs %+v", message.Actions, actions)
	}
}

func TestFromEinoMessageKeepsAllToolCallsWhenModelReturnsMultiple(t *testing.T) {
	message, actions, err := fromEinoMessage(&schema.Message{
		Role: schema.Assistant,
		ToolCalls: []schema.ToolCall{
			{
				ID:   "call_1",
				Type: "function",
				Function: schema.FunctionCall{
					Name:      "activate_skill",
					Arguments: `{"skill_names":["file-inspector"]}`,
				},
			},
			{
				ID:   "call_2",
				Type: "function",
				Function: schema.FunctionCall{
					Name:      "file_list",
					Arguments: `{"path":"."}`,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("fromEinoMessage() error = %v", err)
	}
	if len(actions) != 2 {
		t.Fatalf("unexpected action count: %d", len(actions))
	}
	first := actions[0]
	second := actions[1]
	if first.ToolCallID != "call_1" || first.ToolName != "activate_skill" {
		t.Fatalf("unexpected first action: %+v", first)
	}
	if second.ToolCallID != "call_2" || second.ToolName != "file_list" {
		t.Fatalf("unexpected second action: %+v", second)
	}
	if len(message.Actions) != 2 {
		t.Fatalf("message actions mismatch: %+v vs %+v", message.Actions, actions)
	}
}
