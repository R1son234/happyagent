package runtime

import (
	"context"
	"fmt"
	"strings"

	"happyagent/internal/skills"
)

func (s *SkillSession) Activate(ctx context.Context, names []string) (string, error) {
	_ = ctx
	if len(names) == 0 {
		return "", fmt.Errorf("activate_skill requires at least one skill name")
	}

	var builder strings.Builder
	var activatedAny bool

	for _, rawName := range names {
		name := strings.TrimSpace(rawName)
		if name == "" {
			continue
		}

		if active, ok := s.activeByName[name]; ok {
			builder.WriteString(formatActivatedSkill(active))
			builder.WriteString("\n")
			continue
		}

		skill, err := s.loader.Load(name)
		if err != nil {
			return "", err
		}

		active := skills.ActivatedSkill{
			Name:   skill.Name,
			Prompt: skill.Prompt,
		}

		s.activeByName[skill.Name] = active
		s.activationSeq = append(s.activationSeq, skill.Name)
		builder.WriteString(formatActivatedSkill(active))
		builder.WriteString("\n")
		activatedAny = true
	}

	if !activatedAny && builder.Len() == 0 {
		return "", fmt.Errorf("activate_skill requires at least one non-empty skill name")
	}

	return strings.TrimSpace(builder.String()), nil
}

func formatActivatedSkill(skill skills.ActivatedSkill) string {
	var builder strings.Builder
	builder.WriteString("Activated skill ")
	builder.WriteString(skill.Name)
	builder.WriteString(".")

	if strings.TrimSpace(skill.Prompt) != "" {
		builder.WriteString("\nPrompt:\n")
		builder.WriteString(skill.Prompt)
	}

	return strings.TrimSpace(builder.String())
}
