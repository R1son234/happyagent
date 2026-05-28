package runtime

import (
	"context"
	"encoding/json"
	"log"
	"strings"

	"happyagent/internal/agents"
	"happyagent/internal/background"
	"happyagent/internal/engine"
	"happyagent/internal/mcp"
	"happyagent/internal/memory"
	"happyagent/internal/observe"
	"happyagent/internal/policy"
	"happyagent/internal/profile"
	"happyagent/internal/skills"
	"happyagent/internal/tasks"
	"happyagent/internal/tools"
	"happyagent/internal/validator"
	"happyagent/internal/worktree"
)

type RunRequest struct {
	Input           string
	SystemPrompt    string
	ProfileName     string
	SessionID       string
	RunID           string
	ApprovedTools   []string
	History         []memory.Turn
	MemorySnapshot  string
	OnStepStart     func(stepIndex int)
	OnToolCallStart func(toolName string)
	OnToolCallEnd   func(toolName string, succeeded bool)
	OnTodosUpdated  func(todos []tools.TodoItem)
	ChildAgentID    string
	ChildTaskID     string
}

type RunResult struct {
	Output       string
	Steps        []engine.StepRecord
	Trace        engine.RunTrace
	SystemPrompt string
	ProfileName  string
	Events       []observe.Event
}

type Runtime struct {
	runner              engine.Runner
	tools               []tools.Definition
	maxObservationBytes int
	offload             engine.OffloadConfig
	mcpManager          *mcp.Manager
	skillLoader         *skills.Loader
	profileDir          string
	memoryStore         *memory.LongTermStore
	taskStore           *tasks.Store
	agentStore          *agents.Store
	backgroundStore     *background.Store
	worktreeManager     *worktree.Manager
}

func (r *Runtime) Run(ctx context.Context, req RunRequest) (RunResult, error) {
	prepared, err := r.prepareRun(req)
	if err != nil {
		return RunResult{}, err
	}
	recorder := observe.NewRecorder()
	recorder.Add("run_start", "starting runtime run", map[string]string{
		"profile": prepared.profileName,
		"session": req.SessionID,
	})

	skillSession, err := NewSkillSession(prepared.skillLoader, prepared.systemPrompt, prepared.toolDefs)
	if err != nil {
		return RunResult{}, err
	}
	capabilitySession := NewCapabilitySession(skillSession, r.mcpManager)
	ctx = tools.WithActivateSkillProvider(ctx, skillSession)
	ctx = tools.WithCapabilityProvider(ctx, capabilitySession)

	toolDefs, err := skillSession.ToolDefs()
	if err != nil {
		return RunResult{}, err
	}
	taskProvider := runtimeTaskProvider{store: r.taskStore}
	agentProvider := runtimeAgentProvider{runtime: r, parent: req, prepared: prepared, toolDefs: toolDefs}
	ctx = tools.WithTaskProvider(ctx, taskProvider)
	ctx = tools.WithAgentProvider(ctx, agentProvider)
	ctx = tools.WithWorktreeProvider(ctx, runtimeWorktreeProvider{manager: r.worktreeManager})
	ctx = tools.WithBackgroundStore(ctx, r.backgroundStore)

	result, err := r.runner.Run(ctx, engine.RunInput{
		Input:          req.Input,
		SystemPrompt:   skillSession.SystemPrompt(),
		RuntimeContext: prepared.runtimeContext,
		ToolDefs:       toolDefs,
		Config: engine.RunConfig{
			MaxObservationBytes: r.maxObservationBytes,
			Offload:             prepared.offload,
		},
		Hooks: prepared.hookPipeline(req, recorder),
	})
	if err != nil {
		recorder.Add("run_error", err.Error(), map[string]string{
			"category": observe.ClassifyError(err),
		})
		return RunResult{
			Steps:        result.Steps,
			Trace:        result.Trace,
			SystemPrompt: skillSession.SystemPrompt(),
			ProfileName:  prepared.profileName,
			Events:       recorder.Events(),
		}, err
	}
	recorder.Add("run_completed", "run completed", map[string]string{
		"profile": prepared.profileName,
	})

	return RunResult{
		Output:       result.Output,
		Steps:        result.Steps,
		Trace:        result.Trace,
		SystemPrompt: skillSession.SystemPrompt(),
		ProfileName:  prepared.profileName,
		Events:       recorder.Events(),
	}, nil
}

func (r *Runtime) Close() error {
	if r.mcpManager == nil {
		return nil
	}
	return r.mcpManager.Close()
}

func (r *Runtime) MemoryStore() *memory.LongTermStore {
	return r.memoryStore
}

func (r *Runtime) ListResources() []mcp.ResourceInfo {
	if r.mcpManager == nil {
		return nil
	}
	return r.mcpManager.ListResources()
}

