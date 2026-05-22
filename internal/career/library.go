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
		"面试资料库首页.md":                                      renderHomeIndex(now),
		filepath.Join(WorkspaceDirExperiences, "面经总览.md"): renderExperienceIndex(now),
		filepath.Join(WorkspaceDirPrepare, "复习资料总览.md"):   renderPrepareIndex(now),
		filepath.Join(WorkspaceDirJD, "岗位汇总.md"):          renderJDIndex(now),
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
	splitItems, err := w.EnsureSplitJDMaterials(now)
	if err != nil {
		return ReviewLibraryResult{}, err
	}
	_, index, err := w.Status()
	if err != nil {
		return ReviewLibraryResult{}, err
	}
	var generated []string
	for _, item := range splitItems {
		generated = append(generated, item.Path)
	}
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

	questionBankRel := filepath.Join(WorkspaceDirPrepare, domain.Slug, fmt.Sprintf("%s题库.md", safeFileName(sourceItem.Title)))
	if err := w.writeWorkspaceText(questionBankRel, renderTopicQuestionBank(ctx, ReviewTopic{Name: sourceItem.Title, Slug: slugForPath(sourceItem.Title)}, sourceItem, now)); err != nil {
		return nil, err
	}
	paths = append(paths, filepath.ToSlash(questionBankRel))
	if err := w.refreshExperienceIndex(domain, topics, now); err != nil {
		return nil, err
	}
	projectPaths, err := w.writeProjectQADocuments(ctx, now)
	if err != nil {
		return nil, err
	}
	paths = append(paths, projectPaths...)
	if err := w.refreshPrepareIndexFromWorkspace(now); err != nil {
		return nil, err
	}
	paths = append(paths, filepath.ToSlash(filepath.Join(WorkspaceDirPrepare, "复习资料总览.md")))
	if err := w.refreshJDIndex(ctx, now); err != nil {
		return nil, err
	}
	paths = append(paths, filepath.ToSlash(filepath.Join(WorkspaceDirJD, "岗位汇总.md")))
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
	rel := filepath.Join(WorkspaceInternalDir, "sources", domain.Slug, stableName+".md")
	if err := w.writeWorkspaceText(rel, renderSourceMaterial(sourceItem, content, now)); err != nil {
		return "", err
	}
	return filepath.ToSlash(rel), nil
}

func (w *Workspace) refreshExperienceIndex(domain ReviewDomain, topics []ReviewTopic, now time.Time) error {
	var b strings.Builder
	b.WriteString("# 面经总览\n\n")
	b.WriteString("## 来源入口\n\n")
	b.WriteString("- 公开面经原文保存在本目录顶层；扩展题库写入复习资料库。\n")
	if len(topics) > 0 {
		b.WriteString("\n## 扩展题库\n\n")
		for _, topic := range topics {
			b.WriteString(fmt.Sprintf("- %s题库：%s\n", topic.Name, filepath.ToSlash(filepath.Join(WorkspaceDirPrepare, domain.Slug, topic.Name+"题库.md"))))
		}
	}
	return w.writeWorkspaceText(filepath.Join(WorkspaceDirExperiences, "面经总览.md"), b.String())
}

