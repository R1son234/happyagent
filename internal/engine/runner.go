package engine

import (
	"context"
	"fmt"
	"time"

	"happyagent/internal/llm"
	"happyagent/internal/protocol"
	"happyagent/internal/runlog"
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
	currentInput := input
	startedAt := time.Now()

	for step := 0; step < r.loop.maxSteps; step++ {
		planResult, err := r.loop.planStep(ctx, currentInput, &state)
		if err != nil {
			return RunResult{}, err
		}

		executionStartedAt := time.Now()
		result, err := r.loop.executeStep(ctx, &state, &currentInput, planResult.Actions)
		if err != nil {
			return RunResult{}, err
		}
		executionDuration := time.Since(executionStartedAt)

		state.Steps = append(state.Steps, StepRecord{
			Index:                   step + 1,
			Actions:                 append([]Action(nil), planResult.Actions...),
			Observation:             result.Observation,
			ModelUsage:              planResult.Usage,
			PlanningDurationMillis:  planResult.Duration.Milliseconds(),
			ExecutionDurationMillis: executionDuration.Milliseconds(),
		})
		runlog.Step(step+1, planResult.Actions, result.Observation)

		if result.Done {
			finishedAt := time.Now()
			return RunResult{
				Output: result.Output,
				Steps:  state.Steps,
				Trace:  buildRunTrace(startedAt, finishedAt, state.Steps),
			}, nil
		}
	}

	return RunResult{}, fmt.Errorf("loop stopped after reaching max steps (%d)", r.loop.maxSteps)
}

func buildRunTrace(startedAt time.Time, finishedAt time.Time, steps []StepRecord) RunTrace {
	trace := RunTrace{
		StartedAt:       startedAt,
		FinishedAt:      finishedAt,
		DurationMillis:  finishedAt.Sub(startedAt).Milliseconds(),
		StepCount:       len(steps),
		ToolCallsByName: map[string]int{},
	}

	for _, step := range steps {
		trace.PromptTokens += step.ModelUsage.PromptTokens
		trace.CompletionTokens += step.ModelUsage.CompletionTokens
		trace.TotalTokens += step.ModelUsage.TotalTokens
		for _, action := range step.Actions {
			if action.Type != protocol.ActionToolCall {
				continue
			}
			trace.ToolCallCount++
			trace.ToolCallsByName[action.ToolName]++
		}
	}

	return trace
}
