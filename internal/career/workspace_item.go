package career

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

var nonSlugCharPattern = regexp.MustCompile(`[^a-z0-9]+`)

func (w *Workspace) AddJD(content string, now time.Time) (WorkspaceItem, error) {
	return w.addMaterial(WorkspaceTypeJD, content, now)
}

func (w *Workspace) AddMaterial(itemType string, content string, now time.Time) (WorkspaceItem, error) {
	itemType = strings.ToLower(strings.TrimSpace(itemType))
	if itemType == WorkspaceTypeJD {
		return w.AddJD(content, now)
	}
	if !IsSupportedWorkspaceType(itemType) {
		return WorkspaceItem{}, fmt.Errorf("unsupported workspace item type %q", itemType)
	}
	return w.addMaterial(itemType, content, now)
}

func (w *Workspace) AddMaterialFromFile(input WorkspaceFileInput) (WorkspaceItem, error) {
	itemType := strings.ToLower(strings.TrimSpace(input.ItemType))
	if !IsSupportedWorkspaceType(itemType) {
		return WorkspaceItem{}, fmt.Errorf("unsupported workspace item type %q", itemType)
	}
	content := strings.TrimSpace(input.Text)
	if content == "" && strings.TrimSpace(input.OriginalPath) == "" {
		return WorkspaceItem{}, fmt.Errorf("%s file text must not be empty", itemType)
	}
	now := input.Now
	if now.IsZero() {
		now = time.Now()
	}
	title := strings.TrimSpace(input.Title)
	if title == "" {
		title = strings.TrimSuffix(input.OriginalName, filepath.Ext(input.OriginalName))
	}
	if title == "" {
		title = inferMaterialTitle(itemType, content)
	}
	id := w.uniqueMaterialID(itemType, title, now)
	internalDir := internalItemRelDir(id)
	internalAbsDir := filepath.Join(w.Root, internalDir)
	if err := os.MkdirAll(internalAbsDir, 0o755); err != nil {
		return WorkspaceItem{}, fmt.Errorf("create %s dir: %w", itemType, err)
	}
	sourceRel := filepath.Join(internalDir, "extracted.md")
	sourceContent := content
	if sourceContent == "" {
		sourceContent = extractionFailurePlaceholder(filepath.Base(input.OriginalPath), input.ExtractError)
	}
	if err := os.WriteFile(filepath.Join(w.Root, sourceRel), []byte(sourceContent+"\n"), 0o644); err != nil {
		return WorkspaceItem{}, fmt.Errorf("write extracted %s source: %w", itemType, err)
	}
	originalRel := ""
	visibleRel := uniqueRelPath(w.Root, userVisibleMaterialRel(itemType, title, input.OriginalName))
	if strings.TrimSpace(input.OriginalPath) != "" {
		originalRel = uniqueRelPath(w.Root, archiveRel(now, input.OriginalName))
		if err := copyFile(input.OriginalPath, filepath.Join(w.Root, originalRel)); err != nil {
			return WorkspaceItem{}, err
		}
	}
	if itemType == WorkspaceTypeResume && strings.TrimSpace(input.OriginalPath) != "" {
		resumeOriginalRel := filepath.Join(WorkspaceDirResume, safeFileNameWithExt(input.OriginalName, filepath.Ext(input.OriginalPath)))
		resumeOriginalRel = uniqueRelPath(w.Root, resumeOriginalRel)
		if err := copyFile(input.OriginalPath, filepath.Join(w.Root, resumeOriginalRel)); err != nil {
			return WorkspaceItem{}, err
		}
	}
	if err := w.writeWorkspaceText(visibleRel, renderUserVisibleMaterial(itemType, title, sourceContent, input, now)); err != nil {
		return WorkspaceItem{}, err
	}
	metadata := WorkspaceItemMetadata{
		ID:                 id,
		Title:              title,
		Type:               itemType,
		CreatedAt:          now,
		Source:             filepath.ToSlash(sourceRel),
		Original:           filepath.ToSlash(originalRel),
		ExternalSource:     filepath.Clean(strings.TrimSpace(input.OriginalPath)),
		Extractor:          input.Extractor,
		MIMEType:           input.MIMEType,
		ExtractStatus:      normalizeExtractStatus(input.ExtractStatus),
		ExtractError:       strings.TrimSpace(input.ExtractError),
		ContentFingerprint: input.ContentFingerprint,
	}
	if err := w.writeJSON(filepath.Join(internalAbsDir, "metadata.json"), metadata); err != nil {
		return WorkspaceItem{}, err
	}
	item := WorkspaceItem{
		ID:        id,
		Type:      itemType,
		Title:     title,
		Path:      filepath.ToSlash(visibleRel),
		Tags:      inferMaterialTags(itemType, content),
		CreatedAt: now,
		Summary:   summarizeMaterial(sourceContent),
		Metadata:  metadata,
	}
	if err := w.upsertIndexItem(item); err != nil {
		return WorkspaceItem{}, err
	}
	if err := w.updateActivePointers(itemType, sourceRel, now); err != nil {
		return WorkspaceItem{}, err
	}
	return item, nil
}

