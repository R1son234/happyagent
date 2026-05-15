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

func TestAddMaterialFromFileAvoidsSameSecondChineseTitleCollision(t *testing.T) {
	root := filepath.Join(t.TempDir(), "career")
	now := time.Date(2026, 5, 14, 14, 37, 18, 0, time.UTC)
	ws, err := OpenWorkspace(root, now)
	if err != nil {
		t.Fatalf("OpenWorkspace() error = %v", err)
	}
	sourceDir := t.TempDir()
	firstPath := filepath.Join(sourceDir, "字节Agent开发三面面经.md")
	secondPath := filepath.Join(sourceDir, "字节Agent开发二面面经.md")
	if err := os.WriteFile(firstPath, []byte("# 字节Agent开发三面面经\n三面内容"), 0o644); err != nil {
		t.Fatalf("write first source: %v", err)
	}
	if err := os.WriteFile(secondPath, []byte("# 字节Agent开发二面面经\n二面内容"), 0o644); err != nil {
		t.Fatalf("write second source: %v", err)
	}

	first, err := ws.AddMaterialFromFile(WorkspaceFileInput{
		ItemType:     WorkspaceTypeExperiences,
		Text:         "# 字节Agent开发三面面经\n三面内容",
		OriginalPath: firstPath,
		OriginalName: filepath.Base(firstPath),
		Now:          now,
	})
	if err != nil {
		t.Fatalf("AddMaterialFromFile(first) error = %v", err)
	}
	second, err := ws.AddMaterialFromFile(WorkspaceFileInput{
		ItemType:     WorkspaceTypeExperiences,
		Text:         "# 字节Agent开发二面面经\n二面内容",
		OriginalPath: secondPath,
		OriginalName: filepath.Base(secondPath),
		Now:          now,
	})
	if err != nil {
		t.Fatalf("AddMaterialFromFile(second) error = %v", err)
	}

	if first.ID == second.ID || first.Path == second.Path {
		t.Fatalf("expected distinct material paths, got first=%+v second=%+v", first, second)
	}
	for _, item := range []WorkspaceItem{first, second} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(item.Path))); err != nil {
			t.Fatalf("expected extracted file %s: %v", item.Path, err)
		}
	}
	firstData, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(first.Path)))
	if err != nil {
		t.Fatalf("read first extracted: %v", err)
	}
	secondData, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(second.Path)))
	if err != nil {
		t.Fatalf("read second extracted: %v", err)
	}
	if !strings.Contains(string(firstData), "三面内容") || !strings.Contains(string(secondData), "二面内容") {
		t.Fatalf("unexpected extracted contents:\nfirst=%s\nsecond=%s", firstData, secondData)
	}
}

func TestAddGuidedMaterialWritesClassificationRecord(t *testing.T) {
	root := filepath.Join(t.TempDir(), "career")
	now := time.Date(2026, 5, 10, 20, 15, 0, 0, time.UTC)
	ws, err := OpenWorkspace(root, now)
	if err != nil {
		t.Fatalf("OpenWorkspace() error = %v", err)
	}
	classification := InputClassification{
		Type:       WorkspaceTypeJD,
		Confidence: 0.9,
		Signals:    []string{"岗位职责", "任职要求"},
		ShouldSave: true,
		Reason:     "test classification",
		RulePath:   "jd",
	}
	result, err := ws.AddGuidedMaterial(GuidedMaterialInput{
		ItemType:       WorkspaceTypeJD,
		Classification: classification,
		Content:        "# Sample Role\n岗位职责：负责增长分析。\n任职要求：熟悉内容策略。",
		SourceLabel:    "inbox/jd.md",
		Now:            now,
	})
	if err != nil {
		t.Fatalf("AddGuidedMaterial() error = %v", err)
	}
	if result.Item.Type != WorkspaceTypeJD || result.RecordRel == "" {
		t.Fatalf("unexpected guided result: %+v", result)
	}
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(result.RecordRel)))
	if err != nil {
		t.Fatalf("read classification record: %v", err)
	}
	record := string(data)
	for _, expected := range []string{"classified_type: jd", "confidence: 0.90", "matched_signals: 岗位职责, 任职要求", "destination:", "active_pointer_updated: active_jd"} {
		if !strings.Contains(record, expected) {
			t.Fatalf("classification record missing %q:\n%s", expected, record)
		}
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
		result.RecordRel,
	} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("expected derived file %s: %v", rel, err)
		}
	}
	if len(result.GeneratedPaths) == 0 {
		t.Fatalf("expected generated review library paths")
	}
	if result.Domain.Slug == "" || result.Domain.Slug == "job-description" {
		t.Fatalf("expected dynamic domain, got %+v", result.Domain)
	}
	joinedPaths := strings.Join(result.GeneratedPaths, "\n")
	if strings.Contains(joinedPaths, "面经来源与复习清单") || strings.Contains(joinedPaths, "domain-") {
		t.Fatalf("unexpected legacy generated path: %+v", result.GeneratedPaths)
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

func TestContentFingerprintIsDeterministic(t *testing.T) {
	text := "# JD\n岗位职责：负责增长分析。\n任职要求：熟悉内容策略。"
	fp1 := ContentFingerprint(text)
	fp2 := ContentFingerprint(text)
	if fp1 != fp2 {
		t.Fatalf("expected deterministic fingerprint, got %s and %s", fp1, fp2)
	}
	if fp1 == "" {
		t.Fatalf("expected non-empty fingerprint")
	}
}

func TestContentFingerprintDiffersForDifferentContent(t *testing.T) {
	fp1 := ContentFingerprint("# JD\n岗位职责：负责增长分析。")
	fp2 := ContentFingerprint("# JD\n岗位职责：负责内容运营。")
	if fp1 == fp2 {
		t.Fatalf("expected different fingerprints for different content, both got %s", fp1)
	}
}

func TestContentFingerprintNormalizesWhitespace(t *testing.T) {
	fp1 := ContentFingerprint("  hello   world  ")
	fp2 := ContentFingerprint("hello world")
	if fp1 != fp2 {
		t.Fatalf("expected same fingerprint after whitespace normalization, got %s and %s", fp1, fp2)
	}
}
