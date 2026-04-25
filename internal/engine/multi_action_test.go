package engine

import (
	"context"
	"encoding/json"
	"testing"

	"happyagent/internal/llm"
	"happyagent/internal/protocol"
	"happyagent/internal/tools"
)

type stubClient struct {
	responses []llm.ChatResponse
	index     int
}

func (c *stubClient) Chat(ctx context.Context, req llm.ChatRequest) (llm.ChatResponse, error) {
	if c.index >= len(c.responses) {
		return llm.ChatResponse{}, nil
	}
	resp := c.responses[c.index]
	c.index++
	return resp, nil
}

type stubTool struct {
	def tools.Definition
	run func(call tools.Call) (tools.Result, error)
}

func (t stubTool) Definition() tools.Definition { return t.def }

func (t stubTool) Execute(ctx context.Context, call tools.Call) (tools.Result, error) {
	return t.run(call)
}

func TestRunnerExecutesMultipleToolCallsInSingleModelTurn(t *testing.T) {
	registry := tools.NewRegistry()
	registry.MustRegister(stubTool{
		def: tools.Definition{Name: "activate_skill"},
		run: func(call tools.Call) (tools.Result, error) {
			return tools.Result{Output: "Activated skill file-inspector."}, nil
		},
	})
	registry.MustRegister(stubTool{
		def: tools.Definition{Name: "file_list"},
		run: func(call tools.Call) (tools.Result, error) {
			var args struct {
				Path string `json:"path"`
			}
			if err := json.Unmarshal(call.Arguments, &args); err != nil {
				t.Fatalf("unmarshal args: %v", err)
			}
			if args.Path != "." {
				t.Fatalf("unexpected path: %q", args.Path)
			}
			return tools.Result{Output: "README.md\ninternal/\nskills/"}, nil
		},
	})

	client := &stubClient{
		responses: []llm.ChatResponse{
			{
				Message: llm.Message{
					Role: protocol.RoleAssistant,
					Actions: []protocol.Action{
						{
							Type:       protocol.ActionToolCall,
							ToolCallID: "call_activate",
							ToolName:   "activate_skill",
							Arguments:  json.RawMessage(`{"skill_names":["file-inspector"]}`),
						},
						{
							Type:       protocol.ActionToolCall,
							ToolCallID: "call_list",
							ToolName:   "file_list",
							Arguments:  json.RawMessage(`{"path":"."}`),
						},
					},
				},
				Actions: []protocol.Action{
					{
						Type:       protocol.ActionToolCall,
						ToolCallID: "call_activate",
						ToolName:   "activate_skill",
						Arguments:  json.RawMessage(`{"skill_names":["file-inspector"]}`),
					},
					{
						Type:       protocol.ActionToolCall,
						ToolCallID: "call_list",
						ToolName:   "file_list",
						Arguments:  json.RawMessage(`{"path":"."}`),
					},
				},
			},
			{
				Message: llm.Message{
					Role:    protocol.RoleAssistant,
					Content: `{"type":"final_answer","content":"done"}`,
				},
			},
		},
	}

	runner := NewRunner(client, registry, 4)
	input := RunInput{
		Input:        "inspect repo",
		SystemPrompt: "base prompt",
		ToolDefs: []tools.Definition{
			{Name: "activate_skill"},
		},
		AfterToolCall: func(ctx context.Context, toolName string, callErr error, input *RunInput) error {
			if toolName == "activate_skill" {
				input.ToolDefs = append(input.ToolDefs, tools.Definition{Name: "file_list"})
			}
			return nil
		},
	}

	result, err := runner.Run(context.Background(), input)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Output != "done" {
		t.Fatalf("unexpected output: %q", result.Output)
	}
	if len(result.Steps) != 2 {
		t.Fatalf("unexpected step count: %d", len(result.Steps))
	}
	if len(result.Steps[0].Actions) != 2 {
		t.Fatalf("unexpected first step actions: %+v", result.Steps[0].Actions)
	}
	if result.Steps[0].Actions[0].ToolName != "activate_skill" || result.Steps[0].Actions[1].ToolName != "file_list" {
		t.Fatalf("unexpected first step tool order: %+v", result.Steps[0].Actions)
	}
	if result.Steps[0].Observation != "Activated skill file-inspector.\n\nREADME.md\ninternal/\nskills/" {
		t.Fatalf("unexpected first step observation: %q", result.Steps[0].Observation)
	}
}