func (w *Workspace) AddGuidedMaterial(input GuidedMaterialInput) (GuidedMaterialResult, error) {
	now := input.Now
	if now.IsZero() {
		now = time.Now()
	}
	itemType := strings.ToLower(strings.TrimSpace(input.ItemType))
	if itemType == "" {
		itemType = strings.ToLower(strings.TrimSpace(input.Classification.Type))
	}
	if itemType == "" || itemType == WorkspaceTypeGeneral {
		itemType = WorkspaceTypeRecord
	}
	classification := input.Classification
	if classification.Type == "" {
		classification.Type = itemType
	}
	if classification.RulePath == "" {
		guide, err := w.LoadGuide()
		if err == nil {
			classification.RulePath = classificationRulePath(guide, itemType)
		}
	}
	var item WorkspaceItem
	var err error
	if strings.TrimSpace(input.File.OriginalPath) != "" || strings.TrimSpace(input.File.Text) != "" {
		fileInput := input.File
		fileInput.ItemType = itemType
		fileInput.Now = now
		item, err = w.AddMaterialFromFile(fileInput)
	} else {
		item, err = w.AddMaterial(itemType, input.Content, now)
	}
	if err != nil {
		return GuidedMaterialResult{}, err
	}
	var syncActions []string
	libraryContent := strings.TrimSpace(input.Content)
	if libraryContent == "" {
		libraryContent = strings.TrimSpace(input.File.Text)
	}
	if item.Type == WorkspaceTypeExperiences && libraryContent != "" {
		_, index, err := w.Status()
		if err != nil {
			return GuidedMaterialResult{}, err
		}
		ctx := w.buildReviewLibraryContext(item, index)
		if strings.TrimSpace(ctx.ExperienceContent) == "" {
			ctx.ExperienceContent = libraryContent
		}
		paths, err := w.writeExperienceReviewLibrary(ctx, item, now)
		if err != nil {
			return GuidedMaterialResult{}, err
		}
		if len(paths) > 0 {
			syncActions = append(syncActions, "review_library:"+strings.Join(paths, ","))
		}
	}
	recordRel, err := w.writeClassificationRecord(item, classification, input.SourceLabel, now, syncActions)
	if err != nil {
		return GuidedMaterialResult{}, err
	}
	return GuidedMaterialResult{Item: item, RecordRel: recordRel}, nil
}

