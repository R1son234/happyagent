package engine

import (
	"context"
	"testing"

	"happyagent/internal/llm"
	"happyagent/internal/protocol"
	"happyagent/internal/tools"
)

func TestRunnerRetriesWhenModelReturnsPlainTextInsteadOfActionJSON(t *testing.T) {
	client := &stubClient{
		responses: []llm.ChatResponse{
			{
				Message: llm.Message{
					Role:    protocol.RoleAssistant,
					Content: "Here is the answer without the required JSON wrapper.",
				},
			},
			{
				Message: llm.Message{
					Role:    protocol.RoleAssistant,
					Content: `{"type":"final_answer","content":"wrapped correctly"}`,
				},
			},
		},
	}

	runner := NewRunner(client, tools.NewRegistry(), 4)
	result, err := runner.Run(context.Background(), RunInput{
		Input:        "say hello",
		SystemPrompt: "reply with JSON action",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Output != "wrapped correctly" {
		t.Fatalf("unexpected output: %q", result.Output)
	}
	if len(result.Steps) != 2 {
		t.Fatalf("unexpected step count: %d", len(result.Steps))
	}
	if result.Steps[0].Observation == "" {
		t.Fatal("expected first step to include format correction observation")
	}
}

func TestRunnerCompletesWhenModelUsesFinalAnswerTool(t *testing.T) {
	client := &stubClient{
		responses: []llm.ChatResponse{
			{
				Message: llm.Message{
					Role:    protocol.RoleAssistant,
					Content: `{"type":"tool_call","tool_name":"final_answer","tool_call_id":"call_1","arguments":{"content":"wrapped correctly"}}`,
				},
			},
		},
	}

	registry := tools.NewRegistry()
	registry.MustRegister(tools.NewFinalAnswerTool())
	runner := NewRunner(client, registry, 4)
	result, err := runner.Run(context.Background(), RunInput{
		Input:        "say hello",
		SystemPrompt: "reply with JSON action",
		ToolDefs: []tools.Definition{
			tools.NewFinalAnswerTool().Definition(),
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Output != "wrapped correctly" {
		t.Fatalf("unexpected output: %q", result.Output)
	}
	if len(result.Steps) != 1 {
		t.Fatalf("unexpected step count: %d", len(result.Steps))
	}
}

func TestRunnerErrorsWhenFinalAnswerToolIsCombinedWithOtherActions(t *testing.T) {
	client := &stubClient{
		responses: []llm.ChatResponse{
			{
				Message: llm.Message{
					Role: protocol.RoleAssistant,
					Actions: []protocol.Action{
						{
							Type:       protocol.ActionToolCall,
							ToolCallID: "call_1",
							ToolName:   tools.FinalAnswerToolName,
							Arguments:  []byte(`{"content":"done"}`),
						},
						{
							Type:       protocol.ActionToolCall,
							ToolCallID: "call_2",
							ToolName:   "file_list",
							Arguments:  []byte(`{"path":"."}`),
						},
					},
				},
				Actions: []protocol.Action{
					{
						Type:       protocol.ActionToolCall,
						ToolCallID: "call_1",
						ToolName:   tools.FinalAnswerToolName,
						Arguments:  []byte(`{"content":"done"}`),
					},
					{
						Type:       protocol.ActionToolCall,
						ToolCallID: "call_2",
						ToolName:   "file_list",
						Arguments:  []byte(`{"path":"."}`),
					},
				},
			},
		},
	}

	registry := tools.NewRegistry()
	registry.MustRegister(tools.NewFinalAnswerTool())
	runner := NewRunner(registryBackedClient(client), registry, 4)
	_, err := runner.Run(context.Background(), RunInput{
		Input:        "say hello",
		SystemPrompt: "reply with JSON action",
		ToolDefs: []tools.Definition{
			tools.NewFinalAnswerTool().Definition(),
			{Name: "file_list"},
		},
	})
	if err == nil || err.Error() != "final_answer tool must be the only action in a step" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func registryBackedClient(client *stubClient) *stubClient {
	return client
}
