package career

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestExtractReferencedFilesFindsMultipleLocalPaths(t *testing.T) {
	input := `我在 "` + filepath.Join("testdata", "resume-sample.docx") + `" 放了简历，` + filepath.Join("testdata", "jd-sample.txt") + ` 是目标 jd，帮我分析一下。`
	paths := extractReferencedFiles(input)
	if len(paths) != 2 {
		t.Fatalf("expected 2 paths, got %+v", paths)
	}
	if paths[0] != filepath.Join("testdata", "resume-sample.docx") || paths[1] != filepath.Join("testdata", "jd-sample.txt") {
		t.Fatalf("unexpected paths: %+v", paths)
	}
}

func TestExtractReferencedDirectoriesFindsDirectoryHints(t *testing.T) {
	input := `我在 test目录 里放了简历，在 ./fixtures 文件夹里放了 JD。`
	dirs := extractReferencedDirectories(input)
	if len(dirs) != 2 {
		t.Fatalf("expected 2 directories, got %+v", dirs)
	}
	if dirs[0] != "test" || dirs[1] != "./fixtures" {
		t.Fatalf("unexpected directories: %+v", dirs)
	}
}

func TestExtractReferencedFilesJoinsChineseDirectoryPhraseAndFileName(t *testing.T) {
	input := `我在mytest目录里放了ai.txt,是我搜集到的jd,你帮我记录分析下`
	paths := extractReferencedFiles(input)
	if len(paths) != 1 {
		t.Fatalf("expected 1 path, got %+v", paths)
	}
	if paths[0] != filepath.Join("mytest", "ai.txt") {
		t.Fatalf("expected joined path, got %+v", paths)
	}
}

func TestExtractReferencedFilesDeduplicatesParenthesizedFileInDirectory(t *testing.T) {
	input := `我在mytest目录里放了我的简历和jd(ai.txt),你帮我记录并分析一下`
	paths := extractReferencedFiles(input)
	if len(paths) != 1 {
		t.Fatalf("expected 1 path, got %+v", paths)
	}
	if paths[0] != filepath.Join("mytest", "ai.txt") {
		t.Fatalf("expected joined path, got %+v", paths)
	}
}

func TestDiscoverFilesInReferencedDirectoriesPrefersResumeDocx(t *testing.T) {
	testDir, err := filepath.Abs("testdata")
	if err != nil {
		t.Fatalf("filepath.Abs() error = %v", err)
	}
	files := discoverFilesInReferencedDirectories("我在 " + testDir + "目录里存了我的简历,一个docx文件,你帮我分析一下")
	if len(files) != 1 {
		t.Fatalf("expected 1 discovered file, got %+v", files)
	}
	expected := filepath.Join(testDir, "resume-sample.docx")
	if files[0] != expected {
		t.Fatalf("expected %q, got %+v", expected, files)
	}
}

func TestDiscoverFilesInReferencedDirectoriesPrefersInterviewExperience(t *testing.T) {
	testDir, err := os.MkdirTemp(".", "careerexp")
	if err != nil {
		t.Fatalf("create fixture dir: %v", err)
	}
	defer os.RemoveAll(testDir)
	resumePath := filepath.Join(testDir, "resume-base-optimized-v2.docx")
	if err := os.WriteFile(resumePath, []byte("not a real docx"), 0o644); err != nil {
		t.Fatalf("write resume fixture: %v", err)
	}
	experiencePath := filepath.Join(testDir, "字节跳动-AI-Agent-面经-2026-04-30.md")
	if err := os.WriteFile(experiencePath, []byte("# 字节跳动 AI Agent 面经"), 0o644); err != nil {
		t.Fatalf("write experience fixture: %v", err)
	}

	input := "我在 " + testDir + "目录里存了一份面经,你帮我记录一下"
	dirs := extractReferencedDirectories(input)
	if len(dirs) != 1 {
		t.Fatalf("expected 1 referenced dir from %q, got %+v", input, dirs)
	}
	if dirs[0] != testDir {
		t.Fatalf("expected referenced dir %q, got %+v", testDir, dirs)
	}
	files := discoverFilesInReferencedDirectories(input)
	if len(files) != 1 {
		t.Fatalf("expected 1 discovered file, got %+v", files)
	}
	if files[0] != experiencePath {
		t.Fatalf("expected %q, got %+v", experiencePath, files)
	}
}

func TestIngestFileExtractsDOCXResume(t *testing.T) {
	ws, err := OpenWorkspace(filepath.Join(t.TempDir(), "career"), time.Now())
	if err != nil {
		t.Fatalf("OpenWorkspace() error = %v", err)
	}
	result, err := IngestFile(context.Background(), ws, IngestRequest{
		Path:      filepath.Join("testdata", "resume-sample.docx"),
		HintType:  WorkspaceTypeResume,
		UserInput: "这是我的简历，帮我看看",
		Now:       time.Now(),
	})
	if err != nil {
		t.Fatalf("IngestFile() error = %v", err)
	}
	if result.Item.Type != WorkspaceTypeResume {
		t.Fatalf("expected resume item, got %+v", result.Item)
	}
	if result.ExtractedRel == "" || result.OriginalRel == "" {
		t.Fatalf("expected original and extracted paths, got %+v", result)
	}
	data, err := os.ReadFile(filepath.Join(ws.Root, filepath.FromSlash(result.ExtractedRel)))
	if err != nil {
		t.Fatalf("read extracted: %v", err)
	}
	text := string(data)
	for _, expected := range []string{"Sample Backend Engineer", "Go", "Agent runtime"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("expected extracted docx to contain %q, got:\n%s", expected, text)
		}
	}
}

