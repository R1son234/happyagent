package memory

import (
	"strings"
)

type Turn struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Strategy struct {
	Enabled  bool `json:"enabled"`
	MaxTurns int  `json:"max_turns"`
	MaxChars int  `json:"max_chars"`
}

type BuildResult struct {
	Text        string
	Trimmed     bool
	SourceTurns int
}

func Build(turns []Turn, strategy Strategy) BuildResult {
	if !strategy.Enabled || len(turns) == 0 {
		return BuildResult{}
	}

	maxTurns := strategy.MaxTurns
	if maxTurns <= 0 || maxTurns > len(turns) {
		maxTurns = len(turns)
	}
	selected := turns[len(turns)-maxTurns:]

	var builder strings.Builder
	builder.WriteString("Recent session memory:\n")
	for _, turn := range selected {
		role := strings.TrimSpace(turn.Role)
		if role == "" {
			role = "unknown"
		}
		builder.WriteString("- ")
		builder.WriteString(role)
		builder.WriteString(": ")
		builder.WriteString(strings.TrimSpace(turn.Content))
		builder.WriteString("\n")
	}

	text := strings.TrimSpace(builder.String())
	trimmed := false
	if strategy.MaxChars > 0 && len(text) > strategy.MaxChars {
		text = text[:strategy.MaxChars]
		text = strings.TrimSpace(text) + "\n...[memory truncated]"
		trimmed = true
	}

	return BuildResult{
		Text:        text,
		Trimmed:     trimmed,
		SourceTurns: len(selected),
	}
}
