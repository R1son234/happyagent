package career

import (
	"testing"

	"happyagent/internal/config"
)

func TestRepoArgParsesAnalyzeRepo(t *testing.T) {
	if got := repoArg([]string{"career", "analyze", "--repo", "examples"}, "."); got != "examples" {
		t.Fatalf("unexpected repo arg: %q", got)
	}
	if got := repoArg([]string{"career", "analyze", "--repo=examples"}, "."); got != "examples" {
		t.Fatalf("unexpected repo arg: %q", got)
	}
	if got := repoArg([]string{"career", "rewrite-resume", "--report", "out.json"}, "."); got != "." {
		t.Fatalf("unexpected repo arg for transform: %q", got)
	}
}

func TestShouldLaunchByDefault(t *testing.T) {
	if !ShouldLaunchByDefault(nil) {
		t.Fatalf("expected default startup with no args to launch career mode")
	}
	if ShouldLaunchByDefault([]string{"--profile", "general-assistant"}) {
		t.Fatalf("did not expect explicit args to launch career mode by default")
	}
}

func TestPrepareConfigRaisesTimeoutsBeforeRuntimeBuild(t *testing.T) {
	cfg := config.Default()
	cfg.Engine.RunTimeoutSeconds = 1
	cfg.LLM.TimeoutSeconds = 1

	PrepareConfig(&cfg, []string{"career"})

	if cfg.Engine.RunTimeoutSeconds < MinAnalyzeTimeoutSeconds {
		t.Fatalf("expected run timeout to be raised for career mode, got %d", cfg.Engine.RunTimeoutSeconds)
	}
	if cfg.LLM.TimeoutSeconds < MinAnalyzeTimeoutSeconds {
		t.Fatalf("expected llm timeout to be raised for career mode, got %d", cfg.LLM.TimeoutSeconds)
	}
}
