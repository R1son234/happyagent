package career

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"happyagent/internal/app"
	"happyagent/internal/config"
	"happyagent/internal/report"
	"happyagent/internal/store"
	"happyagent/internal/terminal"
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
	App           Application
	Config        config.Config
	Stdin         io.Reader
	Stdout        io.Writer
	Stderr        io.Writer
	WorkspaceRoot string
}

func RunCLI(ctx context.Context, args []string, deps Dependencies) error {
	if len(args) == 0 {
		return RunInteractive(deps)
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

func RunInteractive(deps Dependencies) error {
	if deps.App == nil {
		return fmt.Errorf("career interactive requires an application")
	}
	if deps.Stdin == nil {
		deps.Stdin = os.Stdin
	}
	if deps.Stdout == nil {
		deps.Stdout = os.Stdout
	}
	if deps.Stderr == nil {
		deps.Stderr = os.Stderr
	}
	workspaceRoot := deps.WorkspaceRoot
	if strings.TrimSpace(workspaceRoot) == "" {
		workspaceRoot = DefaultWorkspaceRoot
	}
	workspace, err := OpenWorkspace(workspaceRoot, time.Now())
	if err != nil {
		return err
	}
	session, err := deps.App.CreateSession(ProfileName)
	if err != nil {
		return fmt.Errorf("create career session: %w", err)
	}

	printCareerWelcome(deps.Stdout, workspace, session.ID)
	lineReader, err := terminal.NewLineReader(deps.Stdin, deps.Stdout)
	if err != nil {
		return err
	}
	defer lineReader.Close()
	for {
		rawInput, err := lineReader.ReadLine("career> ")
		if err != nil {
			if err == io.EOF {
				fmt.Fprintln(deps.Stdout)
				return nil
			}
			fmt.Fprintln(deps.Stdout)
			return err
		}
		input := strings.TrimSpace(rawInput)
		if input == "" {
			continue
		}
		switch input {
		case "/exit", "/quit":
			return nil
		case "/help":
			printCareerHelp(deps.Stdout)
			continue
		case "/status":
			if err := printWorkspaceStatus(deps.Stdout, workspace); err != nil {
				return err
			}
			continue
		case "/library":
			result, err := workspace.GenerateReviewLibrary(time.Now())
			if err != nil {
				return err
			}
			fmt.Fprintf(deps.Stdout, "assistant> 已刷新可复习资料库：面试资料库首页.md")
			if len(result.Paths) > 0 {
				fmt.Fprintf(deps.Stdout, "；更新 %d 个资料文件", len(result.Paths))
			}
			fmt.Fprintln(deps.Stdout)
			continue
		}
		if isCommandHelpQuestion(input) {
			printCareerHelp(deps.Stdout)
			continue
		}
		if strings.HasPrefix(input, "/export") {
			if err := handleExportCommand(deps.Stdout, workspace, input); err != nil {
				return err
			}
			continue
		}
		if strings.HasPrefix(input, "/add") {
			if err := handleAddCommand(deps.Stdout, lineReader, workspace, input); err != nil {
				return err
			}
			continue
		}
		if err := handleNaturalLanguageInput(deps, workspace, session.ID, input); err != nil {
			return err
		}
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
		SessionID:     session.ID,
		ProfileName:   ProfileName,
		Input:         prompt,
		SystemPrompt:  deps.Config.Engine.SystemPrompt,
		ApprovedTools: deps.Config.Tools.ApprovedTools,
	})
	if err != nil {
		return fmt.Errorf("run career analysis: %w", err)
	}

	careerReport, record, err := parseOrRepairReport(ctx, deps, session.ID, record)
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
