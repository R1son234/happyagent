package engine

import (
	"context"
	"strings"
	"testing"

	"happyagent/internal/llm"
	"happyagent/internal/protocol"
	"happyagent/internal/tools"
)

func TestRunnerAppendsTodoReminderToToolResultsWhileTodosAreUnfinished(t *testing.T) {
	registry := tools.NewRegistry()
	registry.MustRegister(tools.NewWriteTodosTool())
	registry.MustRegister(stubTool{
		def: tools.Definition{Name: "file_read"},
		run: func(call tools.Call) (tools.Result, error) {
			return tools.Result{Output: "file contents"}, nil
		},
	})

	client := &stubClient{
		responses: []llm.ChatResponse{
			{
				Message: llm.Message{
					Role: protocol.RoleAssistant,
					Actions: []protocol.Action{{
						Type:       protocol.ActionToolCall,
						ToolCallID: "call_todos",
						ToolName:   tools.WriteTodosToolName,
						Arguments:  []byte(`{"todos":[{"content":"Read file","status":"in_progress","priority":"high"},{"content":"Summarize","status":"pending"}]}`),
					}},
				},
				Actions: []protocol.Action{{
					Type:       protocol.ActionToolCall,
					ToolCallID: "call_todos",
					ToolName:   tools.WriteTodosToolName,
					Arguments:  []byte(`{"todos":[{"content":"Read file","status":"in_progress","priority":"high"},{"content":"Summarize","status":"pending"}]}`),
				}},
			},
			{
				Message: llm.Message{
					Role: protocol.RoleAssistant,
					Actions: []protocol.Action{{
						Type:       protocol.ActionToolCall,
						ToolCallID: "call_read",
						ToolName:   "file_read",
						Arguments:  []byte(`{"path":"README.md"}`),
					}},
				},
				Actions: []protocol.Action{{
					Type:       protocol.ActionToolCall,
					ToolCallID: "call_read",
					ToolName:   "file_read",
					Arguments:  []byte(`{"path":"README.md"}`),
				}},
			},
			{
				Message: llm.Message{
					Role: protocol.RoleAssistant,
					Actions: []protocol.Action{{
						Type:       protocol.ActionToolCall,
						ToolCallID: "call_done",
						ToolName:   tools.WriteTodosToolName,
						Arguments:  []byte(`{"todos":[{"content":"Read file","status":"completed"},{"content":"Summarize","status":"completed"}]}`),
					}},
				},
				Actions: []protocol.Action{{
					Type:       protocol.ActionToolCall,
					ToolCallID: "call_done",
					ToolName:   tools.WriteTodosToolName,
					Arguments:  []byte(`{"todos":[{"content":"Read file","status":"completed"},{"content":"Summarize","status":"completed"}]}`),
				}},
			},
			{
				Message: llm.Message{
					Role:    protocol.RoleAssistant,
					Content: `{"type":"final_answer","content":"done"}`,
				},
			},
		},
	}

	runner := NewRunner(client, registry, 5)
	var todoUpdates [][]tools.TodoItem
	result, err := runner.Run(context.Background(), RunInput{
		Input:        "read and summarize",
		SystemPrompt: "base prompt",
		ToolDefs: []tools.Definition{
			tools.NewWriteTodosTool().Definition(),
			{Name: "file_read"},
		},
		Hooks: RunHooks{
			OnTodosUpdated: func(todos []tools.TodoItem) {
				todoUpdates = append(todoUpdates, todos)
			},
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Output != "done" {
		t.Fatalf("unexpected output: %q", result.Output)
	}
	if len(client.requests) < 3 {
		t.Fatalf("unexpected request count: %d", len(client.requests))
	}

	secondRequestMessages := client.requests[1].Messages
	writeTodosObservation := secondRequestMessages[len(secondRequestMessages)-1]
	if writeTodosObservation.Role != protocol.RoleTool {
		t.Fatalf("expected write_todos tool observation, got %+v", writeTodosObservation)
	}
	if !strings.Contains(writeTodosObservation.Content, "TODO plan updated") ||
		!strings.Contains(writeTodosObservation.Content, "There're still some TODOs not marked as 'completed'") {
		t.Fatalf("expected write_todos observation to include reminder, got %q", writeTodosObservation.Content)
	}

	thirdRequestMessages := client.requests[2].Messages
	fileReadObservation := thirdRequestMessages[len(thirdRequestMessages)-1]
	if fileReadObservation.Role != protocol.RoleTool {
		t.Fatalf("expected file_read tool observation, got %+v", fileReadObservation)
	}
	if !strings.Contains(fileReadObservation.Content, "file contents") ||
		!strings.Contains(fileReadObservation.Content, "There're still some TODOs not marked as 'completed'") {
		t.Fatalf("expected file_read observation to include reminder, got %q", fileReadObservation.Content)
	}
	if len(todoUpdates) != 2 {
		t.Fatalf("expected 2 todo update hooks, got %d", len(todoUpdates))
	}
	if todoUpdates[0][0].Content != "Read file" || todoUpdates[0][0].Status != "in_progress" {
		t.Fatalf("unexpected first todo update: %+v", todoUpdates[0])
	}
	if todoUpdates[1][0].Status != "completed" || todoUpdates[1][1].Status != "completed" {
		t.Fatalf("unexpected final todo update: %+v", todoUpdates[1])
	}
}

func TestRunnerBlocksFinalAnswerWhenTodosAreUnfinished(t *testing.T) {
	registry := tools.NewRegistry()
	registry.MustRegister(tools.NewWriteTodosTool())
	registry.MustRegister(tools.NewFinalAnswerTool())

	client := &stubClient{
		responses: []llm.ChatResponse{
			{
				Message: llm.Message{
					Role: protocol.RoleAssistant,
					Actions: []protocol.Action{{
						Type:       protocol.ActionToolCall,
						ToolCallID: "call_todos",
						ToolName:   tools.WriteTodosToolName,
						Arguments:  []byte(`{"todos":[{"content":"Read file","status":"pending"}]}`),
					}},
				},
				Actions: []protocol.Action{{
					Type:       protocol.ActionToolCall,
					ToolCallID: "call_todos",
					ToolName:   tools.WriteTodosToolName,
					Arguments:  []byte(`{"todos":[{"content":"Read file","status":"pending"}]}`),
				}},
			},
			{
				Message: llm.Message{
					Role: protocol.RoleAssistant,
					Actions: []protocol.Action{{
						Type:       protocol.ActionToolCall,
						ToolCallID: "call_final_blocked",
						ToolName:   tools.FinalAnswerToolName,
						Arguments:  []byte(`{"content":"done too early"}`),
					}},
				},
				Actions: []protocol.Action{{
					Type:       protocol.ActionToolCall,
					ToolCallID: "call_final_blocked",
					ToolName:   tools.FinalAnswerToolName,
					Arguments:  []byte(`{"content":"done too early"}`),
				}},
			},
			{
				Message: llm.Message{
					Role: protocol.RoleAssistant,
					Actions: []protocol.Action{{
						Type:       protocol.ActionToolCall,
						ToolCallID: "call_todos_done",
						ToolName:   tools.WriteTodosToolName,
						Arguments:  []byte(`{"todos":[{"content":"Read file","status":"completed"}]}`),
					}},
				},
				Actions: []protocol.Action{{
					Type:       protocol.ActionToolCall,
					ToolCallID: "call_todos_done",
					ToolName:   tools.WriteTodosToolName,
					Arguments:  []byte(`{"todos":[{"content":"Read file","status":"completed"}]}`),
				}},
			},
			{
				Message: llm.Message{
					Role: protocol.RoleAssistant,
					Actions: []protocol.Action{{
						Type:       protocol.ActionToolCall,
						ToolCallID: "call_final",
						ToolName:   tools.FinalAnswerToolName,
						Arguments:  []byte(`{"content":"done"}`),
					}},
				},
				Actions: []protocol.Action{{
					Type:       protocol.ActionToolCall,
					ToolCallID: "call_final",
					ToolName:   tools.FinalAnswerToolName,
					Arguments:  []byte(`{"content":"done"}`),
				}},
			},
		},
	}

	runner := NewRunner(client, registry, 5)
	result, err := runner.Run(context.Background(), RunInput{
		Input:        "finish carefully",
		SystemPrompt: "base prompt",
		ToolDefs: []tools.Definition{
			tools.NewWriteTodosTool().Definition(),
			tools.NewFinalAnswerTool().Definition(),
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Output != "done" {
		t.Fatalf("unexpected output: %q", result.Output)
	}
	if len(result.Steps) < 2 || !strings.Contains(result.Steps[1].Observation, "still has unfinished items") {
		t.Fatalf("expected blocked final_answer reminder, got %+v", result.Steps)
	}
	if result.Steps[1].ToolCalls[0].Status != protocol.ToolCallStatusBlocked {
		t.Fatalf("expected blocked final_answer tool call, got %+v", result.Steps[1].ToolCalls)
	}
}

func TestRunnerBlocksDirectFinalAnswerWhenTodosAreUnfinished(t *testing.T) {
	registry := tools.NewRegistry()
	registry.MustRegister(tools.NewWriteTodosTool())

	client := &stubClient{
		responses: []llm.ChatResponse{
			{
				Message: llm.Message{
					Role: protocol.RoleAssistant,
					Actions: []protocol.Action{{
						Type:       protocol.ActionToolCall,
						ToolCallID: "call_todos",
						ToolName:   tools.WriteTodosToolName,
						Arguments:  []byte(`{"todos":[{"content":"Check docs","status":"pending"}]}`),
					}},
				},
				Actions: []protocol.Action{{
					Type:       protocol.ActionToolCall,
					ToolCallID: "call_todos",
					ToolName:   tools.WriteTodosToolName,
					Arguments:  []byte(`{"todos":[{"content":"Check docs","status":"pending"}]}`),
				}},
			},
			{
				Message: llm.Message{
					Role:    protocol.RoleAssistant,
					Content: `{"type":"final_answer","content":"too early"}`,
				},
			},
			{
				Message: llm.Message{
					Role: protocol.RoleAssistant,
					Actions: []protocol.Action{{
						Type:       protocol.ActionToolCall,
						ToolCallID: "call_todos_done",
						ToolName:   tools.WriteTodosToolName,
						Arguments:  []byte(`{"todos":[{"content":"Check docs","status":"completed"}]}`),
					}},
				},
				Actions: []protocol.Action{{
					Type:       protocol.ActionToolCall,
					ToolCallID: "call_todos_done",
					ToolName:   tools.WriteTodosToolName,
					Arguments:  []byte(`{"todos":[{"content":"Check docs","status":"completed"}]}`),
				}},
			},
			{
				Message: llm.Message{
					Role:    protocol.RoleAssistant,
					Content: `{"type":"final_answer","content":"done"}`,
				},
			},
		},
	}

	runner := NewRunner(client, registry, 5)
	result, err := runner.Run(context.Background(), RunInput{
		Input:        "finish carefully",
		SystemPrompt: "base prompt",
		ToolDefs: []tools.Definition{
			tools.NewWriteTodosTool().Definition(),
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Output != "done" {
		t.Fatalf("unexpected output: %q", result.Output)
	}
	if len(result.Steps) < 2 || !strings.Contains(result.Steps[1].Observation, "still has unfinished items") {
		t.Fatalf("expected direct final answer reminder, got %+v", result.Steps)
	}
	if len(client.requests) < 3 {
		t.Fatalf("unexpected request count: %d", len(client.requests))
	}
	reminder := client.requests[2].Messages[len(client.requests[2].Messages)-1]
	if reminder.Role != protocol.RoleSystem {
		t.Fatalf("expected system reminder, got %+v", reminder)
	}
}
