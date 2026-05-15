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
	relDir := materialRelDir(itemType, id)
	absDir := filepath.Join(w.Root, relDir)
	if err := os.MkdirAll(absDir, 0o755); err != nil {
		return WorkspaceItem{}, fmt.Errorf("create %s dir: %w", itemType, err)
	}
	sourceRel := filepath.Join(relDir, "extracted.md")
	sourceContent := content
	if sourceContent == "" {
		sourceContent = extractionFailurePlaceholder(filepath.Base(input.OriginalPath), input.ExtractError)
	}
	if err := os.WriteFile(filepath.Join(w.Root, sourceRel), []byte(sourceContent+"\n"), 0o644); err != nil {
		return WorkspaceItem{}, fmt.Errorf("write extracted %s source: %w", itemType, err)
	}
	originalRel := ""
	if strings.TrimSpace(input.OriginalPath) != "" {
		originalRel = filepath.Join(relDir, "source"+filepath.Ext(input.OriginalPath))
		if err := copyFile(input.OriginalPath, filepath.Join(w.Root, originalRel)); err != nil {
			return WorkspaceItem{}, err
		}
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
	if err := w.writeJSON(filepath.Join(absDir, "metadata.json"), metadata); err != nil {
		return WorkspaceItem{}, err
	}
	item := WorkspaceItem{
		ID:        id,
		Type:      itemType,
		Title:     title,
		Path:      filepath.ToSlash(sourceRel),
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
	recordRel, err := w.writeClassificationRecord(item, classification, input.SourceLabel, now, nil)
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
	relDir := materialRelDir(itemType, id)
	absDir := filepath.Join(w.Root, relDir)
	if err := os.MkdirAll(absDir, 0o755); err != nil {
		return WorkspaceItem{}, fmt.Errorf("create %s dir: %w", itemType, err)
	}
	sourceRel := filepath.Join(relDir, "extracted.md")
	sourceAbs := filepath.Join(w.Root, sourceRel)
	if err := os.WriteFile(sourceAbs, []byte(content+"\n"), 0o644); err != nil {
		return WorkspaceItem{}, fmt.Errorf("write %s source: %w", itemType, err)
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
	if err := w.writeJSON(filepath.Join(absDir, "metadata.json"), metadata); err != nil {
		return WorkspaceItem{}, err
	}

	item := WorkspaceItem{
		ID:        id,
		Type:      itemType,
		Title:     title,
		Path:      filepath.ToSlash(sourceRel),
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
		return "jd"
	case WorkspaceTypeResume:
		return "resume"
	case WorkspaceTypePrepare:
		return "prepare"
	case WorkspaceTypeExperiences:
		return "experiences"
	case WorkspaceTypeMyInterviews:
		return "my-interviews"
	case WorkspaceTypeRecord:
		return filepath.Join("record", "unclassified")
	default:
		return filepath.Join("record", "unclassified")
	}
}

func materialID(itemType string, title string, now time.Time) string {
	return fmt.Sprintf("%s-%s-%s", itemIDPrefix(itemType), now.Format("20060102-150405"), slug(title))
}

func (w *Workspace) uniqueMaterialID(itemType string, title string, now time.Time) string {
	baseID := materialID(itemType, title, now)
	if _, err := os.Stat(filepath.Join(w.Root, materialRelDir(itemType, baseID))); err != nil {
		return baseID
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-%d", baseID, i)
		if _, err := os.Stat(filepath.Join(w.Root, materialRelDir(itemType, candidate))); err != nil {
			return candidate
		}
	}
}

func materialRelDir(itemType string, id string) string {
	if strings.ToLower(strings.TrimSpace(itemType)) == WorkspaceTypeResume {
		return filepath.Join("resume", "versions", id)
	}
	return filepath.Join(workspaceTypeDir(itemType), id)
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
