package career

import (
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
		"面试资料库首页.md":                            renderHomeIndex(now),
		filepath.Join("experiences", "面经总览.md"): renderExperienceIndex(now),
		filepath.Join("prepare", "项目专项总览.md"):   renderPrepareIndex(now),
		filepath.Join("jd", "JD 汇总.md"):         renderJDIndex(now),
	}
	for rel, content := range files {
		if err := w.writeWorkspaceTextIfMissing(rel, content); err != nil {
			return err
		}
	}
	return nil
}

func (w *Workspace) GenerateReviewLibrary(now time.Time) (ReviewLibraryResult, error) {
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
		ctx := w.buildReviewLibraryContext(item, index)
		if strings.TrimSpace(ctx.ExperienceContent) == "" {
			continue
		}
		paths, err := w.writeExperienceReviewLibrary(ctx, item, now)
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
	combined := strings.Join([]string{experienceItem.Title, jd.Title, jdContent, expContent, resumeContent}, "\n")
	domain := inferReviewDomain(combined, experienceItem.Title)
	roleName := inferRoleName(jdContent, expContent, experienceItem.Title, domain)
	topics := inferReviewTopics(expContent + "\n" + jdContent)
	if len(topics) == 0 {
		topics = []ReviewTopic{{Name: "通用高频问题", Slug: "general-questions"}}
	}
	return ReviewLibraryContext{
		ResumePath:        resume.Path,
		ResumeContent:     resumeContent,
		JDPath:            jd.Path,
		JDContent:         jdContent,
		ExperiencePath:    experienceItem.Path,
		ExperienceContent: expContent,
		RoleName:          roleName,
		Domain:            domain,
		Topics:            topics,
	}
}

func (w *Workspace) writeExperienceReviewLibrary(ctx ReviewLibraryContext, sourceItem WorkspaceItem, now time.Time) ([]string, error) {
	domain := ctx.Domain
	topics := ctx.Topics
	var paths []string
	sourceRel, err := w.writeExperienceSource(domain, sourceItem, ctx.ExperienceContent, now)
	if err != nil {
		return nil, err
	}
	paths = append(paths, sourceRel)

	packageRel := filepath.Join("experiences", domain.Slug, fmt.Sprintf("%s 面经资料包.md", domain.Name))
	if err := w.writeWorkspaceText(packageRel, renderDomainPackage(domain, topics, sourceRel, now)); err != nil {
		return nil, err
	}
	paths = append(paths, filepath.ToSlash(packageRel))

	observationsRel := filepath.Join("experiences", domain.Slug, fmt.Sprintf("%s 面经链接与公司观察.md", domain.Name))
	if err := w.writeWorkspaceText(observationsRel, renderExperienceObservations(domain, sourceItem, sourceRel, ctx.ExperienceContent, now)); err != nil {
		return nil, err
	}
	paths = append(paths, filepath.ToSlash(observationsRel))

	for _, topic := range topics {
		topicRel := filepath.Join("experiences", domain.Slug, fmt.Sprintf("%s题库.md", topic.Name))
		if err := w.writeWorkspaceText(topicRel, renderTopicQuestionBank(ctx, topic, sourceItem, now)); err != nil {
			return nil, err
		}
		paths = append(paths, filepath.ToSlash(topicRel))
	}
	if err := w.refreshExperienceIndex(domain, topics, now); err != nil {
		return nil, err
	}
	rolePaths, err := w.writeRoleReviewDocuments(ctx, now)
	if err != nil {
		return nil, err
	}
	paths = append(paths, rolePaths...)
	projectPaths, err := w.writeProjectQADocuments(ctx, now)
	if err != nil {
		return nil, err
	}
	paths = append(paths, projectPaths...)
	if err := w.refreshJDIndex(ctx, now); err != nil {
		return nil, err
	}
	paths = append(paths, filepath.ToSlash(filepath.Join("jd", "JD 汇总.md")))
	return paths, nil
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
	rel := filepath.Join("experiences", domain.Slug, "sources", stableName+".md")
	if err := w.writeWorkspaceText(rel, renderSourceMaterial(sourceItem, content, now)); err != nil {
		return "", err
	}
	return filepath.ToSlash(rel), nil
}

func (w *Workspace) refreshExperienceIndex(domain ReviewDomain, topics []ReviewTopic, now time.Time) error {
	var b strings.Builder
	b.WriteString(frontmatter("面经总览", []string{"moc", "interview/experience"}, now))
	b.WriteString("# 面经总览\n\n")
	b.WriteString("## 方向入口\n\n")
	b.WriteString(fmt.Sprintf("- [[experiences/%s/%s 面经资料包|%s 面经资料包]]\n", domain.Slug, domain.Name, domain.Name))
	b.WriteString("\n## 最近更新\n\n")
	for _, topic := range topics {
		b.WriteString(fmt.Sprintf("- [[experiences/%s/%s题库|%s题库]]\n", domain.Slug, topic.Name, topic.Name))
	}
	return w.writeWorkspaceText(filepath.Join("experiences", "面经总览.md"), b.String())
}

