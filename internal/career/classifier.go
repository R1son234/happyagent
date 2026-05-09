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
	addSignals := func(itemType string, values ...string) {
		for _, value := range values {
			if strings.Contains(lower, strings.ToLower(value)) {
				scores[itemType]++
				signals[itemType] = append(signals[itemType], value)
			}
		}
	}

	addSignals(WorkspaceTypeResume, "简历", "resume", "工作经历", "教育经历", "专业技能", "求职意向", "work experience", "education")
	addSignals(WorkspaceTypePrepare, "项目", "project", "项目追问", "项目亮点", "项目难点", "技术方案", "证据口径", "技术栈", "架构", "repository", "repo", "github", "system design")
	addSignals(WorkspaceTypeExperiences, "面经", "面试题", "一面", "二面", "三面", "interview experience", "面试经验", "高频题", "公开面经")
	addSignals(WorkspaceTypeMyInterviews, "面试记录", "刚面完", "刚才面试", "面试复盘", "我回答", "面试官问我", "面试官问", "现场表现", "interviewer asked", "asked me")
	addSignals(WorkspaceTypeRecord, "笔记", "复习", "知识点", "review note", "study note", "todo", "待复习", "导入记录", "处理记录")

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
	switch strings.ToLower(strings.TrimSpace(itemType)) {
	case WorkspaceTypeGeneral,
		WorkspaceTypeJD,
		WorkspaceTypeResume,
		WorkspaceTypeExperiences,
		WorkspaceTypePrepare,
		WorkspaceTypeMyInterviews,
		WorkspaceTypeRecord:
		return true
	default:
		return false
	}
}
