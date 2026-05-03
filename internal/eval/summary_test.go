package eval

import (
	"strings"
	"testing"

	"happyagent/internal/engine"
)

func TestRenderMarkdownSummary(t *testing.T) {
	result := SuiteResult{
		Suite:                     "career",
		Description:               "Career eval",
		CaseCount:                 2,
		PassedCount:               1,
		FailedCount:               1,
		SuccessRate:               0.5,
		DurationMillis:            1200,
		AverageSteps:              3,
		AverageToolCalls:          4,
		AverageExecutedTools:      3,
		AverageSuccessfulTools:    2,
		TotalTokens:               1000,
		ToolCallsByName:           map[string]int{"file_read": 2},
		ExecutedToolCallsByName:   map[string]int{"file_read": 2},
		SuccessfulToolCallsByName: map[string]int{"file_read": 1},
		ErrorCategories:           map[string]int{"model": 1},
		Results: []CaseResult{
			{
				Name:           "pass-case",
				Profile:        "career-copilot",
				Success:        true,
				Trace:          engine.RunTrace{StepCount: 2, ToolCallCount: 3},
				DurationMillis: 500,
			},
			{
				Name:           "fail-case",
				Profile:        "career-copilot",
				Success:        false,
				FailureReasons: []string{"missing_expected_output"},
				MissingOutput:  []string{"risk_flags"},
				Trace:          engine.RunTrace{StepCount: 4, ToolCallCount: 5},
				DurationMillis: 700,
			},
		},
	}

	summary := RenderMarkdownSummary(result)
	for _, expected := range []string{
		"# Eval Summary",
		"Suite: `career`",
		"Success rate: 0.50",
		"| `file_read` | 2 | 2 | 1 |",
		"| FAIL | `fail-case`",
		"Missing output: `risk_flags`",
	} {
		if !strings.Contains(summary, expected) {
			t.Fatalf("summary missing %q:\n%s", expected, summary)
		}
	}
}
