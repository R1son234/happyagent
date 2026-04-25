package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

type Loader struct {
	dir string
}

func NewLoader(dir string) *Loader {
	return &Loader{dir: dir}
}

func (l *Loader) LoadCatalog() ([]Metadata, error) {
	entries, err := os.ReadDir(l.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read skills dir %q: %w", l.dir, err)
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	var loaded []Metadata

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skill, err := ParseSkillMetadataDir(filepath.Join(l.dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		loaded = append(loaded, skill)
	}

	return loaded, nil
}

func (l *Loader) Load(name string) (*Skill, error) {
	entries, err := os.ReadDir(l.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("skill %q not found in %q", name, l.dir)
		}
		return nil, fmt.Errorf("read skills dir %q: %w", l.dir, err)
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

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
