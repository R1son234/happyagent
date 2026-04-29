package engine

import (
	"context"
	"fmt"
	"strings"
	"time"

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
const defaultMaxObservationBytes = 8 * 1024

const (
	toolCallStatusUnavailable = "unavailable"
	toolCallStatusBlocked     = "blocked"
	toolCallStatusFailed      = "failed"
	toolCallStatusSucceeded   = "succeeded"
)

func (r *loopRunner) planStep(ctx context.Context, input RunInput, state *LoopState) (PlanStepResult, error) {
	startedAt := time.Now()
	resp, err := r.client.Chat(ctx, llm.ChatRequest{
		Messages: BuildMessages(input, *state),
		Tools:    BuildToolSpecs(input.ToolDefs),
	})
	if err != nil {
		return PlanStepResult{}, fmt.Errorf("chat with model: %w", err)
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

	return PlanStepResult{
		Actions:  actions,
		Usage:    resp.Usage,
		Duration: time.Since(startedAt),
	}, nil
}

func (r *loopRunner) executeStep(ctx context.Context, state *LoopState, input *RunInput, actions []Action) (StepResult, error) {
	if len(actions) == 0 {
		return StepResult{}, fmt.Errorf("step requires at least one action")
	}
	if err := validateStepActions(actions); err != nil {
		return StepResult{}, err
	}

	if len(actions) == 1 && actions[0].Type == protocol.ActionFinalAnswer {
		if input.ValidateFinalAnswer != nil {
			if err := input.ValidateFinalAnswer(actions[0].Content); err != nil {
				observation := truncateObservation(err.Error(), input.MaxObservationBytes)
				state.Messages = append(state.Messages, MessageEnvelope{
					Role:    protocol.RoleUser,
					Content: observation,
				})
				return StepResult{Observation: observation}, nil
			}
		}
		return StepResult{
			Done:   true,
			Output: actions[0].Content,
		}, nil
	}
	if len(actions) == 1 && actions[0].Type == actionInvalidResponse {
		observation := truncateObservation(actions[0].Content, input.MaxObservationBytes)
		state.Messages = append(state.Messages, MessageEnvelope{
			Role:    protocol.RoleUser,
			Content: observation,
		})
		return StepResult{Observation: observation}, nil
	}

	observations := make([]string, 0, len(actions))
	toolCalls := make([]ToolCallRecord, 0, len(actions))
	for _, action := range actions {
		if action.Type != protocol.ActionToolCall {
			return StepResult{}, fmt.Errorf("unsupported action type %q in multi-action step", action.Type)
		}

		outcome, err := r.executeToolCall(ctx, state, input, action)
		if err != nil {
			return StepResult{}, err
		}
		toolCalls = append(toolCalls, outcome.ToolCall)
		if action.ToolName == tools.FinalAnswerToolName {
			return StepResult{
				Done:      true,
				Output:    outcome.Observation,
				ToolCalls: toolCalls,
			}, nil
		}
		observations = append(observations, outcome.Observation)
	}

	return StepResult{
		Observation: truncateObservation(strings.Join(observations, "\n\n"), input.MaxObservationBytes),
		ToolCalls:   toolCalls,
	}, nil
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

type toolCallOutcome struct {
	Observation string
	ToolCall    ToolCallRecord
}

func (r *loopRunner) executeToolCall(ctx context.Context, state *LoopState, input *RunInput, action Action) (toolCallOutcome, error) {
	if !toolAllowed(input.ToolDefs, action.ToolName) {
		observation := truncateObservation("tool error: tool "+action.ToolName+" is not available in the current context", input.MaxObservationBytes)
		appendToolObservation(state, action, observation)
		return toolCallOutcome{
			Observation: observation,
			ToolCall:    ToolCallRecord{ToolName: action.ToolName, Status: toolCallStatusUnavailable},
		}, nil
	}
	if input.BeforeToolCall != nil {
		observation, handled, err := input.BeforeToolCall(ctx, action, input)
		if err != nil {
			return toolCallOutcome{}, err
		}
		if handled {
			observation = truncateObservation(observation, input.MaxObservationBytes)
			appendToolObservation(state, action, observation)
			return toolCallOutcome{
				Observation: observation,
				ToolCall:    ToolCallRecord{ToolName: action.ToolName, Status: toolCallStatusBlocked},
			}, nil
		}
	}

	result, err := r.registry.Execute(ctx, tools.Call{
		Name:      action.ToolName,
		Arguments: action.Arguments,
	})
	if err != nil {
		observation := truncateObservation("tool error: "+err.Error(), input.MaxObservationBytes)
		appendToolObservation(state, action, observation)
		return toolCallOutcome{
			Observation: observation,
			ToolCall:    ToolCallRecord{ToolName: action.ToolName, Status: toolCallStatusFailed},
		}, nil
	}
	if input.AfterToolCall != nil {
		if err := input.AfterToolCall(ctx, action.ToolName, nil, input); err != nil {
			return toolCallOutcome{}, err
		}
	}
	observation := truncateObservation(result.Output, input.MaxObservationBytes)
	appendToolObservation(state, action, observation)
	if action.ToolName == tools.FinalAnswerToolName && input.ValidateFinalAnswer != nil {
		if err := input.ValidateFinalAnswer(observation); err != nil {
			observation = truncateObservation(err.Error(), input.MaxObservationBytes)
			appendToolObservation(state, action, observation)
			return toolCallOutcome{
				Observation: observation,
				ToolCall:    ToolCallRecord{ToolName: action.ToolName, Status: toolCallStatusFailed},
			}, nil
		}
	}
	return toolCallOutcome{
		Observation: observation,
		ToolCall:    ToolCallRecord{ToolName: action.ToolName, Status: toolCallStatusSucceeded},
	}, nil
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

func truncateObservation(observation string, maxBytes int) string {
	if maxBytes <= 0 {
		maxBytes = defaultMaxObservationBytes
	}
	if len(observation) <= maxBytes {
		return observation
	}
	if maxBytes <= len("\n...[observation truncated]") {
		return observation[:maxBytes]
	}
	return observation[:maxBytes-len("\n...[observation truncated]")] + "\n...[observation truncated]"
}
