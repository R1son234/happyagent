package career

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type ReviewLibraryResult struct {
	Paths []string
}

type ReviewQuestionBankGenerator interface {
	GenerateQuestionBank(ctx context.Context, req ReviewQuestionBankRequest) (ReviewQuestionBank, error)
}

// ReviewQuestionBankSetGenerator generates a complete review library set
// with domain name, multiple topics, and project extractions in one LLM call.
type ReviewQuestionBankSetGenerator interface {
	GenerateQuestionBankSet(ctx context.Context, req ReviewQuestionBankSetRequest) (ReviewQuestionBankSet, error)
}

type ReviewQuestionBankRequest struct {
	WorkspaceRoot string
	Domain        ReviewDomain
	Topic         ReviewTopic
	SourceItem    WorkspaceItem
	Context       ReviewLibraryContext
}

type ReviewQuestionBank struct {
	TopicName string           `json:"topic_name"`
	Questions []ReviewQuestion `json:"questions"`
}

// ReviewQuestionBankSet is the LLM-driven output containing domain name,
// multiple topic-based question banks, and extracted projects.
type ReviewQuestionBankSet struct {
	DomainName string               `json:"domain_name"`
	Topics     []ReviewQuestionBank `json:"topics"`
	Projects   []ReviewProjectInput `json:"projects"`
}

// ReviewProjectInput is a project extracted by the LLM from the resume.
type ReviewProjectInput struct {
	ProjectName   string   `json:"project_name"`
	EvidenceLines []string `json:"evidence_lines"`
	StarHint      string   `json:"star_hint"`
}

// UnmarshalJSON implements custom JSON unmarshaling for ReviewProjectInput
// to handle LLM returning string instead of []string for certain fields.
func (p *ReviewProjectInput) UnmarshalJSON(data []byte) error {
	type rawProject struct {
		ProjectName   string          `json:"project_name"`
		EvidenceLines json.RawMessage `json:"evidence_lines"`
		StarHint      string          `json:"star_hint"`
	}
	var raw rawProject
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	p.ProjectName = raw.ProjectName
	p.StarHint = raw.StarHint

	var err error
	p.EvidenceLines, err = unmarshalStringOrArray(raw.EvidenceLines)
	if err != nil {
		return fmt.Errorf("evidence_lines: %w", err)
	}
	return nil
}

type ReviewQuestion struct {
	Question              string   `json:"question"`
	ExamPoints            []string `json:"exam_points"`
	Answer                string   `json:"answer"`
	ResumeBasedAnswer     string   `json:"resume_based_answer"`
	Followups             []string `json:"followups"`
	RiskOrMissingEvidence []string `json:"risk_or_missing_evidence"`
	SourcePaths           []string `json:"source_paths"`
}

// UnmarshalJSON implements custom JSON unmarshaling for ReviewQuestion
// to handle LLM returning string instead of []string for certain fields.
func (q *ReviewQuestion) UnmarshalJSON(data []byte) error {
	type rawQuestion struct {
		Question              string          `json:"question"`
		ExamPoints            json.RawMessage `json:"exam_points"`
		Answer                string          `json:"answer"`
		ResumeBasedAnswer     string          `json:"resume_based_answer"`
		Followups             json.RawMessage `json:"followups"`
		RiskOrMissingEvidence json.RawMessage `json:"risk_or_missing_evidence"`
		SourcePaths           json.RawMessage `json:"source_paths"`
	}
	var raw rawQuestion
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	q.Question = raw.Question
	q.Answer = raw.Answer
	q.ResumeBasedAnswer = raw.ResumeBasedAnswer

	var err error
	q.ExamPoints, err = unmarshalStringOrArray(raw.ExamPoints)
	if err != nil {
		return fmt.Errorf("exam_points: %w", err)
	}
	q.Followups, err = unmarshalStringOrArray(raw.Followups)
	if err != nil {
		return fmt.Errorf("followups: %w", err)
	}
	q.RiskOrMissingEvidence, err = unmarshalStringOrArray(raw.RiskOrMissingEvidence)
	if err != nil {
		return fmt.Errorf("risk_or_missing_evidence: %w", err)
	}
	q.SourcePaths, err = unmarshalStringOrArray(raw.SourcePaths)
	if err != nil {
		return fmt.Errorf("source_paths: %w", err)
	}
	return nil
}

