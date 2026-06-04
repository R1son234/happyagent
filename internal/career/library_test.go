package career

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOpenWorkspaceCreatesReviewLibraryEntryPoints(t *testing.T) {
	root := filepath.Join(t.TempDir(), "career")
	ws, err := OpenWorkspace(root, time.Date(2026, 5, 15, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("OpenWorkspace() error = %v", err)
	}
	for _, rel := range []string{
		"面试资料库首页.md",
		filepath.Join(WorkspaceDirExperiences, "面经总览.md"),
		filepath.Join(WorkspaceDirPrepare, "复习资料总览.md"),
		filepath.Join(WorkspaceDirJD, "岗位汇总.md"),
	} {
		data, err := os.ReadFile(filepath.Join(ws.Root, rel))
		if err != nil {
			t.Fatalf("expected review library entry %s: %v", rel, err)
		}
		if strings.HasPrefix(strings.TrimSpace(string(data)), "---") || strings.Contains(string(data), "[[") {
			t.Fatalf("expected plain markdown in %s:\n%s", rel, data)
		}
	}
}

func TestArchivePublicInterviewExperienceGeneratesDynamicDirections(t *testing.T) {
	root := filepath.Join(t.TempDir(), "career")
	now := time.Date(2026, 5, 15, 11, 0, 0, 0, time.UTC)
	ws, err := OpenWorkspace(root, now)
	if err != nil {
		t.Fatalf("OpenWorkspace() error = %v", err)
	}

	first, err := ws.ArchivePublicInterviewExperience("示例方向A公开面经：一面问示例能力甲、示例机制乙和项目难点。", now)
	if err != nil {
		t.Fatalf("ArchivePublicInterviewExperience(first) error = %v", err)
	}
	second, err := ws.ArchivePublicInterviewExperience("示例方向B公开面经：追问示例流程、示例文档、协作策略和项目复盘。", now.Add(time.Minute))
	if err != nil {
		t.Fatalf("ArchivePublicInterviewExperience(second) error = %v", err)
	}

	for _, paths := range [][]string{first.GeneratedPaths, second.GeneratedPaths} {
		if strings.Contains(strings.Join(paths, "\n"), WorkspaceDirPrepare+"/") {
			t.Fatalf("archive should not generate LLM question banks without generator: %+v", paths)
		}
	}
	for _, paths := range [][]string{first.GeneratedPaths, second.GeneratedPaths} {
		for _, path := range paths {
			if strings.HasPrefix(path, WorkspaceDirExperiences+"/") && strings.Count(path, "/") > 1 {
				t.Fatalf("public interview experience should not generate visible experience subdir paths: %+v", paths)
			}
		}
	}
	if strings.Contains(strings.Join(first.GeneratedPaths, "\n"), WorkspaceDirMyInterviews+"/") ||
		strings.Contains(strings.Join(second.GeneratedPaths, "\n"), WorkspaceDirMyInterviews+"/") {
		t.Fatalf("public interview experience should not generate my-interviews paths: first=%+v second=%+v", first.GeneratedPaths, second.GeneratedPaths)
	}
}

func TestReviewLibraryWritesDeepDocumentsFromResumeJDExperience(t *testing.T) {
	root := filepath.Join(t.TempDir(), "career")
	now := time.Date(2026, 5, 15, 13, 0, 0, 0, time.UTC)
	ws, err := OpenWorkspace(root, now)
	if err != nil {
		t.Fatalf("OpenWorkspace() error = %v", err)
	}
	if _, err := ws.AddMaterial(WorkspaceTypeResume, `知页简历
求职意向：示例岗位
项目介绍
示例项目一
角色：示例项目负责人
负责示例方案设计、任务编排、质量验证和复盘，累计沉淀 12 个示例场景。
示例项目二
负责示例链路治理、异常处理、监控和回滚方案。`, now); err != nil {
		t.Fatalf("AddMaterial(resume) error = %v", err)
	}
	if _, err := ws.AddMaterial(WorkspaceTypeJD, `# 示例公司 示例岗位 JD
岗位职责：负责示例系统建设、任务调度、状态监控和质量验证。
任职要求：熟悉示例工程实践、问题拆解、协作沟通和复盘沉淀。`, now); err != nil {
		t.Fatalf("AddMaterial(jd) error = %v", err)
	}
	if _, err := ws.AddMaterial(WorkspaceTypeExperiences, `# 示例公司 示例岗位面经整理
- 你如何设计一个可验证的示例系统？
- 上一段经历中如何处理项目难点？
- 你如何理解任务调度和状态监控？
- 你的个人优势是什么？请结合经历说明。`, now); err != nil {
		t.Fatalf("AddMaterial(experience) error = %v", err)
	}
	result, err := ws.GenerateReviewLibraryWithGenerator(context.Background(), now, fakeQuestionBankGenerator{})
	if err != nil {
		t.Fatalf("GenerateReviewLibraryWithGenerator() error = %v", err)
	}
	joined := strings.Join(result.Paths, "\n")
	if strings.Contains(joined, WorkspaceDirMyInterviews+"/") {
		t.Fatalf("public interview experience should not generate my-interviews paths: %+v", result.Paths)
	}
	if !strings.Contains(joined, WorkspaceDirPrepare+"/") {
		t.Fatalf("expected question banks under prepare, got %+v", result.Paths)
	}
	bankPath := firstPathWithPrefixAndSuffix(result.Paths, WorkspaceDirPrepare+"/", "题库.md")
	bank, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(bankPath)))
	if err != nil {
		t.Fatalf("expected question bank %s: %v\npaths=%+v", bankPath, err, result.Paths)
	}
	for _, expected := range []string{"LLM 标准答案", "示例项目一", "12 个示例场景", "风险 / 待补证据"} {
		if !strings.Contains(string(bank), expected) {
			t.Fatalf("question bank missing %q:\n%s", expected, bank)
		}
	}
	if strings.Contains(joined, "-interview-qa.md") {
		t.Fatalf("review library should not create project QA files under prepare: %+v", result.Paths)
	}
}

