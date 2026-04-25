package runtime

import (
	"encoding/json"
	"sort"

	"happyagent/internal/mcp"
	"happyagent/internal/skills"
)

type CapabilitySession struct {
	skillSession *SkillSession
	mcpManager   *mcp.Manager
}

func NewCapabilitySession(skillSession *SkillSession, mcpManager *mcp.Manager) *CapabilitySession {
	return &CapabilitySession{
		skillSession: skillSession,
		mcpManager:   mcpManager,
	}
}

func (s *CapabilitySession) CapabilitiesJSON() (string, error) {
	payload := struct {
		Skills       []skills.Metadata  `json:"skills"`
		ActiveSkills []string           `json:"active_skills"`
		MCPResources []mcp.ResourceInfo `json:"mcp_resources"`
	}{
		Skills:       []skills.Metadata{},
		ActiveSkills: []string{},
		MCPResources: append([]mcp.ResourceInfo{}, s.listMCPResources()...),
	}
	if s.skillSession != nil {
		payload.Skills = append([]skills.Metadata{}, s.skillSession.Catalog()...)
		for _, skill := range s.skillSession.ActiveSkills() {
			payload.ActiveSkills = append(payload.ActiveSkills, skill.Name)
		}
	}

	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (s *CapabilitySession) listMCPResources() []mcp.ResourceInfo {
	if s.mcpManager == nil {
		return nil
	}
	resources := s.mcpManager.ListResources()
	sort.Slice(resources, func(i, j int) bool {
		if resources[i].ServerName == resources[j].ServerName {
			return resources[i].URI < resources[j].URI
		}
		return resources[i].ServerName < resources[j].ServerName
	})
	return resources
}
