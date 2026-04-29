package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"happyagent/internal/app"
	"happyagent/internal/career"
	"happyagent/internal/config"
	"happyagent/internal/observe"
	"happyagent/internal/report"
	"happyagent/internal/runlog"
	"happyagent/internal/runtime"
	"happyagent/internal/store"
)

const (
	readyMessageFormat  = "happyagent is ready. model=%s max_steps=%d\n"
	startMessageFormat  = "starting run: model=%s timeout=%ds config=%s\n"
	modelAPICallMessage = "calling model API..."
)

type sessionApplication interface {
	CreateSession(profileName string) (store.SessionRecord, error)
	AppendUserTurn(ctx context.Context, req app.AppendTurnRequest) (store.RunRecord, error)
	GetSession(id string) (store.SessionRecord, error)
	GetRun(id string) (store.RunRecord, error)
	ReplayRun(id string) (store.RunRecord, error)
	HistoricalMetrics() (observe.Metrics, error)
}

func main() {
	var traceJSONPath string
	var profileName string
	var sessionMode bool
	var interactiveMode bool
	var sessionID string
	var showSessionID string
	var showRunID string
	var replayRunID string
	var printMetrics bool
	var approvedToolsCSV string

	flag.StringVar(&traceJSONPath, "trace-json", "", "optional path for a structured trace json report")
	flag.StringVar(&profileName, "profile", "general-assistant", "profile name loaded from profiles/<name>/profile.json")
	flag.BoolVar(&sessionMode, "session", false, "create or continue a persistent session")
	flag.BoolVar(&interactiveMode, "interactive", false, "start an interactive multi-turn session")
	flag.BoolVar(&interactiveMode, "i", false, "shorthand for --interactive")
	flag.StringVar(&sessionID, "session-id", "", "existing session id to continue")
	flag.StringVar(&showSessionID, "show-session", "", "print a stored session record as JSON")
	flag.StringVar(&showRunID, "show-run", "", "print a stored run record as JSON")
	flag.StringVar(&replayRunID, "replay-run", "", "print a stored run record and trace as JSON")
	flag.BoolVar(&printMetrics, "metrics", false, "print persisted metrics in JSON and Prometheus text")
	flag.StringVar(&approvedToolsCSV, "approve-tools", "", "comma-separated dangerous tools to allow for this run")
	flag.Parse()

	logSession := initRunLog()
	if logSession != nil {
		defer logSession.Close()
		logSession.Enable()
		defer runlog.Disable()
	}

	cfg, err := config.Load()
	if err != nil {
		exitf("load config: %v", err)
	}
	if isCareerCommand(os.Args[1:]) {
		cfg.Tools.RootDir = careerRepoArg(os.Args[1:], cfg.Tools.RootDir)
		if cfg.Engine.LoopMaxSteps < career.MinAnalyzeLoopSteps {
			cfg.Engine.LoopMaxSteps = career.MinAnalyzeLoopSteps
		}
		if cfg.Engine.RunTimeoutSeconds < career.MinAnalyzeTimeoutSeconds {
			cfg.Engine.RunTimeoutSeconds = career.MinAnalyzeTimeoutSeconds
		}
		if cfg.LLM.TimeoutSeconds < career.MinAnalyzeTimeoutSeconds {
			cfg.LLM.TimeoutSeconds = career.MinAnalyzeTimeoutSeconds
		}
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

	application, err := buildApplication(rt)
	if err != nil {
		exitf("build app: %v", err)
	}
	if isCareerCommand(os.Args[1:]) {
		ctx, cancel := career.ContextWithConfiguredTimeout(context.Background(), cfg)
		defer cancel()
		if err := career.RunCLI(ctx, os.Args[2:], career.Dependencies{
			App:    application,
			Config: cfg,
			Stdout: os.Stdout,
			Stderr: os.Stderr,
		}); err != nil {
			exitf("career: %v", err)
		}
		return
	}

	if showSessionID != "" {
		record, err := application.GetSession(showSessionID)
		if err != nil {
			exitf("get session: %v", err)
		}
		printJSON(record)
		return
	}
	if showRunID != "" {
		record, err := application.GetRun(showRunID)
		if err != nil {
			exitf("get run: %v", err)
		}
		printJSON(record)
		return
	}
	if replayRunID != "" {
		record, err := application.ReplayRun(replayRunID)
		if err != nil {
			exitf("replay run: %v", err)
		}
		printJSON(record)
		return
	}
	if printMetrics {
		metrics, err := application.HistoricalMetrics()
		if err != nil {
			exitf("load metrics: %v", err)
		}
		printJSON(metrics)
		fmt.Fprintln(os.Stdout, metrics.PrometheusText())
		return
	}

	prompt := strings.TrimSpace(strings.Join(flag.Args(), " "))
	if interactiveMode {
		if traceJSONPath != "" {
			exitf("trace-json is not supported in interactive mode")
		}
		resolvedSessionID, createdSession, err := resolveSession(application, sessionID, profileName, true)
		if err != nil {
			exitf("resolve session: %v", err)
		}
		if err := runInteractiveSession(application, cfg, os.Stdin, os.Stdout, os.Stderr, resolvedSessionID, profileName, parseCSV(approvedToolsCSV), prompt, createdSession, logSession); err != nil {
			exitf("interactive session: %v", err)
		}
		return
	}

	if prompt == "" {
		fmt.Fprintf(os.Stdout, readyMessageFormat, cfg.LLM.Model, cfg.Engine.LoopMaxSteps)
		if logSession != nil {
			fmt.Fprintf(os.Stdout, "run log: %s\n", logSession.Path())
		}
		return
	}

	resolvedSessionID, createdSession, err := resolveSession(application, sessionID, profileName, sessionMode)
	if err != nil {
		exitf("resolve session: %v", err)
	}

	fmt.Fprintf(os.Stderr, startMessageFormat, cfg.LLM.Model, cfg.Engine.RunTimeoutSeconds, config.ConfigPath())
	if logSession != nil {
		fmt.Fprintf(os.Stderr, "run log: %s\n", logSession.Path())
	}
	runlog.Section("Run Input", prompt)
	runlog.Linef("Model: `%s`", cfg.LLM.Model)
	runlog.Linef("Timeout: `%ds`", cfg.Engine.RunTimeoutSeconds)
	runlog.Linef("Profile: `%s`", profileName)
	runlog.Linef("Session: `%s`", resolvedSessionID)
	runlog.Linef("")
	fmt.Fprintln(os.Stderr, modelAPICallMessage)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.Engine.RunTimeoutSeconds)*time.Second)
	defer cancel()

	record, err := application.AppendUserTurn(ctx, app.AppendTurnRequest{
		SessionID:     resolvedSessionID,
		ProfileName:   profileName,
		Input:         prompt,
		SystemPrompt:  cfg.Engine.SystemPrompt,
		ApprovedTools: parseCSV(approvedToolsCSV),
	})
	if err != nil {
		if record.ID != "" {
			fmt.Fprintf(os.Stderr, "run_id=%s session_id=%s\n", record.ID, record.SessionID)
		}
		exitf("run agent: %v", err)
	}

	if createdSession {
		fmt.Fprintf(os.Stderr, "created_session=%s\n", resolvedSessionID)
	}
	fmt.Fprintf(os.Stderr, "run_id=%s session_id=%s\n", record.ID, record.SessionID)
	runlog.Section("Final Output", record.Output)

	if traceJSONPath != "" {
		traceReport := report.RunReport{
			RunID:         record.ID,
			SessionID:     record.SessionID,
			Profile:       record.Profile,
			Model:         cfg.LLM.Model,
			Input:         record.Input,
			Output:        record.Output,
			Status:        record.Status,
			ErrorCategory: record.ErrorCategory,
			Trace:         record.Trace,
			Steps:         record.Steps,
			SystemPrompt:  record.SystemPrompt,
			Events:        record.Events,
		}
		if err := report.WriteJSON(traceJSONPath, traceReport); err != nil {
			exitf("write trace json: %v", err)
		}
		fmt.Fprintf(os.Stderr, "trace json: %s\n", traceJSONPath)
	}
	fmt.Fprintln(os.Stdout, record.Output)
}