func renderHomeIndex(now time.Time) string {
	return frontmatter("面试资料库首页", []string{"moc", "interview", "career"}, now) + `# 面试资料库首页

## 核心入口

- [[JD 汇总]]
- [[面经总览]]
- [[项目专项总览]]

## 资料分层

| 层级 | 目录 | 用途 |
| --- | --- | --- |
| JD | ` + "`jd/`" + ` | 岗位职责、关键词和匹配关系 |
| 通用面经 | ` + "`experiences/`" + ` | 公开面经、跨公司高频题、通用答案 |
| 项目专项 | ` + "`prepare/`" + ` | 个人项目介绍、深挖追问、证据口径 |
| 具体岗位 | ` + "`my-interviews/`" + ` | 单个岗位的临阵材料、真实复盘 |
| 过程记录 | ` + "`record/`" + ` | 导入、分类、生成过程记录 |
`
}

func renderExperienceIndex(now time.Time) string {
	return frontmatter("面经总览", []string{"moc", "interview/experience"}, now) + "# 面经总览\n\n## 方向入口\n\n- 暂无方向资料。导入公开面经或运行 `/library` 后会自动更新。\n"
}

func renderPrepareIndex(now time.Time) string {
	return frontmatter("项目专项总览", []string{"moc", "interview/prepare"}, now) + "# 项目专项总览\n\n## 项目入口\n\n- 暂无项目专项资料。导入项目材料后会自动更新。\n"
}

func renderJDIndex(now time.Time) string {
	return frontmatter("JD 汇总", []string{"moc", "interview/jd"}, now) + "# JD 汇总\n\n## JD 入口\n\n- 暂无 JD。导入 JD 后会自动更新。\n"
}

func renderDomainPackage(domain ReviewDomain, topics []ReviewTopic, sourceRel string, now time.Time) string {
	var b strings.Builder
	b.WriteString(frontmatter(domain.Name+" 面经资料包", []string{"moc", "interview/experience", domain.Slug}, now))
	b.WriteString("# " + domain.Name + " 面经资料包\n\n")
	b.WriteString("## 维护边界\n\n")
	b.WriteString("本目录维护该方向的公开面经、跨公司高频题、知识点和可说出口答案。新增内容按主题补到对应文档，不再堆回单个长文档。\n\n")
	b.WriteString("## 文档地图\n\n")
	b.WriteString("| 文档 | 职责 |\n| --- | --- |\n")
	b.WriteString(fmt.Sprintf("| [[%s|来源资料]] | 保存公开面经原文或来源摘要，复习时作为来源回溯。 |\n", strings.TrimSuffix(sourceRel, ".md")))
	for _, topic := range topics {
		b.WriteString(fmt.Sprintf("| [[%s题库]] | 维护%s相关问题、答题要点、追问和项目映射。 |\n", topic.Name, topic.Name))
	}
	b.WriteString(fmt.Sprintf("| [[%s 面经链接与公司观察]] | 保存来源、公司岗位画像和高频追问观察。 |\n", domain.Name))
	return b.String()
}

func renderExperienceObservations(domain ReviewDomain, sourceItem WorkspaceItem, sourceRel string, content string, now time.Time) string {
	var b strings.Builder
	b.WriteString(frontmatter(domain.Name+" 面经链接与公司观察", []string{"interview/experience", domain.Slug}, now))
	b.WriteString("# " + domain.Name + " 面经链接与公司观察\n\n")
	b.WriteString("## 来源\n\n")
	b.WriteString(fmt.Sprintf("- 来源资料：`%s`\n", sourceRel))
	b.WriteString(fmt.Sprintf("- 原始归档：`%s`\n", sourceItem.Path))
	b.WriteString("- 资料性质：公开面经，不是用户真实面试记录。\n")
	b.WriteString(fmt.Sprintf("- 方向识别：%s（%s）\n\n", domain.Name, domain.Confidence))
	b.WriteString("## 高频信号\n\n")
	for _, signal := range extractSignals(content, 8) {
		b.WriteString("- " + signal + "\n")
	}
	return b.String()
}

