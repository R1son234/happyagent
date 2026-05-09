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
	"record/migrations",
	"record/unclassified",
	"record/generated",
}

var legacyWorkspaceDirs = []string{
	"jds",
	"resumes",
	"projects",
	"interview_experience",
	"interview_records",
	"review_notes",
	"reports",
	"exports",
	"search_sources",
}

var legacyPathPrefixes = []struct {
	old string
	new string
}{
	{"resumes/", "resume/"},
	{"jds/", "jd/"},
	{"projects/", "prepare/"},
	{"interview_experience/", "experiences/"},
	{"interview_records/", "my-interviews/"},
	{"review_notes/", "record/unclassified/review_notes/"},
	{"search_sources/", "experiences/sources/"},
	{"reports/", "record/generated/reports/"},
	{"exports/", "record/generated/exports/"},
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
	if err := ws.MigrateLegacyLayout(now); err != nil {
		return nil, err
	}
	return ws, nil
}

func (w *Workspace) MigrateLegacyLayout(now time.Time) error {
	if now.IsZero() {
		now = time.Now()
	}
	var existing []string
	for _, dir := range legacyWorkspaceDirs {
		if info, err := os.Stat(filepath.Join(w.Root, dir)); err == nil && info.IsDir() {
			existing = append(existing, dir)
		} else if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("stat legacy workspace dir %q: %w", dir, err)
		}
	}
	if len(existing) == 0 {
		return nil
	}
	for _, dir := range workspaceDirs {
		if err := os.MkdirAll(filepath.Join(w.Root, dir), 0o755); err != nil {
			return fmt.Errorf("create career workspace dir %q: %w", dir, err)
		}
	}
	var logLines []string
	logLines = append(logLines, "# Legacy Career Workspace Migration", "", fmt.Sprintf("- time: %s", now.Format(time.RFC3339)), "")
	moveRules := []struct {
		old string
		new string
	}{
		{"resumes", "resume"},
		{"jds", "jd"},
		{"projects", "prepare"},
		{"interview_experience", "experiences"},
		{"interview_records", "my-interviews"},
		{"review_notes", filepath.Join("record", "unclassified", "review_notes")},
		{"search_sources", filepath.Join("experiences", "sources")},
		{"reports", filepath.Join("record", "generated", "reports")},
		{"exports", filepath.Join("record", "generated", "exports")},
	}
	for _, rule := range moveRules {
		moved, err := w.moveLegacyDir(rule.old, rule.new)
		if err != nil {
			return err
		}
		if moved > 0 {
			logLines = append(logLines, fmt.Sprintf("- moved `%s/` -> `%s/` (%d entries)", rule.old, filepath.ToSlash(rule.new), moved))
		}
	}
	if err := w.rewriteLegacyIndexAndMetadata(now); err != nil {
		return err
	}
	for _, dir := range legacyWorkspaceDirs {
		abs := filepath.Join(w.Root, dir)
		empty, err := isDirEmpty(abs)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("check legacy workspace dir %q: %w", dir, err)
		}
		if empty {
			if err := os.Remove(abs); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("remove empty legacy workspace dir %q: %w", dir, err)
			}
			continue
		}
		logLines = append(logLines, fmt.Sprintf("- left non-empty legacy directory for manual review: `%s/`", dir))
	}
	if len(logLines) <= 4 {
		return nil
	}
	name := fmt.Sprintf("%s-legacy-layout.md", now.Format("20060102-150405"))
	path := filepath.Join(w.Root, "record", "migrations", name)
	if err := os.WriteFile(path, []byte(strings.Join(logLines, "\n")+"\n"), 0o644); err != nil {
		return fmt.Errorf("write migration record: %w", err)
	}
	return nil
}

func (w *Workspace) moveLegacyDir(oldDir string, newDir string) (int, error) {
	oldAbs := filepath.Join(w.Root, oldDir)
	entries, err := os.ReadDir(oldAbs)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("read legacy workspace dir %q: %w", oldDir, err)
	}
	if len(entries) == 0 {
		return 0, nil
	}
	newAbs := filepath.Join(w.Root, newDir)
	if err := os.MkdirAll(newAbs, 0o755); err != nil {
		return 0, fmt.Errorf("create migration destination %q: %w", newDir, err)
	}
	moved := 0
	for _, entry := range entries {
		src := filepath.Join(oldAbs, entry.Name())
		dst := filepath.Join(newAbs, entry.Name())
		if _, err := os.Stat(dst); err == nil {
			dst = filepath.Join(newAbs, fmt.Sprintf("%s-from-%s", entry.Name(), oldDir))
		} else if err != nil && !os.IsNotExist(err) {
			return moved, fmt.Errorf("stat migration destination %q: %w", dst, err)
		}
		if err := os.Rename(src, dst); err != nil {
			return moved, fmt.Errorf("move %q to %q: %w", src, dst, err)
		}
		moved++
	}
	return moved, nil
}

