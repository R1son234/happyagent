package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

func TestRunnerRetriesFinalAnswerToolAfterValidationFailure(t *testing.T) {
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
							Arguments:  []byte(`{"content":"{\"summary\":\"too small\"}"}`),
						},
					},
				},
				Actions: []protocol.Action{
					{
						Type:       protocol.ActionToolCall,
						ToolCallID: "call_1",
						ToolName:   tools.FinalAnswerToolName,
						Arguments:  []byte(`{"content":"{\"summary\":\"too small\"}"}`),
					},
				},
			},
			{
				Message: llm.Message{
					Role: protocol.RoleAssistant,
					Actions: []protocol.Action{
						{
							Type:       protocol.ActionToolCall,
							ToolCallID: "call_2",
							ToolName:   tools.FinalAnswerToolName,
							Arguments:  []byte(`{"content":"valid"}`),
						},
					},
				},
				Actions: []protocol.Action{
					{
						Type:       protocol.ActionToolCall,
						ToolCallID: "call_2",
						ToolName:   tools.FinalAnswerToolName,
						Arguments:  []byte(`{"content":"valid"}`),
					},
				},
			},
		},
	}

	registry := tools.NewRegistry()
	registry.MustRegister(tools.NewFinalAnswerTool())
	runner := NewRunner(client, registry, 3)
	result, err := runner.Run(context.Background(), RunInput{
		Input:        "finish",
		SystemPrompt: "reply with JSON action",
		ToolDefs: []tools.Definition{
			tools.NewFinalAnswerTool().Definition(),
		},
		Hooks: RunHooks{
			ValidateFinalAnswer: func(content string) error {
				if content != "valid" {
					return fmt.Errorf("invalid final answer")
				}
				return nil
			},
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Output != "valid" {
		t.Fatalf("unexpected output: %q", result.Output)
	}
	if len(result.Steps) != 2 {
		t.Fatalf("expected validation retry to use 2 steps, got %d", len(result.Steps))
	}
	if result.Steps[0].Observation != "invalid final answer" {
		t.Fatalf("unexpected validation observation: %q", result.Steps[0].Observation)
	}
}

func TestRunnerBlocksMisleadingFinalAnswerAfterDeliveryToolUnavailable(t *testing.T) {
	client := &stubClient{
		responses: []llm.ChatResponse{
			{
				Message: llm.Message{
					Role: protocol.RoleAssistant,
					Actions: []protocol.Action{{
						Type:       protocol.ActionToolCall,
						ToolCallID: "call_write",
						ToolName:   "file_write",
						Arguments:  []byte(`{"path":"out.md","content":"content"}`),
					}},
				},
				Actions: []protocol.Action{{
					Type:       protocol.ActionToolCall,
					ToolCallID: "call_write",
					ToolName:   "file_write",
					Arguments:  []byte(`{"path":"out.md","content":"content"}`),
				}},
			},
			{
				Message: llm.Message{
					Role:    protocol.RoleAssistant,
					Content: `{"type":"final_answer","content":"已保存到 out.md"}`,
				},
			},
			{
				Message: llm.Message{
					Role:    protocol.RoleAssistant,
					Content: `{"type":"final_answer","content":"未写入 out.md；完整内容：content"}`,
				},
			},
		},
	}

	runner := NewRunner(client, tools.NewRegistry(), 4)
	result, err := runner.Run(context.Background(), RunInput{
		Input:        "write a file",
		SystemPrompt: "reply with JSON action",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Output != "未写入 out.md；完整内容：content" {
		t.Fatalf("unexpected output: %q", result.Output)
	}
	if len(result.Steps) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(result.Steps))
	}
	if result.Steps[0].ToolCalls[0].Status != protocol.ToolCallStatusUnavailable {
		t.Fatalf("expected unavailable file_write, got %+v", result.Steps[0].ToolCalls)
	}
	if !strings.Contains(result.Steps[1].Observation, "previous delivery tool call failed") {
		t.Fatalf("expected delivery failure reminder, got %q", result.Steps[1].Observation)
	}
	if result.Trace.UnavailableToolCallCount != 1 || result.Trace.BlockedToolCallCount != 0 {
		t.Fatalf("unexpected trace counts: %+v", result.Trace)
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
	runner := NewRunner(client, registry, 4)
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

func TestRunnerTruncatesToolObservations(t *testing.T) {
	registry := tools.NewRegistry()
	registry.MustRegister(stubTool{
		def: tools.Definition{Name: "file_read"},
		run: func(call tools.Call) (tools.Result, error) {
			return tools.Result{Output: strings.Repeat("x", 64)}, nil
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
							ToolCallID: "call_1",
							ToolName:   "file_read",
							Arguments:  []byte(`{"path":"README.md"}`),
						},
					},
				},
				Actions: []protocol.Action{
					{
						Type:       protocol.ActionToolCall,
						ToolCallID: "call_1",
						ToolName:   "file_read",
						Arguments:  []byte(`{"path":"README.md"}`),
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
	result, err := runner.Run(context.Background(), RunInput{
		Input:        "read file",
		SystemPrompt: "reply with JSON action",
		Config: RunConfig{
			MaxObservationBytes: 48,
		},
		ToolDefs: []tools.Definition{
			{Name: "file_read"},
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(result.Steps[0].Observation, "[observation truncated]") {
		t.Fatalf("expected truncated observation, got %q", result.Steps[0].Observation)
	}
	if len(result.Steps[0].Observation) > 48 {
		t.Fatalf("expected observation length <= 48, got %d", len(result.Steps[0].Observation))
	}
}

func TestRunnerDoesNotTruncateFinalAnswerToolOutput(t *testing.T) {
	registry := tools.NewRegistry()
	registry.MustRegister(tools.NewFinalAnswerTool())

	longOutput := strings.Repeat("x", 128)
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
							Arguments:  []byte(`{"content":"` + longOutput + `"}`),
						},
					},
				},
				Actions: []protocol.Action{
					{
						Type:       protocol.ActionToolCall,
						ToolCallID: "call_1",
						ToolName:   tools.FinalAnswerToolName,
						Arguments:  []byte(`{"content":"` + longOutput + `"}`),
					},
				},
			},
		},
	}

	runner := NewRunner(client, registry, 4)
	result, err := runner.Run(context.Background(), RunInput{
		Input:        "finish",
		SystemPrompt: "reply with tool call",
		Config: RunConfig{
			MaxObservationBytes: 512,
		},
		ToolDefs: []tools.Definition{
			tools.NewFinalAnswerTool().Definition(),
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Output != longOutput {
		t.Fatalf("final output was truncated: got len %d want %d", len(result.Output), len(longOutput))
	}
	if result.Steps[0].Observation != "" {
		t.Fatalf("final answer step observation should stay out of trace, got %q", result.Steps[0].Observation)
	}
}

func TestRunnerOffloadsLargeToolObservation(t *testing.T) {
	root := t.TempDir()
	registry := tools.NewRegistry()
	largeOutput := strings.Repeat("x", 128)
	registry.MustRegister(stubTool{
		def: tools.Definition{Name: "shell"},
		run: func(call tools.Call) (tools.Result, error) {
			return tools.Result{Output: largeOutput}, nil
		},
	})

	client := &stubClient{
		responses: []llm.ChatResponse{
			{
				Message: llm.Message{
					Role: protocol.RoleAssistant,
					Actions: []protocol.Action{{
						Type:       protocol.ActionToolCall,
						ToolCallID: "call_1",
						ToolName:   "shell",
						Arguments:  []byte(`{"argv":["echo","hello"]}`),
					}},
				},
				Actions: []protocol.Action{{
					Type:       protocol.ActionToolCall,
					ToolCallID: "call_1",
					ToolName:   "shell",
					Arguments:  []byte(`{"argv":["echo","hello"]}`),
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

	runner := NewRunner(client, registry, 4)
	result, err := runner.Run(context.Background(), RunInput{
		Input:        "run shell",
		SystemPrompt: "reply with JSON action",
		Config: RunConfig{
			MaxObservationBytes: 512,
			Offload: OffloadConfig{
				Enabled:  true,
				MinBytes: 32,
				Dir:      ".happyagent/offload",
				RootDir:  root,
				RunID:    "run-1",
			},
		},
		ToolDefs: []tools.Definition{{Name: "shell"}},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	observation := result.Steps[0].Observation
	if !strings.Contains(observation, "[offloaded tool result]") {
		t.Fatalf("expected offload observation, got %q", observation)
	}
	call := result.Steps[0].ToolCalls[0]
	if !call.Offloaded || call.OffloadedBytes != len(largeOutput) || call.OffloadPath == "" {
		t.Fatalf("unexpected tool call record: %+v", call)
	}
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(call.OffloadPath)))
	if err != nil {
		t.Fatalf("read offloaded output: %v", err)
	}
	if string(data) != largeOutput {
		t.Fatalf("unexpected offloaded data")
	}
	if result.Trace.OffloadedToolResultCount != 1 || result.Trace.OffloadedToolResultBytes != len(largeOutput) {
		t.Fatalf("unexpected trace offload counters: %+v", result.Trace)
	}
	if result.Trace.OffloadedToolResultsByName["shell"] != 1 {
		t.Fatalf("unexpected trace offload by name: %+v", result.Trace.OffloadedToolResultsByName)
	}
}

func TestRunnerDoesNotReoffloadOffloadFileReads(t *testing.T) {
	root := t.TempDir()
	registry := tools.NewRegistry()
	largeOutput := strings.Repeat("x", 128)
	registry.MustRegister(stubTool{
		def: tools.Definition{Name: "file_read"},
		run: func(call tools.Call) (tools.Result, error) {
			return tools.Result{Output: largeOutput}, nil
		},
	})

	client := &stubClient{
		responses: []llm.ChatResponse{
			{
				Message: llm.Message{Role: protocol.RoleAssistant, Actions: []protocol.Action{{
					Type:       protocol.ActionToolCall,
					ToolCallID: "call_1",
					ToolName:   "file_read",
					Arguments:  []byte(`{"path":".happyagent/offload/run-1/step-1-file_read.txt"}`),
				}}},
				Actions: []protocol.Action{{
					Type:       protocol.ActionToolCall,
					ToolCallID: "call_1",
					ToolName:   "file_read",
					Arguments:  []byte(`{"path":".happyagent/offload/run-1/step-1-file_read.txt"}`),
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

	runner := NewRunner(client, registry, 4)
	result, err := runner.Run(context.Background(), RunInput{
		Input:        "read offload",
		SystemPrompt: "reply with JSON action",
		Config: RunConfig{
			MaxObservationBytes: 48,
			Offload: OffloadConfig{
				Enabled:  true,
				MinBytes: 32,
				Dir:      ".happyagent/offload",
				RootDir:  root,
				RunID:    "run-1",
			},
		},
		ToolDefs: []tools.Definition{{Name: "file_read"}},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	observation := result.Steps[0].Observation
	if strings.Contains(observation, "[offloaded tool result]") {
		t.Fatalf("did not expect offload file read to be re-offloaded: %q", observation)
	}
	if !strings.Contains(observation, "[observation truncated]") {
		t.Fatalf("expected offload file read to be truncated in context, got %q", observation)
	}
	if result.Steps[0].ToolCalls[0].Offloaded {
		t.Fatalf("did not expect tool call to be marked offloaded: %+v", result.Steps[0].ToolCalls[0])
	}
}

func TestRunnerFallsBackToTruncationWhenOffloadFails(t *testing.T) {
	registry := tools.NewRegistry()
	registry.MustRegister(stubTool{
		def: tools.Definition{Name: "shell"},
		run: func(call tools.Call) (tools.Result, error) {
			return tools.Result{Output: strings.Repeat("x", 128)}, nil
		},
	})
	client := &stubClient{
		responses: []llm.ChatResponse{
			{
				Message: llm.Message{Role: protocol.RoleAssistant, Actions: []protocol.Action{{
					Type:       protocol.ActionToolCall,
					ToolCallID: "call_1",
					ToolName:   "shell",
					Arguments:  []byte(`{"argv":["echo","hello"]}`),
				}}},
				Actions: []protocol.Action{{
					Type:       protocol.ActionToolCall,
					ToolCallID: "call_1",
					ToolName:   "shell",
					Arguments:  []byte(`{"argv":["echo","hello"]}`),
				}},
			},
			{Message: llm.Message{Role: protocol.RoleAssistant, Content: `{"type":"final_answer","content":"done"}`}},
		},
	}

	runner := NewRunner(client, registry, 4)
	result, err := runner.Run(context.Background(), RunInput{
		Input:        "run shell",
		SystemPrompt: "reply with JSON action",
		Config: RunConfig{
			MaxObservationBytes: 48,
			Offload: OffloadConfig{
				Enabled:  true,
				MinBytes: 32,
				Dir:      "../outside",
				RootDir:  t.TempDir(),
				RunID:    "run-1",
			},
		},
		ToolDefs: []tools.Definition{{Name: "shell"}},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(result.Steps[0].Observation, "[observation truncated]") {
		t.Fatalf("expected fallback truncation, got %q", result.Steps[0].Observation)
	}
	call := result.Steps[0].ToolCalls[0]
	if call.Status != protocol.ToolCallStatusSucceeded || call.Offloaded || call.OffloadError == "" {
		t.Fatalf("unexpected tool call record: %+v", call)
	}
}

func TestRunnerReturnsStepsOnMaxStepsExhaustion(t *testing.T) {
	client := &stubClient{
		responses: []llm.ChatResponse{
			{
				Message: llm.Message{
					Role: protocol.RoleAssistant,
					Actions: []protocol.Action{
						{
							Type:       protocol.ActionToolCall,
							ToolCallID: "call_1",
							ToolName:   "file_read",
							Arguments:  []byte(`{"path":"README.md"}`),
						},
					},
				},
				Actions: []protocol.Action{
					{
						Type:       protocol.ActionToolCall,
						ToolCallID: "call_1",
						ToolName:   "file_read",
						Arguments:  []byte(`{"path":"README.md"}`),
					},
				},
			},
			// No final_answer - will exhaust steps
		},
	}

	registry := tools.NewRegistry()
	registry.MustRegister(stubTool{
		def: tools.Definition{Name: "file_read"},
		run: func(call tools.Call) (tools.Result, error) {
			return tools.Result{Output: "file content"}, nil
		},
	})

	runner := NewRunner(client, registry, 1)
	result, err := runner.Run(context.Background(), RunInput{
		Input:        "read file",
		SystemPrompt: "reply with JSON action",
		ToolDefs:     []tools.Definition{{Name: "file_read"}},
	})
	if err == nil {
		t.Fatal("expected error on max steps exhaustion")
	}
	if len(result.Steps) != 1 {
		t.Fatalf("expected 1 step in result, got %d", len(result.Steps))
	}
	if result.Trace.StepCount != 1 {
		t.Fatalf("expected trace step count 1, got %d", result.Trace.StepCount)
	}
	if result.Trace.TerminationReason != "max_steps_exceeded" {
		t.Fatalf("unexpected termination reason: %q", result.Trace.TerminationReason)
	}
}

func TestFinalAnswerValidationFailureDoesNotDoubleAppendMessage(t *testing.T) {
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
							Arguments:  []byte(`{"content":"bad output"}`),
						},
					},
				},
				Actions: []protocol.Action{
					{
						Type:       protocol.ActionToolCall,
						ToolCallID: "call_1",
						ToolName:   tools.FinalAnswerToolName,
						Arguments:  []byte(`{"content":"bad output"}`),
					},
				},
			},
			{
				Message: llm.Message{
					Role:    protocol.RoleAssistant,
					Content: `{"type":"final_answer","content":"valid"}`,
				},
			},
		},
	}

	registry := tools.NewRegistry()
	registry.MustRegister(tools.NewFinalAnswerTool())
	runner := NewRunner(client, registry, 3)
	_, err := runner.Run(context.Background(), RunInput{
		Input:        "finish",
		SystemPrompt: "reply with JSON action",
		ToolDefs: []tools.Definition{
			tools.NewFinalAnswerTool().Definition(),
		},
		Hooks: RunHooks{
			ValidateFinalAnswer: func(content string) error {
				if content != "valid" {
					return fmt.Errorf("validation failed")
				}
				return nil
			},
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Check that the second LLM request has exactly one tool message for call_1
	if len(client.requests) < 2 {
		t.Fatalf("expected at least 2 requests, got %d", len(client.requests))
	}
	msgs := client.requests[1].Messages
	toolMsgCount := 0
	for _, m := range msgs {
		if m.Role == protocol.RoleTool && m.ToolCallID == "call_1" {
			toolMsgCount++
		}
	}
	if toolMsgCount != 1 {
		t.Fatalf("expected exactly 1 tool message for call_1, got %d", toolMsgCount)
	}
}

func TestRunnerUnsupportedActionTypeReturnsError(t *testing.T) {
	client := &stubClient{
		responses: []llm.ChatResponse{
			{
				Message: llm.Message{
					Role: protocol.RoleAssistant,
					Actions: []protocol.Action{
						{Type: "unknown_type", ToolCallID: "call_1", ToolName: "foo"},
					},
				},
				Actions: []protocol.Action{
					{Type: "unknown_type", ToolCallID: "call_1", ToolName: "foo"},
				},
			},
		},
	}
	runner := NewRunner(client, tools.NewRegistry(), 4)
	_, err := runner.Run(context.Background(), RunInput{
		Input:        "do something",
		SystemPrompt: "test",
		ToolDefs:     []tools.Definition{{Name: "foo"}},
	})
	if err == nil {
		t.Fatal("expected error for unsupported action type")
	}
	if !strings.Contains(err.Error(), "unsupported action type") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBeforeToolCallErrorPropagates(t *testing.T) {
	registry := tools.NewRegistry()
	registry.MustRegister(stubTool{
		def: tools.Definition{Name: "file_read"},
		run: func(call tools.Call) (tools.Result, error) {
			return tools.Result{Output: "content"}, nil
		},
	})
	client := &stubClient{
		responses: []llm.ChatResponse{
			{
				Message: llm.Message{
					Role: protocol.RoleAssistant,
					Actions: []protocol.Action{{
						Type:       protocol.ActionToolCall,
						ToolCallID: "call_1",
						ToolName:   "file_read",
						Arguments:  []byte(`{"path":"x"}`),
					}},
				},
				Actions: []protocol.Action{{
					Type:       protocol.ActionToolCall,
					ToolCallID: "call_1",
					ToolName:   "file_read",
					Arguments:  []byte(`{"path":"x"}`),
				}},
			},
		},
	}
	runner := NewRunner(client, registry, 4)
	expectedErr := fmt.Errorf("before tool call failed")
	_, err := runner.Run(context.Background(), RunInput{
		Input:        "read file",
		SystemPrompt: "test",
		ToolDefs:     []tools.Definition{{Name: "file_read"}},
		Hooks: RunHooks{
			BeforeToolCall: func(ctx context.Context, action Action, input *RunInput) (string, bool, error) {
				return "", false, expectedErr
			},
		},
	})
	if err == nil {
		t.Fatal("expected BeforeToolCall error to propagate")
	}
}