func buildApplication(rt *runtime.Runtime) (*app.Application, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	dataStore, err := store.New(filepath.Join(cwd, ".happyagent", "store"))
	if err != nil {
		return nil, err
	}
	return app.New(rt, dataStore, observe.NewMetrics())
}

func isCareerCommand(args []string) bool {
	return len(args) > 0 && args[0] == "career"
}

func careerRepoArg(args []string, fallback string) string {
	if len(args) < 2 || args[1] != "analyze" {
		return fallback
	}
	for i := 2; i < len(args); i++ {
		if args[i] == "--repo" && i+1 < len(args) {
			return args[i+1]
		}
		if strings.HasPrefix(args[i], "--repo=") {
			return strings.TrimPrefix(args[i], "--repo=")
		}
	}
	return fallback
}

func resolveSession(application sessionApplication, sessionID string, profileName string, sessionMode bool) (string, bool, error) {
	if strings.TrimSpace(sessionID) != "" {
		return sessionID, false, nil
	}
	if !sessionMode {
		session, err := application.CreateSession(profileName)
		if err != nil {
			return "", false, err
		}
		return session.ID, true, nil
	}
	session, err := application.CreateSession(profileName)
	if err != nil {
		return "", false, err
	}
	return session.ID, true, nil
}

func runInteractiveSession(application sessionApplication, cfg config.Config, input io.Reader, output io.Writer, errorOutput io.Writer, sessionID string, profileName string, approvedTools []string, initialPrompt string, createdSession bool, logSession *runlog.Session) error {
	if createdSession {
		fmt.Fprintf(errorOutput, "created_session=%s\n", sessionID)
	}
	fmt.Fprintf(output, "interactive session started. profile=%s session_id=%s\n", profileName, sessionID)
	fmt.Fprintln(output, "type /exit to quit")
	if logSession != nil {
		fmt.Fprintf(output, "run log: %s\n", logSession.Path())
	}

	scanner := bufio.NewScanner(input)
	pendingPrompt := strings.TrimSpace(initialPrompt)

	for {
		prompt := pendingPrompt
		pendingPrompt = ""
		if prompt == "" {
			fmt.Fprint(output, "you> ")
			if !scanner.Scan() {
				if err := scanner.Err(); err != nil {
					return err
				}
				fmt.Fprintln(output)
				return nil
			}
			prompt = strings.TrimSpace(scanner.Text())
		}
		if prompt == "" {
			continue
		}
		switch prompt {
		case "/exit", "/quit":
			return nil
		}

		record, err := runSingleTurn(application, cfg, sessionID, profileName, approvedTools, prompt, errorOutput, logSession)
		if err != nil {
			if record.ID != "" {
				fmt.Fprintf(errorOutput, "run_id=%s session_id=%s\n", record.ID, record.SessionID)
			}
			return fmt.Errorf("run agent: %w", err)
		}
		fmt.Fprintf(errorOutput, "run_id=%s session_id=%s\n", record.ID, record.SessionID)
		fmt.Fprintf(output, "assistant> %s\n", record.Output)
	}
}

