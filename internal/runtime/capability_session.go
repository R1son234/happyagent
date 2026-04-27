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
		Skills                []skills.Metadata  `json:"skills"`
		ActiveSkills          []string           `json:"active_skills"`
		MCPResources          []mcp.ResourceInfo `json:"mcp_resources"`
		MCPResourcesTotal     int                `json:"mcp_resources_total"`
		MCPResourcesTruncated bool               `json:"mcp_resources_truncated"`
	}{
		Skills:       []skills.Metadata{},
		ActiveSkills: []string{},
	}
	if s.skillSession != nil {
		payload.Skills = append([]skills.Metadata{}, s.skillSession.Catalog()...)
		for _, skill := range s.skillSession.ActiveSkills() {
			payload.ActiveSkills = append(payload.ActiveSkills, skill.Name)
		}
	}
	resources, total, truncated := s.listMCPResources()
	payload.MCPResources = append([]mcp.ResourceInfo{}, resources...)
	payload.MCPResourcesTotal = total
	payload.MCPResourcesTruncated = truncated

	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (s *CapabilitySession) listMCPResources() ([]mcp.ResourceInfo, int, bool) {
	if s.mcpManager == nil {
		return nil, 0, false
	}
	resources, total, truncated := s.mcpManager.ListResourcesPreview()
	sort.Slice(resources, func(i, j int) bool {
		if resources[i].ServerName == resources[j].ServerName {
			return resources[i].URI < resources[j].URI
		}
		return resources[i].ServerName < resources[j].ServerName
	})
	return resources, total, truncated
}
