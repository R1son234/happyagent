package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"happyagent/internal/config"
	"happyagent/internal/eval"
	"happyagent/internal/report"
	"happyagent/internal/runtime"
)

const defaultSuitePath = "eval/smoke_cases.json"

func main() {
	var suitePath string
	var outputPath string
	var traceDir string

	flag.StringVar(&suitePath, "cases", defaultSuitePath, "path to eval suite json")
	flag.StringVar(&outputPath, "output", "", "optional path for the suite json report")
	flag.StringVar(&traceDir, "trace-dir", "", "optional directory for per-case trace json reports")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		exitf("load config: %v", err)
	}

	suite, err := eval.LoadSuite(suitePath)
	if err != nil {
		exitf("load eval suite: %v", err)
	}

	rt, err := runtime.NewBuilder().Build(cfg)
	if err != nil {
		exitf("build runtime: %v", err)
	}
	defer func() {
		if err := rt.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "close runtime: %v\n", err)
		}
	}()

	runner := runtimeEvalAdapter{runtime: rt}
	result, err := eval.RunSuite(context.Background(), runner, suite, cfg.Engine.SystemPrompt)
	if err != nil {
		exitf("run eval suite: %v", err)
	}

	if traceDir != "" {
		for i := range result.Results {
			reportPath := filepath.Join(traceDir, sanitizeFilename(result.Results[i].Name)+".trace.json")
			caseReport := eval.BuildCaseReport(cfg.LLM.Model, suite.Cases[i], cfg.Engine.SystemPrompt, result.Results[i])
			if err := report.WriteJSON(reportPath, caseReport); err != nil {
				exitf("write case trace: %v", err)
			}
			result.Results[i].ReportPath = reportPath
		}
	}

	if outputPath != "" {
		if err := report.WriteJSON(outputPath, result); err != nil {
			exitf("write eval report: %v", err)
		}
	}

	printSummary(result, outputPath, traceDir)
	if result.FailedCount > 0 {
		os.Exit(1)
	}
}

type runtimeEvalAdapter struct {
	runtime *runtime.Runtime
}

func (a runtimeEvalAdapter) Run(ctx context.Context, req eval.RunRequest) (eval.RunResult, error) {
	result, err := a.runtime.Run(ctx, runtime.RunRequest{
		Input:        req.Input,
		SystemPrompt: req.SystemPrompt,
		ProfileName:  req.ProfileName,
	})
	if err != nil {
		return eval.RunResult{}, err
	}
	return eval.RunResult{
		Output:      result.Output,
		Steps:       result.Steps,
		Trace:       result.Trace,
		ProfileName: result.ProfileName,
	}, nil
}

func printSummary(result eval.SuiteResult, outputPath string, traceDir string) {
	fmt.Fprintf(os.Stdout, "suite=%s passed=%d failed=%d success_rate=%.2f avg_steps=%.2f avg_tool_calls=%.2f avg_duration_ms=%.2f total_tokens=%d\n",
		result.Suite,
		result.PassedCount,
		result.FailedCount,
		result.SuccessRate,
		result.AverageSteps,
		result.AverageToolCalls,
		result.AverageDurationMs,
		result.TotalTokens,
	)
	for _, caseResult := range result.Results {
		status := "PASS"
		if !caseResult.Success {
			status = "FAIL"
		}
		fmt.Fprintf(os.Stdout, "[%s] %s steps=%d tool_calls=%d duration_ms=%d\n",
			status,
			caseResult.Name,
			caseResult.Trace.StepCount,
			caseResult.Trace.ToolCallCount,
			caseResult.DurationMillis,
		)
		if len(caseResult.FailureReasons) > 0 {
			fmt.Fprintf(os.Stdout, "  reasons=%s\n", strings.Join(caseResult.FailureReasons, ","))
		}
		if caseResult.ReportPath != "" {
			fmt.Fprintf(os.Stdout, "  trace=%s\n", caseResult.ReportPath)
		}
	}
	if outputPath != "" {
		fmt.Fprintf(os.Stdout, "suite_report=%s\n", outputPath)
	}
	if traceDir != "" {
		fmt.Fprintf(os.Stdout, "trace_dir=%s\n", traceDir)
	}
}

func sanitizeFilename(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return "case"
	}
	var builder strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		default:
			builder.WriteByte('-')
		}
	}
	name := strings.Trim(builder.String(), "-")
	if name == "" {
		return "case"
	}
	return name
}

func exitf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
