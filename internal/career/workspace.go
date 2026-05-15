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
	SourceFingerprint    string `json:"source_fingerprint,omitempty"`
	ContentFingerprint  string `json:"content_fingerprint,omitempty"`
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
		metaPath := filepath.Join(w.Root, filepath.FromSlash(item.Path))
		metaPath = filepath.Join(filepath.Dir(metaPath), "metadata.json")
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
