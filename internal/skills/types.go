package skills

type Skill struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Prompt      string `json:"prompt"`
}

type ActivatedSkill struct {
	Name   string `json:"name"`
	Prompt string `json:"prompt"`
}

type Metadata struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Prompt      string `json:"prompt"`
}

type Spec struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}