// unmarshalStringOrArray handles JSON values that can be either a string or an array of strings.
func unmarshalStringOrArray(data json.RawMessage) ([]string, error) {
	if len(data) == 0 || string(data) == "null" {
		return nil, nil
	}
	// Try array first
	var arr []string
	if err := json.Unmarshal(data, &arr); err == nil {
		return arr, nil
	}
	// Try single string
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		if strings.TrimSpace(s) == "" {
			return nil, nil
		}
		return []string{s}, nil
	}
	return nil, fmt.Errorf("cannot unmarshal %s into string or []string", string(data))
}

type ReviewDomain struct {
	Slug       string
	Name       string
	Confidence string
}

type ReviewTopic struct {
	Name string
	Slug string
}

type ReviewLibraryContext struct {
	ResumePath        string
	ResumeContent     string
	JDPath            string
	JDContent         string
	ExperiencePath    string
	ExperienceContent string
	Analysis          string
	RoleName          string
	Domain            ReviewDomain
	Topics            []ReviewTopic
}

func (w *Workspace) EnsureReviewLibrarySkeleton(now time.Time) error {
	if now.IsZero() {
		now = time.Now()
	}
	files := map[string]string{
		"面试资料库首页.md": renderHomeIndex(now),
	}
	for rel, content := range files {
		if err := w.writeWorkspaceTextIfMissing(rel, content); err != nil {
			return err
		}
	}
	return nil
}

func (w *Workspace) GenerateReviewLibrary(now time.Time) (ReviewLibraryResult, error) {
	return w.GenerateReviewLibraryWithGenerator(context.Background(), now, nil)
}

func (w *Workspace) GenerateReviewLibraryWithGenerator(ctx context.Context, now time.Time, generator ReviewQuestionBankGenerator) (ReviewLibraryResult, error) {
	if now.IsZero() {
		now = time.Now()
	}
	if err := w.EnsureReviewLibrarySkeleton(now); err != nil {
		return ReviewLibraryResult{}, err
	}
	_, index, err := w.Status()
	if err != nil {
		return ReviewLibraryResult{}, err
	}
	var generated []string
	for _, item := range index.Items {
		if item.Type != WorkspaceTypeExperiences {
			continue
		}
		reviewCtx := w.buildReviewLibraryContext(item, index)
		if strings.TrimSpace(reviewCtx.ExperienceContent) == "" {
			continue
		}
		if generator == nil {
			return ReviewLibraryResult{}, fmt.Errorf("review library question bank generation requires LLM generator")
		}
		paths, err := w.writeExperienceReviewLibrary(ctx, reviewCtx, item, now, generator)
		if err != nil {
			return ReviewLibraryResult{}, err
		}
		generated = append(generated, paths...)
	}
	sort.Strings(generated)
	return ReviewLibraryResult{Paths: uniqueStrings(generated)}, nil
}

func (w *Workspace) buildReviewLibraryContext(experienceItem WorkspaceItem, index WorkspaceIndex) ReviewLibraryContext {
	expContent := readExcerpt(w, experienceItem.Path, 0)
	resume := latestItemOfType(index, WorkspaceTypeResume)
	jd := latestItemOfType(index, WorkspaceTypeJD)
	resumeContent := readExcerpt(w, resume.Path, 0)
	jdContent := readExcerpt(w, jd.Path, 0)
	return ReviewLibraryContext{
		ResumePath:        resume.Path,
		ResumeContent:     resumeContent,
		JDPath:            jd.Path,
		JDContent:         jdContent,
		ExperiencePath:    experienceItem.Path,
		ExperienceContent: expContent,
		Domain:            ReviewDomain{Slug: "general", Name: "通用", Confidence: "low"},
		Topics:            []ReviewTopic{{Name: "通用高频问题", Slug: "general-questions"}},
	}
}

