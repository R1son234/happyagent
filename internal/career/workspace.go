package career

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"happyagent/internal/jsonfile"
)

const DefaultWorkspaceRoot = "career-workspace"

var workspaceDirs = []string{
	"inbox",
	WorkspaceDirResume,
	WorkspaceDirJD,
	WorkspaceDirExperiences,
	WorkspaceDirPrepare,
	WorkspaceDirMyInterviews,
	WorkspaceDirArchive,
	WorkspaceDirOutputs,
	filepath.Join(WorkspaceDirOutputs, "runs"),
	WorkspaceInternalDir,
	filepath.Join(WorkspaceInternalDir, "items"),
	filepath.Join(WorkspaceInternalDir, "record"),
	filepath.Join(WorkspaceInternalDir, "record", "imports"),
	filepath.Join(WorkspaceInternalDir, "record", "unclassified"),
	filepath.Join(WorkspaceInternalDir, "record", "generated"),
}

const (
	WorkspaceDirResume       = "我的简历"
	WorkspaceDirJD           = "岗位明细"
	WorkspaceDirExperiences  = "面经汇总"
	WorkspaceDirPrepare      = "复习资料库"
	WorkspaceDirMyInterviews = "我的面试"
	WorkspaceDirArchive      = "已归档"
	WorkspaceDirOutputs      = "输出报告"
	WorkspaceInternalDir     = ".happyagent/workspace"
)

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
	ID                 string    `json:"id"`
	Title              string    `json:"title"`
	Type               string    `json:"type"`
	CreatedAt          time.Time `json:"created_at"`
	Source             string    `json:"source"`
	Original           string    `json:"original,omitempty"`
	ExternalSource     string    `json:"external_source,omitempty"`
	Extractor          string    `json:"extractor,omitempty"`
	MIMEType           string    `json:"mime_type,omitempty"`
	ExtractStatus      string    `json:"extract_status,omitempty"`
	ExtractError       string    `json:"extract_error,omitempty"`
	SourceFingerprint  string    `json:"source_fingerprint,omitempty"`
	ContentFingerprint string    `json:"content_fingerprint,omitempty"`
}

type WorkspaceFileInput struct {
	ItemType           string
	Title              string
	Text               string
	OriginalPath       string
	OriginalName       string
	Now                time.Time
	Extractor          string
	MIMEType           string
	ExtractStatus      string
	ExtractError       string
	ContentFingerprint string
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
	GeneratedPaths []string
	Domain         ReviewDomain
}

func OpenWorkspace(root string, now time.Time) (*Workspace, error) {
	if strings.TrimSpace(root) == "" {
		root = DefaultWorkspaceRoot
	}
	if now.IsZero() {
		now = time.Now()
	}
	if err := migrateWorkspaceToUserVisibleLayout(root, now); err != nil {
		return nil, err
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
	if err := ws.EnsureReviewLibrarySkeleton(now); err != nil {
		return nil, err
	}
	return ws, nil
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

func (w *Workspace) metadataPath() string {
	return filepath.Join(w.Root, WorkspaceInternalDir, "workspace.json")
}

func (w *Workspace) indexPath() string {
	return filepath.Join(w.Root, WorkspaceInternalDir, "index.json")
}

func (w *Workspace) guidePath() string {
	return filepath.Join(w.Root, WorkspaceInternalDir, WorkspaceGuideFileName)
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

func migrateWorkspaceToUserVisibleLayout(root string, now time.Time) error {
	if strings.TrimSpace(root) == "" {
		root = DefaultWorkspaceRoot
	}
	if _, err := os.Stat(root); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat workspace root: %w", err)
	}
	var moves []workspaceMove
	for _, pair := range []struct {
		from string
		to   string
	}{
		{"resume", WorkspaceDirResume},
		{"jd", WorkspaceDirJD},
		{"experiences", WorkspaceDirExperiences},
		{"prepare", WorkspaceDirPrepare},
		{"my-interviews", WorkspaceDirMyInterviews},
		{"outputs", WorkspaceDirOutputs},
		{"record", filepath.Join(WorkspaceInternalDir, "record")},
		{"workspace.json", filepath.Join(WorkspaceInternalDir, "workspace.json")},
		{"index.json", filepath.Join(WorkspaceInternalDir, "index.json")},
		{WorkspaceGuideFileName, filepath.Join(WorkspaceInternalDir, WorkspaceGuideFileName)},
	} {
		moved, err := moveWorkspacePath(root, pair.from, pair.to)
		if err != nil {
			return err
		}
		moves = append(moves, moved...)
	}
	if len(moves) == 0 {
		return nil
	}
	if err := rewriteWorkspaceIndexPaths(filepath.Join(root, WorkspaceInternalDir, "index.json")); err != nil {
		return err
	}
	if err := rewriteWorkspaceMetadataPaths(filepath.Join(root, WorkspaceInternalDir, "workspace.json")); err != nil {
		return err
	}
	return writeMigrationLog(root, now, moves)
}

type workspaceMove struct {
	From string `json:"from"`
	To   string `json:"to"`
}

func moveWorkspacePath(root string, fromRel string, toRel string) ([]workspaceMove, error) {
	from := filepath.Join(root, filepath.FromSlash(fromRel))
	if _, err := os.Stat(from); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("stat old workspace path %q: %w", fromRel, err)
	}
	to := filepath.Join(root, filepath.FromSlash(toRel))
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return nil, fmt.Errorf("create migration parent %q: %w", toRel, err)
	}
	if _, err := os.Stat(to); os.IsNotExist(err) {
		if err := os.Rename(from, to); err != nil {
			return nil, fmt.Errorf("migrate %q to %q: %w", fromRel, toRel, err)
		}
		return []workspaceMove{{From: filepath.ToSlash(fromRel), To: filepath.ToSlash(toRel)}}, nil
	}
	info, err := os.Stat(from)
	if err != nil {
		return nil, fmt.Errorf("stat migration source %q: %w", fromRel, err)
	}
	if !info.IsDir() {
		target := uniquePath(to)
		if err := os.Rename(from, target); err != nil {
			return nil, fmt.Errorf("migrate %q to %q: %w", fromRel, toRel, err)
		}
		return []workspaceMove{{From: filepath.ToSlash(fromRel), To: filepath.ToSlash(relFromRoot(root, target))}}, nil
	}
	entries, err := os.ReadDir(from)
	if err != nil {
		return nil, fmt.Errorf("read migration source %q: %w", fromRel, err)
	}
	var moves []workspaceMove
	for _, entry := range entries {
		childFrom := filepath.Join(from, entry.Name())
		childTo := uniquePath(filepath.Join(to, entry.Name()))
		if err := os.Rename(childFrom, childTo); err != nil {
			return nil, fmt.Errorf("migrate %q to %q: %w", relFromRoot(root, childFrom), relFromRoot(root, childTo), err)
		}
		moves = append(moves, workspaceMove{From: filepath.ToSlash(relFromRoot(root, childFrom)), To: filepath.ToSlash(relFromRoot(root, childTo))})
	}
	if err := os.Remove(from); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("remove migrated dir %q: %w", fromRel, err)
	}
	return moves, nil
}

