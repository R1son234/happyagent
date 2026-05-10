package career

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const WorkspaceGuideFileName = "workspace.guide.json"

const (
	SaveModeMaterialDir = "material_dir"
	SaveModeRoleDir     = "role_dir"
	SaveModeRecordDir   = "record_dir"
)

type WorkspaceGuide struct {
	Version     int             `json:"version"`
	Name        string          `json:"name"`
	DemoTopic   string          `json:"demo_topic"`
	Directories []DirectoryRule `json:"directories"`
	SyncRules   []SyncRule      `json:"sync_rules,omitempty"`
	Markdown    MarkdownPolicy  `json:"markdown,omitempty"`
}

type DirectoryRule struct {
	Type             string   `json:"type"`
	Path             string   `json:"path"`
	Title            string   `json:"title"`
	Responsibility   string   `json:"responsibility"`
	Signals          []string `json:"signals,omitempty"`
	HintSignals      []string `json:"hint_signals,omitempty"`
	FilenameSignals  []string `json:"filename_signals,omitempty"`
	RequiredSections []string `json:"required_sections,omitempty"`
	ActivePointer    string   `json:"active_pointer,omitempty"`
	SaveMode         string   `json:"save_mode"`
	MOC              string   `json:"moc,omitempty"`
}

type SyncRule struct {
	When     string `json:"when"`
	CopyTo   string `json:"copy_to"`
	Template string `json:"template"`
}

type MarkdownPolicy struct {
	PropertiesRequired bool `json:"properties_required"`
	Wikilink           bool `json:"wikilink"`
	MOCUpdate          bool `json:"moc_update"`
	DemoRedaction      bool `json:"demo_redaction"`
}

func DefaultWorkspaceGuide() WorkspaceGuide {
	return WorkspaceGuide{
		Version:   1,
		Name:      "career-copilot-default",
		DemoTopic: "市场营销",
		Directories: []DirectoryRule{
			{
				Type:             WorkspaceTypeResume,
				Path:             "resume",
				Title:            "简历",
				Responsibility:   "保存当前可用简历、简历版本和简历提取文本。",
				Signals:          []string{"resume", "cv", "简历", "工作经历", "教育经历", "项目经历", "专业技能", "求职意向", "work experience", "education"},
				HintSignals:      []string{"resume", "cv", "简历"},
				FilenameSignals:  []string{"resume", "cv", "简历"},
				RequiredSections: []string{"工作经历", "项目经历", "专业技能"},
				ActivePointer:    "current_resume",
				SaveMode:         SaveModeMaterialDir,
			},
			{
				Type:             WorkspaceTypeJD,
				Path:             "jd",
				Title:            "JD",
				Responsibility:   "保存岗位描述、OCR 文本、岗位画像、关键词和匹配关系。",
				Signals:          []string{"job description", "responsibilities", "requirements", "qualifications", "jd", "岗位职责", "职位描述", "任职要求", "岗位要求", "加分项"},
				HintSignals:      []string{"jd", "job description", "岗位", "职位"},
				FilenameSignals:  []string{"jd", "job", "岗位", "职位"},
				RequiredSections: []string{"岗位职责", "任职要求", "技术关键词", "匹配关系"},
				ActivePointer:    "active_jd",
				SaveMode:         SaveModeMaterialDir,
			},
			{
				Type:             WorkspaceTypeExperiences,
				Path:             "experiences",
				Title:            "公开面经",
				Responsibility:   "保存公开面经、跨公司高频题、通用 QA 和来源材料。",
				Signals:          []string{"面经", "面试题", "一面", "二面", "三面", "interview experience", "面试经验", "高频题", "公开面经"},
				HintSignals:      []string{"interview experience", "面经"},
				FilenameSignals:  []string{"interview", "experience", "面经", "面试题"},
				RequiredSections: []string{"面试官想考", "可说出口答案", "可能追问", "可关联项目"},
				SaveMode:         SaveModeMaterialDir,
			},
			{
				Type:             WorkspaceTypePrepare,
				Path:             "prepare",
				Title:            "项目专项",
				Responsibility:   "保存个人项目介绍、项目追问、证据口径和岗位化表达。",
				Signals:          []string{"项目", "project", "项目追问", "项目亮点", "项目难点", "技术方案", "证据口径", "技术栈", "架构", "repository", "repo", "github", "system design"},
				HintSignals:      []string{"project", "项目"},
				FilenameSignals:  []string{"project", "portfolio", "项目"},
				RequiredSections: []string{"回答", "追问", "证据口径"},
				ActivePointer:    "active_project",
				SaveMode:         SaveModeMaterialDir,
			},
			{
				Type:             WorkspaceTypeMyInterviews,
				Path:             "my-interviews",
				Title:            "真实面试记录",
				Responsibility:   "保存具体公司或岗位的作战材料、真实面试记录和复盘。",
				Signals:          []string{"面试记录", "刚面完", "刚才面试", "面试复盘", "我回答", "面试官问我", "面试官问", "现场表现", "interviewer asked", "asked me"},
				HintSignals:      []string{"interview record", "面试记录", "复盘"},
				FilenameSignals:  []string{"interview", "record", "面试记录", "复盘"},
				RequiredSections: []string{"基本信息", "原始问题", "复盘"},
				SaveMode:         SaveModeRoleDir,
			},
			{
				Type:            WorkspaceTypeRecord,
				Path:            "record",
				Title:           "操作记录",
				Responsibility:  "保存导入记录、分类结果、生成过程和未分类材料。",
				Signals:         []string{"笔记", "复习", "知识点", "review note", "study note", "todo", "待复习", "导入记录", "处理记录"},
				HintSignals:     []string{"review note", "study note", "笔记", "复习"},
				FilenameSignals: []string{"review", "study", "note", "record", "记录", "笔记", "复习"},
				SaveMode:        SaveModeRecordDir,
			},
		},
		SyncRules: []SyncRule{
			{When: "my-interviews contains 通用题", CopyTo: WorkspaceTypeExperiences, Template: "experience_qa"},
			{When: "my-interviews contains 项目深挖", CopyTo: WorkspaceTypePrepare, Template: "project_qa"},
			{When: "experiences contains 项目追问 or 证据口径", CopyTo: WorkspaceTypePrepare, Template: "project_qa"},
		},
		Markdown: MarkdownPolicy{
			PropertiesRequired: true,
			Wikilink:           true,
			MOCUpdate:          true,
			DemoRedaction:      true,
		},
	}
}