func renderTopicQuestionBank(ctx ReviewLibraryContext, topic ReviewTopic, sourceItem WorkspaceItem, now time.Time) string {
	questions := inferQuestionsForTopic(topic.Name, ctx.ExperienceContent)
	if len(questions) == 0 {
		questions = []string{inferQuestionForTopic(topic.Name, ctx.ExperienceContent)}
	}
	var b strings.Builder
	b.WriteString(frontmatter(topic.Name+"题库", []string{"interview/experience", ctx.Domain.Slug}, now))
	b.WriteString("# " + topic.Name + "题库\n\n")
	b.WriteString(fmt.Sprintf("> 来源：`%s`。公开面经资料，不是用户真实面试记录。\n\n", sourceItem.Path))
	for i, question := range questions {
		b.WriteString(fmt.Sprintf("## Q%d：%s\n\n", i+1, question))
		b.WriteString("### 面试官想考\n\n")
		for _, point := range topicExamSignals(topic.Name) {
			b.WriteString("- " + point + "\n")
		}
		b.WriteString("\n### 可直接说的答案\n\n")
		b.WriteString(renderAnswerForQuestion(question, ctx))
		b.WriteString("\n\n### 结合简历的项目映射\n\n")
		for _, evidence := range resumeEvidenceBullets(ctx.ResumeContent, 4) {
			b.WriteString("- " + evidence + "\n")
		}
		b.WriteString("\n### 可能追问\n\n")
		for _, follow := range followupQuestions(question) {
			b.WriteString("- " + follow + "\n")
		}
		b.WriteString("\n### 证据边界\n\n")
		b.WriteString("- 已证实：" + evidenceBoundary(ctx.ResumeContent) + "\n")
		b.WriteString("- 待确认：作品集截图、后台数据截图、具体复盘报告原文和面试目标公司的业务信息。\n\n")
	}
	return b.String()
}

func renderSourceMaterial(sourceItem WorkspaceItem, content string, now time.Time) string {
	var b strings.Builder
	b.WriteString(frontmatter(sourceItem.Title, []string{"interview/source"}, now))
	b.WriteString("# " + sourceItem.Title + "\n\n")
	b.WriteString("> 公开面经源资料，用于回溯题目来源；不是用户真实面试记录。\n\n")
	b.WriteString(strings.TrimSpace(content))
	b.WriteString("\n")
	return b.String()
}

func frontmatter(title string, tags []string, now time.Time) string {
	if now.IsZero() {
		now = time.Now()
	}
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("title: " + title + "\n")
	b.WriteString("tags:\n")
	for _, tag := range tags {
		if strings.TrimSpace(tag) == "" {
			continue
		}
		b.WriteString("  - " + tag + "\n")
	}
	b.WriteString("status: active\n")
	b.WriteString("updated: " + now.Format("2006-01-02") + "\n")
	b.WriteString("---\n\n")
	return b.String()
}

func (w *Workspace) writeRoleReviewDocuments(ctx ReviewLibraryContext, now time.Time) ([]string, error) {
	roleDir := filepath.Join("my-interviews", safeFileName(ctx.RoleName))
	files := map[string]string{
		filepath.Join(roleDir, "00-JD结构化画像.md"):        renderJDProfile(ctx, now),
		filepath.Join(roleDir, "01-临阵抗拷打主文档.md"):       renderCrammingDoc(ctx, now),
		filepath.Join(roleDir, "02-复习计划.md"):           renderReviewPlan(ctx, now),
		filepath.Join(roleDir, "03-面经来源与JD关联补充.md"):    renderExperienceJDLink(ctx, now),
		filepath.Join(roleDir, ctx.RoleName+"岗作战页.md"): renderRoleBattlePage(ctx, now),
	}
	var paths []string
	for rel, content := range files {
		if err := w.writeWorkspaceText(rel, content); err != nil {
			return nil, err
		}
		paths = append(paths, filepath.ToSlash(rel))
	}
	return paths, nil
}

func (w *Workspace) writeProjectQADocuments(ctx ReviewLibraryContext, now time.Time) ([]string, error) {
	projects := inferProjectsFromResume(ctx.ResumeContent)
	if len(projects) == 0 {
		return nil, nil
	}
	var paths []string
	for _, project := range projects {
		rel := filepath.Join("prepare", slugForPath(project.Name)+"-interview-qa.md")
		if err := w.writeWorkspaceText(rel, renderProjectQA(project, ctx, now)); err != nil {
			return nil, err
		}
		paths = append(paths, filepath.ToSlash(rel))
	}
	if err := w.refreshPrepareIndex(projects, now); err != nil {
		return nil, err
	}
	paths = append(paths, filepath.ToSlash(filepath.Join("prepare", "项目专项总览.md")))
	return paths, nil
}

func (w *Workspace) refreshPrepareIndex(projects []ResumeProject, now time.Time) error {
	var b strings.Builder
	b.WriteString(frontmatter("项目专项总览", []string{"moc", "interview/prepare"}, now))
	b.WriteString("# 项目专项总览\n\n## 项目入口\n\n")
	for _, project := range projects {
		b.WriteString(fmt.Sprintf("- [[%s-interview-qa|%s]]\n", slugForPath(project.Name), project.Name))
	}
	return w.writeWorkspaceText(filepath.Join("prepare", "项目专项总览.md"), b.String())
}