func uniquePath(path string) string {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path
	}
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-%d%s", base, i, ext)
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
}

func relFromRoot(root string, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(rel)
}

func rewriteWorkspaceIndexPaths(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read migrated index: %w", err)
	}
	var index WorkspaceIndex
	if err := json.Unmarshal(data, &index); err != nil {
		return fmt.Errorf("parse migrated index: %w", err)
	}
	for i := range index.Items {
		index.Items[i].Path = rewriteVisibleRel(index.Items[i].Path)
	}
	formatted, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(formatted, '\n'), 0o644)
}

func rewriteWorkspaceMetadataPaths(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read migrated metadata: %w", err)
	}
	var meta WorkspaceMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return fmt.Errorf("parse migrated metadata: %w", err)
	}
	meta.CurrentResume = rewriteVisibleRel(meta.CurrentResume)
	meta.ActiveJD = rewriteVisibleRel(meta.ActiveJD)
	meta.ActiveProject = rewriteVisibleRel(meta.ActiveProject)
	formatted, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(formatted, '\n'), 0o644)
}

func rewriteVisibleRel(path string) string {
	path = filepath.ToSlash(strings.TrimSpace(path))
	replacements := []struct {
		from string
		to   string
	}{
		{"resume/", WorkspaceDirResume + "/"},
		{"jd/", WorkspaceDirJD + "/"},
		{"experiences/", WorkspaceDirExperiences + "/"},
		{"prepare/", WorkspaceDirPrepare + "/"},
		{"my-interviews/", WorkspaceDirMyInterviews + "/"},
		{"outputs/", WorkspaceDirOutputs + "/"},
		{"record/", WorkspaceInternalDir + "/record/"},
	}
	for _, replacement := range replacements {
		if strings.HasPrefix(path, replacement.from) {
			return replacement.to + strings.TrimPrefix(path, replacement.from)
		}
	}
	return path
}

func writeMigrationLog(root string, now time.Time, moves []workspaceMove) error {
	if now.IsZero() {
		now = time.Now()
	}
	dir := filepath.Join(root, WorkspaceInternalDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, fmt.Sprintf("migration-%s.json", now.Format("20060102-150405")))
	data, err := json.MarshalIndent(struct {
		MovedAt time.Time       `json:"moved_at"`
		Moves   []workspaceMove `json:"moves"`
	}{MovedAt: now, Moves: moves}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// ContentFingerprint computes a SHA-256 hash of normalized text content.
func ContentFingerprint(text string) string {
	normalized := strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	hash := sha256.Sum256([]byte(normalized))
	return fmt.Sprintf("%x", hash[:16])
}

// FindExistingByContentFingerprint searches the workspace index and metadata for an item
// of the given type with a matching content fingerprint. Returns the existing item and true
// if found, or a zero WorkspaceItem and false otherwise.
func (w *Workspace) FindExistingByContentFingerprint(itemType string, contentFingerprint string) (WorkspaceItem, bool) {
	if contentFingerprint == "" {
		return WorkspaceItem{}, false
	}
	index, err := w.ReadIndex()
	if err != nil {
		return WorkspaceItem{}, false
	}
	for _, item := range index.Items {
		if item.Type != itemType {
			continue
		}
		metaPath := filepath.Join(w.Root, internalItemRelDir(item.ID), "metadata.json")
		var meta WorkspaceItemMetadata
		if err := w.readJSON(metaPath, &meta); err != nil {
			continue
		}
		if meta.ContentFingerprint == contentFingerprint {
			return item, true
		}
	}
	return WorkspaceItem{}, false
}
