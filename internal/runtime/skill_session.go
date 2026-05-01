package runtime

import (
	"context"
	"sort"

	"happyagent/internal/skills"
	"happyagent/internal/tools"
)

const activateSkillToolName = "activate_skill"
const listCapabilitiesToolName = "list_capabilities"

type SkillSession struct {
	loader        *skills.Loader
	basePrompt    string
	catalog       []skills.Metadata
	baseToolDefs  []tools.Definition
	activateDef   tools.Definition
	listCapsDef   tools.Definition
	activeByName  map[string]skills.ActivatedSkill
	activationSeq []string
}

func NewSkillSession(loader *skills.Loader, basePrompt string, defs []tools.Definition) (*SkillSession, error) {
	catalog, err := loader.LoadCatalog()
	if err != nil {
		return nil, err
	}

	return &SkillSession{
		loader:       loader,
		basePrompt:   basePrompt,
		catalog:      catalog,
		baseToolDefs: append([]tools.Definition(nil), defs...),
		activateDef:  activateSkillDefinition(catalog),
		listCapsDef:  listCapabilitiesDefinition(),
		activeByName: make(map[string]skills.ActivatedSkill),
	}, nil
}

func (s *SkillSession) ActivateToolDef() tools.Definition {
	return activateSkillDefinition(s.catalog)
}

func (s *SkillSession) Definition() tools.Definition {
	return s.ActivateToolDef()
}

func (s *SkillSession) ActivateSkills(ctx context.Context, skillNames []string) (string, error) {
	return s.Activate(ctx, skillNames)
}

func (s *SkillSession) Catalog() []skills.Metadata {
	return append([]skills.Metadata(nil), s.catalog...)
}

func (s *SkillSession) SystemPrompt() string {
	return s.basePrompt
}

func (s *SkillSession) ToolDefs() ([]tools.Definition, error) {
	defs := make([]tools.Definition, 0, len(s.baseToolDefs)+2)
	defs = append(defs, s.listCapsDef)
	if len(s.catalog) > 0 {
		defs = append(defs, s.ActivateToolDef())
	}
	defs = append(defs, s.baseToolDefs...)
	return defs, nil
}

func activateSkillDefinition(catalog []skills.Metadata) tools.Definition {
	description := "Activate one or more skills by name to load their detailed instructions."
	if len(catalog) == 0 {
		description = "No skills are available."
	}

	return tools.Definition{
		Name:        activateSkillToolName,
		Description: description,
		InputSchema: `{"type":"object","properties":{"skill_names":{"type":"array","items":{"type":"string"},"description":"Skill names to activate."}},"required":["skill_names"]}`,
	}
}

func listCapabilitiesDefinition() tools.Definition {
	return tools.Definition{
		Name:        listCapabilitiesToolName,
		Description: "Return currently available skills, active skills, and MCP resources.",
		InputSchema: `{"type":"object","properties":{},"additionalProperties":false}`,
	}
}

func (s *SkillSession) ActiveSkills() []skills.ActivatedSkill {
	active := make([]skills.ActivatedSkill, 0, len(s.activationSeq))
	for _, name := range s.activationSeq {
		active = append(active, s.activeByName[name])
	}
	return active
}

func (s *SkillSession) AvailableToolNames() ([]string, error) {
	defs, err := s.ToolDefs()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(defs))
	for _, def := range defs {
		names = append(names, def.Name)
	}
	sort.Strings(names)
	return names, nil
}
