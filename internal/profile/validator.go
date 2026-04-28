package profile

import (
	"fmt"
	"strings"
)

type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	if e.Field == "" {
		return e.Message
	}
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

func Validate(p Profile, dirName string) error {
	if strings.TrimSpace(p.Name) == "" {
		return &ValidationError{Field: "name", Message: "must not be empty"}
	}
	if strings.TrimSpace(p.SystemPrompt) == "" {
		return &ValidationError{Field: "system_prompt", Message: "must not be empty"}
	}
	if hasDuplicate(p.EnabledTools) {
		return &ValidationError{Field: "enabled_tools", Message: "must not contain duplicates"}
	}
	if hasDuplicate(p.EnabledSkills) {
		return &ValidationError{Field: "enabled_skills", Message: "must not contain duplicates"}
	}
	if dirName != "" && dirName != p.Name {
		return &ValidationError{Field: "name", Message: fmt.Sprintf("must match profile directory %q", dirName)}
	}
	return nil
}

func hasDuplicate(values []string) bool {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			return true
		}
		seen[value] = struct{}{}
	}
	return false
}