func (w *Workspace) GenerateReviewLibraryWithSetGenerator(ctx context.Context, now time.Time, generator ReviewQuestionBankSetGenerator) (ReviewLibraryResult, error) {
	if now.IsZero() {
		now = time.Now()
	}
	if err := w.EnsureReviewLibrarySkeleton(now); err != nil {
		return ReviewLibraryResult{}, err
	}
	_, index, err := w.Status()
	if err != nil {
		return ReviewLibraryResult{}, err
	}
	var generated []string
	for _, item := range index.Items {
		if item.Type != WorkspaceTypeExperiences {
			continue
		}
		reviewCtx := w.buildReviewLibraryContext(item, index)
		if strings.TrimSpace(reviewCtx.ExperienceContent) == "" {
			continue
		}
		if generator == nil {
			return ReviewLibraryResult{}, fmt.Errorf("review library question bank generation requires LLM generator")
		}
		paths, err := w.writeExperienceReviewLibraryFromSet(ctx, reviewCtx, item, now, generator)
		if err != nil {
			return ReviewLibraryResult{}, err
		}
		generated = append(generated, paths...)
	}
	sort.Strings(generated)
	return ReviewLibraryResult{Paths: uniqueStrings(generated)}, nil
}

func (w *Workspace) writeExperienceReviewLibraryFromSet(runCtx context.Context, ctx ReviewLibraryContext, sourceItem WorkspaceItem, now time.Time, generator ReviewQuestionBankSetGenerator) ([]string, error) {
	if generator == nil {
		return nil, fmt.Errorf("review question bank generation requires LLM generator")
	}

	existingTopics := w.scanExistingTopicNames()

	set, err := generator.GenerateQuestionBankSet(runCtx, ReviewQuestionBankSetRequest{
		WorkspaceRoot:      w.Root,
		SourceItem:         sourceItem,
		Context:            ctx,
		ExistingTopicNames: existingTopics,
	})
	if err != nil {
		return nil, fmt.Errorf("generate question bank set with LLM: %w", err)
	}

	domainSlug := safeFileName(set.DomainName)
	return withWorkspaceMutationLock(func() ([]string, error) {
		var paths []string

		sourcePaths, err := w.writeExperienceSourceOnly(ctx, sourceItem, now)
		if err != nil {
			return nil, err
		}
		paths = append(paths, sourcePaths...)

		for _, bank := range set.Topics {
			topicName := safeFileName(bank.TopicName)
			questionBankRel := filepath.Join(WorkspaceDirPrepare, domainSlug, fmt.Sprintf("%s题库.md", topicName))
			if err := w.writeWorkspaceText(questionBankRel, renderLLMQuestionBankFromSet(bank, ctx, sourceItem)); err != nil {
				return nil, err
			}
			paths = append(paths, filepath.ToSlash(questionBankRel))
		}

		for _, project := range set.Projects {
			if strings.TrimSpace(project.ProjectName) == "" {
				continue
			}
			projectSlug := slugForPath(project.ProjectName)
			projectRel := filepath.Join(WorkspaceDirProjectPack, fmt.Sprintf("%s-interview-qa.md", projectSlug))
			projectAbs := filepath.Join(w.Root, filepath.FromSlash(projectRel))
			if _, statErr := os.Stat(projectAbs); statErr == nil {
				continue
			}
			if err := w.writeWorkspaceText(projectRel, renderProjectQAFromInput(project, ctx, sourceItem, now)); err != nil {
				return nil, err
			}
			paths = append(paths, filepath.ToSlash(projectRel))
		}

		return paths, nil
	})
}

// scanExistingTopicNames scans the prepare directory for existing topic bank files
// and returns their names (without the "题库.md" suffix) for dedup.
func (w *Workspace) scanExistingTopicNames() []string {
	root := filepath.Join(w.Root, WorkspaceDirPrepare)
	var names []string
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		name := filepath.Base(path)
		if strings.HasSuffix(name, "题库.md") {
			topicName := strings.TrimSuffix(name, "题库.md")
			names = append(names, topicName)
		}
		return nil
	})
	return names
}

