package career

import (
	"path/filepath"
	"strings"
)

const (
	WorkspaceTypeGeneral      = "general"
	WorkspaceTypeJD           = "jd"
	WorkspaceTypeResume       = "resume"
	WorkspaceTypeExperiences  = "experiences"
	WorkspaceTypePrepare      = "prepare"
	WorkspaceTypeProject      = "project"
	WorkspaceTypeMyInterviews = "my-interviews"
	WorkspaceTypeRecord       = "record"
)

type InputClassification struct {
	Type       string       `json:"type"`
	Confidence float64      `json:"confidence"`
	Signals    []string     `json:"signals,omitempty"`
	ShouldSave bool         `json:"should_save"`
	Reason     string       `json:"reason,omitempty"`
	RulePath   string       `json:"rule_path,omitempty"`
	Meta       LLMTraceMeta `json:"meta,omitempty"`
}

type workspaceTypeDefinition struct {
	Type        string
	DisplayName string
}

var workspaceTypeDefinitions = []workspaceTypeDefinition{
	{
		Type:        WorkspaceTypeJD,
		DisplayName: "JD",
	},
	{
		Type:        WorkspaceTypeResume,
		DisplayName: "简历",
	},
	{
		Type:        WorkspaceTypePrepare,
		DisplayName: "项目素材",
	},
	{
		Type:        WorkspaceTypeProject,
		DisplayName: "项目专项",
	},
	{
		Type:        WorkspaceTypeExperiences,
		DisplayName: "面经",
	},
	{
		Type:        WorkspaceTypeMyInterviews,
		DisplayName: "面试记录",
	},
	{
		Type:        WorkspaceTypeRecord,
		DisplayName: "复习笔记",
	},
}

func IsSupportedWorkspaceType(itemType string) bool {
	itemType = strings.ToLower(strings.TrimSpace(itemType))
	if itemType == WorkspaceTypeGeneral {
		return true
	}
	for _, definition := range workspaceTypeDefinitions {
		if definition.Type == itemType {
			return true
		}
	}
	return false
}

func workspaceTypeDisplayName(itemType string) string {
	for _, definition := range workspaceTypeDefinitions {
		if definition.Type == itemType {
			return definition.DisplayName
		}
	}
	return itemType
}

func classificationRulePath(guide WorkspaceGuide, itemType string) string {
	if rule, ok := guide.Directory(itemType); ok {
		return filepath.ToSlash(rule.Path)
	}
	return ""
}
