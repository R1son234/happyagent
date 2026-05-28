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
	if _, err := currentInput.Hooks.Emit(ctx, HookContext{Event: HookRunStart, RunInput: &currentInput, State: &state, StartedAt: startedAt}); err != nil {
		return RunResult{}, err
	}
	if _, err := currentInput.Hooks.Emit(ctx, HookContext{Event: HookUserPromptSubmit, RunInput: &currentInput, State: &state, Content: currentInput.Input}); err != nil {
		return RunResult{}, err
	}

	for step := 0; step < r.loop.maxSteps; step++ {
		if _, err := currentInput.Hooks.Emit(ctx, HookContext{Event: HookBeforeModelCall, RunInput: &currentInput, State: &state, StepIndex: step + 1}); err != nil {
			return RunResult{}, err
		}
		planResult, err := r.loop.planStep(ctx, currentInput, &state)
		if err != nil {
			_, _ = currentInput.Hooks.Emit(ctx, HookContext{Event: HookRunError, RunInput: &currentInput, State: &state, StepIndex: step + 1, Err: err})
			return RunResult{}, err
		}
		if _, err := currentInput.Hooks.Emit(ctx, HookContext{Event: HookAfterModelCall, RunInput: &currentInput, State: &state, StepIndex: step + 1}); err != nil {
			return RunResult{}, err
		}

		executionStartedAt := time.Now()
		result, err := r.loop.executeStep(ctx, &state, &currentInput, planResult.Actions, step+1)
		if err != nil {
			_, _ = currentInput.Hooks.Emit(ctx, HookContext{Event: HookRunError, RunInput: &currentInput, State: &state, StepIndex: step + 1, Err: err})
			return RunResult{}, err
		}
		executionDuration := time.Since(executionStartedAt)

		state.Steps = append(state.Steps, StepRecord{
			Index:                   step + 1,
			Actions:                 append([]Action(nil), planResult.Actions...),
			ToolCalls:               append([]ToolCallRecord(nil), result.ToolCalls...),
			Observation:             result.Observation,
			ModelUsage:              planResult.Usage,
			PlanningDurationMillis:  planResult.Duration.Milliseconds(),
			ExecutionDurationMillis: executionDuration.Milliseconds(),
		})
		runlog.Step(step+1, planResult.Actions, result.Observation)

		if result.Done {
			if decision, err := currentInput.Hooks.Emit(ctx, HookContext{Event: HookStop, RunInput: &currentInput, State: &state, StepIndex: step + 1, Content: result.Output}); err != nil {
				return RunResult{}, err
			} else if decision.Kind == HookDecisionForceContinue {
				appendSystemReminder(&state, decision.Message)
				continue
			}
			finishedAt := time.Now()
			return RunResult{
				Output: result.Output,
				Steps:  state.Steps,
				Trace:  buildRunTrace(startedAt, finishedAt, state, protocol.RunStatusCompleted),
			}, nil
		}
	}

	finishedAt := time.Now()
	return RunResult{
		Steps: state.Steps,
		Trace: buildRunTrace(startedAt, finishedAt, state, "max_steps_exceeded"),
	}, fmt.Errorf("loop stopped after reaching max steps (%d)", r.loop.maxSteps)
}

func buildRunTrace(startedAt time.Time, finishedAt time.Time, state LoopState, terminationReason string) RunTrace {
	steps := state.Steps
	trace := RunTrace{
		StartedAt:                  startedAt,
		FinishedAt:                 finishedAt,
		DurationMillis:             finishedAt.Sub(startedAt).Milliseconds(),
		TerminationReason:          terminationReason,
		StepCount:                  len(steps),
		ToolCallsByName:            map[string]int{},
		ExecutedToolCallsByName:    map[string]int{},
		SuccessfulToolCallsByName:  map[string]int{},
		OffloadedToolResultsByName: map[string]int{},
	}
	trace.HookDecisions = append([]HookDecisionRecord(nil), state.HookDecisions...)
	trace.CompactionEvents = append([]CompactionEvent(nil), state.CompactionEvents...)
	trace.RecoveryAttempts = append([]RecoveryAttempt(nil), state.RecoveryAttempts...)
	trace.TranscriptPath = state.TranscriptPath

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
		for _, toolCall := range step.ToolCalls {
			if toolCall.Status == protocol.ToolCallStatusFailed || toolCall.Status == protocol.ToolCallStatusSucceeded {
				trace.ExecutedToolCallCount++
				trace.ExecutedToolCallsByName[toolCall.ToolName]++
			}
			if toolCall.Status == protocol.ToolCallStatusSucceeded {
				trace.SuccessfulToolCallCount++
				trace.SuccessfulToolCallsByName[toolCall.ToolName]++
			}
			if toolCall.Status == protocol.ToolCallStatusFailed {
				trace.FailedToolCallCount++
				if trace.FailedToolCallsByName == nil {
					trace.FailedToolCallsByName = map[string]int{}
				}
				trace.FailedToolCallsByName[toolCall.ToolName]++
			}
			if toolCall.Status == protocol.ToolCallStatusUnavailable {
				trace.UnavailableToolCallCount++
				if trace.UnavailableToolCallsByName == nil {
					trace.UnavailableToolCallsByName = map[string]int{}
				}
				trace.UnavailableToolCallsByName[toolCall.ToolName]++
			}
			if toolCall.Status == protocol.ToolCallStatusBlocked {
				trace.BlockedToolCallCount++
				if trace.BlockedToolCallsByName == nil {
					trace.BlockedToolCallsByName = map[string]int{}
				}
				trace.BlockedToolCallsByName[toolCall.ToolName]++
			}
			if toolCall.Offloaded {
				trace.OffloadedToolResultCount++
				trace.OffloadedToolResultBytes += toolCall.OffloadedBytes
				trace.OffloadedToolResultsByName[toolCall.ToolName]++
			}
		}
	}

	return trace
}
