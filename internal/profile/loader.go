package profile

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

const fileName = "profile.json"

func LoadAll(dir string) ([]Profile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("read profiles dir %q: %w", dir, err)
		}
		return nil, fmt.Errorf("read profiles dir %q: %w", dir, err)
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	profiles := make([]Profile, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		profile, err := loadProfileFile(filepath.Join(dir, entry.Name(), fileName), entry.Name())
		if err != nil {
			return nil, err
		}
		profiles = append(profiles, profile)
	}

	return profiles, nil
}

func LoadByName(dir string, name string) (Profile, error) {
	if name == "" {
		return Profile{}, fmt.Errorf("profile name must not be empty")
	}
	return loadProfileFile(filepath.Join(dir, name, fileName), name)
}

func loadProfileFile(path string, dirName string) (Profile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Profile{}, fmt.Errorf("read profile %q: %w", dirName, err)
		}
		return Profile{}, fmt.Errorf("read profile %q: %w", dirName, err)
	}

	var p Profile
	if err := json.Unmarshal(data, &p); err != nil {
		return Profile{}, fmt.Errorf("parse profile %q: %w", dirName, err)
	}
	if err := Validate(p, dirName); err != nil {
		return Profile{}, fmt.Errorf("validate profile %q: %w", dirName, err)
	}
	return p, nil
}
