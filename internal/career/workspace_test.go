package career

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOpenWorkspaceCreatesCareerDirsAndFiles(t *testing.T) {
	root := filepath.Join(t.TempDir(), "career")
	now := time.Date(2026, 4, 30, 15, 30, 0, 0, time.FixedZone("CST", 8*60*60))

	ws, err := OpenWorkspace(root, now)
	if err != nil {
		t.Fatalf("OpenWorkspace() error = %v", err)
	}
	if ws.Root != root {
		t.Fatalf("unexpected root: %s", ws.Root)
	}
	for _, rel := range []string{
		"workspace.json",
		"index.json",
		"inbox",
		"jds",
		"resumes",
		"projects",
		"interview_experience",
		"interview_records",
		"review_notes",
		"reports",
		"exports",
	} {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Fatalf("workspace missing %s: %v", rel, err)
		}
	}
	meta, index, err := ws.Status()
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if meta.Version != 1 || !meta.CreatedAt.Equal(now) {
		t.Fatalf("unexpected metadata: %+v", meta)
	}
	if len(index.Items) != 0 {
		t.Fatalf("unexpected index items: %+v", index.Items)
	}
}

func TestAddJDSavesSourceMetadataAndIndex(t *testing.T) {
	root := filepath.Join(t.TempDir(), "career")
	now := time.Date(2026, 4, 30, 15, 45, 0, 0, time.UTC)
	ws, err := OpenWorkspace(root, now)
	if err != nil {
		t.Fatalf("OpenWorkspace() error = %v", err)
	}

	item, err := ws.AddJD(`# AI Agent Backend Engineer

岗位职责：
- Build Go backend services for Agent runtime and RAG workflows.

任职要求：
- Familiar with MCP, LLM, observability, and tool calling.
`, now)
	if err != nil {
		t.Fatalf("AddJD() error = %v", err)
	}
	if item.Type != "jd" {
		t.Fatalf("unexpected item type: %+v", item)
	}
	if item.Title != "AI Agent Backend Engineer" {
		t.Fatalf("unexpected title: %q", item.Title)
	}
	if item.Path == "" {
		t.Fatalf("expected source path")
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(item.Path))); err != nil {
		t.Fatalf("source was not written: %v", err)
	}
	metadataPath := filepath.Join(root, "jds", item.ID, "metadata.json")
	if _, err := os.Stat(metadataPath); err != nil {
		t.Fatalf("metadata was not written: %v", err)
	}
	meta, index, err := ws.Status()
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if meta.ActiveJD != item.Path {
		t.Fatalf("unexpected active jd: %q", meta.ActiveJD)
	}
	if len(index.Items) != 1 || index.Items[0].ID != item.ID {
		t.Fatalf("index not updated: %+v", index.Items)
	}
}

func TestAddMaterialSavesResumeVersionAndUpdatesCurrentResume(t *testing.T) {
	root := filepath.Join(t.TempDir(), "career")
	now := time.Date(2026, 4, 30, 16, 0, 0, 0, time.UTC)
	ws, err := OpenWorkspace(root, now)
	if err != nil {
		t.Fatalf("OpenWorkspace() error = %v", err)
	}

	item, err := ws.AddMaterial(WorkspaceTypeResume, "简历\n工作经历：Go 后端开发\n项目经历：Agent runtime", now)
	if err != nil {
		t.Fatalf("AddMaterial() error = %v", err)
	}
	if item.Type != WorkspaceTypeResume {
		t.Fatalf("unexpected type: %+v", item)
	}
	if filepath.Dir(filepath.FromSlash(item.Path)) != filepath.Join("resumes", "versions", item.ID) {
		t.Fatalf("unexpected resume path: %s", item.Path)
	}
	meta, index, err := ws.Status()
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if meta.CurrentResume != item.Path {
		t.Fatalf("current resume not updated: %q", meta.CurrentResume)
	}
	if len(index.Items) != 1 || index.Items[0].Type != WorkspaceTypeResume {
		t.Fatalf("index not updated: %+v", index.Items)
	}
}

func TestAddMaterialFromFileStoresOriginalAndMetadata(t *testing.T) {
	root := filepath.Join(t.TempDir(), "career")
	now := time.Date(2026, 4, 30, 16, 5, 0, 0, time.UTC)
	ws, err := OpenWorkspace(root, now)
	if err != nil {
		t.Fatalf("OpenWorkspace() error = %v", err)
	}
	sourceDir := t.TempDir()
	sourcePath := filepath.Join(sourceDir, "resume.txt")
	if err := os.WriteFile(sourcePath, []byte("简历\n工作经历：Go 后端\n"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	item, err := ws.AddMaterialFromFile(WorkspaceFileInput{
		ItemType:      WorkspaceTypeResume,
		Text:          "简历\n工作经历：Go 后端",
		OriginalPath:  sourcePath,
		OriginalName:  "resume.txt",
		Now:           now,
		Extractor:     "plain_text",
		MIMEType:      "text/plain",
		ExtractStatus: "ok",
	})
	if err != nil {
		t.Fatalf("AddMaterialFromFile() error = %v", err)
	}
	if !strings.HasSuffix(item.Path, "/extracted.md") {
		t.Fatalf("unexpected path: %s", item.Path)
	}
	if item.Metadata.Original == "" || item.Metadata.Source != item.Path {
		t.Fatalf("unexpected metadata: %+v", item.Metadata)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(item.Metadata.Original))); err != nil {
		t.Fatalf("expected copied original file: %v", err)
	}
}

func TestLooksLikeJD(t *testing.T) {
	if !LooksLikeJD("岗位职责：负责 RAG 后端服务和 Agent runtime。\n任职要求：熟悉 Go、MCP、LLM tool calling。") {
		t.Fatalf("expected Chinese JD to be detected")
	}
	if LooksLikeJD("今天面试问到了 MCP，我回答得一般。") {
		t.Fatalf("short interview note should not be detected as JD")
	}
}
