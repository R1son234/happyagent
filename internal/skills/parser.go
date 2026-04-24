package skills

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

func ParseSkillDir(dir string) (Skill, error) {
	specPath := filepath.Join(dir, "skill.yaml")
	data, err := os.ReadFile(specPath)
	if err != nil {
		return Skill{}, fmt.Errorf("read skill spec %q: %w", specPath, err)
	}

	var spec Spec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return Skill{}, fmt.Errorf("parse skill spec %q: %w", specPath, err)
	}
	if spec.Name == "" {
		return Skill{}, fmt.Errorf("skill spec %q must define name", specPath)
	}

	promptFile := spec.PromptFile
	if promptFile == "" {
		promptFile = "prompt.md"
	}
	promptPath := filepath.Join(dir, promptFile)
	promptData, err := os.ReadFile(promptPath)
	if err != nil {
		return Skill{}, fmt.Errorf("read skill prompt %q: %w", promptPath, err)
	}

	return Skill{
		Name:        spec.Name,
		Description: spec.Description,
		Prompt:      string(promptData),
		Tools:       spec.Tools,
		Resources:   spec.Resources,
	}, nil
}
