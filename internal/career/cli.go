package career

import (
	"strings"

	"happyagent/internal/config"
)

func IsCommand(args []string) bool {
	return len(args) > 0 && args[0] == "career"
}

func ShouldLaunchByDefault(args []string) bool {
	return len(args) == 0
}

func PrepareConfig(cfg *config.Config, args []string) {
	cfg.Tools.RootDir = repoArg(args, cfg.Tools.RootDir)
	if cfg.Engine.LoopMaxSteps < MinAnalyzeLoopSteps {
		cfg.Engine.LoopMaxSteps = MinAnalyzeLoopSteps
	}
	if cfg.Engine.RunTimeoutSeconds < MinAnalyzeTimeoutSeconds {
		cfg.Engine.RunTimeoutSeconds = MinAnalyzeTimeoutSeconds
	}
	if cfg.LLM.TimeoutSeconds < MinAnalyzeTimeoutSeconds {
		cfg.LLM.TimeoutSeconds = MinAnalyzeTimeoutSeconds
	}
}

func repoArg(args []string, fallback string) string {
	if len(args) < 2 || args[1] != "analyze" {
		return fallback
	}
	for i := 2; i < len(args); i++ {
		if args[i] == "--repo" && i+1 < len(args) {
			return args[i+1]
		}
		if strings.HasPrefix(args[i], "--repo=") {
			return strings.TrimPrefix(args[i], "--repo=")
		}
	}
	return fallback
}
