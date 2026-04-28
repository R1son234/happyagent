package profile

import "encoding/json"

type Profile struct {
	Name           string          `json:"name"`
	Description    string          `json:"description,omitempty"`
	SystemPrompt   string          `json:"system_prompt"`
	EnabledTools   []string        `json:"enabled_tools"`
	EnabledSkills  []string        `json:"enabled_skills"`
	MemoryStrategy json.RawMessage `json:"memory_strategy,omitempty"`
	OutputSchema   json.RawMessage `json:"output_schema,omitempty"`
	EvalSuite      json.RawMessage `json:"eval_suite,omitempty"`
}

type ResolvedProfile struct {
	Profile
	EnabledToolSet  map[string]struct{}
	EnabledSkillSet map[string]struct{}
}

func Resolve(p Profile) ResolvedProfile {
	return ResolvedProfile{
		Profile:         p,
		EnabledToolSet:  toSet(p.EnabledTools),
		EnabledSkillSet: toSet(p.EnabledSkills),
	}
}

func toSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	return set
}
