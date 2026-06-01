package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"happyagent/internal/agents"
	"happyagent/internal/engine"
	"happyagent/internal/observe"
	"happyagent/internal/tools"
)

const leadAgentID = "lead"

type runtimeAgentProvider struct {
	runtime  *Runtime
	parent   RunRequest
	prepared preparedRun
	toolDefs []tools.Definition
}

func (p runtimeAgentProvider) AgentTask(ctx context.Context, args json.RawMessage) (string, error) {
	if p.runtime == nil || p.runtime.agentStore == nil {
		return "", fmt.Errorf("agent store is unavailable")
	}
	var input agentRunInput
	if err := json.Unmarshal(args, &input); err != nil {
		return "", fmt.Errorf("decode agent_task arguments: %w", err)
	}
	if strings.TrimSpace(input.Prompt) == "" {
		return "", fmt.Errorf("agent_task prompt cannot be empty")
	}
	agent, run, result, err := p.runChild(ctx, input, false)
	output := map[string]any{
		"agent": agent,
		"run":   run,
	}
	if err != nil {
		output["error"] = err.Error()
		return marshalToolJSON(output, nil)
	}
	output["summary"] = result.Output
	return marshalToolJSON(output, nil)
}

func (p runtimeAgentProvider) AgentSpawn(ctx context.Context, args json.RawMessage) (string, error) {
	if p.runtime == nil || p.runtime.agentStore == nil {
		return "", fmt.Errorf("agent store is unavailable")
	}
	var input agentRunInput
	if err := json.Unmarshal(args, &input); err != nil {
		return "", fmt.Errorf("decode agent_spawn arguments: %w", err)
	}
	if strings.TrimSpace(input.Prompt) == "" {
		return "", fmt.Errorf("agent_spawn prompt cannot be empty")
	}
	agent, err := p.registerAgent(input)
	if err != nil {
		return "", err
	}
	runID := fmt.Sprintf("%s-%d", agent.ID, time.Now().UnixNano())
	var jobID string
	if p.runtime.backgroundStore != nil {
		job := p.runtime.backgroundStore.Start("team")
		jobID = job.ID
	}
	go func() {
		childInput := input
		childInput.AgentID = agent.ID
		childInput.RunID = runID
		_, run, result, runErr := p.runChild(context.Background(), childInput, true)
		content := result.Output
		kind := "result"
		if runErr != nil {
			kind = "failure"
			content = runErr.Error()
		}
		_, _ = p.runtime.agentStore.AppendMessage(agents.MailboxMessage{
			From:    agent.ID,
			To:      leadAgentID,
			Kind:    kind,
			Content: content,
			RunID:   run.ID,
		})
		if p.runtime.backgroundStore != nil && jobID != "" {
			_ = p.runtime.backgroundStore.Complete(jobID, content, runErr)
		}
	}()
	return marshalToolJSON(map[string]any{
		"agent":  agent,
		"status": "spawned",
	}, nil)
}