func ensureSkillLoader(loader *skills.Loader) *skills.Loader {
	if loader != nil {
		return loader
	}
	return skills.NewLoader("")
}

type preparedRun struct {
	systemPrompt   string
	runtimeContext string
	profileName    string
	toolDefs       []tools.Definition
	skillLoader    *skills.Loader
	outputSchema   string
	policy         *policy.Engine
	offload        engine.OffloadConfig
	policyRules    []policy.Rule
	background     *background.Store
	agentStore     *agents.Store
}

func (r *Runtime) prepareRun(req RunRequest) (preparedRun, error) {
	baseSkillLoader := ensureSkillLoader(r.skillLoader)
	prepared := preparedRun{
		systemPrompt: req.SystemPrompt,
		toolDefs:     append([]tools.Definition(nil), r.tools...),
		skillLoader:  baseSkillLoader,
		policy:       policy.New(req.ApprovedTools, nil),
		offload:      r.offload,
		background:   r.backgroundStore,
		agentStore:   r.agentStore,
	}
	prepared.offload.RunID = req.RunID
	if req.ProfileName == "" {
		prepared.runtimeContext = assembleRuntimeContext(memory.Build(req.History, memory.Strategy{}), req.MemorySnapshot)
		return prepared, nil
	}

	loaded, err := profile.LoadByName(r.profileDir, req.ProfileName)
	if err != nil {
		return preparedRun{}, err
	}
	resolved := profile.Resolve(loaded)

	prepared.profileName = resolved.Name
	prepared.systemPrompt = resolved.SystemPrompt
	prepared.toolDefs = filterToolDefinitions(r.tools, resolved.EnabledToolSet)
	prepared.skillLoader = baseSkillLoader.WithAllowedNames(resolved.EnabledSkills)
	if len(resolved.OutputSchema) > 0 {
		if err := json.Unmarshal(resolved.OutputSchema, &prepared.outputSchema); err != nil {
			log.Printf("WARNING: output_schema is not a JSON string, using raw value: %v", err)
			prepared.outputSchema = strings.TrimSpace(string(resolved.OutputSchema))
		}
	}
	if len(resolved.PolicyRules) > 0 {
		if err := json.Unmarshal(resolved.PolicyRules, &prepared.policyRules); err != nil {
			return preparedRun{}, err
		}
	}
	prepared.policy = policy.New(req.ApprovedTools, nil, prepared.policyRules...)
	memoryResult := memory.Build(req.History, parseMemoryStrategy(resolved.MemoryStrategy))
	prepared.runtimeContext = assembleRuntimeContext(memoryResult, req.MemorySnapshot)
	return prepared, nil
}

func filterToolDefinitions(defs []tools.Definition, allowed map[string]struct{}) []tools.Definition {
	filtered := make([]tools.Definition, 0, len(defs))
	for _, def := range defs {
		if _, ok := allowed[def.Name]; ok {
			filtered = append(filtered, def)
		}
	}
	return filtered
}

func (p preparedRun) hookPipeline(req RunRequest, recorder *observe.Recorder) engine.HookPipeline {
	return engine.NewHookPipeline(
		engine.HookHandlerFunc{HandlerName: "runtime_callbacks", Fn: runtimeCallbackHook(req)},
		engine.HookHandlerFunc{HandlerName: "background_notifications", Fn: p.backgroundNotificationHook()},
		engine.HookHandlerFunc{HandlerName: "policy", Fn: p.policyHook(req, recorder)},
		engine.HookHandlerFunc{HandlerName: "output_validator", Fn: p.finalAnswerHook(recorder)},
	)
}

func (p preparedRun) backgroundNotificationHook() func(ctx context.Context, event engine.HookContext) (engine.HookDecision, error) {
	return func(ctx context.Context, event engine.HookContext) (engine.HookDecision, error) {
		_ = ctx
		if event.Event != engine.HookBeforeModelCall || p.background == nil {
			return engine.HookDecision{Kind: engine.HookDecisionContinue}, nil
		}
		jobs := p.background.UnconsumedFinished()
		if len(jobs) == 0 {
			return engine.HookDecision{Kind: engine.HookDecisionContinue}, nil
		}
		data, err := json.MarshalIndent(jobs, "", "  ")
		if err != nil {
			return engine.HookDecision{}, err
		}
		return engine.HookDecision{
			Kind:        engine.HookDecisionInjectMessage,
			MessageRole: "system",
			Message:     "<background_notifications>\n" + string(data) + "\n</background_notifications>",
			Reason:      "background jobs finished",
		}, nil
	}
}