func (w *Workspace) writeExperienceReviewLibrary(runCtx context.Context, ctx ReviewLibraryContext, sourceItem WorkspaceItem, now time.Time, generator ReviewQuestionBankGenerator) ([]string, error) {
	if generator == nil {
		return nil, fmt.Errorf("review question bank generation requires LLM generator")
	}
	domain := ctx.Domain
	topics := ctx.Topics
	var paths []string
	sourcePaths, err := w.writeExperienceSourceOnly(ctx, sourceItem, now)
	if err != nil {
		return nil, err
	}
	paths = append(paths, sourcePaths...)

	for _, topic := range topics {
		bank, err := generator.GenerateQuestionBank(runCtx, ReviewQuestionBankRequest{
			WorkspaceRoot: w.Root,
			Domain:        domain,
			Topic:         topic,
			SourceItem:    sourceItem,
			Context:       ctx,
		})
		if err != nil {
			return nil, fmt.Errorf("generate %s question bank with LLM: %w", topic.Name, err)
		}
		questionBankRel := filepath.Join(WorkspaceDirPrepare, domain.Slug, fmt.Sprintf("%s题库.md", safeFileName(firstNonEmpty(bank.TopicName, topic.Name))))
		if err := w.writeWorkspaceText(questionBankRel, renderLLMQuestionBank(bank, ctx, topic, sourceItem)); err != nil {
			return nil, err
		}
		paths = append(paths, filepath.ToSlash(questionBankRel))
	}
	return paths, nil
}

func (w *Workspace) writeExperienceSourceOnly(ctx ReviewLibraryContext, sourceItem WorkspaceItem, now time.Time) ([]string, error) {
	sourceRel, err := w.writeExperienceSource(ctx.Domain, sourceItem, ctx.ExperienceContent, now)
	if err != nil {
		return nil, err
	}
	return []string{sourceRel}, nil
}

func (w *Workspace) writeExperienceSource(domain ReviewDomain, sourceItem WorkspaceItem, content string, now time.Time) (string, error) {
	name := sourceItem.Title
	if strings.TrimSpace(name) == "" {
		name = "公开面经"
	}
	stableName := slugForPath(sourceItem.ID)
	if stableName == "" || stableName == "job-description" {
		stableName = slugForPath(name)
	}
	rel := filepath.Join(WorkspaceInternalDir, "sources", domain.Slug, stableName+".md")
	if err := w.writeWorkspaceText(rel, renderSourceMaterial(sourceItem, content, now)); err != nil {
		return "", err
	}
	return filepath.ToSlash(rel), nil
}

func renderHomeIndex(now time.Time) string {
	return `# 面试资料库首页

## 核心入口

- 岗位明细/
- 面经汇总/
- 复习资料库/

## 资料分层

| 层级 | 目录 | 用途 |
| --- | --- | --- |
| 岗位明细 | ` + "`岗位明细/`" + ` | 岗位职责、关键词和匹配关系 |
| 面经汇总 | ` + "`面经汇总/`" + ` | 公开面经、跨公司高频题、通用答案 |
| 复习资料库 | ` + "`复习资料库/`" + ` | 分类知识、项目复习、深挖追问、证据口径 |
| 我的面试 | ` + "`我的面试/`" + ` | 单个岗位的临阵材料、真实复盘 |
| 已归档 | ` + "`已归档/`" + ` | 已处理的原始材料 |
`
}

func renderDomainPackage(domain ReviewDomain, topics []ReviewTopic, sourceRel string, now time.Time) string {
	var b strings.Builder
	b.WriteString("# " + domain.Name + " 面经资料包\n\n")
	b.WriteString("## 维护边界\n\n")
	b.WriteString("本目录维护该方向的公开面经、跨公司高频题、知识点和可说出口答案。新增内容按主题补到对应文档，不再堆回单个长文档。\n\n")
	b.WriteString("## 文档地图\n\n")
	b.WriteString("| 文档 | 职责 |\n| --- | --- |\n")
	b.WriteString(fmt.Sprintf("| 来源资料 | `%s`，保存公开面经原文或来源摘要，复习时作为来源回溯。 |\n", sourceRel))
	for _, topic := range topics {
		b.WriteString(fmt.Sprintf("| %s题库 | `../%s/%s/%s题库.md`，扩展题目、答题要点、追问和项目映射。 |\n", topic.Name, WorkspaceDirPrepare, domain.Slug, topic.Name))
	}
	b.WriteString(fmt.Sprintf("| %s 面经链接与公司观察 | 保存来源、公司岗位画像和高频追问观察。 |\n", domain.Name))
	return b.String()
}

