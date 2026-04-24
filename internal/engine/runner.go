package engine

import (
	"context"
	"fmt"

	"happyagent/internal/llm"
	"happyagent/internal/tools"
)

type Runner interface {
	Run(ctx context.Context, input RunInput) (RunResult, error)
}

type runner struct {
	loop *loopRunner
}

func NewRunner(client llm.Client, registry *tools.Registry, maxSteps int) Runner {
	return &runner{
		loop: &loopRunner{
			client:   client,
			registry: registry,
			maxSteps: maxSteps,
		},
	}
}

func (r *runner) Run(ctx context.Context, input RunInput) (RunResult, error) {
	state := LoopState{}

	for step := 0; step < r.loop.maxSteps; step++ {
		action, err := r.loop.planStep(ctx, input, &state)
		if err != nil {
			return RunResult{}, err
		}

		result, err := r.loop.executeStep(ctx, &state, action)
		if err != nil {
			return RunResult{}, err
		}

		state.Steps = append(state.Steps, StepRecord{
			Index:       step + 1,
			Action:      action,
			Observation: result.Observation,
		})

		if result.Done {
			return RunResult{
				Output: result.Output,
				Steps:  state.Steps,
			}, nil
		}
	}

	return RunResult{}, fmt.Errorf("loop stopped after reaching max steps (%d)", r.loop.maxSteps)
}
