package engine

import (
	"context"
	"encoding/json"
	"fmt"

	"happyagent/internal/llm"
	"happyagent/internal/tools"
)

type loopRunner struct {
	client   llm.Client
	registry *tools.Registry
	maxSteps int
}

func (r *loopRunner) planStep(ctx context.Context, input RunInput, state *LoopState) (Action, error) {
	resp, err := r.client.Chat(ctx, llm.ChatRequest{
		Messages: BuildMessages(input, *state),
		Tools:    BuildToolSpecs(input.ToolDefs),
	})
	if err != nil {
		return Action{}, fmt.Errorf("chat with model: %w", err)
	}

	action, err := ParseAction(resp.Message.Content)
	if err != nil {
		return Action{}, fmt.Errorf("parse model output: %w", err)
	}

	state.Messages = append(state.Messages, MessageEnvelope{
		Role:    "assistant",
		Content: resp.Message.Content,
	})

	return action, nil
}

func (r *loopRunner) executeStep(ctx context.Context, state *LoopState, action Action) (StepResult, error) {
	switch action.Type {
	case "final_answer":
		return StepResult{
			Done:   true,
			Output: action.Content,
		}, nil
	case "tool_call":
		result, err := r.registry.Execute(ctx, tools.Call{
			Name:      action.ToolName,
			Arguments: json.RawMessage(action.Arguments),
		})
		if err != nil {
			observation := "tool error: " + err.Error()
			state.Messages = append(state.Messages, MessageEnvelope{
				Role:    "tool",
				Content: observation,
			})
			return StepResult{Observation: observation}, nil
		}

		state.Messages = append(state.Messages, MessageEnvelope{
			Role:    "tool",
			Content: result.Output,
		})
		return StepResult{Observation: result.Output}, nil
	default:
		return StepResult{}, fmt.Errorf("unsupported action type %q", action.Type)
	}
}
