package runtime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"happyagent/internal/agents"
	"happyagent/internal/engine"
	"happyagent/internal/memory"
	"happyagent/internal/observe"
	"happyagent/internal/policy"
	"happyagent/internal/protocol"
	"happyagent/internal/skills"
	"happyagent/internal/tasks"
	"happyagent/internal/tools"
)

func TestPrepareRunAppliesProfilePromptToolsAndSkills(t *testing.T) {
	root := t.TempDir()
	skillsDir := filepath.Join(root, "skills")
	profilesDir := filepath.Join(root, "profiles")
	writeTestSkill(t, skillsDir, "allowed-skill", "allowed", "allowed prompt")
	writeTestSkill(t, skillsDir, "blocked-skill", "blocked", "blocked prompt")
	writeTestProfile(t, profilesDir, "career-copilot", `{
  "name": "career-copilot",
  "system_prompt": "career prompt",
  "enabled_tools": ["final_answer", "file_read"],
  "enabled_skills": ["allowed-skill"]
}`)

	rt := &Runtime{
		tools: []tools.Definition{
			{Name: tools.FinalAnswerToolName},
			{Name: "file_read"},
			{Name: "shell"},
		},
		skillLoader: skills.NewLoader(skillsDir),
		profileDir:  profilesDir,
	}

	prepared, err := rt.prepareRun(RunRequest{
		Input:        "help",
		SystemPrompt: "base prompt",
		ProfileName:  "career-copilot",
	})
	if err != nil {
		t.Fatalf("prepareRun() error = %v", err)
	}
	if prepared.systemPrompt != "career prompt" {
		t.Fatalf("unexpected system prompt: %q", prepared.systemPrompt)
	}
	if len(prepared.toolDefs) != 2 || prepared.toolDefs[0].Name != tools.FinalAnswerToolName || prepared.toolDefs[1].Name != "file_read" {
		t.Fatalf("unexpected tool defs: %+v", prepared.toolDefs)
	}

	catalog, err := prepared.skillLoader.LoadCatalog()
	if err != nil {
		t.Fatalf("LoadCatalog() error = %v", err)
	}
	if len(catalog) != 1 || catalog[0].Name != "allowed-skill" {
		t.Fatalf("unexpected catalog: %+v", catalog)
	}
	if _, err := prepared.skillLoader.Load("blocked-skill"); err == nil {
		t.Fatalf("expected blocked skill to be unavailable")
	}
}

func TestPrepareRunKeepsMCPReadResourceWhenProfileEnablesIt(t *testing.T) {
	root := t.TempDir()
	profilesDir := filepath.Join(root, "profiles")
	writeTestProfile(t, profilesDir, "general-assistant", `{
  "name": "general-assistant",
  "system_prompt": "general prompt",
  "enabled_tools": ["final_answer", "mcp_read_resource"],
  "enabled_skills": []
}`)

	rt := &Runtime{
		tools: []tools.Definition{
			{Name: tools.FinalAnswerToolName},
			{Name: "file_read"},
			{Name: "mcp_read_resource"},
		},
		skillLoader: skills.NewLoader(filepath.Join(root, "skills")),
		profileDir:  profilesDir,
	}

	prepared, err := rt.prepareRun(RunRequest{
		Input:        "read resource",
		SystemPrompt: "base prompt",
		ProfileName:  "general-assistant",
	})
	if err != nil {
		t.Fatalf("prepareRun() error = %v", err)
	}
	if len(prepared.toolDefs) != 2 || prepared.toolDefs[0].Name != tools.FinalAnswerToolName || prepared.toolDefs[1].Name != "mcp_read_resource" {
		t.Fatalf("unexpected tool defs: %+v", prepared.toolDefs)
	}
}