func (w *Workspace) refreshJDIndex(ctx ReviewLibraryContext, now time.Time) error {
	var b strings.Builder
	b.WriteString(frontmatter("JD 汇总", []string{"moc", "interview/jd"}, now))
	b.WriteString("# JD 汇总\n\n")
	b.WriteString("## 当前岗位\n\n")
	b.WriteString(fmt.Sprintf("- [[my-interviews/%s/00-JD结构化画像|%s JD 结构化画像]]\n", safeFileName(ctx.RoleName), ctx.RoleName))
	if strings.TrimSpace(ctx.JDPath) != "" {
		b.WriteString(fmt.Sprintf("- 源资料：`%s`\n", ctx.JDPath))
	}
	return w.writeWorkspaceText(filepath.Join("jd", "JD 汇总.md"), b.String())
}

func renderJDProfile(ctx ReviewLibraryContext, now time.Time) string {
	var b strings.Builder
	b.WriteString(frontmatter("JD结构化画像", []string{"interview/jd", ctx.Domain.Slug}, now))
	b.WriteString("# JD结构化画像\n\n")
	b.WriteString("## 岗位目标\n\n")
	b.WriteString("- " + firstNonEmptyLine(ctx.JDContent, ctx.RoleName) + "\n\n")
	b.WriteString("## 核心职责与关键词\n\n")
	for _, signal := range extractSignals(ctx.JDContent, 8) {
		b.WriteString("- " + signal + "\n")
	}
	b.WriteString("\n## 简历匹配证据\n\n")
	for _, evidence := range resumeEvidenceBullets(ctx.ResumeContent, 6) {
		b.WriteString("- " + evidence + "\n")
	}
	b.WriteString("\n## 缺口与补证据建议\n\n")
	b.WriteString("- 补充作品集截图、账号后台数据、投放复盘报告和具体内容案例。\n")
	b.WriteString("- 面试前把每个数据结果对应到具体动作，避免只背数字。\n")
	return b.String()
}

func renderCrammingDoc(ctx ReviewLibraryContext, now time.Time) string {
	questions := inferQuestionsForTopic("", ctx.ExperienceContent)
	var b strings.Builder
	b.WriteString(frontmatter("临阵抗拷打主文档", []string{"interview/brief", ctx.Domain.Slug}, now))
	b.WriteString("# 临阵抗拷打主文档\n\n")
	b.WriteString("## 先背 3 个核心答案\n\n")
	b.WriteString("### 1. 自我介绍\n\n")
	b.WriteString(renderSelfIntro(ctx) + "\n\n")
	b.WriteString("### 2. 最强项目案例\n\n")
	b.WriteString(renderBestProjectPitch(ctx) + "\n\n")
	b.WriteString("### 3. 岗位理解\n\n")
	b.WriteString(renderRoleUnderstanding(ctx) + "\n\n")
	b.WriteString("## 高频追问\n\n")
	for _, q := range firstN(questions, 8) {
		b.WriteString("- " + q + "\n")
	}
	b.WriteString("\n## 面试前 30 分钟\n\n")
	b.WriteString("- 看 `00-JD结构化画像.md` 的岗位关键词和匹配证据。\n")
	b.WriteString("- 看本页 3 个核心答案。\n")
	b.WriteString("- 看 `03-面经来源与JD关联补充.md` 的高频追问。\n")
	return b.String()
}

func renderReviewPlan(ctx ReviewLibraryContext, now time.Time) string {
	var b strings.Builder
	b.WriteString(frontmatter("复习计划", []string{"interview/plan", ctx.Domain.Slug}, now))
	b.WriteString("# 复习计划\n\n")
	b.WriteString("## Day 1\n\n")
	b.WriteString("- 梳理 JD 关键词，背熟自我介绍和岗位理解。\n")
	b.WriteString("- 准备作品集截图、账号主页、爆款内容和数据后台。\n\n")
	b.WriteString("## Day 2\n\n")
	b.WriteString("- 按题库逐题口述，重点练项目深挖和账号诊断。\n")
	b.WriteString("- 检查每个量化结果是否有证据图或原文支撑。\n")
	return b.String()
}

func renderExperienceJDLink(ctx ReviewLibraryContext, now time.Time) string {
	var b strings.Builder
	b.WriteString(frontmatter("面经来源与JD关联补充", []string{"interview/experience", ctx.Domain.Slug}, now))
	b.WriteString("# 面经来源与JD关联补充\n\n")
	b.WriteString("## 来源\n\n")
	b.WriteString(fmt.Sprintf("- JD：`%s`\n", emptyIfBlank(ctx.JDPath)))
	b.WriteString(fmt.Sprintf("- 面经：`%s`\n", emptyIfBlank(ctx.ExperiencePath)))
	b.WriteString(fmt.Sprintf("- 简历：`%s`\n\n", emptyIfBlank(ctx.ResumePath)))
	b.WriteString("## JD 与面经交集\n\n")
	for _, signal := range intersectSignals(ctx.JDContent, ctx.ExperienceContent, 8) {
		b.WriteString("- " + signal + "\n")
	}
	b.WriteString("\n## 需要补的材料\n\n")
	b.WriteString("- 对应每个高频追问准备一个真实项目截图或复盘材料。\n")
	return b.String()
}

