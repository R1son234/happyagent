package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
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

func (r *loopRunner) executeStep(ctx context.Context, state *LoopState, input *RunInput, actions []Action, stepIndex int) (StepResult, error) {
	if len(actions) == 0 {
		return StepResult{}, fmt.Errorf("step requires at least one action")
	}
	if err := validateStepActions(actions); err != nil {
		return StepResult{}, err
	}

	if len(actions) == 1 && actions[0].Type == protocol.ActionFinalAnswer {
		if reminder, ok := unfinishedTodoFinalAnswerReminder(state); ok {
			appendSystemReminder(state, reminder)
			return StepResult{Observation: reminder}, nil
		}
		if reminder, ok := unresolvedDeliveryFailureFinalAnswerReminder(state, actions[0].Content); ok {
			appendSystemReminder(state, reminder)
			return StepResult{Observation: reminder}, nil
		}
		if input.Hooks.ValidateFinalAnswer != nil {
			if err := input.Hooks.ValidateFinalAnswer(actions[0].Content); err != nil {
				observation := truncateObservation(err.Error(), input.Config.MaxObservationBytes)
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
		observation := truncateObservation(actions[0].Content, input.Config.MaxObservationBytes)
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

		outcome, err := r.executeToolCall(ctx, state, input, action, stepIndex)
		if err != nil {
			return StepResult{}, err
		}
		toolCalls = append(toolCalls, outcome.ToolCall)
		if action.ToolName == tools.FinalAnswerToolName {
			if outcome.ToolCall.Status != protocol.ToolCallStatusSucceeded {
				return StepResult{
					Observation: outcome.Observation,
					ToolCalls:   toolCalls,
				}, nil
			}
			return StepResult{
				Done:      true,
				Output:    outcome.Output,
				ToolCalls: toolCalls,
			}, nil
		}
		observations = append(observations, outcome.Observation)
	}

	return StepResult{
		Observation: truncateObservation(strings.Join(observations, "\n\n"), input.Config.MaxObservationBytes),
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
	Output      string
	ToolCall    ToolCallRecord
}

func (r *loopRunner) executeToolCall(ctx context.Context, state *LoopState, input *RunInput, action Action, stepIndex int) (toolCallOutcome, error) {
	if !toolAllowed(input.ToolDefs, action.ToolName) {
		observation := truncateObservation("tool error: tool "+action.ToolName+" is not available in the current context", input.Config.MaxObservationBytes)
		appendToolObservation(state, action, observation)
		recordDeliveryToolFailure(state, action.ToolName, protocol.ToolCallStatusUnavailable, observation)
		return toolCallOutcome{
			Observation: observation,
			ToolCall:    ToolCallRecord{ToolName: action.ToolName, Status: protocol.ToolCallStatusUnavailable},
		}, nil
	}
	if action.ToolName == tools.FinalAnswerToolName {
		if reminder, ok := unfinishedTodoFinalAnswerReminder(state); ok {
			observation := truncateObservation(reminder, input.Config.MaxObservationBytes)
			appendToolObservation(state, action, observation)
			return toolCallOutcome{
				Observation: observation,
				ToolCall:    ToolCallRecord{ToolName: action.ToolName, Status: protocol.ToolCallStatusBlocked},
			}, nil
		}
		content := finalAnswerContentFromAction(action)
		if reminder, ok := unresolvedDeliveryFailureFinalAnswerReminder(state, content); ok {
			observation := truncateObservation(reminder, input.Config.MaxObservationBytes)
			appendToolObservation(state, action, observation)
			return toolCallOutcome{
				Observation: observation,
				ToolCall:    ToolCallRecord{ToolName: action.ToolName, Status: protocol.ToolCallStatusBlocked},
			}, nil
		}
	}
	if input.Hooks.BeforeToolCall != nil {
		observation, handled, err := input.Hooks.BeforeToolCall(ctx, action, input)
		if err != nil {
			return toolCallOutcome{}, err
		}
		if handled {
			observation = truncateObservation(observation, input.Config.MaxObservationBytes)
			appendToolObservation(state, action, observation)
			return toolCallOutcome{
				Observation: observation,
				ToolCall:    ToolCallRecord{ToolName: action.ToolName, Status: protocol.ToolCallStatusBlocked},
			}, nil
		}
	}

	if input.Hooks.OnToolCallStart != nil {
		input.Hooks.OnToolCallStart(action.ToolName)
	}
	result, err := r.registry.Execute(ctx, tools.Call{
		Name:      action.ToolName,
		Arguments: action.Arguments,
	})
	if err != nil {
		if input.Hooks.OnToolCallEnd != nil {
			input.Hooks.OnToolCallEnd(action.ToolName, false)
		}
		observation := truncateObservation("tool error: "+err.Error(), input.Config.MaxObservationBytes)
		appendToolObservation(state, action, observation)
		recordDeliveryToolFailure(state, action.ToolName, protocol.ToolCallStatusFailed, observation)
		return toolCallOutcome{
			Observation: observation,
			ToolCall:    ToolCallRecord{ToolName: action.ToolName, Status: protocol.ToolCallStatusFailed},
		}, nil
	}
	if action.ToolName == tools.WriteTodosToolName {
		todos, err := tools.DecodeWriteTodosArguments(action.Arguments)
		if err != nil {
			if input.Hooks.OnToolCallEnd != nil {
				input.Hooks.OnToolCallEnd(action.ToolName, false)
			}
			observation := truncateObservation("tool error: "+err.Error(), input.Config.MaxObservationBytes)
			appendToolObservation(state, action, observation)
			recordDeliveryToolFailure(state, action.ToolName, protocol.ToolCallStatusFailed, observation)
			return toolCallOutcome{
				Observation: observation,
				ToolCall:    ToolCallRecord{ToolName: action.ToolName, Status: protocol.ToolCallStatusFailed},
			}, nil
		}
		state.Todos = todos
		if input.Hooks.OnTodosUpdated != nil {
			input.Hooks.OnTodosUpdated(todos)
		}
	}
	if input.Hooks.AfterToolCall != nil {
		if err := input.Hooks.AfterToolCall(ctx, action.ToolName, nil, input); err != nil {
			return toolCallOutcome{}, err
		}
	}
	rawOutput := result.Output
	observation := rawOutput
	toolCall := ToolCallRecord{ToolName: action.ToolName, Status: protocol.ToolCallStatusSucceeded}
	if action.ToolName != tools.FinalAnswerToolName {
		if isOffloadFileRead(action, input.Config.Offload) {
			observation = truncateObservation(rawOutput, input.Config.MaxObservationBytes)
		} else {
			offloaded, err := maybeOffloadObservation(input.Config.Offload, action.ToolName, stepIndex, rawOutput, offloadSourceLabel(action))
			if err != nil {
				toolCall.OffloadError = err.Error()
				observation = truncateObservation(rawOutput, input.Config.MaxObservationBytes)
			} else if offloaded.Offloaded {
				observation = offloaded.Observation
				toolCall.Offloaded = true
				toolCall.OffloadPath = offloaded.Path
				toolCall.OffloadedBytes = offloaded.Bytes
			} else {
				observation = truncateObservation(rawOutput, input.Config.MaxObservationBytes)
			}
		}
	}
	if action.ToolName != tools.FinalAnswerToolName {
		observation = appendTodoProgressReminder(observation, state)
	}
	if action.ToolName == tools.FinalAnswerToolName && input.Hooks.ValidateFinalAnswer != nil {
		if err := input.Hooks.ValidateFinalAnswer(rawOutput); err != nil {
			if input.Hooks.OnToolCallEnd != nil {
				input.Hooks.OnToolCallEnd(action.ToolName, false)
			}
			observation = truncateObservation(err.Error(), input.Config.MaxObservationBytes)
			appendToolObservation(state, action, observation)
			return toolCallOutcome{
				Observation: observation,
				ToolCall:    ToolCallRecord{ToolName: action.ToolName, Status: protocol.ToolCallStatusFailed},
			}, nil
		}
	}
	appendToolObservation(state, action, observation)
	clearDeliveryToolFailure(state, action.ToolName)
	if input.Hooks.OnToolCallEnd != nil {
		input.Hooks.OnToolCallEnd(action.ToolName, true)
	}
	return toolCallOutcome{
		Observation: observation,
		Output:      rawOutput,
		ToolCall:    toolCall,
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

func appendTodoProgressReminder(observation string, state *LoopState) string {
	reminder, ok := unfinishedTodoProgressReminder(state)
	if !ok {
		return observation
	}
	if strings.TrimSpace(observation) == "" {
		return reminder
	}
	return observation + "\n\n" + reminder
}

func appendSystemReminder(state *LoopState, reminder string) {
	state.Messages = append(state.Messages, MessageEnvelope{
		Role:    protocol.RoleSystem,
		Content: reminder,
	})
}

func recordDeliveryToolFailure(state *LoopState, toolName string, status string, observation string) {
	if !isDeliveryTool(toolName) {
		return
	}
	if state.DeliveryToolFailures == nil {
		state.DeliveryToolFailures = map[string]string{}
	}
	state.DeliveryToolFailures[toolName] = status + ": " + observation
}

func clearDeliveryToolFailure(state *LoopState, toolName string) {
	if state.DeliveryToolFailures == nil || !isDeliveryTool(toolName) {
		return
	}
	delete(state.DeliveryToolFailures, toolName)
	if len(state.DeliveryToolFailures) == 0 {
		state.DeliveryToolFailures = nil
	}
}

func isDeliveryTool(toolName string) bool {
	switch toolName {
	case "file_write", "file_patch", "file_delete":
		return true
	default:
		return false
	}
}

func unfinishedTodoProgressReminder(state *LoopState) (string, bool) {
	if !hasUnfinishedTodos(state.Todos) {
		return "", false
	}
	return `<system-reminder>
- There're still some TODOs not marked as 'completed', update the TODO list before your next action
- Running multiple tasks in parallel is allowed, and mark multiple TODOs as 'in_progress' if applicable
- Add/update/remove TODOs from your plan according to the latest information we collect if necessary
</system-reminder>`, true
}

func unfinishedTodoFinalAnswerReminder(state *LoopState) (string, bool) {
	if !hasUnfinishedTodos(state.Todos) {
		return "", false
	}
	return `<system-reminder>
- You attempted to produce a final answer, but the TODO list still has unfinished items.
- Continue working on the unfinished TODOs, or call ` + "`write_todos`" + ` to update/remove TODOs that are no longer needed.
- Only call ` + "`final_answer`" + ` after the TODO list accurately reflects completed or intentionally removed work.
</system-reminder>`, true
}

func unresolvedDeliveryFailureFinalAnswerReminder(state *LoopState, finalContent string) (string, bool) {
	if len(state.DeliveryToolFailures) == 0 {
		return "", false
	}
	if finalAnswerAcknowledgesDeliveryFailure(finalContent) {
		return "", false
	}
	var tools []string
	for toolName := range state.DeliveryToolFailures {
		tools = append(tools, toolName)
	}
	sort.Strings(tools)
	return `<system-reminder>
- A previous delivery tool call failed or was unavailable: ` + strings.Join(tools, ", ") + `.
- Do not claim that requested files were saved, written, deleted, or patched until the relevant delivery tool succeeds.
- Retry with an available tool, adjust the plan, or provide a final answer that explicitly says the requested filesystem change was not completed and includes the full recoverable content or exact next step.
</system-reminder>`, true
}

func finalAnswerContentFromAction(action Action) string {
	if action.ToolName != tools.FinalAnswerToolName {
		return action.Content
	}
	var input struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(action.Arguments, &input); err != nil {
		return ""
	}
	return input.Content
}

func finalAnswerAcknowledgesDeliveryFailure(content string) bool {
	normalized := strings.ToLower(strings.TrimSpace(content))
	if normalized == "" {
		return false
	}
	acknowledgements := []string{
		"未写入",
		"未保存",
		"没有写入",
		"没有保存",
		"无法写入",
		"无法保存",
		"写入失败",
		"保存失败",
		"not written",
		"not saved",
		"not completed",
		"was not written",
		"was not saved",
		"write failed",
		"save failed",
	}
	for _, acknowledgement := range acknowledgements {
		if strings.Contains(normalized, acknowledgement) {
			return true
		}
	}
	return false
}

func hasUnfinishedTodos(todos []tools.TodoItem) bool {
	for _, todo := range todos {
		if todo.Status != "completed" {
			return true
		}
	}
	return false
}

func offloadSourceLabel(action Action) string {
	if action.ToolName != "file_read" {
		return ""
	}
	path, ok := fileReadPath(action)
	if !ok {
		return ""
	}
	return filepath.ToSlash(filepath.Clean(path))
}

func isOffloadFileRead(action Action, config OffloadConfig) bool {
	if action.ToolName != "file_read" {
		return false
	}
	path, ok := fileReadPath(action)
	if !ok {
		return false
	}
	offloadDir := strings.TrimSpace(config.Dir)
	if offloadDir == "" {
		offloadDir = ".happyagent/offload"
	}
	cleanPath := filepath.ToSlash(filepath.Clean(path))
	cleanDir := filepath.ToSlash(filepath.Clean(offloadDir))
	return cleanPath == cleanDir || strings.HasPrefix(cleanPath, cleanDir+"/")
}

func fileReadPath(action Action) (string, bool) {
	var input struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(action.Arguments, &input); err != nil {
		return "", false
	}
	if strings.TrimSpace(input.Path) == "" {
		return "", false
	}
	return input.Path, true
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
