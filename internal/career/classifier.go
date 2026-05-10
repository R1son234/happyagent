package career

import (
	"sort"
	"strings"
)

const (
	WorkspaceTypeGeneral      = "general"
	WorkspaceTypeJD           = "jd"
	WorkspaceTypeResume       = "resume"
	WorkspaceTypeExperiences  = "experiences"
	WorkspaceTypePrepare      = "prepare"
	WorkspaceTypeMyInterviews = "my-interviews"
	WorkspaceTypeRecord       = "record"
)

type InputClassification struct {
	Type       string   `json:"type"`
	Confidence float64  `json:"confidence"`
	Signals    []string `json:"signals,omitempty"`
	ShouldSave bool     `json:"should_save"`
}

type workspaceTypeDefinition struct {
	Type                  string
	DisplayName           string
	HintSignals           []string
	ClassificationSignals []string
	FilenameSignals       []string
}

var workspaceTypeDefinitions = []workspaceTypeDefinition{
	{
		Type:                  WorkspaceTypeJD,
		DisplayName:           "JD",
		HintSignals:           []string{"jd", "job description", "岗位", "职位"},
		ClassificationSignals: []string{"job description", "responsibilities", "requirements", "qualifications", "岗位职责", "职位描述", "任职要求", "岗位要求", "加分项"},
		FilenameSignals:       []string{"jd", "job", "岗位", "职位"},
	},
	{
		Type:                  WorkspaceTypeResume,
		DisplayName:           "简历",
		HintSignals:           []string{"resume", "cv", "简历"},
		ClassificationSignals: []string{"简历", "resume", "工作经历", "教育经历", "专业技能", "求职意向", "work experience", "education"},
		FilenameSignals:       []string{"resume", "cv", "简历"},
	},
	{
		Type:                  WorkspaceTypePrepare,
		DisplayName:           "项目素材",
		HintSignals:           []string{"project", "项目"},
		ClassificationSignals: []string{"项目", "project", "项目追问", "项目亮点", "项目难点", "技术方案", "证据口径", "技术栈", "架构", "repository", "repo", "github", "system design"},
		FilenameSignals:       []string{"project", "portfolio", "项目"},
	},
	{
		Type:                  WorkspaceTypeExperiences,
		DisplayName:           "面经",
		HintSignals:           []string{"interview experience", "面经"},
		ClassificationSignals: []string{"面经", "面试题", "一面", "二面", "三面", "interview experience", "面试经验", "高频题", "公开面经"},
		FilenameSignals:       []string{"interview", "experience", "面经", "面试题"},
	},
	{
		Type:                  WorkspaceTypeMyInterviews,
		DisplayName:           "面试记录",
		HintSignals:           []string{"interview record", "面试记录", "复盘"},
		ClassificationSignals: []string{"面试记录", "刚面完", "刚才面试", "面试复盘", "我回答", "面试官问我", "面试官问", "现场表现", "interviewer asked", "asked me"},
		FilenameSignals:       []string{"interview", "record", "面试记录", "复盘"},
	},
	{
		Type:                  WorkspaceTypeRecord,
		DisplayName:           "复习笔记",
		HintSignals:           []string{"review note", "study note", "笔记", "复习"},
		ClassificationSignals: []string{"笔记", "复习", "知识点", "review note", "study note", "todo", "待复习", "导入记录", "处理记录"},
		FilenameSignals:       []string{"review", "study", "note", "record", "记录", "笔记", "复习"},
	},
}

func ClassifyInput(content string) InputClassification {
	content = strings.TrimSpace(content)
	if content == "" {
		return InputClassification{Type: WorkspaceTypeGeneral}
	}
	if LooksLikeJD(content) {
		return InputClassification{
			Type:       WorkspaceTypeJD,
			Confidence: 0.9,
			Signals:    []string{"jd_signals"},
			ShouldSave: true,
		}
	}
	lower := strings.ToLower(content)
	scores := map[string]int{}
	signals := map[string][]string{}
	addSignals := func(itemType string, values []string) {
		for _, value := range values {
			if containsSignal(lower, content, value) {
				scores[itemType]++
				signals[itemType] = append(signals[itemType], value)
			}
		}
	}

	for _, definition := range workspaceTypeDefinitions {
		if definition.Type != WorkspaceTypeJD {
			addSignals(definition.Type, definition.ClassificationSignals)
		}
	}

	typeOrder := []string{
		WorkspaceTypeMyInterviews,
		WorkspaceTypeExperiences,
		WorkspaceTypeResume,
		WorkspaceTypePrepare,
		WorkspaceTypeRecord,
	}
	sort.SliceStable(typeOrder, func(i, j int) bool {
		return scores[typeOrder[i]] > scores[typeOrder[j]]
	})
	best := typeOrder[0]
	score := scores[best]
	if score == 0 {
		return InputClassification{Type: WorkspaceTypeGeneral, Confidence: 0.2}
	}
	confidence := 0.45 + float64(score)*0.15
	if len([]rune(content)) > 120 {
		confidence += 0.1
	}
	if confidence > 0.9 {
		confidence = 0.9
	}
	return InputClassification{
		Type:       best,
		Confidence: confidence,
		Signals:    signals[best],
		ShouldSave: confidence >= 0.65,
	}
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

func detectWorkspaceTypeBySignals(input string) string {
	normalized := strings.ToLower(input)
	for _, definition := range workspaceTypeDefinitions {
		if matchesAnySignal(normalized, input, definition.HintSignals) {
			return definition.Type
		}
	}
	return ""
}

func workspaceTypeFilenameSignals(itemType string) []string {
	for _, definition := range workspaceTypeDefinitions {
		if definition.Type == itemType {
			return definition.FilenameSignals
		}
	}
	return nil
}

func matchesAnySignal(normalized string, original string, signals []string) bool {
	for _, signal := range signals {
		if containsSignal(normalized, original, signal) {
			return true
		}
	}
	return false
}

func containsSignal(normalized string, original string, signal string) bool {
	if signal == "" {
		return false
	}
	lowerSignal := strings.ToLower(signal)
	return strings.Contains(normalized, lowerSignal) || strings.Contains(original, signal)
}