func renderLLMQuestionBank(bank ReviewQuestionBank, ctx ReviewLibraryContext, topic ReviewTopic, sourceItem WorkspaceItem) string {
	topicName := firstNonEmpty(bank.TopicName, topic.Name)
	var b strings.Builder
	b.WriteString("# " + topicName + "题库\n\n")
	for i, item := range bank.Questions {
		question := strings.TrimSpace(item.Question)
		if question == "" {
			question = fmt.Sprintf("%s相关问题", topicName)
		}
		b.WriteString(fmt.Sprintf("## Q%d：%s\n\n", i+1, question))
		b.WriteString("### 考点\n\n")
		writeBullets(&b, item.ExamPoints, []string{"待补充：LLM 输出缺少考点。"})
		b.WriteString("\n### 标准答案\n\n")
		b.WriteString(strings.TrimSpace(item.Answer))
		b.WriteString("\n\n### 结合我的简历怎么答\n\n")
		b.WriteString(strings.TrimSpace(item.ResumeBasedAnswer))
		b.WriteString("\n\n### 可追问\n\n")
		writeBullets(&b, item.Followups, []string{"待补充：LLM 输出缺少追问。"})
		b.WriteString("\n### 风险 / 待补证据\n\n")
		writeBullets(&b, item.RiskOrMissingEvidence, []string{"待补证据：需要补充可验证项目材料、截图、指标来源或复盘原文。"})
		b.WriteString("\n### 关联资料\n\n")
		sourcePaths := item.SourcePaths
		if len(sourcePaths) == 0 {
			sourcePaths = []string{sourceItem.Path, emptyIfBlank(ctx.ResumePath), emptyIfBlank(ctx.JDPath)}
		}
		writeBullets(&b, sourcePaths, []string{sourceItem.Path})
		b.WriteString("\n")
	}
	return b.String()
}

func renderLLMQuestionBankFromSet(bank ReviewQuestionBank, ctx ReviewLibraryContext, sourceItem WorkspaceItem) string {
	topicName := strings.TrimSpace(bank.TopicName)
	if topicName == "" {
		topicName = "通用高频问题"
	}
	var b strings.Builder
	b.WriteString("# " + topicName + "题库\n\n")
	for i, item := range bank.Questions {
		question := strings.TrimSpace(item.Question)
		if question == "" {
			question = fmt.Sprintf("%s相关问题", topicName)
		}
		b.WriteString(fmt.Sprintf("## Q%d：%s\n\n", i+1, question))
		b.WriteString("### 考点\n\n")
		writeBullets(&b, item.ExamPoints, []string{"待补充：LLM 输出缺少考点。"})
		b.WriteString("\n### 标准答案\n\n")
		b.WriteString(strings.TrimSpace(item.Answer))
		b.WriteString("\n\n### 结合我的简历怎么答\n\n")
		b.WriteString(strings.TrimSpace(item.ResumeBasedAnswer))
		b.WriteString("\n\n### 可追问\n\n")
		writeBullets(&b, item.Followups, []string{"待补充：LLM 输出缺少追问。"})
		b.WriteString("\n### 风险 / 待补证据\n\n")
		writeBullets(&b, item.RiskOrMissingEvidence, []string{"待补证据：需要补充可验证项目材料、截图、指标来源或复盘原文。"})
		b.WriteString("\n### 关联资料\n\n")
		sourcePaths := item.SourcePaths
		if len(sourcePaths) == 0 {
			sourcePaths = []string{sourceItem.Path, emptyIfBlank(ctx.ResumePath), emptyIfBlank(ctx.JDPath)}
		}
		writeBullets(&b, sourcePaths, []string{sourceItem.Path})
		b.WriteString("\n")
	}
	return b.String()
}

