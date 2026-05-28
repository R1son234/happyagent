package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"happyagent/internal/llm"
)

type RecoveryAttempt struct {
	Reason  string `json:"reason"`
	Attempt int    `json:"attempt"`
	Action  string `json:"action"`
	Error   string `json:"error,omitempty"`
}

func (r *loopRunner) chatWithRecovery(ctx context.Context, input RunInput, state *LoopState) (llm.ChatResponse, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		resp, err := r.client.Chat(ctx, llm.ChatRequest{
			Messages: BuildMessages(input, *state),
			Tools:    BuildToolSpecs(input.ToolDefs),
		})
		if err == nil {
			return resp, nil
		}
		lastErr = err
		reason := classifyModelError(err)
		switch reason {
		case "prompt_too_long":
			state.RecoveryAttempts = append(state.RecoveryAttempts, RecoveryAttempt{
				Reason:  reason,
				Attempt: attempt + 1,
				Action:  "reactive_compact",
				Error:   err.Error(),
			})
			if compactErr := reactiveCompact(state, input); compactErr != nil {
				return llm.ChatResponse{}, fmt.Errorf("reactive compact after prompt-too-long: %w", compactErr)
			}
			continue
		case "rate_or_overloaded", "timeout":
			delay := time.Duration(250*(1<<attempt)) * time.Millisecond
			state.RecoveryAttempts = append(state.RecoveryAttempts, RecoveryAttempt{
				Reason:  reason,
				Attempt: attempt + 1,
				Action:  "retry_after_backoff",
				Error:   err.Error(),
			})
			select {
			case <-ctx.Done():
				return llm.ChatResponse{}, ctx.Err()
			case <-time.After(delay):
			}
			continue
		default:
			return llm.ChatResponse{}, err
		}
	}
	return llm.ChatResponse{}, lastErr
}

func (r *loopRunner) recoverInvalidStructuredResponse(ctx context.Context, input RunInput, state *LoopState, resp llm.ChatResponse, parseErr error) (llm.ChatResponse, []Action, bool, error) {
	if !shouldContinueStructuredResponse(resp, parseErr) {
		return llm.ChatResponse{}, nil, false, nil
	}
	state.RecoveryAttempts = append(state.RecoveryAttempts, RecoveryAttempt{
		Reason:  "max_output_or_incomplete_json",
		Attempt: 1,
		Action:  "continuation_prompt",
		Error:   parseErr.Error(),
	})
	messages := BuildMessages(input, *state)
	messages = append(messages, llm.Message{
		Role:    "assistant",
		Content: resp.Message.Content,
	})
	messages = append(messages, llm.Message{
		Role:    "user",
		Content: "The previous assistant message was cut off or invalid JSON. Return the complete intended action again as exactly one valid JSON action object. Do not include markdown, prose, or a suffix-only continuation.",
	})
	recovered, err := r.client.Chat(ctx, llm.ChatRequest{
		Messages: messages,
		Tools:    BuildToolSpecs(input.ToolDefs),
	})
	if err != nil {
		state.RecoveryAttempts = append(state.RecoveryAttempts, RecoveryAttempt{
			Reason:  "max_output_or_incomplete_json",
			Attempt: 2,
			Action:  "continuation_failed",
			Error:   err.Error(),
		})
		return llm.ChatResponse{}, nil, false, nil
	}
	var actions []Action
	if len(recovered.Actions) > 0 {
		actions = append([]Action(nil), recovered.Actions...)
	} else {
		action, err := ParseAction(recovered.Message.Content)
		if err != nil {
			state.RecoveryAttempts = append(state.RecoveryAttempts, RecoveryAttempt{
				Reason:  "invalid_structured_response",
				Attempt: 2,
				Action:  "continuation_parse_failed",
				Error:   err.Error(),
			})
			return llm.ChatResponse{}, nil, false, nil
		}
		actions = []Action{action}
	}
	state.RecoveryAttempts = append(state.RecoveryAttempts, RecoveryAttempt{
		Reason:  "max_output_or_incomplete_json",
		Attempt: 2,
		Action:  "continuation_succeeded",
	})
	return recovered, actions, true, nil
}

func shouldContinueStructuredResponse(resp llm.ChatResponse, err error) bool {
	reason := strings.ToLower(strings.TrimSpace(resp.FinishReason))
	if reason == "length" || reason == "max_tokens" || reason == "content_filter" {
		return true
	}
	if err == nil {
		return false
	}
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	var syntax *json.SyntaxError
	if errors.As(err, &syntax) {
		message := strings.ToLower(err.Error())
		return strings.Contains(message, "unexpected end") || strings.Contains(message, "unexpected eof")
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unexpected end of json input") || strings.Contains(message, "unexpected eof")
}

func classifyModelError(err error) string {
	if err == nil {
		return ""
	}
	value := strings.ToLower(err.Error())
	switch {
	case strings.Contains(value, "prompt") && strings.Contains(value, "long"):
		return "prompt_too_long"
	case strings.Contains(value, "context") && strings.Contains(value, "length"):
		return "prompt_too_long"
	case strings.Contains(value, "429"), strings.Contains(value, "rate limit"), strings.Contains(value, "overloaded"), strings.Contains(value, "529"):
		return "rate_or_overloaded"
	case strings.Contains(value, "timeout"), strings.Contains(value, "deadline exceeded"):
		return "timeout"
	default:
		return "model_error"
	}
}