func renderRoleBattlePage(ctx ReviewLibraryContext, now time.Time) string {
	var b strings.Builder
	b.WriteString(frontmatter(ctx.RoleName+"岗作战页", []string{"interview/role", ctx.Domain.Slug}, now))
	b.WriteString("# " + ctx.RoleName + "岗作战页\n\n")
	b.WriteString("## 一句话策略\n\n")
	b.WriteString("围绕岗位关键词，把简历中的真实平台运营、内容策划、数据复盘和合作经历讲成可验证案例。\n\n")
	b.WriteString("## 最强匹配证据\n\n")
	for _, evidence := range resumeEvidenceBullets(ctx.ResumeContent, 5) {
		b.WriteString("- " + evidence + "\n")
	}
	b.WriteString("\n## 最可能被问\n\n")
	for _, q := range firstN(inferQuestionsForTopic("", ctx.ExperienceContent), 6) {
		b.WriteString("- " + q + "\n")
	}
	b.WriteString("\n## 先看哪些文件\n\n")
	b.WriteString("- [[00-JD结构化画像]]\n")
	b.WriteString("- [[01-临阵抗拷打主文档]]\n")
	b.WriteString("- [[03-面经来源与JD关联补充]]\n")
	return b.String()
}

type ResumeProject struct {
	Name  string
	Lines []string
}

func renderProjectQA(project ResumeProject, ctx ReviewLibraryContext, now time.Time) string {
	var b strings.Builder
	b.WriteString(frontmatter(project.Name+" 面试 QA", []string{"interview/prepare", ctx.Domain.Slug}, now))
	b.WriteString("# " + project.Name + " 面试 QA\n\n")
	b.WriteString("## 项目一句话\n\n")
	b.WriteString("- " + strings.Join(firstN(project.Lines, 2), "；") + "\n\n")
	b.WriteString("## STAR 回答\n\n")
	b.WriteString("- Situation：围绕岗位需要，说明当时的业务目标和用户场景。\n")
	b.WriteString("- Task：说明自己负责的内容、投放、协作或数据分析任务。\n")
	b.WriteString("- Action：结合简历证据讲清具体动作。\n")
	b.WriteString("- Result：只使用简历中已经提供的数字和结果，不补造指标。\n\n")
	b.WriteString("## 数据与证据\n\n")
	for _, line := range project.Lines {
		b.WriteString("- " + line + "\n")
	}
	b.WriteString("\n## 可能追问\n\n")
	for _, q := range firstN(inferQuestionsForTopic("项目深挖", ctx.ExperienceContent), 4) {
		b.WriteString("- " + q + "\n")
	}
	b.WriteString("\n## 风险点\n\n")
	b.WriteString("- 不要把“参与/协助”讲成完全 owner，除非简历或材料明确支持。\n")
	b.WriteString("- 没有截图或后台数据前，不扩展新的量化指标。\n")
	return b.String()
}

func inferReviewDomain(content string, title string) ReviewDomain {
	if label := explicitDomainLabel(title, content); label != "" {
		return ReviewDomain{Slug: domainSlug(label), Name: label, Confidence: "high"}
	}
	text := strings.ToLower(title + "\n" + content)
	candidates := []struct {
		slug    string
		name    string
		signals []string
	}{
		{slug: "ai-agent", name: "AI Agent", signals: []string{"ai agent", "agent", "llm", "rag", "tool calling", "智能体", "大模型"}},
		{slug: "backend", name: "后端", signals: []string{"backend", "后端", "java", "golang", "go ", "mysql", "redis", "mq", "微服务"}},
		{slug: "product", name: "产品", signals: []string{"product", "产品经理", "prd", "需求分析", "用户研究"}},
		{slug: "operations", name: "运营", signals: []string{"运营", "增长", "活动", "转化", "留存", "用户增长"}},
		{slug: "marketing", name: "市场营销", signals: []string{"marketing", "市场营销", "品牌", "投放", "商业化"}},
		{slug: "design", name: "设计", signals: []string{"designer", "设计师", "作品集", "交互设计", "视觉设计"}},
	}
	best := ReviewDomain{Slug: "general", Name: "通用", Confidence: "low"}
	bestScore := 0
	for _, candidate := range candidates {
		score := 0
		for _, signal := range candidate.signals {
			if strings.Contains(text, strings.ToLower(signal)) {
				score++
			}
		}
		if score > bestScore {
			bestScore = score
			best = ReviewDomain{Slug: candidate.slug, Name: candidate.name, Confidence: "medium"}
		}
	}
	if bestScore >= 2 {
		best.Confidence = "high"
	}
	return best
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

func inferRoleName(jdContent string, experienceContent string, title string, domain ReviewDomain) string {
	for _, source := range []string{jdContent, experienceContent, title, domain.Name} {
		for _, line := range strings.Split(source, "\n") {
			line = strings.TrimSpace(strings.Trim(line, "# 　\t"))
			if line == "" {
				continue
			}
			line = strings.TrimSuffix(line, " JD 对照")
			line = strings.TrimSuffix(line, "面经整理")
			if strings.Contains(line, "实习生") || strings.Contains(line, "工程师") || strings.Contains(line, "经理") || strings.Contains(strings.ToLower(line), "intern") {
				return cleanRoleName(line)
			}
		}
	}
	return cleanRoleName(domain.Name)
}

func cleanRoleName(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, "/", "")
	value = strings.ReplaceAll(value, "\\", "")
	value = strings.Join(strings.Fields(value), "")
	if value == "" {
		return "通用岗位"
	}
	if len([]rune(value)) > 36 {
		return string([]rune(value)[:36])
	}
	return value
}