func runtimeCallbackHook(req RunRequest) func(ctx context.Context, event engine.HookContext) (engine.HookDecision, error) {
	return func(ctx context.Context, event engine.HookContext) (engine.HookDecision, error) {
		_ = ctx
		switch event.Event {
		case engine.HookBeforeModelCall:
			if req.OnStepStart != nil {
				req.OnStepStart(event.StepIndex)
			}
		case engine.HookPreToolUse:
			if req.OnToolCallStart != nil {
				req.OnToolCallStart(event.ToolName)
			}
		case engine.HookPostToolUse:
			if req.OnToolCallEnd != nil {
				req.OnToolCallEnd(event.ToolName, event.Err == nil)
			}
			if event.ToolName == tools.WriteTodosToolName && req.OnTodosUpdated != nil && event.State != nil {
				req.OnTodosUpdated(event.State.Todos)
			}
		}
		return engine.HookDecision{Kind: engine.HookDecisionContinue}, nil
	}
}

func (p preparedRun) policyHook(req RunRequest, recorder *observe.Recorder) func(ctx context.Context, event engine.HookContext) (engine.HookDecision, error) {
	return func(ctx context.Context, event engine.HookContext) (engine.HookDecision, error) {
		_ = ctx
		if event.Event != engine.HookPreToolUse || event.ToolDef == nil || event.Action == nil {
			return engine.HookDecision{Kind: engine.HookDecisionContinue}, nil
		}
		decision, reason := p.policy.DecideRequest(policy.Request{
			Tool:    *event.ToolDef,
			Args:    event.Action.Arguments,
			Profile: p.profileName,
		})
		if decision == policy.DecisionAllow || decision == policy.DecisionPassthrough {
			recorder.Add("tool_allowed", "tool allowed", map[string]string{"tool": event.ToolDef.Name})
			return engine.HookDecision{Kind: engine.HookDecisionContinue}, nil
		}
		recorder.Add("tool_denied", reason, map[string]string{
			"tool":     event.ToolDef.Name,
			"decision": string(decision),
		})
		if decision == policy.DecisionAsk && req.ChildAgentID != "" && p.agentStore != nil {
			_, _ = p.agentStore.AppendMessage(agents.MailboxMessage{
				From:    req.ChildAgentID,
				To:      leadAgentID,
				Kind:    "permission_request",
				Content: permissionRequestContent(event.ToolDef.Name, event.Action.Arguments, reason, req.ChildTaskID),
			})
		}
		return engine.HookDecision{
			Kind:        engine.HookDecisionBlockWithObservation,
			Observation: "tool error: " + reason,
			Reason:      reason,
		}, nil
	}
}

func (p preparedRun) finalAnswerHook(recorder *observe.Recorder) func(ctx context.Context, event engine.HookContext) (engine.HookDecision, error) {
	return func(ctx context.Context, event engine.HookContext) (engine.HookDecision, error) {
		_ = ctx
		if event.Event != engine.HookBeforeFinalAnswer {
			return engine.HookDecision{Kind: engine.HookDecisionContinue}, nil
		}
		if err := validator.ValidateOutput(p.outputSchema, event.Content); err != nil {
			recorder.Add("output_validation_failed", err.Error(), map[string]string{"schema": p.outputSchema})
			return engine.HookDecision{
				Kind:        engine.HookDecisionBlockWithObservation,
				Observation: err.Error(),
				Reason:      err.Error(),
			}, nil
		}
		if p.outputSchema != "" {
			recorder.Add("output_validation_passed", "output schema validated", map[string]string{"schema": p.outputSchema})
		}
		return engine.HookDecision{Kind: engine.HookDecisionContinue}, nil
	}
}

func permissionRequestContent(toolName string, args json.RawMessage, reason string, taskID string) string {
	payload := map[string]string{
		"tool":             toolName,
		"argument_summary": truncateString(string(args), 500),
		"risk_reason":      reason,
	}
	if taskID != "" {
		payload["task_id"] = taskID
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return reason
	}
	return string(data)
}

func truncateString(value string, max int) string {
	value = strings.TrimSpace(value)
	if max <= 0 || len(value) <= max {
		return value
	}
	return value[:max] + "...[truncated]"
}

func parseMemoryStrategy(raw json.RawMessage) memory.Strategy {
	if len(raw) == 0 {
		return memory.Strategy{}
	}
	var strategy memory.Strategy
	if err := json.Unmarshal(raw, &strategy); err != nil {
		log.Printf("WARNING: failed to parse memory_strategy JSON, using defaults: %v", err)
		return memory.Strategy{}
	}
	return strategy
}

func assembleRuntimeContext(memoryResult memory.BuildResult, memorySnapshot string) string {
	parts := make([]string, 0, 2)
	if strings.TrimSpace(memoryResult.Text) != "" {
		parts = append(parts, memoryResult.Text)
	}
	if strings.TrimSpace(memorySnapshot) != "" {
		parts = append(parts, memorySnapshot)
	}
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}