func (w *Workspace) LoadGuide() (WorkspaceGuide, error) {
	path := filepath.Join(w.Root, WorkspaceGuideFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultWorkspaceGuide(), nil
		}
		return WorkspaceGuide{}, fmt.Errorf("read workspace guide: %w", err)
	}
	var guide WorkspaceGuide
	if err := json.Unmarshal(data, &guide); err != nil {
		return WorkspaceGuide{}, fmt.Errorf("parse workspace guide: %w", err)
	}
	if err := guide.Validate(); err != nil {
		return WorkspaceGuide{}, err
	}
	return guide, nil
}

func (g WorkspaceGuide) Validate() error {
	if g.Version == 0 {
		return fmt.Errorf("workspace guide version must not be empty")
	}
	if strings.TrimSpace(g.DemoTopic) == "" {
		return fmt.Errorf("workspace guide demo_topic must not be empty")
	}
	if len(g.Directories) == 0 {
		return fmt.Errorf("workspace guide directories must not be empty")
	}
	seen := map[string]bool{}
	for _, rule := range g.Directories {
		itemType := strings.ToLower(strings.TrimSpace(rule.Type))
		if !IsSupportedWorkspaceType(itemType) || itemType == WorkspaceTypeGeneral {
			return fmt.Errorf("workspace guide directory type %q is not supported", rule.Type)
		}
		if seen[itemType] {
			return fmt.Errorf("workspace guide directory type %q is duplicated", itemType)
		}
		seen[itemType] = true
		if err := validateWorkspaceRelPath(rule.Path); err != nil {
			return fmt.Errorf("workspace guide directory %q path: %w", itemType, err)
		}
		if rule.MOC != "" {
			if err := validateWorkspaceRelPath(rule.MOC); err != nil {
				return fmt.Errorf("workspace guide directory %q moc: %w", itemType, err)
			}
		}
		switch strings.TrimSpace(rule.SaveMode) {
		case SaveModeMaterialDir, SaveModeRoleDir, SaveModeRecordDir:
		default:
			return fmt.Errorf("workspace guide directory %q has unsupported save_mode %q", itemType, rule.SaveMode)
		}
	}
	return nil
}

func validateWorkspaceRelPath(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("must not be empty")
	}
	if filepath.IsAbs(path) {
		return fmt.Errorf("must be relative")
	}
	clean := filepath.Clean(path)
	if clean == "." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || clean == ".." {
		return fmt.Errorf("must stay inside workspace")
	}
	return nil
}

func (g WorkspaceGuide) Directory(itemType string) (DirectoryRule, bool) {
	itemType = strings.ToLower(strings.TrimSpace(itemType))
	for _, rule := range g.Directories {
		if strings.ToLower(strings.TrimSpace(rule.Type)) == itemType {
			return rule, true
		}
	}
	return DirectoryRule{}, false
}

func (g WorkspaceGuide) PromptSummary() string {
	rules := append([]DirectoryRule(nil), g.Directories...)
	sort.SliceStable(rules, func(i, j int) bool {
		return rules[i].Path < rules[j].Path
	})
	var b strings.Builder
	b.WriteString("Workspace directory guide:\n")
	for _, rule := range rules {
		b.WriteString(fmt.Sprintf("- %s (%s): %s", rule.Type, filepath.ToSlash(rule.Path), strings.TrimSpace(rule.Responsibility)))
		if len(rule.RequiredSections) > 0 {
			b.WriteString(" Required sections: ")
			b.WriteString(strings.Join(rule.RequiredSections, ", "))
			b.WriteString(".")
		}
		b.WriteString("\n")
	}
	if g.Markdown.DemoRedaction {
		b.WriteString(fmt.Sprintf("- Use %q as the neutral demo topic in generated examples, specs, templates, and tests.\n", g.DemoTopic))
	}
	if len(g.SyncRules) > 0 {
		b.WriteString("Sync rules:\n")
		for _, rule := range g.SyncRules {
			b.WriteString(fmt.Sprintf("- When %s, copy to %s with template %s.\n", rule.When, rule.CopyTo, rule.Template))
		}
	}
	return strings.TrimSpace(b.String())
}
