package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"happyagent/internal/config"
	"happyagent/internal/runtime"
)

func main() {
	var (
		configPath string
		prompt     string
	)

	flag.StringVar(&configPath, "config", "", "optional path to a JSON config file; defaults to ./happyagent.local.json when present")
	flag.StringVar(&prompt, "prompt", "", "task for the agent to run")
	flag.Parse()

	configPath = resolveConfigPath(configPath)

	cfg, err := config.Load(configPath)
	if err != nil {
		exitf("load config: %v", err)
	}

	rt, err := runtime.NewBuilder().Build(cfg)
	if err != nil {
		exitf("build runtime: %v", err)
	}

	if prompt == "" {
		fmt.Fprintf(os.Stdout, "happyagent is ready. model=%s max_steps=%d\n", cfg.LLM.Model, cfg.Engine.LoopMaxSteps)
		return
	}

	fmt.Fprintf(os.Stderr, "starting run: model=%s timeout=%ds config=%s\n", cfg.LLM.Model, cfg.Engine.RunTimeoutSeconds, displayConfigPath(configPath))
	fmt.Fprintln(os.Stderr, "calling model API...")

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.Engine.RunTimeoutSeconds)*time.Second)
	defer cancel()

	result, err := rt.Run(ctx, runtime.RunRequest{
		Input:        prompt,
		SystemPrompt: cfg.Engine.SystemPrompt,
	})
	if err != nil {
		exitf("run agent: %v", err)
	}

	fmt.Fprintln(os.Stdout, result.Output)
}

func exitf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

func resolveConfigPath(flagValue string) string {
	if flagValue != "" {
		return flagValue
	}

	const defaultConfigPath = "happyagent.local.json"
	if _, err := os.Stat(defaultConfigPath); err == nil {
		return defaultConfigPath
	}

	return ""
}

func displayConfigPath(path string) string {
	if path == "" {
		return "(env/defaults only)"
	}
	return path
}
