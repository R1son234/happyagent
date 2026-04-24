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
	Skill        string
}

type RunResult struct {
	Output string
	Steps  []engine.StepRecord
}

type Runtime struct {
	runner       engine.Runner
	tools        []tools.Definition
	mcpManager   *mcp.Manager
	skillLoader  *skills.Loader
	defaultSkill string
}

func (r *Runtime) Run(ctx context.Context, req RunRequest) (RunResult, error) {
	selectedSkill := req.Skill
	if selectedSkill == "" {
		selectedSkill = r.defaultSkill
	}

	skill, err := r.skillLoader.Load(selectedSkill)
	if err != nil {
		return RunResult{}, err
	}

	injection, err := skills.Inject(ctx, req.SystemPrompt, skill, r.tools, r.mcpManager)
	if err != nil {
		return RunResult{}, err
	}

	result, err := r.runner.Run(ctx, engine.RunInput{
		Input:        req.Input,
		SystemPrompt: injection.SystemPrompt,
		ToolDefs:     injection.ToolDefs,
	})
	if err != nil {
		return RunResult{}, err
	}

	return RunResult{
		Output: result.Output,
		Steps:  result.Steps,
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
