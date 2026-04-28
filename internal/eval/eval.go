package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"happyagent/internal/engine"
	"happyagent/internal/observe"
	"happyagent/internal/protocol"
	"happyagent/internal/report"
)

type Suite struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Cases       []Case `json:"cases"`
}

type Case struct {
	Name                   string   `json:"name"`
	Prompt                 string   `json:"prompt"`
	Profile                string   `json:"profile,omitempty"`
	SystemPrompt           string   `json:"system_prompt,omitempty"`
	TimeoutSeconds         int      `json:"timeout_seconds,omitempty"`
	ExpectedOutputContains []string `json:"expected_output_contains,omitempty"`
	RequiredTools          []string `json:"required_tools,omitempty"`
	MaxSteps               int      `json:"max_steps,omitempty"`
}

type Runner interface {
	Run(ctx context.Context, req RunRequest) (RunResult, error)
}

type RunRequest struct {
	Input        string
	SystemPrompt string
	ProfileName  string
}

type RunResult struct {
	Output      string
	Steps       []engine.StepRecord
	Trace       engine.RunTrace
	ProfileName string
}

type SuiteResult struct {
	Suite             string         `json:"suite"`
	Description       string         `json:"description,omitempty"`
	CaseCount         int            `json:"case_count"`
	PassedCount       int            `json:"passed_count"`
	FailedCount       int            `json:"failed_count"`
	SuccessRate       float64        `json:"success_rate"`
	DurationMillis    int64          `json:"duration_millis"`
	AverageSteps      float64        `json:"average_steps"`
	AverageToolCalls  float64        `json:"average_tool_calls"`
	AverageDurationMs float64        `json:"average_duration_millis"`
	PromptTokens      int            `json:"prompt_tokens"`
	CompletionTokens  int            `json:"completion_tokens"`
	TotalTokens       int            `json:"total_tokens"`
	ToolCallsByName   map[string]int `json:"tool_calls_by_name"`
	ErrorCategories   map[string]int `json:"error_categories"`
	Results           []CaseResult   `json:"results"`
}

type CaseResult struct {
	Name           string              `json:"name"`
	Profile        string              `json:"profile,omitempty"`
	Success        bool                `json:"success"`
	Error          string              `json:"error,omitempty"`
	ErrorCategory  string              `json:"error_category,omitempty"`
	FailureReasons []string            `json:"failure_reasons,omitempty"`
	MissingOutput  []string            `json:"missing_output,omitempty"`
	MissingTools   []string            `json:"missing_tools,omitempty"`
	Output         string              `json:"output"`
	Trace          engine.RunTrace     `json:"trace"`
	Steps          []engine.StepRecord `json:"steps"`
	DurationMillis int64               `json:"duration_millis"`
	ReportPath     string              `json:"report_path,omitempty"`
}

func LoadSuite(path string) (Suite, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Suite{}, fmt.Errorf("read eval suite %q: %w", path, err)
	}

	var suite Suite
	if err := json.Unmarshal(data, &suite); err != nil {
		return Suite{}, fmt.Errorf("parse eval suite %q: %w", path, err)
	}
	if err := validateSuite(suite); err != nil {
		return Suite{}, err
	}
	return suite, nil
}

func RunSuite(ctx context.Context, runner Runner, suite Suite, defaultSystemPrompt string) (SuiteResult, error) {
	startedAt := time.Now()
	results := make([]CaseResult, 0, len(suite.Cases))
	summary := SuiteResult{
		Suite:           suite.Name,
		Description:     suite.Description,
		CaseCount:       len(suite.Cases),
		ToolCallsByName: map[string]int{},
		ErrorCategories: map[string]int{},
	}

	var totalSteps int
	var totalToolCalls int
	var totalDuration int64

	for _, testCase := range suite.Cases {
		result := runCase(ctx, runner, testCase, defaultSystemPrompt)
		results = append(results, result)
		if result.Success {
			summary.PassedCount++
		} else {
			summary.FailedCount++
			if result.ErrorCategory != "" {
				summary.ErrorCategories[result.ErrorCategory]++
			}
		}

		totalSteps += result.Trace.StepCount
		totalToolCalls += result.Trace.ToolCallCount
		totalDuration += result.DurationMillis
		summary.PromptTokens += result.Trace.PromptTokens
		summary.CompletionTokens += result.Trace.CompletionTokens
		summary.TotalTokens += result.Trace.TotalTokens
		for toolName, count := range result.Trace.ToolCallsByName {
			summary.ToolCallsByName[toolName] += count
		}
	}

	summary.Results = results
	summary.DurationMillis = time.Since(startedAt).Milliseconds()
	if summary.CaseCount > 0 {
		summary.SuccessRate = float64(summary.PassedCount) / float64(summary.CaseCount)
		summary.AverageSteps = float64(totalSteps) / float64(summary.CaseCount)
		summary.AverageToolCalls = float64(totalToolCalls) / float64(summary.CaseCount)
		summary.AverageDurationMs = float64(totalDuration) / float64(summary.CaseCount)
	}

	return summary, nil
}

