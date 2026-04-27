package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"happyagent/internal/config"
	"happyagent/internal/report"
	"happyagent/internal/runlog"
	"happyagent/internal/runtime"
)

const (
	readyMessageFormat  = "happyagent is ready. model=%s max_steps=%d\n"
	startMessageFormat  = "starting run: model=%s timeout=%ds config=%s\n"
	modelAPICallMessage = "calling model API..."
)

func main() {
	var traceJSONPath string
	flag.StringVar(&traceJSONPath, "trace-json", "", "optional path for a structured trace json report")
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

	rt, err := runtime.NewBuilder().Build(cfg)
	if err != nil {
		exitf("build runtime: %v", err)
	}
	defer func() {
		if err := rt.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "close runtime: %v\n", err)
		}
	}()

	prompt := strings.TrimSpace(strings.Join(flag.Args(), " "))
	if prompt == "" {
		fmt.Fprintf(os.Stdout, readyMessageFormat, cfg.LLM.Model, cfg.Engine.LoopMaxSteps)
		if logSession != nil {
			fmt.Fprintf(os.Stdout, "run log: %s\n", logSession.Path())
		}
		return
	}

	fmt.Fprintf(os.Stderr, startMessageFormat, cfg.LLM.Model, cfg.Engine.RunTimeoutSeconds, config.ConfigPath())
	if logSession != nil {
		fmt.Fprintf(os.Stderr, "run log: %s\n", logSession.Path())
	}
	runlog.Section("Run Input", prompt)
	runlog.Linef("Model: `%s`", cfg.LLM.Model)
	runlog.Linef("Timeout: `%ds`", cfg.Engine.RunTimeoutSeconds)
	runlog.Linef("")
	fmt.Fprintln(os.Stderr, modelAPICallMessage)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.Engine.RunTimeoutSeconds)*time.Second)
	defer cancel()

	result, err := rt.Run(ctx, runtime.RunRequest{
		Input:        prompt,
		SystemPrompt: cfg.Engine.SystemPrompt,
	})
	if err != nil {
		exitf("run agent: %v", err)
	}

	runlog.Section("Final Output", result.Output)
	if traceJSONPath != "" {
		traceReport := report.RunReport{
			Model:        cfg.LLM.Model,
			Input:        prompt,
			Output:       result.Output,
			Trace:        result.Trace,
			Steps:        result.Steps,
			SystemPrompt: cfg.Engine.SystemPrompt,
		}
		if err := report.WriteJSON(traceJSONPath, traceReport); err != nil {
			exitf("write trace json: %v", err)
		}
		fmt.Fprintf(os.Stderr, "trace json: %s\n", traceJSONPath)
	}
	fmt.Fprintln(os.Stdout, result.Output)
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