func renderHomeIndex(now time.Time) string {
	return `# 面试资料库首页

## 核心入口

- 岗位明细/岗位汇总.md
- 面经汇总/面经总览.md
- 复习资料库/复习资料总览.md

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

func renderExperienceIndex(now time.Time) string {
	return "# 面经总览\n\n## 方向入口\n\n- 暂无方向资料。导入公开面经或运行 `/library` 后会自动更新。\n"
}

func renderPrepareIndex(now time.Time) string {
	return "# 复习资料总览\n\n## 资料入口\n\n- 暂无复习资料。导入项目材料、知识点或补充资料后会自动更新。\n"
}

func renderJDIndex(now time.Time) string {
	return "# 岗位汇总\n\n## 岗位入口\n\n- 暂无岗位明细。导入 JD 后会自动更新。\n"
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

func renderExperienceObservations(domain ReviewDomain, sourceItem WorkspaceItem, sourceRel string, content string, now time.Time) string {
	var b strings.Builder
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
	b.WriteString("# " + topic.Name + "题库\n\n")
	b.WriteString(fmt.Sprintf("> 来源：`%s`。公开面经资料，不是用户真实面试记录。\n\n", sourceItem.Path))
	for i, question := range questions {
		b.WriteString(fmt.Sprintf("## Q%d：%s\n\n", i+1, question))
		b.WriteString("### 考点\n\n")
		for _, point := range topicExamSignals(topic.Name) {
			b.WriteString("- " + point + "\n")
		}
		b.WriteString("\n### 标准答案\n\n")
		b.WriteString(renderAnswerForQuestion(question, ctx))
		b.WriteString("\n\n### 结合我的简历怎么答\n\n")
		for _, evidence := range resumeEvidenceBullets(ctx.ResumeContent, 6) {
			b.WriteString("- " + evidence + "\n")
		}
		b.WriteString("\n### 可追问\n\n")
		for _, follow := range followupQuestions(question) {
			b.WriteString("- " + follow + "\n")
		}
		b.WriteString("\n### 风险 / 待补证据\n\n")
		b.WriteString("- 已有证据：" + evidenceBoundary(ctx.ResumeContent) + "\n")
		b.WriteString("- 待补证据：目标岗位背景、项目原始材料、关键指标来源、复盘原文或可展示交付物。\n\n")
		b.WriteString("### 关联资料\n\n")
		b.WriteString(fmt.Sprintf("- 来源面经：`%s`\n", sourceItem.Path))
		b.WriteString(fmt.Sprintf("- 当前简历：`%s`\n", emptyIfBlank(ctx.ResumePath)))
		b.WriteString(fmt.Sprintf("- 当前 JD：`%s`\n\n", emptyIfBlank(ctx.JDPath)))
	}
	return b.String()
}

func renderSourceMaterial(sourceItem WorkspaceItem, content string, now time.Time) string {
	var b strings.Builder
	b.WriteString("# " + sourceItem.Title + "\n\n")
	b.WriteString("> 公开面经源资料，用于回溯题目来源；不是用户真实面试记录。\n\n")
	b.WriteString(strings.TrimSpace(content))
	b.WriteString("\n")
	return b.String()
}

func frontmatter(title string, tags []string, now time.Time) string {
	return ""
}

func (w *Workspace) writeRoleReviewDocuments(ctx ReviewLibraryContext, now time.Time) ([]string, error) {
	roleDir := filepath.Join(WorkspaceDirMyInterviews, safeFileName(ctx.RoleName))
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
		rel := filepath.Join(WorkspaceDirPrepare, slugForPath(project.Name)+"-interview-qa.md")
		if err := w.writeWorkspaceText(rel, renderProjectQA(project, ctx, now)); err != nil {
			return nil, err
		}
		paths = append(paths, filepath.ToSlash(rel))
	}
	if err := w.refreshPrepareIndex(projects, now); err != nil {
		return nil, err
	}
	paths = append(paths, filepath.ToSlash(filepath.Join(WorkspaceDirPrepare, "复习资料总览.md")))
	return paths, nil
}

func (w *Workspace) refreshPrepareIndex(projects []ResumeProject, now time.Time) error {
	var b strings.Builder
	b.WriteString("# 复习资料总览\n\n## 项目入口\n\n")
	for _, project := range projects {
		b.WriteString(fmt.Sprintf("- %s：%s\n", project.Name, slugForPath(project.Name)+"-interview-qa.md"))
	}
	return w.writeWorkspaceText(filepath.Join(WorkspaceDirPrepare, "复习资料总览.md"), b.String())
}

func (w *Workspace) refreshPrepareIndexFromWorkspace(now time.Time) error {
	root := filepath.Join(w.Root, WorkspaceDirPrepare)
	var entries []string
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil
		}
		if filepath.Base(rel) == "复习资料总览.md" || filepath.Ext(rel) != ".md" {
			return nil
		}
		entries = append(entries, filepath.ToSlash(rel))
		return nil
	})
	sort.Strings(entries)
	var b strings.Builder
	b.WriteString("# 复习资料总览\n\n")
	if len(entries) == 0 {
		b.WriteString("## 资料入口\n\n- 暂无复习资料。导入项目材料、知识点或补充资料后会自动更新。\n")
		return w.writeWorkspaceText(filepath.Join(WorkspaceDirPrepare, "复习资料总览.md"), b.String())
	}
	b.WriteString("## 资料入口\n\n")
	for _, entry := range entries {
		title := strings.TrimSuffix(filepath.Base(entry), filepath.Ext(entry))
		b.WriteString(fmt.Sprintf("- %s：%s\n", title, entry))
	}
	return w.writeWorkspaceText(filepath.Join(WorkspaceDirPrepare, "复习资料总览.md"), b.String())
}

func (w *Workspace) refreshJDIndex(ctx ReviewLibraryContext, now time.Time) error {
	var b strings.Builder
	b.WriteString("# 岗位汇总\n\n")
	b.WriteString("## 当前 JD\n\n")
	if strings.TrimSpace(ctx.JDPath) != "" {
		b.WriteString(fmt.Sprintf("- 源资料：`%s`\n", ctx.JDPath))
	} else {
		b.WriteString("- 暂无明确目标 JD。选择或导入单个 JD 后再生成具体岗位作战材料。\n")
	}
	return w.writeWorkspaceText(filepath.Join(WorkspaceDirJD, "岗位汇总.md"), b.String())
}

func renderJDProfile(ctx ReviewLibraryContext, now time.Time) string {
	var b strings.Builder
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
	b.WriteString("- 00-JD结构化画像.md\n")
	b.WriteString("- 01-临阵抗拷打主文档.md\n")
	b.WriteString("- 03-面经来源与JD关联补充.md\n")
	return b.String()
}

type ResumeProject struct {
	Name  string
	Lines []string
}

func renderProjectQA(project ResumeProject, ctx ReviewLibraryContext, now time.Time) string {
	var b strings.Builder
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
	if label := firstMeaningfulLabel(title + "\n" + content); label != "" {
		return ReviewDomain{Slug: domainSlug(label), Name: label, Confidence: "medium"}
	}
	return ReviewDomain{Slug: "general", Name: "通用", Confidence: "low"}
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
	s := slug(strings.ToLower(label))
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
	inFollowupSection := false
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(strings.Trim(line, "-*# 　\t"))
		if line == "" {
			continue
		}
		if strings.Contains(line, "可追问") || strings.Contains(line, "追问参考答案") {
			inFollowupSection = true
			continue
		}
		if strings.HasPrefix(line, "参考答案") || strings.HasPrefix(line, "面试问题") {
			inFollowupSection = false
			continue
		}
		if inFollowupSection || isGenericFollowupQuestion(line) {
			continue
		}
		if strings.Contains(line, "？") || strings.Contains(line, "?") || strings.HasPrefix(line, "你") || strings.Contains(line, "如何") || strings.Contains(line, "怎么") || strings.Contains(line, "能否") {
			questions = append(questions, normalizeQuestion(line))
		}
	}
	if len(questions) == 0 {
		questions = []string{
			"你如何理解这个问题背后的岗位要求？",
			"你会用哪个真实项目支撑这个回答？",
			"这个方案的风险、边界和验证方式是什么？",
		}
	}
	return uniqueStrings(firstN(questions, 8))
}

func isGenericFollowupQuestion(line string) bool {
	generic := []string{
		"你有什么证据支撑这个判断",
		"如果面试官继续追问细节",
		"这个方案的边界和风险是什么",
		"你会举哪个例子",
	}
	for _, value := range generic {
		if strings.Contains(line, value) {
			return true
		}
	}
	return false
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
	return []string{
		"是否能说明问题背后的核心能力要求",
		"是否能把回答绑定到当前 JD 和简历证据",
		"是否能讲清关键取舍、风险边界和后续验证方式",
	}
}

func renderAnswerForQuestion(question string, ctx ReviewLibraryContext) string {
	if strings.Contains(question, "自我介绍") {
		return renderSelfIntro(ctx)
	}
	return "我会先给结论，再说明判断标准和关键取舍，最后绑定到当前简历里可验证的项目证据。没有材料支撑的经历、指标或结果不直接说成事实，而是标记为待补证据。"
}

func resumeEvidenceBullets(resume string, limit int) []string {
	var out []string
	inProjectSection := false
	for _, line := range strings.Split(resume, "\n") {
		line = strings.TrimSpace(strings.Trim(line, "• \t"))
		if line == "" {
			continue
		}
		if isProjectSectionHeading(line) {
			inProjectSection = true
		}
		if inProjectSection && looksLikeProjectTitle(line) {
			out = append(out, line)
		}
		if looksLikeEvidenceLine(line) {
			out = append(out, line)
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
	if strings.Contains(question, "项目") || strings.Contains(question, "经历") {
		return []string{"这个项目里你具体负责哪一块？", "最难的地方是什么？", "如果重做一次你会怎么优化？"}
	}
	return []string{"你有什么证据支撑这个判断？", "这个方案的边界和风险是什么？", "如果面试官继续追问细节，你会举哪个例子？"}
}

func renderSelfIntro(ctx ReviewLibraryContext) string {
	return "我目前求职方向是" + ctx.RoleName + "。我会围绕目标 JD 的核心要求，突出简历中已经有证据支撑的项目、职责、技术栈和结果。当前可引用的证据包括：" + strings.Join(resumeEvidenceBullets(ctx.ResumeContent, 3), "；") + "。"
}

func renderBestProjectPitch(ctx ReviewLibraryContext) string {
	projects := inferProjectsFromResume(ctx.ResumeContent)
	if len(projects) == 0 {
		return "目前材料里还缺少完整项目证据，需要补充一个可讲清目标、动作、结果和复盘的真实项目。"
	}
	return "我会优先讲「" + projects[0].Name + "」：先说业务目标，再讲自己负责的动作，最后回到材料中已有的数据和结果。"
}

func renderRoleUnderstanding(ctx ReviewLibraryContext) string {
	return "我理解这个岗位的核心要求需要从 JD 原文中拆解：先确认职责、能力关键词和交付目标，再把它们映射到自己真实做过的项目证据上。缺少证据的部分不硬讲，面试前优先补齐材料或准备边界说明。"
}

func inferProjectsFromResume(resume string) []ResumeProject {
	lines := strings.Split(resume, "\n")
	var projects []ResumeProject
	inProjectSection := false
	for i, line := range lines {
		clean := strings.TrimSpace(line)
		if clean == "" {
			continue
		}
		if isProjectSectionHeading(clean) {
			inProjectSection = true
			continue
		}
		if inProjectSection && looksLikeProjectTitle(clean) {
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
	keywords := sharedTerms(a, b)
	var out []string
	for _, keyword := range keywords {
		out = append(out, keyword)
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
	var topics []ReviewTopic
	for _, question := range inferQuestionsForTopic("", content) {
		name := inferTopicNameFromQuestion(question)
		if name == "" {
			continue
		}
		topics = append(topics, ReviewTopic{Name: name, Slug: slugForPath(name)})
	}
	if len(topics) == 0 {
		if label := firstMeaningfulLabel(content); label != "" {
			topics = append(topics, ReviewTopic{Name: label, Slug: slugForPath(label)})
		}
	}
	return uniqueTopics(topics)
}

func inferQuestionForTopic(topic string, content string) string {
	return firstQuestionLikeLine(content)
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

func firstMeaningfulLabel(content string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(strings.Trim(line, "#-* 　\t"))
		if line == "" || strings.HasPrefix(line, "---") {
			continue
		}
		line = strings.TrimSuffix(line, "面经资料包")
		line = strings.TrimSuffix(line, "面经整理")
		line = strings.TrimSuffix(line, "公开面经")
		line = strings.TrimSpace(strings.Trim(line, "：:-—| "))
		if line == "" || isGenericDomainLabel(line) {
			continue
		}
		if len([]rune(line)) > 24 {
			line = string([]rune(line)[:24])
		}
		return line
	}
	return ""
}

func looksLikeEvidenceLine(line string) bool {
	if len([]rune(line)) < 8 {
		return false
	}
	if strings.Contains(line, "：") || strings.Contains(line, ":") {
		return true
	}
	for _, r := range line {
		if r >= '0' && r <= '9' {
			return true
		}
	}
	return strings.Contains(line, "负责") || strings.Contains(line, "设计") || strings.Contains(line, "实现") || strings.Contains(line, "优化") || strings.Contains(line, "支撑")
}

func looksLikeProjectTitle(line string) bool {
	if len([]rune(line)) < 4 || len([]rune(line)) > 80 {
		return false
	}
	if strings.Contains(line, "：") || strings.Contains(line, ":") {
		return false
	}
	for _, prefix := range []string{"负责", "累计", "支撑", "实现", "沉淀", "优化", "角色"} {
		if strings.HasPrefix(line, prefix) {
			return false
		}
	}
	return !strings.HasSuffix(line, "背景") && !strings.HasSuffix(line, "经历") && !strings.HasSuffix(line, "技能")
}

func isProjectSectionHeading(line string) bool {
	line = strings.TrimSpace(strings.Trim(line, "# 　\t"))
	switch line {
	case "项目", "项目介绍", "项目经历", "项目经验", "项目材料":
		return true
	default:
		return false
	}
}

func sharedTerms(a string, b string) []string {
	termsA := contentTerms(a)
	termsB := map[string]bool{}
	for _, term := range contentTerms(b) {
		termsB[term] = true
	}
	var out []string
	seen := map[string]bool{}
	for _, term := range termsA {
		if seen[term] || !termsB[term] {
			continue
		}
		seen[term] = true
		out = append(out, term)
	}
	return out
}

func contentTerms(content string) []string {
	fields := strings.FieldsFunc(content, func(r rune) bool {
		return r == ' ' || r == '\n' || r == '\t' || r == '，' || r == '。' || r == '、' || r == '；' || r == ';' || r == ',' || r == '.' || r == '(' || r == ')' || r == '（' || r == '）' || r == ':' || r == '：'
	})
	seen := map[string]bool{}
	var out []string
	for _, field := range fields {
		field = strings.TrimSpace(strings.Trim(field, "-*#`\"'"))
		if len([]rune(field)) < 2 || len([]rune(field)) > 24 {
			continue
		}
		lower := strings.ToLower(field)
		if isStopTerm(lower) || seen[field] {
			continue
		}
		seen[field] = true
		out = append(out, field)
	}
	return out
}

func isStopTerm(term string) bool {
	switch term {
	case "the", "and", "or", "to", "of", "in", "for", "with", "一个", "这个", "怎么", "如何", "什么", "以及", "需要", "负责":
		return true
	default:
		return false
	}
}

func inferTopicNameFromQuestion(question string) string {
	question = strings.TrimSpace(question)
	question = strings.TrimPrefix(question, "Q：")
	question = strings.TrimPrefix(question, "Q:")
	question = strings.Trim(question, "？?。；; ")
	if question == "" {
		return ""
	}
	separators := []string{"怎么", "如何", "为什么", "是否", "能否", "？", "?"}
	best := question
	for _, sep := range separators {
		if idx := strings.Index(best, sep); idx > 0 {
			best = strings.TrimSpace(best[:idx])
		}
	}
	if len([]rune(best)) < 2 {
		best = question
	}
	if len([]rune(best)) > 16 {
		best = string([]rune(best)[:16])
	}
	return strings.TrimSpace(best)
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
