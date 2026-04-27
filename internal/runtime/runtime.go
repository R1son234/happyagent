package runtime

import (
	"context"
	"fmt"

	"happyagent/internal/engine"
	"happyagent/internal/mcp"
	"happyagent/internal/skills"
	"happyagent/internal/tools"
)

type RunRequest struct {
	Input        string
	SystemPrompt string
}

type RunResult struct {
	Output string
	Steps  []engine.StepRecord
	Trace  engine.RunTrace
}

type Runtime struct {
	runner              engine.Runner
	tools               []tools.Definition
	maxObservationBytes int
	mcpManager          *mcp.Manager
	skillLoader         *skills.Loader
}

func (r *Runtime) Run(ctx context.Context, req RunRequest) (RunResult, error) {
	skillSession, err := NewSkillSession(r.skillLoader, req.SystemPrompt, r.tools)
	if err != nil {
		return RunResult{}, err
	}
	capabilitySession := NewCapabilitySession(skillSession, r.mcpManager)
	ctx = tools.WithActivateSkillProvider(ctx, skillSession)
	ctx = tools.WithCapabilityProvider(ctx, capabilitySession)

	toolDefs, err := skillSession.ToolDefs()
	if err != nil {
		return RunResult{}, err
	}

	result, err := r.runner.Run(ctx, engine.RunInput{
		Input:               req.Input,
		SystemPrompt:        skillSession.SystemPrompt(),
		ToolDefs:            toolDefs,
		MaxObservationBytes: r.maxObservationBytes,
	})
	if err != nil {
		return RunResult{}, err
	}

	return RunResult{
		Output: result.Output,
		Steps:  result.Steps,
		Trace:  result.Trace,
	}, nil
}

func (r *Runtime) Close() error {
	if r.mcpManager == nil {
		return nil
	}
	return r.mcpManager.Close()
}

func (r *Runtime) ListResources() []mcp.ResourceInfo {
	if r.mcpManager == nil {
		return nil
	}
	return r.mcpManager.ListResources()
}

func ensureSkillLoader(loader *skills.Loader) *skills.Loader {
	if loader != nil {
		return loader
	}
	return skills.NewLoader("")
}

func validateSkillDir(dir string) error {
	if dir == "" {
		return fmt.Errorf("skills dir must not be empty")
	}
	return nil
}