func safeFileName(value string) string {
	value = cleanRoleName(value)
	replacer := strings.NewReplacer("/", "", "\\", "", ":", "", "*", "", "?", "", "\"", "", "<", "", ">", "", "|", "")
	value = strings.TrimSpace(replacer.Replace(value))
	if value == "" {
		return "通用岗位"
	}
	return value
}

func explicitDomainLabel(title string, content string) string {
	lines := []string{title}
	for _, line := range strings.Split(content, "\n") {
		lines = append(lines, line)
		if len(lines) >= 6 {
			break
		}
	}
	for _, line := range lines {
		line = strings.TrimSpace(strings.Trim(line, "# 　\t"))
		if line == "" {
			continue
		}
		for _, marker := range []string{"公开面经", "面经", "面试题", "岗位"} {
			if idx := strings.Index(line, marker); idx > 0 {
				label := strings.TrimSpace(strings.Trim(line[:idx], "：:-—| "))
				if isGenericDomainLabel(label) {
					continue
				}
				if label != "" && len([]rune(label)) <= 24 {
					return label
				}
			}
		}
	}
	return ""
}

func isGenericDomainLabel(label string) bool {
	switch strings.TrimSpace(label) {
	case "", "公开", "公开面试", "面试", "真实", "我的", "我":
		return true
	default:
		return false
	}
}

func domainSlug(label string) string {
	normalized := strings.ToLower(label)
	replacements := []struct {
		from string
		to   string
	}{
		{"市场营销", "marketing"},
		{"新媒体运营", "new-media-operations"},
		{"内容运营", "content-operations"},
		{"直播", "live-streaming"},
		{"实习生", "intern"},
		{"产品经理", "product-manager"},
		{"后端", "backend"},
		{"设计", "design"},
		{"运营", "operations"},
		{"智能体", "ai-agent"},
	}
	for _, replacement := range replacements {
		normalized = strings.ReplaceAll(normalized, replacement.from, "-"+replacement.to+"-")
	}
	s := slug(normalized)
	if s != "job-description" {
		return s
	}
	fp := ContentFingerprint(label)
	if len(fp) > 8 {
		fp = fp[:8]
	}
	return "domain-" + fp
}

func inferQuestionsForTopic(topic string, content string) []string {
	var questions []string
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(strings.Trim(line, "-*# 　\t"))
		if line == "" {
			continue
		}
		if strings.Contains(line, "？") || strings.Contains(line, "?") || strings.HasPrefix(line, "你") || strings.Contains(line, "如何") || strings.Contains(line, "怎么") || strings.Contains(line, "能否") {
			questions = append(questions, normalizeQuestion(line))
		}
	}
	if len(questions) == 0 {
		switch topic {
		case "项目深挖":
			questions = []string{
				"上一段经历中你如何做出高表现内容或项目结果？",
				"你最有成就感的一段经历是什么，为什么？",
				"如果数据不理想，你会如何定位问题并调整？",
			}
		case "业务与策略":
			questions = []string{
				"你如何理解这个岗位的业务目标？",
				"给你一个账号，你会如何判断问题并提出优化建议？",
				"运营需要关注哪些核心指标？",
			}
		case "行为面试":
			questions = []string{
				"你的个人优势是什么，请结合经历说明。",
				"朋友或同事会如何评价你？",
			}
		}
	}
	return uniqueStrings(firstN(questions, 8))
}

