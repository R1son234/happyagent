package skills

import (
	"fmt"
	"os"
	"path/filepath"
)

type Loader struct {
	dir string
}

func NewLoader(dir string) *Loader {
	return &Loader{dir: dir}
}

func (l *Loader) Load(name string) (*Skill, error) {
	if name == "" {
		return nil, nil
	}

	entries, err := os.ReadDir(l.dir)
	if err != nil {
		return nil, fmt.Errorf("read skills dir %q: %w", l.dir, err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skill, err := ParseSkillDir(filepath.Join(l.dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		if skill.Name == name {
			return &skill, nil
		}
	}

	return nil, fmt.Errorf("skill %q not found in %q", name, l.dir)
}