func renderProjectQAFromInput(project ReviewProjectInput, ctx ReviewLibraryContext, sourceItem WorkspaceItem, now time.Time) string {
	var b strings.Builder
	b.WriteString("# " + project.ProjectName + " 面试 QA\n\n")
	if project.StarHint != "" {
		b.WriteString("## STAR 回答提示\n\n")
		b.WriteString(project.StarHint + "\n\n")
	}
	b.WriteString("## 简历证据\n\n")
	for _, line := range project.EvidenceLines {
		line = strings.TrimSpace(line)
		if line != "" {
			b.WriteString("- " + line + "\n")
		}
	}
	if len(project.EvidenceLines) == 0 {
		b.WriteString("- 待补充：需要从简历中提取相关项目证据。\n")
	}
	b.WriteString("\n## 风险点\n\n")
	b.WriteString("- 不要把「参与/协助」讲成完全 owner，除非简历或材料明确支持。\n")
	b.WriteString("- 没有截图或后台数据前，不扩展新的量化指标。\n")
	return b.String()
}

func writeBullets(b *strings.Builder, values []string, fallback []string) {
	written := false
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		b.WriteString("- " + value + "\n")
		written = true
	}
	if written {
		return
	}
	for _, value := range fallback {
		value = strings.TrimSpace(value)
		if value != "" {
			b.WriteString("- " + value + "\n")
		}
	}
}

func ParseReviewQuestionBankJSON(data []byte) (ReviewQuestionBank, error) {
	var bank ReviewQuestionBank
	if err := json.Unmarshal(data, &bank); err != nil {
		return ReviewQuestionBank{}, fmt.Errorf("parse review question bank json: %w", err)
	}
	if err := ValidateReviewQuestionBank(bank); err != nil {
		return ReviewQuestionBank{}, err
	}
	return bank, nil
}

func ParseReviewQuestionBankString(output string) (ReviewQuestionBank, error) {
	cleaned := cleanLLMJSON(output)
	return ParseReviewQuestionBankJSON([]byte(cleaned))
}

func ValidateReviewQuestionBank(bank ReviewQuestionBank) error {
	if strings.TrimSpace(bank.TopicName) == "" {
		return fmt.Errorf("review question bank missing topic_name")
	}
	if len(bank.Questions) == 0 {
		return fmt.Errorf("review question bank missing questions")
	}
	for i, question := range bank.Questions {
		if strings.TrimSpace(question.Question) == "" {
			return fmt.Errorf("review question bank questions[%d].question must not be empty", i)
		}
		if len(nonEmptyStrings(question.ExamPoints)) == 0 {
			return fmt.Errorf("review question bank questions[%d].exam_points must not be empty", i)
		}
		if strings.TrimSpace(question.Answer) == "" {
			return fmt.Errorf("review question bank questions[%d].answer must not be empty", i)
		}
		if strings.TrimSpace(question.ResumeBasedAnswer) == "" {
			return fmt.Errorf("review question bank questions[%d].resume_based_answer must not be empty", i)
		}
		if len(nonEmptyStrings(question.Followups)) == 0 {
			return fmt.Errorf("review question bank questions[%d].followups must not be empty", i)
		}
		// risk_or_missing_evidence is optional - some questions may have no risks
		if len(nonEmptyStrings(question.SourcePaths)) == 0 {
			return fmt.Errorf("review question bank questions[%d].source_paths must not be empty", i)
		}
	}
	return nil
}

func nonEmptyStrings(values []string) []string {
	var out []string
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			out = append(out, value)
		}
	}
	return out
}

func ParseReviewQuestionBankSetJSON(data []byte) (ReviewQuestionBankSet, error) {
	var set ReviewQuestionBankSet
	if err := json.Unmarshal(data, &set); err != nil {
		return ReviewQuestionBankSet{}, fmt.Errorf("parse review question bank set json: %w", err)
	}
	if err := ValidateReviewQuestionBankSet(set); err != nil {
		return ReviewQuestionBankSet{}, err
	}
	return set, nil
}

func ParseReviewQuestionBankSetString(output string) (ReviewQuestionBankSet, error) {
	cleaned := cleanLLMJSON(output)
	return ParseReviewQuestionBankSetJSON([]byte(cleaned))
}