func runSingleTurn(application sessionApplication, cfg config.Config, sessionID string, profileName string, approvedTools []string, prompt string, errorOutput io.Writer, logSession *runlog.Session) (store.RunRecord, error) {
	fmt.Fprintf(errorOutput, startMessageFormat, cfg.LLM.Model, cfg.Engine.RunTimeoutSeconds, config.ConfigPath())
	if logSession != nil {
		fmt.Fprintf(errorOutput, "run log: %s\n", logSession.Path())
	}
	runlog.Section("Run Input", prompt)
	runlog.Linef("Model: `%s`", cfg.LLM.Model)
	runlog.Linef("Timeout: `%ds`", cfg.Engine.RunTimeoutSeconds)
	runlog.Linef("Profile: `%s`", profileName)
	runlog.Linef("Session: `%s`", sessionID)
	runlog.Linef("")
	fmt.Fprintln(errorOutput, modelAPICallMessage)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.Engine.RunTimeoutSeconds)*time.Second)
	defer cancel()

	record, err := application.AppendUserTurn(ctx, app.AppendTurnRequest{
		SessionID:     sessionID,
		ProfileName:   profileName,
		Input:         prompt,
		SystemPrompt:  cfg.Engine.SystemPrompt,
		ApprovedTools: approvedTools,
	})
	if err != nil {
		return record, err
	}
	runlog.Section("Final Output", record.Output)
	return record, nil
}

func parseCSV(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func printJSON(value any) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		exitf("marshal json: %v", err)
	}
	fmt.Fprintln(os.Stdout, string(data))
}

func exitf(format string, args ...any) {
	runlog.Section("Error", fmt.Sprintf(format, args...))
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

func initRunLog() *runlog.Session {
	cwd, err := os.Getwd()
	if err != nil {
		return nil
	}

	session, err := runlog.NewSession(cwd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "init run log: %v\n", err)
		return nil
	}

	if abs, err := filepath.Abs(session.Path()); err == nil {
		runlog.Section("Log File", abs)
	}
	return session
}