func (w *Workspace) addMaterial(itemType string, content string, now time.Time) (WorkspaceItem, error) {
	itemType = strings.ToLower(strings.TrimSpace(itemType))
	content = strings.TrimSpace(content)
	if content == "" {
		return WorkspaceItem{}, fmt.Errorf("%s content must not be empty", itemType)
	}
	if now.IsZero() {
		now = time.Now()
	}
	title := inferMaterialTitle(itemType, content)
	id := w.uniqueMaterialID(itemType, title, now)
	internalDir := internalItemRelDir(id)
	internalAbsDir := filepath.Join(w.Root, internalDir)
	if err := os.MkdirAll(internalAbsDir, 0o755); err != nil {
		return WorkspaceItem{}, fmt.Errorf("create %s dir: %w", itemType, err)
	}
	sourceRel := filepath.Join(internalDir, "extracted.md")
	sourceAbs := filepath.Join(w.Root, sourceRel)
	if err := os.WriteFile(sourceAbs, []byte(content+"\n"), 0o644); err != nil {
		return WorkspaceItem{}, fmt.Errorf("write %s source: %w", itemType, err)
	}
	visibleRel := uniqueRelPath(w.Root, userVisibleMaterialRel(itemType, title, ""))
	if err := w.writeWorkspaceText(visibleRel, renderUserVisibleMaterial(itemType, title, content, WorkspaceFileInput{}, now)); err != nil {
		return WorkspaceItem{}, err
	}
	metadata := WorkspaceItemMetadata{
		ID:            id,
		Title:         title,
		Type:          itemType,
		CreatedAt:     now,
		Source:        filepath.ToSlash(sourceRel),
		Extractor:     "inline_text",
		MIMEType:      "text/markdown",
		ExtractStatus: "ok",
	}
	if err := w.writeJSON(filepath.Join(internalAbsDir, "metadata.json"), metadata); err != nil {
		return WorkspaceItem{}, err
	}

	item := WorkspaceItem{
		ID:        id,
		Type:      itemType,
		Title:     title,
		Path:      filepath.ToSlash(visibleRel),
		Tags:      inferMaterialTags(itemType, content),
		CreatedAt: now,
		Summary:   summarizeMaterial(content),
		Metadata:  metadata,
	}
	if err := w.upsertIndexItem(item); err != nil {
		return WorkspaceItem{}, err
	}
	if err := w.updateActivePointers(itemType, sourceRel, now); err != nil {
		return WorkspaceItem{}, err
	}
	return item, nil
}

func (w *Workspace) updateActivePointers(itemType string, sourceRel string, now time.Time) error {
	meta, err := w.ReadMetadata()
	if err != nil {
		return err
	}
	switch itemType {
	case WorkspaceTypeJD:
		meta.ActiveJD = filepath.ToSlash(sourceRel)
	case WorkspaceTypeResume:
		meta.CurrentResume = filepath.ToSlash(sourceRel)
	case WorkspaceTypePrepare:
		meta.ActiveProject = filepath.ToSlash(sourceRel)
	}
	meta.UpdatedAt = now
	return w.writeJSON(w.metadataPath(), meta)
}

func activePointerName(itemType string) string {
	switch strings.ToLower(strings.TrimSpace(itemType)) {
	case WorkspaceTypeJD:
		return "active_jd"
	case WorkspaceTypeResume:
		return "current_resume"
	case WorkspaceTypePrepare:
		return "active_project"
	default:
		return ""
	}
}

func normalizeExtractStatus(status string) string {
	status = strings.TrimSpace(status)
	if status == "" {
		return "ok"
	}
	return status
}

func extractionFailurePlaceholder(name string, extractError string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "source file"
	}
	extractError = strings.TrimSpace(extractError)
	if extractError == "" {
		return fmt.Sprintf("Extraction failed for %s.", name)
	}
	return fmt.Sprintf("Extraction failed for %s.\n\nError: %s", name, extractError)
}

func LooksLikeJD(content string) bool {
	normalized := strings.ToLower(content)
	signals := []string{
		"job description",
		"responsibilities",
		"requirements",
		"qualifications",
		"岗位职责",
		"职位描述",
		"任职要求",
		"岗位要求",
		"加分项",
	}
	count := 0
	for _, signal := range signals {
		if strings.Contains(normalized, signal) {
			count++
		}
	}
	return count >= 1 && len([]rune(strings.TrimSpace(content))) >= 40
}

func (w *Workspace) upsertIndexItem(item WorkspaceItem) error {
	index, err := w.ReadIndex()
	if err != nil {
		return err
	}
	replaced := false
	for i, existing := range index.Items {
		if existing.ID == item.ID {
			index.Items[i] = item
			replaced = true
			break
		}
	}
	if !replaced {
		index.Items = append(index.Items, item)
	}
	sort.SliceStable(index.Items, func(i, j int) bool {
		return index.Items[i].CreatedAt.Before(index.Items[j].CreatedAt)
	})
	return w.writeJSON(w.indexPath(), index)
}

func inferMaterialTitle(itemType string, content string) string {
	if itemType == WorkspaceTypeJD {
		return inferJDTitle(content)
	}
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(line, "#"))
		if line == "" {
			continue
		}
		if len([]rune(line)) > 80 {
			line = string([]rune(line)[:80])
		}
		return line
	}
	return itemType
}

func inferJDTitle(content string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(line, "#"))
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		if strings.Contains(lower, "job description") || strings.Contains(line, "职位描述") || strings.Contains(line, "岗位职责") {
			continue
		}
		if len([]rune(line)) > 80 {
			line = string([]rune(line)[:80])
		}
		return line
	}
	return "job-description"
}

