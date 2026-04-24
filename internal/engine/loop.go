package engine

import (
	"context"
	"fmt"

	"happyagent/internal/llm"
	"happyagent/internal/protocol"
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

	var action Action
	if resp.Action != nil {
		action = *resp.Action
	} else {
		action, err = ParseAction(resp.Message.Content)
		if err != nil {
			action = Action{
				Type:    protocol.ActionFinalAnswer,
				Content: resp.Message.Content,
			}
		}
	}

	state.Messages = append(state.Messages, MessageEnvelope{
		Role:             protocol.RoleAssistant,
		Content:          resp.Message.Content,
		ReasoningContent: resp.Message.ReasoningContent,
		Action:           resp.Action,
	})

	return action, nil
}

func (r *loopRunner) executeStep(ctx context.Context, state *LoopState, action Action) (StepResult, error) {
	switch action.Type {
	case protocol.ActionFinalAnswer:
		return StepResult{
			Done:   true,
			Output: action.Content,
		}, nil
	case protocol.ActionToolCall:
		result, err := r.registry.Execute(ctx, tools.Call{
			Name:      action.ToolName,
			Arguments: action.Arguments,
		})
		if err != nil {
			observation := "tool error: " + err.Error()
			state.Messages = append(state.Messages, MessageEnvelope{
				Role:       protocol.RoleTool,
				Content:    observation,
				ToolCallID: action.ToolCallID,
				ToolName:   action.ToolName,
			})
			return StepResult{Observation: observation}, nil
		}

		state.Messages = append(state.Messages, MessageEnvelope{
			Role:       protocol.RoleTool,
			Content:    result.Output,
			ToolCallID: action.ToolCallID,
			ToolName:   action.ToolName,
		})
		return StepResult{Observation: result.Output}, nil
	default:
		return StepResult{}, fmt.Errorf("unsupported action type %q", action.Type)
	}
}