func (p runtimeAgentProvider) AgentSendMessage(ctx context.Context, args json.RawMessage) (string, error) {
	_ = ctx
	if p.runtime == nil || p.runtime.agentStore == nil {
		return "", fmt.Errorf("agent store is unavailable")
	}
	var input struct {
		To      string `json:"to"`
		Kind    string `json:"kind"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return "", fmt.Errorf("decode agent_send_message arguments: %w", err)
	}
	if strings.TrimSpace(input.To) == "" || strings.TrimSpace(input.Content) == "" {
		return "", fmt.Errorf("agent_send_message requires to and content")
	}
	if input.Kind == "" {
		input.Kind = "note"
	}
	message, err := p.runtime.agentStore.AppendMessage(agents.MailboxMessage{
		From:    leadAgentID,
		To:      input.To,
		Kind:    input.Kind,
		Content: input.Content,
	})
	return marshalToolJSON(message, err)
}

func (p runtimeAgentProvider) AgentCheckInbox(ctx context.Context, args json.RawMessage) (string, error) {
	_ = ctx
	if p.runtime == nil || p.runtime.agentStore == nil {
		return "", fmt.Errorf("agent store is unavailable")
	}
	var input struct {
		To string `json:"to"`
	}
	if len(args) > 0 {
		if err := json.Unmarshal(args, &input); err != nil {
			return "", fmt.Errorf("decode agent_check_inbox arguments: %w", err)
		}
	}
	if input.To == "" {
		input.To = leadAgentID
	}
	messages, err := p.runtime.agentStore.ListUnconsumed(input.To)
	if err != nil {
		return "", err
	}
	ids := make([]string, 0, len(messages))
	for _, message := range messages {
		ids = append(ids, message.ID)
	}
	if err := p.runtime.agentStore.MarkConsumed(ids); err != nil {
		return "", err
	}
	return marshalToolJSON(messages, nil)
}

func (p runtimeAgentProvider) AgentShutdown(ctx context.Context, args json.RawMessage) (string, error) {
	_ = ctx
	if p.runtime == nil || p.runtime.agentStore == nil {
		return "", fmt.Errorf("agent store is unavailable")
	}
	var input struct {
		AgentID string `json:"agent_id"`
		Reason  string `json:"reason"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return "", fmt.Errorf("decode agent_shutdown arguments: %w", err)
	}
	if strings.TrimSpace(input.AgentID) == "" {
		return "", fmt.Errorf("agent_shutdown requires agent_id")
	}
	agent, err := p.runtime.agentStore.UpsertAgent(agents.Agent{
		ID:     input.AgentID,
		Name:   input.AgentID,
		Status: agents.AgentStatusStopped,
	})
	if err != nil {
		return "", err
	}
	message, err := p.runtime.agentStore.AppendMessage(agents.MailboxMessage{
		From:    input.AgentID,
		To:      leadAgentID,
		Kind:    "shutdown_ack",
		Content: input.Reason,
	})
	if err != nil {
		return "", err
	}
	return marshalToolJSON(map[string]any{"agent": agent, "message": message}, nil)
}

type agentRunInput struct {
	AgentID      string `json:"agent_id"`
	RunID        string `json:"run_id"`
	Name         string `json:"name"`
	Role         string `json:"role"`
	Prompt       string `json:"prompt"`
	ProfileName  string `json:"profile_name"`
	WorktreePath string `json:"worktree_path"`
	TaskID       string `json:"task_id"`
}

func (p runtimeAgentProvider) registerAgent(input agentRunInput) (agents.Agent, error) {
	id := strings.TrimSpace(input.AgentID)
	if id == "" {
		name := strings.TrimSpace(input.Name)
		if name == "" {
			name = "child"
		}
		id = fmt.Sprintf("%s-%d", slug(name), time.Now().UnixNano())
	}
	return p.runtime.agentStore.UpsertAgent(agents.Agent{
		ID:     id,
		Name:   input.Name,
		Role:   input.Role,
		Status: agents.AgentStatusRunning,
	})
}