func firstPathWithPrefixAndSuffix(paths []string, prefix string, suffix string) string {
	for _, path := range paths {
		if strings.HasPrefix(path, prefix) && strings.HasSuffix(path, suffix) {
			return path
		}
	}
	return ""
}

func TestGenerateReviewLibraryUsesGeneralForUnknownDirection(t *testing.T) {
	root := filepath.Join(t.TempDir(), "career")
	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	ws, err := OpenWorkspace(root, now)
	if err != nil {
		t.Fatalf("OpenWorkspace() error = %v", err)
	}
	if _, err := ws.AddMaterial(WorkspaceTypeExperiences, "公开面经：一面问你如何准备、如何复盘、如何补齐证据。", now); err != nil {
		t.Fatalf("AddMaterial() error = %v", err)
	}
	result, err := ws.GenerateReviewLibraryWithGenerator(context.Background(), now, fakeQuestionBankGenerator{})
	if err != nil {
		t.Fatalf("GenerateReviewLibraryWithGenerator() error = %v", err)
	}
	if len(result.Paths) == 0 {
		t.Fatalf("expected generated review library paths")
	}
	if !strings.Contains(strings.Join(result.Paths, "\n"), WorkspaceDirPrepare+"/") {
		t.Fatalf("expected generated question bank under %s: %+v", WorkspaceDirPrepare, result.Paths)
	}
}

func TestGenerateReviewLibraryRequiresLLMForQuestionBanks(t *testing.T) {
	root := filepath.Join(t.TempDir(), "career")
	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	ws, err := OpenWorkspace(root, now)
	if err != nil {
		t.Fatalf("OpenWorkspace() error = %v", err)
	}
	if _, err := ws.AddMaterial(WorkspaceTypeExperiences, "公开面经：一面问你如何准备、如何复盘、如何补齐证据。", now); err != nil {
		t.Fatalf("AddMaterial() error = %v", err)
	}
	if _, err := ws.GenerateReviewLibrary(now); err == nil || !strings.Contains(err.Error(), "requires LLM generator") {
		t.Fatalf("expected missing LLM generator error, got %v", err)
	}
}

func TestGenerateReviewLibraryDoesNotSplitMultiJDMaterialWithRules(t *testing.T) {
	root := filepath.Join(t.TempDir(), "career")
	now := time.Date(2026, 5, 15, 14, 0, 0, 0, time.UTC)
	ws, err := OpenWorkspace(root, now)
	if err != nil {
		t.Fatalf("OpenWorkspace() error = %v", err)
	}
	if _, err := ws.AddMaterial(WorkspaceTypeJD, `# 示例 JD 汇总

示例公司A 示例岗位A
岗位职责：负责示例能力 A 的方案设计和交付。
任职要求：熟悉示例方案设计和问题拆解。

示例公司B 示例岗位B
职位描述：负责示例能力 B 的服务建设和质量验证。
职位要求：具备示例工程经验和协作能力。`, now); err != nil {
		t.Fatalf("AddMaterial(jd) error = %v", err)
	}
	result, err := ws.GenerateReviewLibrary(now)
	if err != nil {
		t.Fatalf("GenerateReviewLibrary() error = %v", err)
	}
	if len(result.Paths) != 0 {
		t.Fatalf("GenerateReviewLibrary should not split JD or generate paths without experiences, got %+v", result.Paths)
	}
	_, index, err := ws.Status()
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	jdCount := 0
	for _, item := range index.Items {
		if item.Type == WorkspaceTypeJD {
			jdCount++
		}
	}
	if jdCount != 1 {
		t.Fatalf("expected original multi-JD item only, got %d JD items: %+v", jdCount, index.Items)
	}
}

type fakeQuestionBankGenerator struct{}

func (fakeQuestionBankGenerator) GenerateQuestionBank(ctx context.Context, req ReviewQuestionBankRequest) (ReviewQuestionBank, error) {
	return ReviewQuestionBank{
		TopicName: req.Topic.Name,
		Questions: []ReviewQuestion{
			{
				Question:              "示例面试问题：" + req.Topic.Name,
				ExamPoints:            []string{"LLM 考点：" + req.Topic.Name},
				Answer:                "LLM 标准答案：" + req.Topic.Name,
				ResumeBasedAnswer:     "结合简历回答：示例项目一沉淀 12 个示例场景。",
				Followups:             []string{"LLM 追问：你如何验证？"},
				RiskOrMissingEvidence: []string{"待补证据：补充项目原始材料。"},
				SourcePaths:           []string{req.SourceItem.Path, emptyIfBlank(req.Context.ResumePath), emptyIfBlank(req.Context.JDPath)},
			},
		},
	}, nil
}
