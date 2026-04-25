package skills

import "strings"

func BuildActivatedPrompt(basePrompt string, active []ActivatedSkill) string {
	base := basePrompt
	if len(active) == 0 {
		return base
	}

	var builder strings.Builder
	builder.WriteString(base)
	for _, skill := range active {
		if strings.TrimSpace(skill.Prompt) != "" {
			builder.WriteString("\n\nActivated skill ")
			builder.WriteString(skill.Name)
			builder.WriteString(" prompt:\n")
			builder.WriteString(skill.Prompt)
		}
	}
	return builder.String()
}