func (p runtimeAgentProvider) runChild(ctx context.Context, input agentRunInput, async bool) (agents.Agent, agents.AgentRun, engine.RunResult, error) {
	agent, err := p.registerAgent(input)
	if err != nil {
		return agents.Agent{}, agents.AgentRun{}, engine.RunResult{}, err
	}
	runID := strings.TrimSpace(input.RunID)
	if runID == "" {
		runID = fmt.Sprintf("%s-%d", agent.ID, time.Now().UnixNano())
	}
	childReq := RunRequest{
		Input:              childPrompt(agent, input.Prompt),
		ProfileName:        input.ProfileName,
		SessionID:          p.parent.SessionID,
		RunID:              runID,
		ApprovedTools:      p.parent.ApprovedTools,
		ToolScope:          []string{"file_read", "final_answer"},
		SourceReadPaths:    append([]string(nil), p.parent.SourceReadPaths...),
		RequireSourceReads: p.parent.RequireSourceReads,
		ChildAgentID:       agent.ID,
		ChildTaskID:        input.TaskID,
	}
	if childReq.ProfileName == "" {
		childReq.ProfileName = p.prepared.profileName
	}
	prepared, err := p.runtime.prepareRun(childReq)
	if err != nil {
		return agent, agents.AgentRun{}, engine.RunResult{}, err
	}
	if input.TaskID != "" && p.runtime.taskStore != nil {
		if _, err := p.runtime.taskStore.Claim(input.TaskID, agent.ID); err != nil {
			return agent, agents.AgentRun{}, engine.RunResult{}, err
		}
	}
	prepared.toolDefs = withoutAgentTools(prepared.toolDefs)
	skillSession, err := NewSkillSession(prepared.skillLoader, prepared.systemPrompt, prepared.toolDefs)
	if err != nil {
		return agent, agents.AgentRun{}, engine.RunResult{}, err
	}
	capabilitySession := NewCapabilitySession(skillSession, p.runtime.mcpManager)
	childTaskProvider := runtimeTaskProvider{store: p.runtime.taskStore}
	childAgentProvider := runtimeAgentProvider{runtime: p.runtime, parent: childReq, prepared: prepared, toolDefs: prepared.toolDefs}
	childCtx := tools.WithActivateSkillProvider(ctx, skillSession)
	childCtx = tools.WithCapabilityProvider(childCtx, capabilitySession)
	childCtx = tools.WithTaskProvider(childCtx, childTaskProvider)
	childCtx = tools.WithAgentProvider(childCtx, childAgentProvider)
	if input.WorktreePath != "" {
		if p.runtime.worktreeManager == nil {
			return agent, agents.AgentRun{}, engine.RunResult{}, fmt.Errorf("worktree manager is unavailable")
		}
		workdir, err := p.runtime.worktreeManager.Enter(input.WorktreePath)
		if err != nil {
			return agent, agents.AgentRun{}, engine.RunResult{}, err
		}
		childCtx = tools.WithShellWorkdir(childCtx, workdir)
	}
	toolDefs, err := skillSession.ToolDefs()
	if err != nil {
		return agent, agents.AgentRun{}, engine.RunResult{}, err
	}
	toolDefs = withoutAgentTools(toolDefs)
	recorder := observe.NewRecorder()
	result, runErr := p.runtime.runner.Run(childCtx, engine.RunInput{
		Input:          childReq.Input,
		SystemPrompt:   skillSession.SystemPrompt(),
		RuntimeContext: prepared.runtimeContext,
		ToolDefs:       toolDefs,
		Config: engine.RunConfig{
			MaxObservationBytes: p.runtime.maxObservationBytes,
			Offload:             prepared.offload,
		},
		Hooks: prepared.hookPipeline(childReq, recorder),
	})
	tracePath := p.runtime.agentStore.TracePath(runID)
	if data, err := json.MarshalIndent(result.Trace, "", "  "); err == nil {
		_ = os.WriteFile(tracePath, data, 0o644)
	}
	status := "completed"
	errText := ""
	if runErr != nil {
		status = "failed"
		errText = runErr.Error()
	}
	if input.TaskID != "" && p.runtime.taskStore != nil {
		if runErr == nil {
			if _, completeErr := p.runtime.taskStore.Complete(input.TaskID, nil); completeErr != nil {
				status = "failed"
				errText = completeErr.Error()
				runErr = completeErr
			}
		} else {
			_, _ = p.runtime.taskStore.Release(input.TaskID)
		}
	}
	run, err := p.runtime.agentStore.AppendRun(agents.AgentRun{
		ID:        runID,
		AgentID:   agent.ID,
		Prompt:    input.Prompt,
		Output:    result.Output,
		TracePath: tracePath,
		Status:    status,
		Error:     errText,
	})
	if err != nil {
		return agent, run, result, err
	}
	finalStatus := agents.AgentStatusComplete
	if async {
		finalStatus = agents.AgentStatusIdle
	}
	if runErr != nil {
		finalStatus = agents.AgentStatusFailed
	}
	_, _ = p.runtime.agentStore.UpsertAgent(agents.Agent{ID: agent.ID, Name: agent.Name, Role: agent.Role, Status: finalStatus})
	return agent, run, result, runErr
}

func childPrompt(agent agents.Agent, prompt string) string {
	return "You are a child agent named " + agent.Name + ". Role: " + agent.Role + ". Work independently and return a concise evidence-backed summary.\n\nTask:\n" + prompt
}

func withoutAgentTools(defs []tools.Definition) []tools.Definition {
	filtered := make([]tools.Definition, 0, len(defs))
	for _, def := range defs {
		switch def.Name {
		case tools.AgentTaskToolName, tools.AgentSpawnToolName, tools.AgentSendMessageToolName, tools.AgentCheckInboxToolName, tools.AgentShutdownToolName:
			continue
		default:
			filtered = append(filtered, def)
		}
	}
	return filtered
}

func slug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_':
			b.WriteRune('-')
		default:
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "agent"
	}
	return out
}
