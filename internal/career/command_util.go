package career

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"happyagent/internal/app"
	"happyagent/internal/config"
	"happyagent/internal/observe"
	"happyagent/internal/runlog"
	"happyagent/internal/store"
	"happyagent/internal/terminal"
	"happyagent/internal/tools"
)

func runCareerTurn(deps Dependencies, sessionID string, prompt string, classification InputClassification) (store.RunRecord, error) {
	timeout := time.Duration(deps.Config.Engine.RunTimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	runlog.Section("Run Input", prompt)
	runlog.Linef("Model: `%s`", deps.Config.LLM.Model)
	runlog.Linef("Timeout: `%ds`", deps.Config.Engine.RunTimeoutSeconds)
	runlog.Linef("Profile: `%s`", ProfileName)
	runlog.Linef("Session: `%s`", sessionID)
	runlog.Linef("")
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	spinner := terminal.NewSpinner(deps.Stderr)
	spinner.Start("Thinking...")
	defer spinner.Stop()

	record, err := deps.App.AppendUserTurn(ctx, app.AppendTurnRequest{
		SessionID:     sessionID,
		ProfileName:   ProfileName,
		Input:         prompt,
		SystemPrompt:  deps.Config.Engine.SystemPrompt,
		ApprovedTools: deps.Config.Tools.ApprovedTools,
		Events:        []observe.Event{classificationEvent(classification)},
		OnStepStart: func(stepIndex int) {
			spinner.UpdateThinkingMessage(fmt.Sprintf("Thinking... (step %d)", stepIndex))
		},
		OnToolCallStart: func(toolName string) {
			spinner.UpdateMessage(fmt.Sprintf("Executing %s...", toolName))
		},
		OnToolCallEnd: func(toolName string, succeeded bool) {
			if !succeeded {
				spinner.UpdateMessage(fmt.Sprintf("Tool %s failed, thinking...", toolName))
			}
		},
		OnTodosUpdated: func(todos []tools.TodoItem) {
			items := make([]terminal.ChecklistItem, len(todos))
			for i, todo := range todos {
				items[i] = terminal.ChecklistItem{Content: todo.Content, Status: todo.Status}
			}
			spinner.UpdateChecklist(items)
		},
	})
	if err != nil {
		return record, err
	}
	runlog.Section("Final Output", record.Output)
	return record, nil
}

func parseOrRepairReport(ctx context.Context, deps Dependencies, sessionID string, record store.RunRecord) (Report, store.RunRecord, error) {
	careerReport, err := ParseReportString(record.Output)
	if err == nil {
		return careerReport, record, nil
	}
	repairPrompt := BuildReportRepairPrompt(record.Output, err)
	repairedRecord, repairErr := deps.App.AppendUserTurn(ctx, app.AppendTurnRequest{
		SessionID:     sessionID,
		ProfileName:   ProfileName,
		Input:         repairPrompt,
		SystemPrompt:  deps.Config.Engine.SystemPrompt,
		ApprovedTools: deps.Config.Tools.ApprovedTools,
	})
	if repairErr != nil {
		return Report{}, record, fmt.Errorf("parse career report json: %w; repair run failed: %v", err, repairErr)
	}
	careerReport, repairParseErr := ParseReportString(repairedRecord.Output)
	if repairParseErr != nil {
		return Report{}, repairedRecord, fmt.Errorf("parse career report json: %w; repair parse failed: %v", err, repairParseErr)
	}
	return careerReport, repairedRecord, nil
}

func saveMaterial(workspace *Workspace, itemType string, content string) (WorkspaceItem, error) {
	if strings.ToLower(strings.TrimSpace(itemType)) == WorkspaceTypeExperiences {
		result, err := workspace.ArchivePublicInterviewExperience(content, time.Now())
		if err != nil {
			return WorkspaceItem{}, err
		}
		return result.ExperienceItem, nil
	}
	guide, err := workspace.LoadGuide()
	if err != nil {
		return WorkspaceItem{}, err
	}
	classification := ClassifyInputWithGuide(content, guide)
	classification.Type = itemType
	classification.RulePath = classificationRulePath(guide, itemType)
	result, err := workspace.AddGuidedMaterial(GuidedMaterialInput{
		ItemType:       itemType,
		Classification: classification,
		Content:        content,
		SourceLabel:    "natural_language_input",
		Now:            time.Now(),
	})
	if err != nil {
		return WorkspaceItem{}, err
	}
	return result.Item, nil
}

func emptyIfBlank(value string) string {
	if strings.TrimSpace(value) == "" {
		return "(none)"
	}
	return value
}

func classificationEvent(classification InputClassification) observe.Event {
	return observe.Event{
		Time:    time.Now(),
		Type:    "career_input_classified",
		Message: "career input classified",
		Data: map[string]string{
			"type":        classification.Type,
			"confidence":  fmt.Sprintf("%.2f", classification.Confidence),
			"should_save": fmt.Sprintf("%t", classification.ShouldSave),
			"signals":     strings.Join(classification.Signals, ","),
		},
	}
}

func collectedInputPaths(workspaceRoot string, meta WorkspaceMetadata, autoArchived []WorkspaceItem) []string {
	seen := map[string]bool{}
	var paths []string
	for _, candidate := range []string{meta.CurrentResume, meta.ActiveJD, meta.ActiveProject} {
		if strings.TrimSpace(candidate) == "" || seen[candidate] {
			continue
		}
		paths = append(paths, promptWorkspacePath(workspaceRoot, candidate))
		seen[candidate] = true
	}
	for _, item := range autoArchived {
		if strings.TrimSpace(item.Path) == "" || seen[item.Path] {
			continue
		}
		paths = append(paths, promptWorkspacePath(workspaceRoot, item.Path))
		seen[item.Path] = true
	}
	return paths
}

func parseAnalyzeOptions(args []string) (AnalyzeOptions, error) {
	fs := flag.NewFlagSet("career analyze", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	options := AnalyzeOptions{}
	fs.StringVar(&options.JDPath, "jd", "", "path to target job description markdown")
	fs.StringVar(&options.ResumePath, "resume", "", "path to resume draft markdown")
	fs.StringVar(&options.TargetPath, "target", "", "path to career target markdown")
	fs.StringVar(&options.RepoPath, "repo", ".", "repository root to inspect")
	fs.StringVar(&options.MarkdownPath, "out", "", "markdown report output path")
	fs.StringVar(&options.JSONPath, "json", "", "structured career report JSON output path")
	fs.StringVar(&options.TraceJSONPath, "trace-json", "logs/career/latest-trace.json", "runtime trace JSON output path")
	fs.StringVar(&options.TemplatePath, "template", DefaultReportTemplatePath, "markdown template path")
	if err := fs.Parse(args); err != nil {
		return AnalyzeOptions{}, err
	}
	return options, nil
}

func normalizeAnalyzeOptions(options AnalyzeOptions) (AnalyzeOptions, error) {
	if strings.TrimSpace(options.RepoPath) == "" {
		options.RepoPath = "."
	}
	repo, err := filepath.Abs(options.RepoPath)
	if err != nil {
		return AnalyzeOptions{}, fmt.Errorf("resolve repo path: %w", err)
	}
	options.RepoPath = repo
	if strings.TrimSpace(options.MarkdownPath) == "" {
		options.MarkdownPath = filepath.Join(DefaultWorkspaceRoot, WorkspaceDirOutputs, "latest-report.md")
	}
	if strings.TrimSpace(options.JSONPath) == "" {
		options.JSONPath = filepath.Join(DefaultWorkspaceRoot, WorkspaceDirOutputs, "latest-report.json")
	}
	if strings.TrimSpace(options.TraceJSONPath) == "" {
		options.TraceJSONPath = "logs/career/latest-trace.json"
	}
	if strings.TrimSpace(options.TemplatePath) == "" {
		options.TemplatePath = DefaultReportTemplatePath
	}
	return options, nil
}

func validateInputs(options AnalyzeOptions) error {
	required := map[string]string{
		"--jd":     options.JDPath,
		"--resume": options.ResumePath,
		"--target": options.TargetPath,
	}
	for name, path := range required {
		if strings.TrimSpace(path) == "" {
			return fmt.Errorf("%s is required", name)
		}
		if err := ensureReadableFile(path); err != nil {
			return fmt.Errorf("%s %q: %w", name, path, err)
		}
	}
	info, err := os.Stat(options.RepoPath)
	if err != nil {
		return fmt.Errorf("--repo %q: %w", options.RepoPath, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("--repo %q is not a directory", options.RepoPath)
	}
	if err := ensureReadableFile(options.TemplatePath); err != nil {
		return fmt.Errorf("--template %q: %w", options.TemplatePath, err)
	}
	return nil
}

func ensureReadableFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("is a directory")
	}
	return nil
}

func printAnalyzeSummary(output io.Writer, report Report, record store.RunRecord, options AnalyzeOptions) {
	fmt.Fprintln(output, "Career Copilot Report")
	fmt.Fprintln(output)
	fmt.Fprintf(output, "Profile: %s\n", ProfileName)
	fmt.Fprintf(output, "Run: %s\n", record.ID)
	fmt.Fprintf(output, "Session: %s\n", record.SessionID)
	fmt.Fprintln(output)
	fmt.Fprintf(output, "Match: %d/100\n", report.Summary.MatchScore)
	fmt.Fprintln(output, "Strong signals:")
	for _, claim := range TopEvidenceClaims(report, 3) {
		fmt.Fprintf(output, "  - %s\n", claim)
	}
	fmt.Fprintln(output)
	fmt.Fprintln(output, "Top gaps:")
	for _, gap := range TopGapItems(report, 3) {
		fmt.Fprintf(output, "  - %s\n", gap)
	}
	fmt.Fprintln(output)
	fmt.Fprintf(output, "Report: %s\n", options.MarkdownPath)
	fmt.Fprintf(output, "JSON: %s\n", options.JSONPath)
	fmt.Fprintf(output, "Trace: %s\n", options.TraceJSONPath)
}

func runReportTransform(args []string, stdout io.Writer, render func(Report) string) error {
	fs := flag.NewFlagSet("career report transform", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var reportPath string
	var outPath string
	fs.StringVar(&reportPath, "report", "", "career report JSON path")
	fs.StringVar(&outPath, "out", "", "markdown output path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(reportPath) == "" {
		return fmt.Errorf("--report is required")
	}
	careerReport, err := ReadReport(reportPath)
	if err != nil {
		return err
	}
	content := render(careerReport)
	if strings.TrimSpace(outPath) == "" {
		if stdout == nil {
			stdout = io.Discard
		}
		fmt.Fprint(stdout, content)
		return nil
	}
	if err := WriteText(outPath, content); err != nil {
		return err
	}
	if stdout != nil {
		fmt.Fprintf(stdout, "Wrote: %s\n", outPath)
	}
	return nil
}

func ContextWithConfiguredTimeout(parent context.Context, cfg config.Config) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, time.Duration(cfg.Engine.RunTimeoutSeconds)*time.Second)
}
