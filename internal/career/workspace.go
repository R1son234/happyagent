package career

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"happyagent/internal/jsonfile"
)

const DefaultWorkspaceRoot = "career-workspace"

var nonSlugCharPattern = regexp.MustCompile(`[^a-z0-9]+`)

var workspaceDirs = []string{
	"inbox",
	"resume",
	"jd",
	"experiences",
	"prepare",
	"my-interviews",
	"outputs",
	"outputs/runs",
	"record",
	"record/imports",
	"record/unclassified",
	"record/generated",
}

type Workspace struct {
	Root string
}

type WorkspaceMetadata struct {
	Version       int       `json:"version"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	TargetRoles   []string  `json:"target_roles,omitempty"`
	CurrentResume string    `json:"current_resume,omitempty"`
	ActiveJD      string    `json:"active_jd,omitempty"`
	ActiveProject string    `json:"active_project,omitempty"`
}

type WorkspaceIndex struct {
	Items []WorkspaceItem `json:"items"`
}

type WorkspaceItem struct {
	ID        string                `json:"id"`
	Type      string                `json:"type"`
	Title     string                `json:"title"`
	Path      string                `json:"path"`
	Tags      []string              `json:"tags,omitempty"`
	CreatedAt time.Time             `json:"created_at"`
	Summary   string                `json:"summary,omitempty"`
	Metadata  WorkspaceItemMetadata `json:"-"`
}

type WorkspaceItemMetadata struct {
	ID             string    `json:"id"`
	Title          string    `json:"title"`
	Type           string    `json:"type"`
	CreatedAt      time.Time `json:"created_at"`
	Source         string    `json:"source"`
	Original       string    `json:"original,omitempty"`
	ExternalSource string    `json:"external_source,omitempty"`
	Extractor      string    `json:"extractor,omitempty"`
	MIMEType       string    `json:"mime_type,omitempty"`
	ExtractStatus  string    `json:"extract_status,omitempty"`
	ExtractError   string    `json:"extract_error,omitempty"`
}

type WorkspaceFileInput struct {
	ItemType      string
	Title         string
	Text          string
	OriginalPath  string
	OriginalName  string
	Now           time.Time
	Extractor     string
	MIMEType      string
	ExtractStatus string
	ExtractError  string
}

type GuidedMaterialInput struct {
	ItemType       string
	Classification InputClassification
	Content        string
	File           WorkspaceFileInput
	SourceLabel    string
	Now            time.Time
}

type GuidedMaterialResult struct {
	Item      WorkspaceItem
	RecordRel string
}

type PublicInterviewArchiveResult struct {
	ExperienceItem WorkspaceItem
	PrepareItem    WorkspaceItem
	MyInterviewRel string
	RecordRel      string
}

func OpenWorkspace(root string, now time.Time) (*Workspace, error) {
	if strings.TrimSpace(root) == "" {
		root = DefaultWorkspaceRoot
	}
	if now.IsZero() {
		now = time.Now()
	}
	for _, dir := range workspaceDirs {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			return nil, fmt.Errorf("create career workspace dir %q: %w", dir, err)
		}
	}
	ws := &Workspace{Root: root}
	if _, err := os.Stat(ws.metadataPath()); err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("stat workspace metadata: %w", err)
		}
		meta := WorkspaceMetadata{
			Version:   1,
			CreatedAt: now,
			UpdatedAt: now,
		}
		if err := ws.writeJSON(ws.metadataPath(), meta); err != nil {
			return nil, err
		}
	}
	if _, err := os.Stat(ws.indexPath()); err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("stat workspace index: %w", err)
		}
		if err := ws.writeJSON(ws.indexPath(), WorkspaceIndex{Items: []WorkspaceItem{}}); err != nil {
			return nil, err
		}
	}
	if _, err := os.Stat(ws.guidePath()); err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("stat workspace guide: %w", err)
		}
		if err := ws.writeJSON(ws.guidePath(), DefaultWorkspaceGuide()); err != nil {
			return nil, err
		}
	}
	return ws, nil
}

func (w *Workspace) WriteArtifact(kind string, title string, content string, now time.Time) (string, error) {
	if strings.TrimSpace(kind) == "" {
		return "", fmt.Errorf("artifact kind must not be empty")
	}
	if strings.TrimSpace(content) == "" {
		return "", fmt.Errorf("artifact content must not be empty")
	}
	if now.IsZero() {
		now = time.Now()
	}
	dir, itemType := artifactDestination(kind)
	name := fmt.Sprintf("%s-%s.md", kind, now.Format("20060102-150405"))
	rel := filepath.Join(dir, name)
	abs := filepath.Join(w.Root, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return "", fmt.Errorf("create artifact dir: %w", err)
	}
	if strings.TrimSpace(title) != "" && !strings.HasPrefix(content, "# ") {
		content = "# " + title + "\n\n" + content
	}
	if err := os.WriteFile(abs, []byte(strings.TrimSpace(content)+"\n"), 0o644); err != nil {
		return "", fmt.Errorf("write artifact: %w", err)
	}
	item := WorkspaceItem{
		ID:        fmt.Sprintf("%s-%s", kind, now.Format("20060102-150405")),
		Type:      itemType,
		Title:     title,
		Path:      filepath.ToSlash(rel),
		Tags:      []string{kind},
		CreatedAt: now,
		Summary:   summarizeMaterial(content),
	}
	if err := w.upsertIndexItem(item); err != nil {
		return "", err
	}
	return filepath.ToSlash(rel), nil
}

type UserOutputPaths struct {
	LatestMarkdown      string
	TimestampedMarkdown string
	LatestJSON          string
	TimestampedJSON     string
}

func (w *Workspace) LatestOutputPath(kind string, ext string) string {
	if strings.TrimSpace(ext) == "" {
		ext = ".md"
	}
	return filepath.ToSlash(filepath.Join("outputs", latestOutputName(kind)+ext))
}

func (w *Workspace) TimestampedOutputPath(kind string, ext string, now time.Time) string {
	if strings.TrimSpace(ext) == "" {
		ext = ".md"
	}
	if now.IsZero() {
		now = time.Now()
	}
	return filepath.ToSlash(filepath.Join("outputs", "runs", fmt.Sprintf("%s-%s%s", now.Format("20060102-150405"), latestOutputName(kind), ext)))
}

func (w *Workspace) WriteUserOutput(kind string, title string, markdown string, jsonContent []byte, now time.Time) (UserOutputPaths, error) {
	if strings.TrimSpace(kind) == "" {
		return UserOutputPaths{}, fmt.Errorf("user output kind must not be empty")
	}
	if strings.TrimSpace(markdown) == "" && len(jsonContent) == 0 {
		return UserOutputPaths{}, fmt.Errorf("user output content must not be empty")
	}
	if now.IsZero() {
		now = time.Now()
	}
	paths := UserOutputPaths{}
	if strings.TrimSpace(markdown) != "" {
		content := strings.TrimSpace(markdown)
		if strings.TrimSpace(title) != "" && !strings.HasPrefix(content, "# ") {
			content = "# " + title + "\n\n" + content
		}
		paths.LatestMarkdown = w.LatestOutputPath(kind, ".md")
		paths.TimestampedMarkdown = w.TimestampedOutputPath(kind, ".md", now)
		if err := w.writeWorkspaceText(paths.LatestMarkdown, content); err != nil {
			return UserOutputPaths{}, err
		}
		if err := w.writeWorkspaceText(paths.TimestampedMarkdown, content); err != nil {
			return UserOutputPaths{}, err
		}
	}
	if len(jsonContent) > 0 {
		content := append(trimTrailingNewlines(jsonContent), '\n')
		paths.LatestJSON = w.LatestOutputPath(kind, ".json")
		paths.TimestampedJSON = w.TimestampedOutputPath(kind, ".json", now)
		if err := w.writeWorkspaceBytes(paths.LatestJSON, content); err != nil {
			return UserOutputPaths{}, err
		}
		if err := w.writeWorkspaceBytes(paths.TimestampedJSON, content); err != nil {
			return UserOutputPaths{}, err
		}
	}
	return paths, nil
}

func artifactDestination(kind string) (string, string) {
	switch kind {
	case "project-pitch":
		return filepath.Join("prepare", "generated"), WorkspaceTypePrepare
	default:
		return filepath.Join("record", "generated"), WorkspaceTypeRecord
	}
}

func (w *Workspace) Status() (WorkspaceMetadata, WorkspaceIndex, error) {
	meta, err := w.ReadMetadata()
	if err != nil {
		return WorkspaceMetadata{}, WorkspaceIndex{}, err
	}
	index, err := w.ReadIndex()
	if err != nil {
		return WorkspaceMetadata{}, WorkspaceIndex{}, err
	}
	return meta, index, nil
}

func (w *Workspace) ReadMetadata() (WorkspaceMetadata, error) {
	var meta WorkspaceMetadata
	if err := w.readJSON(w.metadataPath(), &meta); err != nil {
		return WorkspaceMetadata{}, err
	}
	return meta, nil
}

func (w *Workspace) ReadIndex() (WorkspaceIndex, error) {
	var index WorkspaceIndex
	if err := w.readJSON(w.indexPath(), &index); err != nil {
		return WorkspaceIndex{}, err
	}
	return index, nil
}

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
	id := materialID(itemType, title, now)
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
		ID:             id,
		Title:          title,
		Type:           itemType,
		CreatedAt:      now,
		Source:         filepath.ToSlash(sourceRel),
		Original:       filepath.ToSlash(originalRel),
		ExternalSource: filepath.Clean(strings.TrimSpace(input.OriginalPath)),
		Extractor:      input.Extractor,
		MIMEType:       input.MIMEType,
		ExtractStatus:  normalizeExtractStatus(input.ExtractStatus),
		ExtractError:   strings.TrimSpace(input.ExtractError),
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

func (w *Workspace) ArchivePublicInterviewExperience(content string, now time.Time) (PublicInterviewArchiveResult, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return PublicInterviewArchiveResult{}, fmt.Errorf("public interview experience content must not be empty")
	}
	if now.IsZero() {
		now = time.Now()
	}
	experienceItem, err := w.AddMaterial(WorkspaceTypeExperiences, content, now)
	if err != nil {
		return PublicInterviewArchiveResult{}, err
	}
	result := PublicInterviewArchiveResult{ExperienceItem: experienceItem}
	if containsPrepareSignals(content) {
		prepareItem, err := w.AddMaterial(WorkspaceTypePrepare, content, now)
		if err != nil {
			return PublicInterviewArchiveResult{}, err
		}
		result.PrepareItem = prepareItem
	}
	roleDir := filepath.Join("my-interviews", "市场营销")
	checklistRel := filepath.Join(roleDir, "面经来源与复习清单.md")
	checklistContent := fmt.Sprintf("# 面经来源与复习清单\n\n- 来源资料：%s\n- 资料性质：公开面经，不是用户真实面试记录。\n- 导入时间：%s\n", experienceItem.Path, now.Format(time.RFC3339))
	if err := w.writeWorkspaceText(checklistRel, checklistContent); err != nil {
		return PublicInterviewArchiveResult{}, err
	}
	result.MyInterviewRel = filepath.ToSlash(checklistRel)
	recordRel := filepath.Join("record", "imports", fmt.Sprintf("%s-public-interview-experience.md", now.Format("20060102-150405")))
	var b strings.Builder
	b.WriteString("# Public Interview Experience Import\n\n")
	b.WriteString(fmt.Sprintf("- source_path: %s\n", experienceItem.Path))
	b.WriteString(fmt.Sprintf("- my_interview_path: %s\n", result.MyInterviewRel))
	if result.PrepareItem.ID != "" {
		b.WriteString(fmt.Sprintf("- prepare_path: %s\n", result.PrepareItem.Path))
	}
	b.WriteString("- note: public interview experience; not a real user interview record\n")
	if err := w.writeWorkspaceText(recordRel, b.String()); err != nil {
		return PublicInterviewArchiveResult{}, err
	}
	result.RecordRel = filepath.ToSlash(recordRel)
	recordItem := WorkspaceItem{
		ID:        fmt.Sprintf("record-%s-public-interview-experience", now.Format("20060102-150405")),
		Type:      WorkspaceTypeRecord,
		Title:     "Public Interview Experience Import",
		Path:      result.RecordRel,
		Tags:      []string{"import", "public-interview-experience"},
		CreatedAt: now,
		Summary:   "Imported public interview experience and recorded derived workspace paths.",
	}
	if err := w.upsertIndexItem(recordItem); err != nil {
		return PublicInterviewArchiveResult{}, err
	}
	return result, nil
}

func (w *Workspace) writeClassificationRecord(item WorkspaceItem, classification InputClassification, sourceLabel string, now time.Time, syncActions []string) (string, error) {
	if now.IsZero() {
		now = time.Now()
	}
	name := fmt.Sprintf("%s-classification-%s.md", now.Format("20060102-150405"), slug(item.ID))
	rel := filepath.Join("record", "imports", name)
	var b strings.Builder
	b.WriteString("# Classification Record\n\n")
	if strings.TrimSpace(sourceLabel) != "" {
		b.WriteString(fmt.Sprintf("- input: %s\n", sourceLabel))
	}
	b.WriteString(fmt.Sprintf("- classified_type: %s\n", item.Type))
	b.WriteString(fmt.Sprintf("- confidence: %.2f\n", classification.Confidence))
	if classification.Reason != "" {
		b.WriteString(fmt.Sprintf("- reason: %s\n", classification.Reason))
	}
	if classification.RulePath != "" {
		b.WriteString(fmt.Sprintf("- rule_path: %s\n", classification.RulePath))
	}
	if len(classification.Signals) > 0 {
		b.WriteString(fmt.Sprintf("- matched_signals: %s\n", strings.Join(classification.Signals, ", ")))
	}
	b.WriteString(fmt.Sprintf("- destination: %s\n", item.Path))
	if item.Metadata.Original != "" {
		b.WriteString(fmt.Sprintf("- original: %s\n", item.Metadata.Original))
	}
	if pointer := activePointerName(item.Type); pointer != "" {
		b.WriteString(fmt.Sprintf("- active_pointer_updated: %s\n", pointer))
	} else {
		b.WriteString("- active_pointer_updated: none\n")
	}
	if len(syncActions) == 0 {
		b.WriteString("- sync_actions: none\n")
	} else {
		b.WriteString(fmt.Sprintf("- sync_actions: %s\n", strings.Join(syncActions, ", ")))
	}
	if err := w.writeWorkspaceText(rel, b.String()); err != nil {
		return "", err
	}
	return filepath.ToSlash(rel), nil
}

func containsPrepareSignals(content string) bool {
	lower := strings.ToLower(content)
	for _, signal := range []string{"项目", "project", "项目追问", "项目亮点", "项目难点", "技术方案", "证据口径"} {
		if strings.Contains(lower, strings.ToLower(signal)) {
			return true
		}
	}
	return false
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
	id := materialID(itemType, title, now)
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

func (w *Workspace) metadataPath() string {
	return filepath.Join(w.Root, "workspace.json")
}

func (w *Workspace) indexPath() string {
	return filepath.Join(w.Root, "index.json")
}

func (w *Workspace) guidePath() string {
	return filepath.Join(w.Root, WorkspaceGuideFileName)
}

func (w *Workspace) writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %q: %w", path, err)
	}
	data = append(data, '\n')
	return jsonfile.WriteBytes(path, data, 0o644)
}

func (w *Workspace) writeWorkspaceText(relPath string, content string) error {
	return w.writeWorkspaceBytes(relPath, []byte(strings.TrimSpace(content)+"\n"))
}

func (w *Workspace) writeWorkspaceBytes(relPath string, content []byte) error {
	abs := filepath.Join(w.Root, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return fmt.Errorf("create workspace text parent: %w", err)
	}
	if err := os.WriteFile(abs, content, 0o644); err != nil {
		return fmt.Errorf("write workspace text %q: %w", relPath, err)
	}
	return nil
}

func latestOutputName(kind string) string {
	switch strings.TrimSpace(kind) {
	case "report", "career-report", "analyze":
		return "latest-report"
	case "resume-review", "resume_review":
		return "latest-resume-review"
	case "interview-brief", "interview_brief":
		return "latest-interview-brief"
	case "gap-plan", "gap_plan":
		return "latest-gap-plan"
	case "interview-review", "interview_review":
		return "latest-interview-review"
	case "jd-match":
		return "latest-jd-match"
	case "project-pitch":
		return "latest-project-pitch"
	case "review-material":
		return "latest-review-material"
	default:
		return "latest-" + slug(kind)
	}
}

func trimTrailingNewlines(data []byte) []byte {
	return []byte(strings.TrimRight(string(data), "\n"))
}

func (w *Workspace) readJSON(path string, dest any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %q: %w", path, err)
	}
	if err := json.Unmarshal(data, dest); err != nil {
		return fmt.Errorf("parse %q: %w", path, err)
	}
	return nil
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