func BuildCaseReport(model string, testCase Case, defaultSystemPrompt string, result CaseResult) report.RunReport {
	systemPrompt := defaultSystemPrompt
	if strings.TrimSpace(testCase.SystemPrompt) != "" {
		systemPrompt = testCase.SystemPrompt
	}
	return report.RunReport{
		Profile:       result.Profile,
		Model:         model,
		Input:         testCase.Prompt,
		Output:        result.Output,
		Status:        map[bool]string{true: "completed", false: "failed"}[result.Success],
		ErrorCategory: result.ErrorCategory,
		Trace:         result.Trace,
		Steps:         result.Steps,
		SystemPrompt:  systemPrompt,
	}
}

func validateSuite(suite Suite) error {
	if strings.TrimSpace(suite.Name) == "" {
		return fmt.Errorf("eval suite name must not be empty")
	}
	if len(suite.Cases) == 0 {
		return fmt.Errorf("eval suite %q must contain at least one case", suite.Name)
	}
	for i, testCase := range suite.Cases {
		if strings.TrimSpace(testCase.Name) == "" {
			return fmt.Errorf("eval suite %q case[%d] name must not be empty", suite.Name, i)
		}
		if strings.TrimSpace(testCase.Prompt) == "" {
			return fmt.Errorf("eval suite %q case %q prompt must not be empty", suite.Name, testCase.Name)
		}
		if testCase.TimeoutSeconds < 0 {
			return fmt.Errorf("eval suite %q case %q timeout_seconds must not be negative", suite.Name, testCase.Name)
		}
	}
	return nil
}

func runCase(ctx context.Context, runner Runner, testCase Case, defaultSystemPrompt string) CaseResult {
	startedAt := time.Now()
	caseCtx := ctx
	cancel := func() {}
	if testCase.TimeoutSeconds > 0 {
		caseCtx, cancel = context.WithTimeout(ctx, time.Duration(testCase.TimeoutSeconds)*time.Second)
	}
	defer cancel()

	systemPrompt := defaultSystemPrompt
	if strings.TrimSpace(testCase.SystemPrompt) != "" {
		systemPrompt = testCase.SystemPrompt
	}

	runResult, err := runner.Run(caseCtx, RunRequest{
		Input:        testCase.Prompt,
		SystemPrompt: systemPrompt,
		ProfileName:  testCase.Profile,
	})
	if err != nil {
		errorCategory := observe.ClassifyError(err)
		return CaseResult{
			Name:           testCase.Name,
			Profile:        testCase.Profile,
			Success:        false,
			Error:          err.Error(),
			ErrorCategory:  errorCategory,
			FailureReasons: []string{"run_error"},
			DurationMillis: time.Since(startedAt).Milliseconds(),
		}
	}

	caseResult := CaseResult{
		Name:           testCase.Name,
		Profile:        runResult.ProfileName,
		Success:        true,
		Output:         runResult.Output,
		Trace:          runResult.Trace,
		Steps:          runResult.Steps,
		DurationMillis: time.Since(startedAt).Milliseconds(),
	}

	for _, expected := range testCase.ExpectedOutputContains {
		if !strings.Contains(runResult.Output, expected) {
			caseResult.MissingOutput = append(caseResult.MissingOutput, expected)
		}
	}

	toolUsage := collectToolUsage(runResult.Steps)
	for _, required := range testCase.RequiredTools {
		if toolUsage[required] == 0 {
			caseResult.MissingTools = append(caseResult.MissingTools, required)
		}
	}

	if testCase.MaxSteps > 0 && runResult.Trace.StepCount > testCase.MaxSteps {
		caseResult.FailureReasons = append(caseResult.FailureReasons, fmt.Sprintf("step_count_exceeded:%d>%d", runResult.Trace.StepCount, testCase.MaxSteps))
	}
	if len(caseResult.MissingOutput) > 0 {
		caseResult.FailureReasons = append(caseResult.FailureReasons, "missing_expected_output")
	}
	if len(caseResult.MissingTools) > 0 {
		caseResult.FailureReasons = append(caseResult.FailureReasons, "missing_required_tools")
	}
	if len(caseResult.FailureReasons) > 0 {
		caseResult.Success = false
	}

	return caseResult
}

func collectToolUsage(steps []engine.StepRecord) map[string]int {
	usage := make(map[string]int)
	for _, step := range steps {
		for _, action := range step.Actions {
			if action.Type != protocol.ActionToolCall {
				continue
			}
			usage[action.ToolName]++
		}
	}
	return usage
}
