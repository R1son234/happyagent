package skills

type Skill struct {
	Name        string
	Description string
	Prompt      string
	Tools       []string
	Resources   []string
}

type Spec struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Tools       []string `yaml:"tools"`
	Resources   []string `yaml:"resources"`
	PromptFile  string   `yaml:"prompt_file"`
}
