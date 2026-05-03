package eval

import (
	"fmt"
	"sort"
	"strings"
)

func RenderMarkdownSummary(result SuiteResult) string {
	var builder strings.Builder
	builder.WriteString("# Eval Summary\n\n")
	builder.WriteString(fmt.Sprintf("- Suite: `%s`\n", result.Suite))
	if strings.TrimSpace(result.Description) != "" {
		builder.WriteString(fmt.Sprintf("- Description: %s\n", result.Description))
	}
	builder.WriteString(fmt.Sprintf("- Cases: %d\n", result.CaseCount))
	builder.WriteString(fmt.Sprintf("- Passed: %d\n", result.PassedCount))
	builder.WriteString(fmt.Sprintf("- Failed: %d\n", result.FailedCount))
	builder.WriteString(fmt.Sprintf("- Success rate: %.2f\n", result.SuccessRate))
	builder.WriteString(fmt.Sprintf("- Duration: %d ms\n", result.DurationMillis))
	builder.WriteString(fmt.Sprintf("- Average steps: %.2f\n", result.AverageSteps))
	builder.WriteString(fmt.Sprintf("- Average tool calls: %.2f\n", result.AverageToolCalls))
	builder.WriteString(fmt.Sprintf("- Average executed tools: %.2f\n", result.AverageExecutedTools))
	builder.WriteString(fmt.Sprintf("- Average successful tools: %.2f\n", result.AverageSuccessfulTools))
	builder.WriteString(fmt.Sprintf("- Total tokens: %d\n", result.TotalTokens))

	builder.WriteString("\n## Tool Calls\n\n")
	writeCountTable(&builder, []string{"Tool", "Attempted", "Executed", "Succeeded"}, mergeToolRows(result))

	builder.WriteString("\n## Cases\n\n")
	builder.WriteString("| Status | Case | Profile | Steps | Tool Calls | Duration |\n")
	builder.WriteString("| --- | --- | --- | ---: | ---: | ---: |\n")
	for _, caseResult := range result.Results {
		status := "PASS"
		if !caseResult.Success {
			status = "FAIL"
		}
		builder.WriteString(fmt.Sprintf("| %s | `%s` | `%s` | %d | %d | %d ms |\n",
			status,
			escapeTable(caseResult.Name),
			escapeTable(caseResult.Profile),
			caseResult.Trace.StepCount,
			caseResult.Trace.ToolCallCount,
			caseResult.DurationMillis,
		))
	}

	if len(result.ErrorCategories) > 0 {
		builder.WriteString("\n## Error Categories\n\n")
		for _, name := range sortedKeys(result.ErrorCategories) {
			builder.WriteString(fmt.Sprintf("- `%s`: %d\n", name, result.ErrorCategories[name]))
		}
	}

	var failed []CaseResult
	for _, caseResult := range result.Results {
		if !caseResult.Success {
			failed = append(failed, caseResult)
		}
	}
	if len(failed) > 0 {
		builder.WriteString("\n## Failures\n\n")
		for _, caseResult := range failed {
			builder.WriteString(fmt.Sprintf("- `%s`: %s\n", caseResult.Name, strings.Join(caseResult.FailureReasons, ", ")))
			if len(caseResult.MissingOutput) > 0 {
				builder.WriteString(fmt.Sprintf("  Missing output: `%s`\n", strings.Join(caseResult.MissingOutput, "`, `")))
			}
			if len(caseResult.MissingTools) > 0 {
				builder.WriteString(fmt.Sprintf("  Missing tools: `%s`\n", strings.Join(caseResult.MissingTools, "`, `")))
			}
			if strings.TrimSpace(caseResult.Error) != "" {
				builder.WriteString(fmt.Sprintf("  Error: %s\n", caseResult.Error))
			}
		}
	}

	return builder.String()
}

func mergeToolRows(result SuiteResult) []toolSummaryRow {
	names := map[string]struct{}{}
	for name := range result.ToolCallsByName {
		names[name] = struct{}{}
	}
	for name := range result.ExecutedToolCallsByName {
		names[name] = struct{}{}
	}
	for name := range result.SuccessfulToolCallsByName {
		names[name] = struct{}{}
	}
	sorted := make([]string, 0, len(names))
	for name := range names {
		sorted = append(sorted, name)
	}
	sort.Strings(sorted)

	rows := make([]toolSummaryRow, 0, len(sorted))
	for _, name := range sorted {
		rows = append(rows, toolSummaryRow{
			Name:      name,
			Attempted: result.ToolCallsByName[name],
			Executed:  result.ExecutedToolCallsByName[name],
			Succeeded: result.SuccessfulToolCallsByName[name],
		})
	}
	return rows
}

func writeCountTable(builder *strings.Builder, headers []string, rows []toolSummaryRow) {
	builder.WriteString("| ")
	builder.WriteString(strings.Join(headers, " | "))
	builder.WriteString(" |\n")
	builder.WriteString("| --- | ---: | ---: | ---: |\n")
	for _, row := range rows {
		builder.WriteString(fmt.Sprintf("| `%s` | %d | %d | %d |\n",
			escapeTable(row.Name),
			row.Attempted,
			row.Executed,
			row.Succeeded,
		))
	}
}

type toolSummaryRow struct {
	Name      string
	Attempted int
	Executed  int
	Succeeded int
}

func sortedKeys(values map[string]int) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func escapeTable(value string) string {
	return strings.ReplaceAll(value, "|", "\\|")
}
