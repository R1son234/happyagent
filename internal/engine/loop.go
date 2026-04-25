package engine

import (
	"context"
	"fmt"
	"strings"

	"happyagent/internal/llm"
	"happyagent/internal/protocol"
	"happyagent/internal/tools"
)

type loopRunner struct {
	client   llm.Client
	registry *tools.Registry
	maxSteps int
}

const actionInvalidResponse = "invalid_response"
const invalidResponseMessage = "format error: return exactly one JSON action object or native tool call response; do not answer with plain text, markdown, or explanations"

func (r *loopRunner) planStep(ctx context.Context, input RunInput, state *LoopState) ([]Action, error) {
	resp, err := r.client.Chat(ctx, llm.ChatRequest{
		Messages: BuildMessages(input, *state),
		Tools:    BuildToolSpecs(input.ToolDefs),
	})
	if err != nil {
		return nil, fmt.Errorf("chat with model: %w", err)
	}

	var actions []Action
	if len(resp.Actions) > 0 {
		actions = append([]Action(nil), resp.Actions...)
	} else {
		var action Action
		action, err = ParseAction(resp.Message.Content)
		if err != nil {
			action = Action{
				Type:    actionInvalidResponse,
				Content: invalidResponseMessage,
			}
		}
		actions = []Action{action}
	}

	state.Messages = append(state.Messages, MessageEnvelope{
		Role:             protocol.RoleAssistant,
		Content:          resp.Message.Content,
		ReasoningContent: resp.Message.ReasoningContent,
		Actions:          append([]Action(nil), actions...),
	})

	return actions, nil
}

func (r *loopRunner) executeStep(ctx context.Context, state *LoopState, input *RunInput, actions []Action) (StepResult, error) {
	if len(actions) == 0 {
		return StepResult{}, fmt.Errorf("step requires at least one action")
	}
	if err := validateStepActions(actions); err != nil {
		return StepResult{}, err
	}

	if len(actions) == 1 && actions[0].Type == protocol.ActionFinalAnswer {
		return StepResult{
			Done:   true,
			Output: actions[0].Content,
		}, nil
	}
	if len(actions) == 1 && actions[0].Type == actionInvalidResponse {
		state.Messages = append(state.Messages, MessageEnvelope{
			Role:    protocol.RoleUser,
			Content: actions[0].Content,
		})
		return StepResult{Observation: actions[0].Content}, nil
	}

	observations := make([]string, 0, len(actions))
	for _, action := range actions {
		if action.Type != protocol.ActionToolCall {
			return StepResult{}, fmt.Errorf("unsupported action type %q in multi-action step", action.Type)
		}

		observation, err := r.executeToolCall(ctx, state, input, action)
		if err != nil {
			return StepResult{}, err
		}
		if action.ToolName == tools.FinalAnswerToolName {
			return StepResult{
				Done:   true,
				Output: observation,
			}, nil
		}
		observations = append(observations, observation)
	}

	return StepResult{Observation: strings.Join(observations, "\n\n")}, nil
}

func validateStepActions(actions []Action) error {
	finalAnswerToolCalls := 0
	for _, action := range actions {
		if action.Type == protocol.ActionToolCall && action.ToolName == tools.FinalAnswerToolName {
			finalAnswerToolCalls++
		}
	}
	if finalAnswerToolCalls == 0 {
		return nil
	}
	if len(actions) != 1 || finalAnswerToolCalls != 1 {
		return fmt.Errorf("final_answer tool must be the only action in a step")
	}
	return nil
}

func (r *loopRunner) executeToolCall(ctx context.Context, state *LoopState, input *RunInput, action Action) (string, error) {
	if !toolAllowed(input.ToolDefs, action.ToolName) {
		observation := "tool error: tool " + action.ToolName + " is not available in the current context"
		appendToolObservation(state, action, observation)
		return observation, nil
	}

	result, err := r.registry.Execute(ctx, tools.Call{
		Name:      action.ToolName,
		Arguments: action.Arguments,
	})
	if err != nil {
		observation := "tool error: " + err.Error()
		appendToolObservation(state, action, observation)
		return observation, nil
	}
	if input.AfterToolCall != nil {
		if err := input.AfterToolCall(ctx, action.ToolName, nil, input); err != nil {
			return "", err
		}
	}

	appendToolObservation(state, action, result.Output)
	return result.Output, nil
}

func appendToolObservation(state *LoopState, action Action, observation string) {
	state.Messages = append(state.Messages, MessageEnvelope{
		Role:       protocol.RoleTool,
		Content:    observation,
		ToolCallID: action.ToolCallID,
		ToolName:   action.ToolName,
	})
}

func toolAllowed(defs []tools.Definition, name string) bool {
	for _, def := range defs {
		if def.Name == name {
			return true
		}
	}
	return false
}
