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
		"resume",
		"jd",
		"experiences",
		"prepare",
		"my-interviews",
		"outputs",
		"outputs/runs",
		"record",
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

	item, err := ws.AddJD(`# Sample Role

岗位职责：
- Coordinate cross-functional projects and organize reusable work artifacts.

任职要求：
- Familiar with stakeholder communication, execution tracking, and review habits.
`, now)
	if err != nil {
		t.Fatalf("AddJD() error = %v", err)
	}
	if item.Type != "jd" {
		t.Fatalf("unexpected item type: %+v", item)
	}
	if item.Title != "Sample Role" {
		t.Fatalf("unexpected title: %q", item.Title)
	}
	if item.Path == "" {
		t.Fatalf("expected source path")
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(item.Path))); err != nil {
		t.Fatalf("source was not written: %v", err)
	}
	metadataPath := filepath.Join(root, "jd", item.ID, "metadata.json")
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

	item, err := ws.AddMaterial(WorkspaceTypeResume, "简历\n工作经历：项目协作\n项目经历：跨部门项目推进", now)
	if err != nil {
		t.Fatalf("AddMaterial() error = %v", err)
	}
	if item.Type != WorkspaceTypeResume {
		t.Fatalf("unexpected type: %+v", item)
	}
	if filepath.Dir(filepath.FromSlash(item.Path)) != filepath.Join("resume", "versions", item.ID) {
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

func TestWriteUserOutputCreatesLatestAndTimestampedFiles(t *testing.T) {
	root := filepath.Join(t.TempDir(), "career")
	now := time.Date(2026, 5, 10, 10, 0, 0, 0, time.UTC)
	ws, err := OpenWorkspace(root, now)
	if err != nil {
		t.Fatalf("OpenWorkspace() error = %v", err)
	}

	paths, err := ws.WriteUserOutput("report", "完整匹配报告", "这里是报告内容。", []byte("{\"ok\":true}"), now)
	if err != nil {
		t.Fatalf("WriteUserOutput() error = %v", err)
	}
	for _, rel := range []string{
		paths.LatestMarkdown,
		paths.TimestampedMarkdown,
		paths.LatestJSON,
		paths.TimestampedJSON,
	} {
		if rel == "" {
			t.Fatalf("expected output path to be set")
		}
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("expected output file %s: %v", rel, err)
		}
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

func TestArchivePublicInterviewExperienceSplitsMaterial(t *testing.T) {
	root := filepath.Join(t.TempDir(), "career")
	now := time.Date(2026, 5, 9, 21, 0, 0, 0, time.UTC)
	ws, err := OpenWorkspace(root, now)
	if err != nil {
		t.Fatalf("OpenWorkspace() error = %v", err)
	}

	result, err := ws.ArchivePublicInterviewExperience("市场营销公开面经：一面问用户增长，高频题包括项目追问、技术方案和证据口径。", now)
	if err != nil {
		t.Fatalf("ArchivePublicInterviewExperience() error = %v", err)
	}
	if result.ExperienceItem.Type != WorkspaceTypeExperiences || !strings.HasPrefix(result.ExperienceItem.Path, "experiences/") {
		t.Fatalf("unexpected experience item: %+v", result.ExperienceItem)
	}
	if result.PrepareItem.Type != WorkspaceTypePrepare || !strings.HasPrefix(result.PrepareItem.Path, "prepare/") {
		t.Fatalf("expected prepare item, got %+v", result.PrepareItem)
	}
	for _, rel := range []string{
		result.MyInterviewRel,
		result.RecordRel,
	} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("expected derived file %s: %v", rel, err)
		}
	}
	if !strings.Contains(result.MyInterviewRel, "my-interviews/市场营销/") {
		t.Fatalf("expected marketing interview directory, got %s", result.MyInterviewRel)
	}
	_, index, err := ws.Status()
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	counts := map[string]int{}
	for _, item := range index.Items {
		counts[item.Type]++
	}
	if counts[WorkspaceTypeExperiences] != 1 || counts[WorkspaceTypePrepare] != 1 || counts[WorkspaceTypeRecord] != 1 {
		t.Fatalf("unexpected index counts: %+v", counts)
	}
}

func TestLooksLikeJD(t *testing.T) {
	if !LooksLikeJD("岗位职责：负责业务规划、跨部门协作、资料整理和结果复盘。\n任职要求：熟悉沟通协调、执行跟踪、文档沉淀和问题分析。") {
		t.Fatalf("expected Chinese JD to be detected")
	}
	if LooksLikeJD("今天面试问到了项目复盘，我回答得一般。") {
		t.Fatalf("short interview note should not be detected as JD")
	}
}