func normalizeQuestion(line string) string {
	line = strings.TrimSpace(line)
	if len([]rune(line)) > 90 {
		line = string([]rune(line)[:90])
	}
	line = strings.TrimRight(line, "。；;")
	if !strings.Contains(line, "？") && !strings.Contains(line, "?") {
		line += "？"
	}
	return line
}

func topicExamSignals(topic string) []string {
	switch topic {
	case "项目深挖":
		return []string{"是否能讲清目标、动作、结果和复盘", "是否能把简历数字对应到具体执行过程", "是否能处理失败和优化追问"}
	case "行为面试":
		return []string{"表达是否稳定真实", "是否能用具体经历支撑个人优势", "是否能说明协作和抗压方式"}
	case "业务与策略":
		return []string{"是否理解用户、内容、数据和转化链路", "是否能从账号现状拆出可执行优化", "是否关注核心指标和复盘闭环"}
	default:
		return []string{"是否能结合岗位目标回答", "是否有真实证据支撑", "是否能说明取舍和复盘"}
	}
}

func renderAnswerForQuestion(question string, ctx ReviewLibraryContext) string {
	lower := strings.ToLower(question)
	switch {
	case strings.Contains(question, "自我介绍"):
		return renderSelfIntro(ctx)
	case strings.Contains(question, "账号") || strings.Contains(question, "运营") || strings.Contains(lower, "account"):
		return "我会按定位、内容、节奏、数据、转化五个维度看。先确认账号面向谁、内容是否聚焦，再看标题封面和发布节奏，最后用播放、完播、互动、收藏、转粉等指标定位问题。结合我的经历，我可以讲个人账号定位、对标账号分析、选题标题、素材制作和复盘迭代，但具体后台截图需要作品集补证据。"
	case strings.Contains(question, "爆款") || strings.Contains(question, "高表现"):
		return "我会用结果先行的方式回答：先说目标和结果，再拆用户洞察、选题来源、标题封面、发布渠道、互动维护和复盘动作。简历里已证实的证据包括多平台投放、KOL 合作、累计曝光和个人账号粉丝，但每个案例的截图和后台数据要在作品集中补齐。"
	default:
		return "我会先说明目标和判断标准，再结合简历中已证实的经历回答。回答时只使用材料里已有事实，例如平台经验、数据复盘、合作次数和账号运营结果；没有证据的部分会标为待确认。"
	}
}

