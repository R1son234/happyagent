package runtime

import (
	"encoding/json"
	"sort"

	"happyagent/internal/mcp"
)

const mcpReadResourceToolName = "mcp_read_resource"

type CapabilitySession struct {
	skillSession *SkillSession
	mcpManager   *mcp.Manager
}

type listedSkill struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func NewCapabilitySession(skillSession *SkillSession, mcpManager *mcp.Manager) *CapabilitySession {
	return &CapabilitySession{
		skillSession: skillSession,
		mcpManager:   mcpManager,
	}
}

func (s *CapabilitySession) CapabilitiesJSON() (string, error) {
	payload := struct {
		AvailableTools           []string           `json:"available_tools"`
		Skills                   []listedSkill      `json:"skills"`
		ActiveSkills             []string           `json:"active_skills"`
		MCPResourceReadSupported bool               `json:"mcp_resource_read_supported"`
		MCPResources             []mcp.ResourceInfo `json:"mcp_resources"`
		MCPResourcesTotal        int                `json:"mcp_resources_total"`
		MCPResourcesTruncated    bool               `json:"mcp_resources_truncated"`
		MCPPrompts               []mcp.PromptInfo   `json:"mcp_prompts"`
		MCPPromptsTotal          int                `json:"mcp_prompts_total"`
	}{
		AvailableTools: []string{},
		Skills:         []listedSkill{},
		ActiveSkills:   []string{},
	}
	if s.skillSession != nil {
		for _, skill := range s.skillSession.Catalog() {
			payload.Skills = append(payload.Skills, listedSkill{
				Name:        skill.Name,
				Description: skill.Description,
			})
		}
		for _, skill := range s.skillSession.ActiveSkills() {
			payload.ActiveSkills = append(payload.ActiveSkills, skill.Name)
		}
		toolNames, err := s.skillSession.AvailableToolNames()
		if err != nil {
			return "", err
		}
		payload.AvailableTools = toolNames
		payload.MCPResourceReadSupported = containsString(toolNames, mcpReadResourceToolName)
	}
	resources, total, truncated := s.listMCPResources()
	payload.MCPResources = append([]mcp.ResourceInfo{}, resources...)
	payload.MCPResourcesTotal = total
	payload.MCPResourcesTruncated = truncated

	prompts := s.listMCPPrompts()
	payload.MCPPrompts = prompts
	payload.MCPPromptsTotal = len(prompts)

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

func (s *CapabilitySession) listMCPPrompts() []mcp.PromptInfo {
	if s.mcpManager == nil {
		return nil
	}
	prompts := s.mcpManager.ListPrompts()
	sort.Slice(prompts, func(i, j int) bool {
		if prompts[i].ServerName == prompts[j].ServerName {
			return prompts[i].Name < prompts[j].Name
		}
		return prompts[i].ServerName < prompts[j].ServerName
	})
	return prompts
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