func inferMaterialTags(itemType string, content string) []string {
	lower := strings.ToLower(content)
	candidates := []string{
		"resume",
		"cv",
		"interview",
		"prepare",
		"portfolio",
		"case study",
		"metrics",
		"impact",
		"leadership",
		"collaboration",
		"communication",
		"writing",
		"operations",
		"analysis",
		"strategy",
	}
	var tags []string
	seen := map[string]bool{itemType: true}
	tags = append(tags, itemType)
	for _, candidate := range candidates {
		if strings.Contains(lower, candidate) && !seen[candidate] {
			tags = append(tags, candidate)
			seen[candidate] = true
		}
	}
	return tags
}

func summarizeMaterial(content string) string {
	compact := strings.Join(strings.Fields(content), " ")
	if len([]rune(compact)) > 180 {
		return string([]rune(compact)[:180])
	}
	return compact
}

func workspaceTypeDir(itemType string) string {
	switch strings.ToLower(strings.TrimSpace(itemType)) {
	case WorkspaceTypeJD:
		return WorkspaceDirJD
	case WorkspaceTypeResume:
		return WorkspaceDirResume
	case WorkspaceTypePrepare:
		return WorkspaceDirPrepare
	case WorkspaceTypeExperiences:
		return WorkspaceDirExperiences
	case WorkspaceTypeMyInterviews:
		return WorkspaceDirMyInterviews
	case WorkspaceTypeRecord:
		return filepath.Join(WorkspaceInternalDir, "record", "unclassified")
	default:
		return filepath.Join(WorkspaceInternalDir, "record", "unclassified")
	}
}

func materialID(itemType string, title string, now time.Time) string {
	return fmt.Sprintf("%s-%s-%s", itemIDPrefix(itemType), now.Format("20060102-150405"), slug(title))
}

func (w *Workspace) uniqueMaterialID(itemType string, title string, now time.Time) string {
	baseID := materialID(itemType, title, now)
	if _, err := os.Stat(filepath.Join(w.Root, internalItemRelDir(baseID))); err != nil {
		return baseID
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-%d", baseID, i)
		if _, err := os.Stat(filepath.Join(w.Root, internalItemRelDir(candidate))); err != nil {
			return candidate
		}
	}
}

func materialRelDir(itemType string, id string) string {
	return userVisibleMaterialDir(itemType)
}

func internalItemRelDir(id string) string {
	return filepath.Join(WorkspaceInternalDir, "items", slug(id))
}

func userVisibleMaterialDir(itemType string) string {
	switch strings.ToLower(strings.TrimSpace(itemType)) {
	case WorkspaceTypeJD:
		return WorkspaceDirJD
	case WorkspaceTypeResume:
		return WorkspaceDirResume
	case WorkspaceTypePrepare:
		return WorkspaceDirPrepare
	case WorkspaceTypeExperiences:
		return WorkspaceDirExperiences
	case WorkspaceTypeMyInterviews:
		return WorkspaceDirMyInterviews
	case WorkspaceTypeRecord:
		return filepath.Join(WorkspaceInternalDir, "record", "unclassified")
	default:
		return WorkspaceDirPrepare
	}
}

func userVisibleMaterialRel(itemType string, title string, originalName string) string {
	dir := userVisibleMaterialDir(itemType)
	if strings.ToLower(strings.TrimSpace(itemType)) == WorkspaceTypeRecord {
		return filepath.Join(dir, safeFileNameWithExt(title, ".md"))
	}
	base := title
	if strings.TrimSpace(base) == "" {
		base = strings.TrimSuffix(originalName, filepath.Ext(originalName))
	}
	if strings.TrimSpace(base) == "" {
		base = itemType
	}
	if strings.ToLower(strings.TrimSpace(itemType)) == WorkspaceTypeExperiences && !strings.Contains(base, "面经") {
		base += "面经"
	}
	return filepath.Join(dir, safeFileNameWithExt(base, ".md"))
}

func archiveRel(now time.Time, name string) string {
	if now.IsZero() {
		now = time.Now()
	}
	return filepath.Join(WorkspaceDirArchive, now.Format("2006-01-02"), safeFileNameWithExt(name, filepath.Ext(name)))
}