func (w *Workspace) rewriteLegacyIndexAndMetadata(now time.Time) error {
	meta, err := w.ReadMetadata()
	if err != nil {
		return err
	}
	meta.CurrentResume = rewriteLegacyRelPath(meta.CurrentResume)
	meta.ActiveJD = rewriteLegacyRelPath(meta.ActiveJD)
	meta.ActiveProject = rewriteLegacyRelPath(meta.ActiveProject)
	meta.UpdatedAt = now
	if err := w.writeJSON(w.metadataPath(), meta); err != nil {
		return err
	}
	index, err := w.ReadIndex()
	if err != nil {
		return err
	}
	for i := range index.Items {
		index.Items[i].Type = normalizeLegacyWorkspaceItemType(index.Items[i].Type)
		index.Items[i].Path = rewriteLegacyRelPath(index.Items[i].Path)
		index.Items[i].Metadata.Type = normalizeLegacyWorkspaceItemType(index.Items[i].Metadata.Type)
		index.Items[i].Metadata.Source = rewriteLegacyRelPath(index.Items[i].Metadata.Source)
		index.Items[i].Metadata.Original = rewriteLegacyRelPath(index.Items[i].Metadata.Original)
	}
	if err := w.writeJSON(w.indexPath(), index); err != nil {
		return err
	}
	return w.rewriteLegacyMetadataFiles()
}

func (w *Workspace) rewriteLegacyMetadataFiles() error {
	return filepath.WalkDir(w.Root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || entry.Name() != "metadata.json" {
			return nil
		}
		var metadata WorkspaceItemMetadata
		if err := w.readJSON(path, &metadata); err != nil {
			return err
		}
		metadata.Type = normalizeLegacyWorkspaceItemType(metadata.Type)
		metadata.Source = rewriteLegacyRelPath(metadata.Source)
		metadata.Original = rewriteLegacyRelPath(metadata.Original)
		return w.writeJSON(path, metadata)
	})
}

func normalizeLegacyWorkspaceItemType(itemType string) string {
	// Used only while migrating old workspace indexes and metadata into the new layout.
	switch strings.ToLower(strings.TrimSpace(itemType)) {
	case "project":
		return WorkspaceTypePrepare
	case "interview_experience":
		return WorkspaceTypeExperiences
	case "interview_record":
		return WorkspaceTypeMyInterviews
	case "review_note":
		return WorkspaceTypeRecord
	default:
		return strings.ToLower(strings.TrimSpace(itemType))
	}
}

func rewriteLegacyRelPath(path string) string {
	path = filepath.ToSlash(strings.TrimSpace(path))
	if path == "" {
		return ""
	}
	for _, prefix := range legacyPathPrefixes {
		if strings.HasPrefix(path, prefix.old) {
			return prefix.new + strings.TrimPrefix(path, prefix.old)
		}
	}
	return path
}

func isDirEmpty(path string) (bool, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return false, err
	}
	return len(entries) == 0, nil
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
	id := fmt.Sprintf("%s-%s-%s", itemIDPrefix(itemType), now.Format("20060102-150405"), slug(title))
	relDir := filepath.Join(workspaceTypeDir(itemType), id)
	if itemType == WorkspaceTypeResume {
		relDir = filepath.Join("resume", "versions", id)
	}
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
	if err := w.updateActivePointers(itemType, relDir, sourceRel, now); err != nil {
		return WorkspaceItem{}, err
	}
	return item, nil
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
	id := fmt.Sprintf("%s-%s-%s", itemIDPrefix(itemType), now.Format("20060102-150405"), slug(title))
	relDir := filepath.Join(workspaceTypeDir(itemType), id)
	absDir := filepath.Join(w.Root, relDir)
	if itemType == WorkspaceTypeResume {
		relDir = filepath.Join("resume", "versions", id)
		absDir = filepath.Join(w.Root, relDir)
	}
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
	if err := w.updateActivePointers(itemType, relDir, sourceRel, now); err != nil {
		return WorkspaceItem{}, err
	}
	return item, nil
}

func (w *Workspace) updateActivePointers(itemType string, relDir string, sourceRel string, now time.Time) error {
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

func (w *Workspace) writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %q: %w", path, err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %q: %w", path, err)
	}
	return nil
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
			tag := candidate
			if !seen[tag] {
				tags = append(tags, tag)
				seen[tag] = true
			}
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