func TestIngestFileUsesContentSignalsWhenHintMissing(t *testing.T) {
	ws, err := OpenWorkspace(filepath.Join(t.TempDir(), "career"), time.Now())
	if err != nil {
		t.Fatalf("OpenWorkspace() error = %v", err)
	}
	sourcePath := filepath.Join(t.TempDir(), "ai.txt")
	content := "# Sample Role\n岗位职责：负责项目规划和跨部门协作。\n任职要求：熟悉沟通协调和执行复盘。\n"
	if err := os.WriteFile(sourcePath, []byte(content), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	result, err := IngestFile(context.Background(), ws, IngestRequest{
		Path:      sourcePath,
		UserInput: "帮我记录一下这个岗位",
		Now:       time.Now(),
	})
	if err != nil {
		t.Fatalf("IngestFile() error = %v", err)
	}
	if result.ItemType != WorkspaceTypeJD {
		t.Fatalf("expected jd type, got %+v", result)
	}
}

func TestIngestInboxArchivesFilesAndRemovesInboxCopies(t *testing.T) {
	ws, err := OpenWorkspace(filepath.Join(t.TempDir(), "career"), time.Now())
	if err != nil {
		t.Fatalf("OpenWorkspace() error = %v", err)
	}
	resumePath := filepath.Join(ws.Root, "inbox", "resume.md")
	jdPath := filepath.Join(ws.Root, "inbox", "jd.txt")
	if err := os.WriteFile(resumePath, []byte("# Resume\n简历：项目增长复盘。"), 0o644); err != nil {
		t.Fatalf("write resume: %v", err)
	}
	if err := os.WriteFile(jdPath, []byte("# JD\n岗位职责：负责增长分析。\n任职要求：熟悉内容策略。"), 0o644); err != nil {
		t.Fatalf("write jd: %v", err)
	}

	result, err := IngestInbox(context.Background(), ws, time.Now())
	if err != nil {
		t.Fatalf("IngestInbox() error = %v", err)
	}
	if len(result.Items) != 2 {
		t.Fatalf("expected 2 ingested items, got %+v", result.Items)
	}
	if _, err := os.Stat(resumePath); !os.IsNotExist(err) {
		t.Fatalf("expected resume inbox copy to be removed, err=%v", err)
	}
	if _, err := os.Stat(jdPath); !os.IsNotExist(err) {
		t.Fatalf("expected jd inbox copy to be removed, err=%v", err)
	}
	archiveEntries, err := os.ReadDir(filepath.Join(ws.Root, WorkspaceDirArchive))
	if err != nil || len(archiveEntries) == 0 {
		t.Fatalf("expected archived originals, entries=%+v err=%v", archiveEntries, err)
	}
}

func TestIngestInboxSkipsAlreadyArchivedFiles(t *testing.T) {
	ws, err := OpenWorkspace(filepath.Join(t.TempDir(), "career"), time.Now())
	if err != nil {
		t.Fatalf("OpenWorkspace() error = %v", err)
	}
	jdPath := filepath.Join(ws.Root, "inbox", "jd.txt")
	content := "# JD\n岗位职责：负责增长分析。\n任职要求：熟悉内容策略。"
	if err := os.WriteFile(jdPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write jd: %v", err)
	}

	// First ingest should create a new item.
	first, err := IngestInbox(context.Background(), ws, time.Now())
	if err != nil {
		t.Fatalf("first IngestInbox() error = %v", err)
	}
	if len(first.Items) != 1 {
		t.Fatalf("expected 1 ingested item on first run, got %d", len(first.Items))
	}
	firstID := first.Items[0].ID

	if err := os.WriteFile(jdPath, []byte(content), 0o644); err != nil {
		t.Fatalf("rewrite jd: %v", err)
	}
	// Second ingest of the same file should skip creating a duplicate item.
	second, err := IngestInbox(context.Background(), ws, time.Now())
	if err != nil {
		t.Fatalf("second IngestInbox() error = %v", err)
	}
	if len(second.Items) != 1 {
		t.Fatalf("expected 1 existing item on second run (dedup), got %d", len(second.Items))
	}
	if second.Items[0].ID != firstID {
		t.Fatalf("expected same item ID on dedup, got first=%s second=%s", firstID, second.Items[0].ID)
	}

	// Index should still have exactly 1 item.
	index, err := ws.ReadIndex()
	if err != nil {
		t.Fatalf("ReadIndex() error = %v", err)
	}
	if len(index.Items) != 1 {
		t.Fatalf("expected 1 index item after dedup, got %d", len(index.Items))
	}
}
