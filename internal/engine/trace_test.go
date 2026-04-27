package engine

import (
	"testing"
	"time"

	"happyagent/internal/llm"
	"happyagent/internal/protocol"
)

func TestBuildRunTraceAggregatesUsageAndToolCalls(t *testing.T) {
	startedAt := time.Unix(100, 0)
	finishedAt := startedAt.Add(2500 * time.Millisecond)

	trace := buildRunTrace(startedAt, finishedAt, []StepRecord{
		{
			Index: 1,
			Actions: []Action{
				{Type: protocol.ActionToolCall, ToolName: "file_list"},
				{Type: protocol.ActionToolCall, ToolName: "file_read"},
			},
			ModelUsage: llm.TokenUsage{
				PromptTokens:     10,
				CompletionTokens: 5,
				TotalTokens:      15,
			},
		},
		{
			Index: 2,
			Actions: []Action{
				{Type: protocol.ActionFinalAnswer, Content: "done"},
			},
			ModelUsage: llm.TokenUsage{
				PromptTokens:     8,
				CompletionTokens: 4,
				TotalTokens:      12,
			},
		},
	})

	if trace.DurationMillis != 2500 {
		t.Fatalf("unexpected duration: %+v", trace)
	}
	if trace.StepCount != 2 || trace.ToolCallCount != 2 {
		t.Fatalf("unexpected counts: %+v", trace)
	}
	if trace.ToolCallsByName["file_list"] != 1 || trace.ToolCallsByName["file_read"] != 1 {
		t.Fatalf("unexpected tool calls: %+v", trace.ToolCallsByName)
	}
	if trace.PromptTokens != 18 || trace.CompletionTokens != 9 || trace.TotalTokens != 27 {
		t.Fatalf("unexpected token usage: %+v", trace)
	}
}
