package runtime

import (
	"context"

	"happyagent/internal/engine"
	"happyagent/internal/tools"
)

type RunRequest struct {
	Input        string
	SystemPrompt string
}

type RunResult struct {
	Output string
	Steps  []engine.StepRecord
}

type Runtime struct {
	runner engine.Runner
	tools  []tools.Definition
}

func (r *Runtime) Run(ctx context.Context, req RunRequest) (RunResult, error) {
	result, err := r.runner.Run(ctx, engine.RunInput{
		Input:        req.Input,
		SystemPrompt: req.SystemPrompt,
		ToolDefs:     r.tools,
	})
	if err != nil {
		return RunResult{}, err
	}

	return RunResult{
		Output: result.Output,
		Steps:  result.Steps,
	}, nil
}
