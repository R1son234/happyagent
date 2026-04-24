package skills

import (
	"context"
	"fmt"
	"strings"

	"happyagent/internal/mcp"
	"happyagent/internal/tools"
)

type InjectionResult struct {
	SystemPrompt string
	ToolDefs     []tools.Definition
}

func Inject(ctx context.Context, basePrompt string, skill *Skill, defs []tools.Definition, manager *mcp.Manager) (InjectionResult, error) {
	if skill == nil {
		return InjectionResult{
			SystemPrompt: basePrompt,
			ToolDefs:     defs,
		}, nil
	}

	filtered, err := filterTools(skill, defs)
	if err != nil {
		return InjectionResult{}, err
	}

	var builder strings.Builder
	builder.WriteString(basePrompt)
	if strings.TrimSpace(skill.Prompt) != "" {
		builder.WriteString("\n\nSkill Prompt:\n")
		builder.WriteString(skill.Prompt)
	}

	if len(skill.Resources) > 0 {
		if manager == nil {
			return InjectionResult{}, fmt.Errorf("skill %q references mcp resources but no mcp manager is configured", skill.Name)
		}
		for _, uri := range skill.Resources {
			content, err := manager.ReadResource(ctx, uri)
			if err != nil {
				return InjectionResult{}, err
			}
			builder.WriteString("\n\nResource ")
			builder.WriteString(uri)
			builder.WriteString(":\n")
			builder.WriteString(content)
		}
	}

	return InjectionResult{
		SystemPrompt: builder.String(),
		ToolDefs:     filtered,
	}, nil
}

func filterTools(skill *Skill, defs []tools.Definition) ([]tools.Definition, error) {
	if skill == nil || len(skill.Tools) == 0 {
		return defs, nil
	}

	allowed := make(map[string]struct{}, len(skill.Tools))
	for _, name := range skill.Tools {
		allowed[name] = struct{}{}
	}

	var filtered []tools.Definition
	for _, def := range defs {
		if _, ok := allowed[def.Name]; ok {
			filtered = append(filtered, def)
		}
	}

	if len(filtered) != len(skill.Tools) {
		for _, name := range skill.Tools {
			found := false
			for _, def := range filtered {
				if def.Name == name {
					found = true
					break
				}
			}
			if !found {
				return nil, fmt.Errorf("skill %q references unknown tool %q", skill.Name, name)
			}
		}
	}

	return filtered, nil
}