func uniqueRelPath(root string, rel string) string {
	return filepath.ToSlash(relFromRoot(root, uniquePath(filepath.Join(root, filepath.FromSlash(rel)))))
}

func safeFileNameWithExt(name string, ext string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "资料"
	}
	if ext == "" {
		ext = filepath.Ext(name)
	}
	name = strings.TrimSuffix(name, filepath.Ext(name))
	name = safeFileName(name)
	if name == "" {
		name = "资料"
	}
	if ext != "" && !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	return name + ext
}

func renderUserVisibleMaterial(itemType string, title string, content string, input WorkspaceFileInput, now time.Time) string {
	switch strings.ToLower(strings.TrimSpace(itemType)) {
	case WorkspaceTypeExperiences:
		return renderInterviewExperienceSummary(title, content, input, now)
	case WorkspaceTypeJD:
		return renderPlainMaterial(title, "岗位明细", content)
	case WorkspaceTypeResume:
		return renderPlainMaterial(title, "我的简历", content)
	case WorkspaceTypePrepare:
		return renderPlainMaterial(title, "复习资料", content)
	case WorkspaceTypeMyInterviews:
		return renderPlainMaterial(title, "我的面试", content)
	default:
		return renderPlainMaterial(title, "资料", content)
	}
}

func renderPlainMaterial(title string, section string, content string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		title = section
	}
	if strings.HasPrefix(strings.TrimSpace(content), "# ") {
		return content
	}
	return fmt.Sprintf("# %s\n\n## %s\n\n%s", title, section, strings.TrimSpace(content))
}

func renderInterviewExperienceSummary(title string, content string, input WorkspaceFileInput, now time.Time) string {
	title = strings.TrimSpace(title)
	if title == "" {
		title = "面经"
	}
	if !strings.Contains(title, "面经") {
		title += "面经"
	}
	questions := inferQuestionsForTopic("", content)
	if len(questions) == 0 {
		questions = []string{"请介绍一下这份面经中最核心的问题。"}
	}
	var b strings.Builder
	b.WriteString("# " + title + "\n\n")
	b.WriteString("## 基础信息\n\n")
	b.WriteString("- 来源：" + firstNonEmpty(input.OriginalName, "用户导入或手动输入") + "\n")
	b.WriteString("- 整理时间：" + now.Format("2006-01-02") + "\n\n")
	b.WriteString("## 原始内容摘要\n\n")
	b.WriteString(summarizeMaterial(content) + "\n\n")
	b.WriteString("## 面试问题与参考答案\n\n")
	for i, question := range questions {
		b.WriteString(fmt.Sprintf("### Q%d：%s\n\n", i+1, question))
		b.WriteString("#### 参考答案\n\n")
		b.WriteString(renderAnswerForQuestion(question, ReviewLibraryContext{ExperienceContent: content}) + "\n\n")
		b.WriteString("#### 可追问问题\n\n")
		followups := followupQuestions(question)
		for _, followup := range followups {
			b.WriteString("- " + followup + "\n")
		}
		b.WriteString("\n#### 追问参考答案\n\n")
		for _, followup := range followups {
			b.WriteString("- " + followup + "：先给结论，再结合已有材料说明证据；材料不足的部分标记为待补充。\n")
		}
		b.WriteString("\n")
	}
	b.WriteString("## 需要补充的材料\n\n")
	b.WriteString("- 可验证的项目数据、截图、复盘文档或岗位背景信息。\n")
	return b.String()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func itemIDPrefix(itemType string) string {
	itemType = strings.ToLower(strings.TrimSpace(itemType))
	switch itemType {
	case WorkspaceTypeExperiences:
		return "experience"
	case WorkspaceTypeMyInterviews:
		return "interview"
	case WorkspaceTypeRecord:
		return "record"
	default:
		return strings.ReplaceAll(itemType, "_", "-")
	}
}

func copyFile(src string, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source file %q: %w", src, err)
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("create parent directory for %q: %w", dst, err)
	}
	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("create copied file %q: %w", dst, err)
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy %q to %q: %w", src, dst, err)
	}
	return nil
}

func slug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = nonSlugCharPattern.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-")
	if value == "" {
		return "job-description"
	}
	if len(value) > 48 {
		value = strings.Trim(value[:48], "-")
	}
	if value == "" {
		return "job-description"
	}
	return value
}
