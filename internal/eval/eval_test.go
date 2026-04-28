package eval

import (
	"context"
	"testing"

	"happyagent/internal/engine"
	"happyagent/internal/protocol"
)

type stubRunner struct {
	result RunResult
	err    error
}

func (r stubRunner) Run(ctx context.Context, req RunRequest) (RunResult, error) {
	return r.result, r.err
}

func TestRunSuiteMarksSuccessfulCase(t *testing.T) {
	suite := Suite{
		Name: "smoke",
		Cases: []Case{
			{
				Name:                   "list-files",
				Prompt:                 "list files",
				Profile:                "general-assistant",
				ExpectedOutputContains: []string{"happyagent"},
				RequiredTools:          []string{"file_list"},
				MaxSteps:               3,
			},
		},
	}

	result, err := RunSuite(context.Background(), stubRunner{
		result: RunResult{
			Output:      "happyagent repo summary",
			ProfileName: "general-assistant",
			Steps: []engine.StepRecord{
				{
					Index: 1,
					Actions: []engine.Action{
						{Type: protocol.ActionToolCall, ToolName: "file_list"},
					},
				},
			},
			Trace: engine.RunTrace{
				StepCount:       1,
				ToolCallCount:   1,
				ToolCallsByName: map[string]int{"file_list": 1},
				TotalTokens:     42,
			},
		},
	}, suite, "base prompt")
	if err != nil {
		t.Fatalf("RunSuite() error = %v", err)
	}
	if result.PassedCount != 1 || result.FailedCount != 0 {
		t.Fatalf("unexpected summary: %+v", result)
	}
	if !result.Results[0].Success {
		t.Fatalf("expected success result: %+v", result.Results[0])
	}
}

func TestRunSuiteMarksMissingOutputAndTools(t *testing.T) {
	suite := Suite{
		Name: "smoke",
		Cases: []Case{
			{
				Name:                   "search-read",
				Prompt:                 "search read",
				Profile:                "career-copilot",
				ExpectedOutputContains: []string{"argv"},
				RequiredTools:          []string{"file_search", "file_read"},
			},
		},
	}

	result, err := RunSuite(context.Background(), stubRunner{
		result: RunResult{
			Output:      "summary without keyword",
			ProfileName: "career-copilot",
			Steps: []engine.StepRecord{
				{
					Index: 1,
					Actions: []engine.Action{
						{Type: protocol.ActionToolCall, ToolName: "file_search"},
					},
				},
			},
			Trace: engine.RunTrace{
				StepCount:       1,
				ToolCallCount:   1,
				ToolCallsByName: map[string]int{"file_search": 1},
			},
		},
	}, suite, "base prompt")
	if err != nil {
		t.Fatalf("RunSuite() error = %v", err)
	}
	if result.FailedCount != 1 {
		t.Fatalf("unexpected summary: %+v", result)
	}
	caseResult := result.Results[0]
	if caseResult.Success {
		t.Fatalf("expected failure result: %+v", caseResult)
	}
	if len(caseResult.MissingOutput) != 1 || caseResult.MissingOutput[0] != "argv" {
		t.Fatalf("unexpected missing output: %+v", caseResult)
	}
	if len(caseResult.MissingTools) != 1 || caseResult.MissingTools[0] != "file_read" {
		t.Fatalf("unexpected missing tools: %+v", caseResult)
	}
	if caseResult.Profile != "career-copilot" {
		t.Fatalf("unexpected profile: %+v", caseResult)
	}
}