// cleanLLMJSON attempts to fix common JSON formatting issues from LLM output.
func cleanLLMJSON(input string) string {
	s := strings.TrimSpace(input)

	// Remove markdown code fences if present
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	s = strings.TrimSpace(s)

	// Try parsing as-is first
	if json.Valid([]byte(s)) {
		return s
	}

	// Fix trailing commas before } or ]
	// This is a common LLM mistake
	result := make([]byte, 0, len(s))
	inString := false
	escaped := false
	for i := 0; i < len(s); i++ {
		ch := s[i]

		if escaped {
			result = append(result, ch)
			escaped = false
			continue
		}

		if ch == '\\' && inString {
			result = append(result, ch)
			escaped = true
			continue
		}

		if ch == '"' {
			inString = !inString
			result = append(result, ch)
			continue
		}

		if inString {
			result = append(result, ch)
			continue
		}

		// Outside string: skip trailing commas
		if ch == ',' {
			// Look ahead to see if next non-whitespace is } or ]
			j := i + 1
			for j < len(s) && (s[j] == ' ' || s[j] == '\t' || s[j] == '\n' || s[j] == '\r') {
				j++
			}
			if j < len(s) && (s[j] == '}' || s[j] == ']') {
				continue // skip the comma
			}
		}

		result = append(result, ch)
	}

	s = string(result)

	// Try parsing again
	if json.Valid([]byte(s)) {
		return s
	}

	// If still invalid, return original (let the parser give a more specific error)
	return strings.TrimSpace(input)
}

func ValidateReviewQuestionBankSet(set ReviewQuestionBankSet) error {
	if strings.TrimSpace(set.DomainName) == "" {
		return fmt.Errorf("review question bank set missing domain_name")
	}
	if len(set.Topics) == 0 {
		return fmt.Errorf("review question bank set missing topics")
	}
	for i, topic := range set.Topics {
		if err := ValidateReviewQuestionBank(topic); err != nil {
			return fmt.Errorf("topics[%d]: %w", i, err)
		}
	}
	return nil
}

func renderSourceMaterial(sourceItem WorkspaceItem, content string, now time.Time) string {
	var b strings.Builder
	b.WriteString("# " + sourceItem.Title + "\n\n")
	b.WriteString("> 公开面经源资料，用于回溯题目来源；不是用户真实面试记录。\n\n")
	b.WriteString(strings.TrimSpace(content))
	b.WriteString("\n")
	return b.String()
}

func latestItemOfType(index WorkspaceIndex, itemType string) WorkspaceItem {
	var latest WorkspaceItem
	for _, item := range index.Items {
		if item.Type != itemType {
			continue
		}
		if latest.ID == "" || item.CreatedAt.After(latest.CreatedAt) {
			latest = item
		}
	}
	return latest
}

func safeFileName(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, "/", "")
	value = strings.ReplaceAll(value, "\\", "")
	value = strings.Join(strings.Fields(value), "")
	if len([]rune(value)) > 36 {
		value = string([]rune(value)[:36])
	}
	// Strip common meaningless prefixes
	for _, prefix := range []string{"面经_", "面经-", "JD_", "岗位JD_", "复习资料_", "题库_"} {
		value = strings.TrimPrefix(value, prefix)
	}
	replacer := strings.NewReplacer("/", "", "\\", "", ":", "", "*", "", "?", "", "\"", "", "<", "", ">", "", "|", "")
	value = strings.TrimSpace(replacer.Replace(value))
	if value == "" {
		return "通用岗位"
	}
	return value
}

func firstN(values []string, limit int) []string {
	if limit <= 0 || len(values) <= limit {
		return values
	}
	return values[:limit]
}

func uniqueStrings(values []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func slugForPath(value string) string {
	s := slug(value)
	if s == "job-description" {
		return fmt.Sprintf("material-%s", ContentFingerprint(value)[:8])
	}
	return s
}

func (w *Workspace) writeWorkspaceTextIfMissing(relPath string, content string) error {
	abs := filepath.Join(w.Root, filepath.FromSlash(relPath))
	if _, err := os.Stat(abs); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat workspace text %q: %w", relPath, err)
	}
	return w.writeWorkspaceText(relPath, content)
}