func resumeEvidenceBullets(resume string, limit int) []string {
	var out []string
	keywords := []string{"Wonderlab", "抖音", "粉丝", "曝光", "KOL", "商务合作", "分析报告", "活动策划", "项目"}
	for _, line := range strings.Split(resume, "\n") {
		line = strings.TrimSpace(strings.Trim(line, "• \t"))
		if line == "" {
			continue
		}
		for _, keyword := range keywords {
			if strings.Contains(strings.ToLower(line), strings.ToLower(keyword)) {
				out = append(out, line)
				break
			}
		}
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	if len(out) == 0 {
		out = append(out, "待确认：当前简历材料里还没有可直接映射的项目证据。")
	}
	return uniqueStrings(out)
}

func evidenceBoundary(resume string) string {
	evidence := resumeEvidenceBullets(resume, 3)
	return strings.Join(evidence, "；")
}

func followupQuestions(question string) []string {
	if strings.Contains(question, "账号") || strings.Contains(question, "运营") {
		return []string{"你当时重点看哪些指标？", "如果播放高但转粉低，你会怎么判断问题？", "你会如何做下一轮迭代？"}
	}
	if strings.Contains(question, "项目") || strings.Contains(question, "经历") {
		return []string{"这个项目里你具体负责哪一块？", "最难的地方是什么？", "如果重做一次你会怎么优化？"}
	}
	return []string{"你有什么证据支撑这个判断？", "如果面试官继续追问细节，你会举哪个例子？"}
}

func renderSelfIntro(ctx ReviewLibraryContext) string {
	return "我目前求职方向是" + ctx.RoleName + "。我会重点突出和岗位相关的内容运营、平台理解、数据复盘和合作沟通经历。简历中已证实的亮点包括：" + strings.Join(resumeEvidenceBullets(ctx.ResumeContent, 3), "；") + "。"
}

func renderBestProjectPitch(ctx ReviewLibraryContext) string {
	projects := inferProjectsFromResume(ctx.ResumeContent)
	if len(projects) == 0 {
		return "目前材料里还缺少完整项目证据，需要补充一个可讲清目标、动作、结果和复盘的真实项目。"
	}
	return "我会优先讲「" + projects[0].Name + "」：先说业务目标，再讲自己负责的动作，最后回到材料中已有的数据和结果。"
}

func renderRoleUnderstanding(ctx ReviewLibraryContext) string {
	return "我理解这个岗位的核心不是单纯发内容，而是围绕目标用户和业务目标，完成用户洞察、内容设计、执行发布、数据反馈和策略迭代的闭环。"
}

func inferProjectsFromResume(resume string) []ResumeProject {
	lines := strings.Split(resume, "\n")
	var projects []ResumeProject
	for i, line := range lines {
		clean := strings.TrimSpace(line)
		if clean == "" {
			continue
		}
		if strings.Contains(clean, "Wonderlab") || strings.Contains(clean, "抖音个人IP") || strings.Contains(clean, "活动策划") {
			project := ResumeProject{Name: clean}
			for j := i; j < len(lines) && j < i+8; j++ {
				l := strings.TrimSpace(strings.Trim(lines[j], "• \t"))
				if l != "" {
					project.Lines = append(project.Lines, l)
				}
			}
			projects = append(projects, project)
		}
	}
	return projects
}

func intersectSignals(a string, b string, limit int) []string {
	keywords := []string{"小红书", "抖音", "内容", "账号", "数据", "复盘", "KOL", "KOC", "热点", "用户", "增长", "转化", "协作"}
	var out []string
	for _, keyword := range keywords {
		if strings.Contains(a, keyword) && strings.Contains(b, keyword) {
			out = append(out, keyword)
		}
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	if len(out) == 0 {
		out = append(out, "当前 JD 与面经交集需要人工确认，建议补充目标公司岗位详情。")
	}
	return out
}

func firstNonEmptyLine(content string, fallback string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(strings.Trim(line, "# 　\t"))
		if line != "" {
			return line
		}
	}
	return fallback
}

func firstN(values []string, limit int) []string {
	if limit <= 0 || len(values) <= limit {
		return values
	}
	return values[:limit]
}

func inferReviewTopics(content string) []ReviewTopic {
	text := strings.ToLower(content)
	candidates := []struct {
		name    string
		slug    string
		signals []string
	}{
		{name: "项目深挖", slug: "project-deep-dive", signals: []string{"项目", "project", "项目追问", "难点", "亮点", "复盘"}},
		{name: "系统设计", slug: "system-design", signals: []string{"系统设计", "架构", "高并发", "可用性", "扩展性"}},
		{name: "数据与存储", slug: "data-storage", signals: []string{"mysql", "redis", "数据库", "缓存", "索引", "事务"}},
		{name: "算法与代码", slug: "algorithm-coding", signals: []string{"算法", "leetcode", "代码", "coding", "sql"}},
		{name: "行为面试", slug: "behavioral", signals: []string{"自我介绍", "离职", "冲突", "协作", "优缺点"}},
		{name: "业务与策略", slug: "business-strategy", signals: []string{"策略", "商业", "增长", "转化", "用户", "产品", "运营", "账号"}},
	}
	var topics []ReviewTopic
	for _, candidate := range candidates {
		for _, signal := range candidate.signals {
			if strings.Contains(text, strings.ToLower(signal)) {
				topics = append(topics, ReviewTopic{Name: candidate.name, Slug: candidate.slug})
				break
			}
		}
	}
	return uniqueTopics(topics)
}

func inferQuestionForTopic(topic string, content string) string {
	switch topic {
	case "项目深挖":
		return "你讲一个最能代表你能力的项目，重点说清楚目标、方案、难点和结果。"
	case "系统设计":
		return "如果让你设计这个系统，你会如何拆模块、处理稳定性和扩展性？"
	case "数据与存储":
		return "你如何设计数据模型、缓存策略和一致性保障？"
	case "算法与代码":
		return "这类代码题你会怎么分析复杂度、边界条件和测试用例？"
	case "行为面试":
		return "讲一次你遇到冲突、压力或失败后的处理过程。"
	case "业务与策略":
		return "你如何理解这个业务目标，并把它拆成可执行方案？"
	default:
		return firstQuestionLikeLine(content)
	}
}

func firstQuestionLikeLine(content string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(line, "-"))
		if line == "" {
			continue
		}
		if strings.Contains(line, "？") || strings.Contains(line, "?") || strings.Contains(line, "问") {
			if len([]rune(line)) > 80 {
				return string([]rune(line)[:80])
			}
			return line
		}
	}
	return "这类问题你会如何结合真实项目回答？"
}

func extractSignals(content string, limit int) []string {
	var out []string
	seen := map[string]bool{}
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(strings.Trim(line, "-*# 　\t"))
		if line == "" || seen[line] {
			continue
		}
		if len([]rune(line)) > 90 {
			line = string([]rune(line)[:90])
		}
		out = append(out, line)
		seen[line] = true
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	if len(out) == 0 {
		out = append(out, "材料中存在公开面经问题，需要结合真实项目证据组织答案。")
	}
	return out
}

func uniqueTopics(topics []ReviewTopic) []ReviewTopic {
	var out []ReviewTopic
	seen := map[string]bool{}
	for _, topic := range topics {
		key := topic.Name
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, topic)
	}
	return out
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
