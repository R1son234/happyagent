package skills

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

func ParseSkillDir(dir string) (Skill, error) {
	meta, err := ParseSkillMetadataDir(dir)
	if err != nil {
		return Skill{}, err
	}

	return Skill{
		Name:        meta.Name,
		Description: meta.Description,
		Prompt:      meta.Prompt,
	}, nil
}

func ParseSkillMetadataDir(dir string) (Metadata, error) {
	skillMDPath := filepath.Join(dir, "SKILL.md")
	data, err := os.ReadFile(skillMDPath)
	if err != nil {
		if os.IsNotExist(err) {
			return Metadata{}, fmt.Errorf("read skill spec %q: %w", skillMDPath, err)
		}
		return Metadata{}, fmt.Errorf("read skill spec %q: %w", skillMDPath, err)
	}
	return parseSkillMarkdown(skillMDPath, data)
}

func parseSkillMarkdown(path string, data []byte) (Metadata, error) {
	frontmatter, body, ok := splitFrontmatter(data)
	if !ok {
		return Metadata{}, fmt.Errorf("skill markdown %q must start with YAML frontmatter", path)
	}

	var spec Spec
	if err := yaml.Unmarshal(frontmatter, &spec); err != nil {
		return Metadata{}, fmt.Errorf("parse skill markdown frontmatter %q: %w", path, err)
	}
	if strings.TrimSpace(spec.Name) == "" {
		return Metadata{}, fmt.Errorf("skill markdown %q must define name", path)
	}

	return Metadata{
		Name:        spec.Name,
		Description: spec.Description,
		Prompt:      strings.TrimSpace(body),
	}, nil
}

func splitFrontmatter(data []byte) ([]byte, string, bool) {
	if !bytes.HasPrefix(data, []byte("---\n")) {
		return nil, "", false
	}

	rest := data[len("---\n"):]
	idx := bytes.Index(rest, []byte("\n---\n"))
	if idx < 0 {
		return nil, "", false
	}

	frontmatter := rest[:idx]
	body := string(rest[idx+len("\n---\n"):])
	return frontmatter, body, true
}
