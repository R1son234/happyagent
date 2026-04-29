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
	"happyagent/internal/report"
	"happyagent/internal/store"
)

const ProfileName = "career-copilot"
const MinAnalyzeLoopSteps = 20
const MinAnalyzeTimeoutSeconds = 180

type Application interface {
	CreateSession(profileName string) (store.SessionRecord, error)
	AppendUserTurn(ctx context.Context, req app.AppendTurnRequest) (store.RunRecord, error)
}

type AnalyzeOptions struct {
	JDPath        string
	ResumePath    string
	TargetPath    string
	RepoPath      string
	MarkdownPath  string
	JSONPath      string
	TraceJSONPath string
	TemplatePath  string
}

type Dependencies struct {
	App    Application
	Config config.Config
	Stdout io.Writer
	Stderr io.Writer
}

func RunCLI(ctx context.Context, args []string, deps Dependencies) error {
	if len(args) == 0 {
		return fmt.Errorf("career command requires a subcommand: analyze, rewrite-resume, interview-brief, or gap-plan")
	}
	switch args[0] {
	case "analyze":
		options, err := parseAnalyzeOptions(args[1:])
		if err != nil {
			return err
		}
		return Analyze(ctx, options, deps)
	case "rewrite-resume":
		return runReportTransform(args[1:], deps.Stdout, RenderResumeBullets)
	case "interview-brief":
		return runReportTransform(args[1:], deps.Stdout, RenderInterviewBrief)
	case "gap-plan":
		return runReportTransform(args[1:], deps.Stdout, RenderGapPlan)
	default:
		return fmt.Errorf("unknown career subcommand %q", args[0])
	}
}

func Analyze(ctx context.Context, options AnalyzeOptions, deps Dependencies) error {
	if deps.App == nil {
		return fmt.Errorf("career analyze requires an application")
	}
	if deps.Stdout == nil {
		deps.Stdout = io.Discard
	}
	if deps.Stderr == nil {
		deps.Stderr = io.Discard
	}
	normalized, err := normalizeAnalyzeOptions(options)
	if err != nil {
		return err
	}
	if err := validateInputs(normalized); err != nil {
		return err
	}

	session, err := deps.App.CreateSession(ProfileName)
	if err != nil {
		return fmt.Errorf("create career session: %w", err)
	}

	prompt := BuildAnalyzePrompt(normalized)
	record, err := deps.App.AppendUserTurn(ctx, app.AppendTurnRequest{
		SessionID:    session.ID,
		ProfileName:  ProfileName,
		Input:        prompt,
		SystemPrompt: deps.Config.Engine.SystemPrompt,
	})
	if err != nil {
		return fmt.Errorf("run career analysis: %w", err)
	}

	careerReport, err := ParseReportString(record.Output)
	if err != nil {
		return err
	}
	careerReport.Appendix.RunID = record.ID
	careerReport.Appendix.SessionID = record.SessionID

	markdown, err := RenderMarkdown(careerReport, normalized.TemplatePath)
	if err != nil {
		return err
	}
	if err := WriteText(normalized.MarkdownPath, markdown); err != nil {
		return err
	}
	if err := WriteReportJSON(normalized.JSONPath, careerReport); err != nil {
		return err
	}
	traceReport := report.RunReport{
		RunID:         record.ID,
		SessionID:     record.SessionID,
		Profile:       record.Profile,
		Model:         deps.Config.LLM.Model,
		Input:         record.Input,
		Output:        record.Output,
		Status:        record.Status,
		ErrorCategory: record.ErrorCategory,
		Trace:         record.Trace,
		Steps:         record.Steps,
		SystemPrompt:  record.SystemPrompt,
		Events:        record.Events,
	}
	if err := report.WriteJSON(normalized.TraceJSONPath, traceReport); err != nil {
		return err
	}

	printAnalyzeSummary(deps.Stdout, careerReport, record, normalized)
	return nil
}

func BuildAnalyzePrompt(options AnalyzeOptions) string {
	return fmt.Sprintf(`Run Career Copilot analysis for an AI Agent backend engineering job search.

Use the career-copilot profile behavior and inspect evidence before concluding.

Inputs:
- JD: %s
- Resume draft: %s
- Target statement: %s
- Repository root: %s

Required tool use:
- Read the JD, resume, and target files with file_read.
- Search the repository with file_search or search_docs for project evidence.
- Read the most relevant source and docs files before writing project evidence.

When complete, call final_answer with valid JSON only, with this exact shape:
{
  "summary": {"target_role": "...", "match_score": 0, "verdict": "..."},
  "jd_analysis": {"required_capabilities": [{"name": "...", "importance": "high|medium|low", "evidence_needed": "..."}]},
  "project_evidence": [{"claim": "...", "evidence": [{"path": "...", "reason": "..."}], "confidence": "high|medium|low"}],
  "resume_rewrite": {"bullets": [{"original": "...", "recommended": "...", "why": "..."}]},
  "interview_brief": {"project_pitch": "...", "architecture_talk_track": "...", "tradeoffs": ["..."], "questions_to_expect": ["..."]},
  "gap_plan": [{"priority": "P0|P1|P2", "item": "...", "why_it_matters": "...", "acceptance": "..."}],
  "risk_flags": [{"statement": "...", "reason": "..."}],
  "appendix": {"files_reviewed": ["..."]}
}

Constraints:
- Do not invent employment history, production usage, metrics, or business impact.
- Every project_evidence item must cite at least one repository path.
- Include at least three resume bullets, three project evidence claims, three gap items, and one risk flag.
- Keep the report actionable for editing a resume and preparing for interviews.`,
		options.JDPath,
		options.ResumePath,
		options.TargetPath,
		options.RepoPath,
	)
}

func parseAnalyzeOptions(args []string) (AnalyzeOptions, error) {
	fs := flag.NewFlagSet("career analyze", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	options := AnalyzeOptions{}
	fs.StringVar(&options.JDPath, "jd", "", "path to target job description markdown")
	fs.StringVar(&options.ResumePath, "resume", "", "path to resume draft markdown")
	fs.StringVar(&options.TargetPath, "target", "", "path to career target markdown")
	fs.StringVar(&options.RepoPath, "repo", ".", "repository root to inspect")
	fs.StringVar(&options.MarkdownPath, "out", "outputs/career-report.md", "markdown report output path")
	fs.StringVar(&options.JSONPath, "json", "outputs/career-report.json", "structured career report JSON output path")
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
		options.MarkdownPath = "outputs/career-report.md"
	}
	if strings.TrimSpace(options.JSONPath) == "" {
		options.JSONPath = "outputs/career-report.json"
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
