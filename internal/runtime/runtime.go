package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"happyagent/internal/engine"
	"happyagent/internal/mcp"
	"happyagent/internal/memory"
	"happyagent/internal/observe"
	"happyagent/internal/policy"
	"happyagent/internal/profile"
	"happyagent/internal/skills"
	"happyagent/internal/tools"
	"happyagent/internal/validator"
)

type RunRequest struct {
	Input         string
	SystemPrompt  string
	ProfileName   string
	SessionID     string
	ApprovedTools []string
	History       []memory.Turn
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
	mcpManager          *mcp.Manager
	skillLoader         *skills.Loader
	profileDir          string
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

	result, err := r.runner.Run(ctx, engine.RunInput{
		Input:               req.Input,
		SystemPrompt:        skillSession.SystemPrompt(),
		RuntimeContext:      prepared.runtimeContext,
		ToolDefs:            toolDefs,
		MaxObservationBytes: r.maxObservationBytes,
		BeforeToolCall:      prepared.beforeToolCall(recorder),
		ValidateFinalAnswer: prepared.validateFinalAnswer(recorder),
	})
	if err != nil {
		recorder.Add("run_error", err.Error(), map[string]string{
			"category": observe.ClassifyError(err),
		})
		return RunResult{}, err
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

func validateSkillDir(dir string) error {
	if dir == "" {
		return fmt.Errorf("skills dir must not be empty")
	}
	return nil
}

type preparedRun struct {
	systemPrompt   string
	runtimeContext string
	profileName    string
	toolDefs       []tools.Definition
	skillLoader    *skills.Loader
	outputSchema   string
	policy         *policy.Engine
}

func (r *Runtime) prepareRun(req RunRequest) (preparedRun, error) {
	baseSkillLoader := ensureSkillLoader(r.skillLoader)
	prepared := preparedRun{
		systemPrompt: req.SystemPrompt,
		toolDefs:     append([]tools.Definition(nil), r.tools...),
		skillLoader:  baseSkillLoader,
		policy:       policy.New(req.ApprovedTools, nil),
	}
	if req.ProfileName == "" {
		prepared.runtimeContext = assembleRuntimeContext(memory.Build(req.History, memory.Strategy{}))
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
			prepared.outputSchema = strings.TrimSpace(string(resolved.OutputSchema))
		}
	}
	memoryResult := memory.Build(req.History, parseMemoryStrategy(resolved.MemoryStrategy))
	prepared.runtimeContext = assembleRuntimeContext(memoryResult)
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

func (p preparedRun) beforeToolCall(recorder *observe.Recorder) func(ctx context.Context, action engine.Action, input *engine.RunInput) (string, bool, error) {
	return func(ctx context.Context, action engine.Action, input *engine.RunInput) (string, bool, error) {
		for _, def := range input.ToolDefs {
			if def.Name != action.ToolName {
				continue
			}
			decision, reason := p.policy.Decide(def)
			if decision == policy.DecisionAllow {
				recorder.Add("tool_allowed", "tool allowed", map[string]string{"tool": def.Name})
				return "", false, nil
			}
			recorder.Add("tool_denied", reason, map[string]string{
				"tool":     def.Name,
				"decision": string(decision),
			})
			return "tool error: " + reason, true, nil
		}
		return "", false, nil
	}
}

func (p preparedRun) validateFinalAnswer(recorder *observe.Recorder) func(content string) error {
	return func(content string) error {
		if err := validator.ValidateOutput(p.outputSchema, content); err != nil {
			recorder.Add("output_validation_failed", err.Error(), map[string]string{
				"schema": p.outputSchema,
			})
			return err
		}
		if p.outputSchema != "" {
			recorder.Add("output_validation_passed", "output schema validated", map[string]string{
				"schema": p.outputSchema,
			})
		}
		return nil
	}
}

func parseMemoryStrategy(raw json.RawMessage) memory.Strategy {
	if len(raw) == 0 {
		return memory.Strategy{}
	}
	var strategy memory.Strategy
	if err := json.Unmarshal(raw, &strategy); err != nil {
		return memory.Strategy{}
	}
	return strategy
}

func assembleRuntimeContext(memoryResult memory.BuildResult) string {
	parts := make([]string, 0, 1)
	if strings.TrimSpace(memoryResult.Text) != "" {
		parts = append(parts, memoryResult.Text)
	}
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}