func TestRunReturnsResolvedProfileMetadata(t *testing.T) {
	root := t.TempDir()
	profilesDir := filepath.Join(root, "profiles")
	writeTestProfile(t, profilesDir, "general-assistant", `{
  "name": "general-assistant",
  "system_prompt": "general prompt",
  "enabled_tools": ["final_answer"],
  "enabled_skills": []
}`)

	rt := &Runtime{
		runner: &stubRunner{
			result: engine.RunResult{Output: "done"},
		},
		tools:       []tools.Definition{{Name: tools.FinalAnswerToolName}},
		skillLoader: skills.NewLoader(filepath.Join(root, "skills")),
		profileDir:  profilesDir,
	}

	result, err := rt.Run(context.Background(), RunRequest{
		Input:        "hello",
		SystemPrompt: "base prompt",
		ProfileName:  "general-assistant",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ProfileName != "general-assistant" {
		t.Fatalf("unexpected profile name: %q", result.ProfileName)
	}
	if result.SystemPrompt != "general prompt" {
		t.Fatalf("unexpected system prompt: %q", result.SystemPrompt)
	}
}

func TestRuntimeAgentProviderClaimsAndCompletesTask(t *testing.T) {
	root := t.TempDir()
	taskStore, err := tasks.NewStore(root)
	if err != nil {
		t.Fatalf("NewStore(tasks) error = %v", err)
	}
	task, err := taskStore.Create(tasks.Task{ID: "task-a", Title: "Task A"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	agentStore, err := agents.NewStore(root)
	if err != nil {
		t.Fatalf("NewStore(agents) error = %v", err)
	}
	rt := &Runtime{
		runner: &stubRunner{result: engine.RunResult{Output: "child summary"}},
		tools: []tools.Definition{
			{Name: tools.FinalAnswerToolName},
		},
		skillLoader: skills.NewLoader(filepath.Join(root, "skills")),
		taskStore:   taskStore,
		agentStore:  agentStore,
	}
	provider := runtimeAgentProvider{runtime: rt, parent: RunRequest{}, prepared: preparedRun{}}
	_, _, _, err = provider.runChild(context.Background(), agentRunInput{
		AgentID: "child-a",
		Name:    "child",
		Prompt:  "do task",
		TaskID:  task.ID,
	}, false)
	if err != nil {
		t.Fatalf("runChild() error = %v", err)
	}
	completed, err := taskStore.Get(task.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if completed.Status != tasks.StatusCompleted || completed.Owner != "child-a" {
		t.Fatalf("expected completed child-owned task, got %+v", completed)
	}
}

func TestPrepareRunAddsMemoryToRuntimeContext(t *testing.T) {
	root := t.TempDir()
	profilesDir := filepath.Join(root, "profiles")
	writeTestProfile(t, profilesDir, "general-assistant", `{
  "name": "general-assistant",
  "system_prompt": "general prompt",
  "enabled_tools": ["final_answer"],
  "enabled_skills": [],
  "memory_strategy": {
    "enabled": true,
    "max_turns": 2,
    "max_chars": 200
  }
}`)

	rt := &Runtime{
		tools:       []tools.Definition{{Name: tools.FinalAnswerToolName}},
		skillLoader: skills.NewLoader(filepath.Join(root, "skills")),
		profileDir:  profilesDir,
	}

	prepared, err := rt.prepareRun(RunRequest{
		Input:       "hello",
		ProfileName: "general-assistant",
		History: []memory.Turn{
			{Role: "user", Content: "first"},
			{Role: "assistant", Content: "second"},
		},
	})
	if err != nil {
		t.Fatalf("prepareRun() error = %v", err)
	}
	if prepared.systemPrompt != "general prompt" {
		t.Fatalf("unexpected system prompt: %q", prepared.systemPrompt)
	}
	if !strings.Contains(prepared.runtimeContext, "Recent session turns") {
		t.Fatalf("expected memory in runtime context: %q", prepared.runtimeContext)
	}
}

func TestPrepareRunAppliesToolScopeAndSuppressesMemoryWhenHistoryEmpty(t *testing.T) {
	root := t.TempDir()
	profilesDir := filepath.Join(root, "profiles")
	writeTestProfile(t, profilesDir, "career-copilot", `{
  "name": "career-copilot",
  "system_prompt": "career prompt",
  "enabled_tools": ["final_answer", "file_read", "file_write"],
  "enabled_skills": [],
  "memory_strategy": {
    "enabled": true,
    "max_turns": 2,
    "max_chars": 200
  }
}`)

	rt := &Runtime{
		tools: []tools.Definition{
			{Name: tools.FinalAnswerToolName},
			{Name: tools.FileReadToolName},
			{Name: "file_write"},
		},
		skillLoader: skills.NewLoader(filepath.Join(root, "skills")),
		profileDir:  profilesDir,
	}

	prepared, err := rt.prepareRun(RunRequest{
		Input:       "structured",
		ProfileName: "career-copilot",
		ToolScope:   []string{tools.FileReadToolName, tools.FinalAnswerToolName},
	})
	if err != nil {
		t.Fatalf("prepareRun() error = %v", err)
	}
	var names []string
	for _, def := range prepared.toolDefs {
		names = append(names, def.Name)
	}
	if strings.Join(names, ",") != "final_answer,file_read" {
		t.Fatalf("tool defs = %v, want final_answer,file_read", names)
	}
	if strings.Contains(prepared.runtimeContext, "Recent session turns") {
		t.Fatalf("expected no runtime history context, got %q", prepared.runtimeContext)
	}
}

func TestPrepareRunDoesNotInjectRAGIntoPrompt(t *testing.T) {
	root := t.TempDir()
	profilesDir := filepath.Join(root, "profiles")
	writeTestProfile(t, profilesDir, "general-assistant", `{
  "name": "general-assistant",
  "system_prompt": "general prompt",
  "enabled_tools": ["final_answer", "search_docs"],
  "enabled_skills": []
}`)

	rt := &Runtime{
		tools: []tools.Definition{
			{Name: tools.FinalAnswerToolName},
			{Name: tools.SearchDocsToolName},
		},
		skillLoader: skills.NewLoader(filepath.Join(root, "skills")),
		profileDir:  profilesDir,
	}

	prepared, err := rt.prepareRun(RunRequest{
		Input:       "architecture references",
		ProfileName: "general-assistant",
	})
	if err != nil {
		t.Fatalf("prepareRun() error = %v", err)
	}
	if strings.Contains(prepared.systemPrompt, "Relevant local references") {
		t.Fatalf("expected RAG content to stay out of system prompt: %q", prepared.systemPrompt)
	}
	if len(prepared.toolDefs) != 2 || prepared.toolDefs[1].Name != tools.SearchDocsToolName {
		t.Fatalf("expected search_docs to remain available as a tool: %+v", prepared.toolDefs)
	}
}

func TestPreparedRunPolicyRequiresApprovalForDangerousTool(t *testing.T) {
	prepared := preparedRun{
		policy: policyEngineForTest(),
	}
	recorder := observe.NewRecorder()
	action := engine.Action{
		Type:      protocol.ActionToolCall,
		ToolName:  "shell",
		Arguments: []byte(`{}`),
	}
	toolDef := tools.Definition{Name: "shell", Dangerous: true}
	decision, err := prepared.policyHook(RunRequest{}, recorder)(context.Background(), engine.HookContext{
		Event:   engine.HookPreToolUse,
		Action:  &action,
		ToolDef: &toolDef,
	})
	if err != nil {
		t.Fatalf("policyHook() error = %v", err)
	}
	if decision.Kind != engine.HookDecisionBlockWithObservation || !strings.Contains(decision.Observation, "approval required") {
		t.Fatalf("unexpected guard result: %+v", decision)
	}
}

func TestPreparedRunSourceBoundFileReadPolicy(t *testing.T) {
	prepared := preparedRun{
		policy: policyEngineForTest(),
	}
	recorder := observe.NewRecorder()
	toolDef := tools.Definition{Name: tools.FileReadToolName}

	allowed, err := prepared.policyHook(RunRequest{SourceReadPaths: []string{"inbox/source.md"}}, recorder)(context.Background(), engine.HookContext{
		Event:   engine.HookPreToolUse,
		Action:  &engine.Action{Type: protocol.ActionToolCall, ToolName: tools.FileReadToolName, Arguments: []byte(`{"path":"inbox/source.md"}`)},
		ToolDef: &toolDef,
	})
	if err != nil {
		t.Fatalf("policyHook() allow error = %v", err)
	}
	if allowed.Kind != engine.HookDecisionContinue {
		t.Fatalf("expected declared path to continue, got %+v", allowed)
	}

	blocked, err := prepared.policyHook(RunRequest{SourceReadPaths: []string{"inbox/source.md"}}, recorder)(context.Background(), engine.HookContext{
		Event:   engine.HookPreToolUse,
		Action:  &engine.Action{Type: protocol.ActionToolCall, ToolName: tools.FileReadToolName, Arguments: []byte(`{"path":"inbox/other.md"}`)},
		ToolDef: &toolDef,
	})
	if err != nil {
		t.Fatalf("policyHook() block error = %v", err)
	}
	if blocked.Kind != engine.HookDecisionBlockWithObservation || !strings.Contains(blocked.Observation, "not a declared source path") {
		t.Fatalf("expected undeclared path block, got %+v", blocked)
	}
}

func TestPreparedRunFinalAnswerRequiresDeclaredSourceReads(t *testing.T) {
	prepared := preparedRun{}
	recorder := observe.NewRecorder()
	state := &engine.LoopState{Messages: []engine.MessageEnvelope{
		{
			Actions: []engine.Action{{
				Type:       protocol.ActionToolCall,
				ToolName:   tools.FileReadToolName,
				ToolCallID: "call_source",
				Arguments:  []byte(`{"path":"inbox/source.md"}`),
			}},
		},
		{
			Role:       protocol.RoleTool,
			ToolName:   tools.FileReadToolName,
			ToolCallID: "call_source",
			Content:    "# source",
		},
	}}
	decision, err := prepared.finalAnswerHook(RunRequest{
		RequireSourceReads: true,
		SourceReadPaths:    []string{"inbox/source.md", "inbox/other.md"},
	}, recorder)(context.Background(), engine.HookContext{
		Event:   engine.HookBeforeFinalAnswer,
		State:   state,
		Content: "{}",
	})
	if err != nil {
		t.Fatalf("finalAnswerHook() error = %v", err)
	}
	if decision.Kind != engine.HookDecisionBlockWithObservation || !strings.Contains(decision.Observation, "inbox/other.md") {
		t.Fatalf("expected missing source read block, got %+v", decision)
	}
}

func TestPreparedRunFinalAnswerDoesNotCountFailedSourceRead(t *testing.T) {
	prepared := preparedRun{}
	recorder := observe.NewRecorder()
	state := &engine.LoopState{Messages: []engine.MessageEnvelope{
		{
			Actions: []engine.Action{{
				Type:       protocol.ActionToolCall,
				ToolName:   tools.FileReadToolName,
				ToolCallID: "call_source",
				Arguments:  []byte(`{"path":"inbox/source.md"}`),
			}},
		},
		{
			Role:       protocol.RoleTool,
			ToolName:   tools.FileReadToolName,
			ToolCallID: "call_source",
			Content:    `tool error: read file "career-workspace/career-workspace/inbox/source.md": no such file or directory`,
		},
	}}
	decision, err := prepared.finalAnswerHook(RunRequest{
		RequireSourceReads: true,
		SourceReadPaths:    []string{"inbox/source.md"},
	}, recorder)(context.Background(), engine.HookContext{
		Event:   engine.HookBeforeFinalAnswer,
		State:   state,
		Content: "{}",
	})
	if err != nil {
		t.Fatalf("finalAnswerHook() error = %v", err)
	}
	if decision.Kind != engine.HookDecisionBlockWithObservation || !strings.Contains(decision.Observation, "inbox/source.md") {
		t.Fatalf("expected failed source read to be missing, got %+v", decision)
	}
}

func TestPreparedRunPolicyBubblesChildPermissionRequest(t *testing.T) {
	store, err := agents.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	prepared := preparedRun{
		policy:     policyEngineForTest(),
		agentStore: store,
	}
	recorder := observe.NewRecorder()
	action := engine.Action{
		Type:      protocol.ActionToolCall,
		ToolName:  "shell",
		Arguments: []byte(`{"argv":["git","status"]}`),
	}
	toolDef := tools.Definition{Name: "shell", Dangerous: true}
	decision, err := prepared.policyHook(RunRequest{ChildAgentID: "child-a", ChildTaskID: "task-a"}, recorder)(context.Background(), engine.HookContext{
		Event:   engine.HookPreToolUse,
		Action:  &action,
		ToolDef: &toolDef,
	})
	if err != nil {
		t.Fatalf("policyHook() error = %v", err)
	}
	if decision.Kind != engine.HookDecisionBlockWithObservation {
		t.Fatalf("unexpected decision: %+v", decision)
	}
	messages, err := store.ListUnconsumed(leadAgentID)
	if err != nil {
		t.Fatalf("ListUnconsumed() error = %v", err)
	}
	if len(messages) != 1 || messages[0].Kind != "permission_request" || !strings.Contains(messages[0].Content, `"task_id":"task-a"`) {
		t.Fatalf("unexpected permission request: %+v", messages)
	}
}

func TestPreparedRunValidateFinalAnswerRejectsInvalidCareerReport(t *testing.T) {
	prepared := preparedRun{outputSchema: "career_report"}
	recorder := observe.NewRecorder()
	decision, err := prepared.finalAnswerHook(RunRequest{}, recorder)(context.Background(), engine.HookContext{
		Event:   engine.HookBeforeFinalAnswer,
		Content: `{"summary":"ok"}`,
	})
	if err != nil {
		t.Fatalf("finalAnswerHook() error = %v", err)
	}
	if decision.Kind != engine.HookDecisionBlockWithObservation || !strings.Contains(decision.Observation, "missing field") {
		t.Fatalf("unexpected validation decision: %+v", decision)
	}
}

type stubRunner struct {
	result engine.RunResult
}

func (s *stubRunner) Run(ctx context.Context, input engine.RunInput) (engine.RunResult, error) {
	return s.result, nil
}

func policyEngineForTest() *policy.Engine {
	return policy.New(nil, nil)
}

func writeTestProfile(t *testing.T, root string, name string, content string) {
	t.Helper()

	profileDir := filepath.Join(root, name)
	if err := os.MkdirAll(profileDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(profileDir, "profile.json"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}
