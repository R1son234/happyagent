package runtime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"happyagent/internal/engine"
	"happyagent/internal/memory"
	"happyagent/internal/observe"
	"happyagent/internal/policy"
	"happyagent/internal/protocol"
	"happyagent/internal/skills"
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

func TestPrepareRunInjectsMemoryIntoPrompt(t *testing.T) {
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
	if !strings.Contains(prepared.systemPrompt, "Recent session memory") {
		t.Fatalf("expected memory in prompt: %q", prepared.systemPrompt)
	}
}

func TestPreparedRunPolicyRequiresApprovalForDangerousTool(t *testing.T) {
	prepared := preparedRun{
		policy: policyEngineForTest(),
	}
	recorder := observe.NewRecorder()
	observation, handled, err := prepared.beforeToolCall(recorder)(context.Background(), engine.Action{
		Type:     protocol.ActionToolCall,
		ToolName: "shell",
	}, &engine.RunInput{
		ToolDefs: []tools.Definition{{Name: "shell", Dangerous: true}},
	})
	if err != nil {
		t.Fatalf("beforeToolCall() error = %v", err)
	}
	if !handled || !strings.Contains(observation, "approval required") {
		t.Fatalf("unexpected guard result: handled=%v observation=%q", handled, observation)
	}
}

func TestPreparedRunValidateFinalAnswerRejectsInvalidCareerReport(t *testing.T) {
	prepared := preparedRun{outputSchema: "career_report"}
	recorder := observe.NewRecorder()
	err := prepared.validateFinalAnswer(recorder)(`{"summary":"ok"}`)
	if err == nil || !strings.Contains(err.Error(), "missing field") {
		t.Fatalf("unexpected validation error: %v", err)
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
