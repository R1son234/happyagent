package career

import (
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
		filepath.Join("experiences", "面经总览.md"),
		filepath.Join("prepare", "项目专项总览.md"),
		filepath.Join("jd", "JD 汇总.md"),
	} {
		data, err := os.ReadFile(filepath.Join(ws.Root, rel))
		if err != nil {
			t.Fatalf("expected review library entry %s: %v", rel, err)
		}
		if !strings.Contains(string(data), "title:") {
			t.Fatalf("expected frontmatter in %s:\n%s", rel, data)
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

	backend, err := ws.ArchivePublicInterviewExperience("后端公开面经：一面问 Go 并发、MySQL 索引、Redis 缓存和项目难点。", now)
	if err != nil {
		t.Fatalf("ArchivePublicInterviewExperience(backend) error = %v", err)
	}
	product, err := ws.ArchivePublicInterviewExperience("产品经理公开面经：追问需求分析、用户研究、PRD、增长策略和项目复盘。", now.Add(time.Minute))
	if err != nil {
		t.Fatalf("ArchivePublicInterviewExperience(product) error = %v", err)
	}

	if backend.Domain.Slug == product.Domain.Slug {
		t.Fatalf("expected different dynamic domains, got backend=%+v product=%+v", backend.Domain, product.Domain)
	}
	for _, rel := range []string{
		filepath.Join("experiences", backend.Domain.Slug, backend.Domain.Name+" 面经资料包.md"),
		filepath.Join("experiences", product.Domain.Slug, product.Domain.Name+" 面经资料包.md"),
	} {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Fatalf("expected generated package %s: %v", rel, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "my-interviews", "市场营销", "面经来源与复习清单.md")); err == nil {
		t.Fatalf("unexpected hard-coded marketing interview checklist")
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
求职意向：市场营销实习生
Wonderlab
市场营销实习生
负责多平台投放、KOL 合作、数据监控和复盘，累计曝光预计超500万。
抖音个人IP账号运营
账号定位、对标分析、选题标题、视频剪辑，累计粉丝8.3W。`, now); err != nil {
		t.Fatalf("AddMaterial(resume) error = %v", err)
	}
	if _, err := ws.AddMaterial(WorkspaceTypeJD, `# 市场营销 / 新媒体运营实习生 JD
负责小红书、抖音等平台内容策划，跟进 KOL/KOC 合作，监控数据并复盘。`, now); err != nil {
		t.Fatalf("AddMaterial(jd) error = %v", err)
	}
	if _, err := ws.AddMaterial(WorkspaceTypeExperiences, `# 市场营销 / 新媒体运营实习生面经整理
- 给你 5 分钟看一个账号，你会如何判断问题并提出优化建议？
- 上一段实习中如何做出爆款？
- 你如何理解运营？
- 你的个人优势是什么？请结合经历说明。`, now); err != nil {
		t.Fatalf("AddMaterial(experience) error = %v", err)
	}
	result, err := ws.GenerateReviewLibrary(now)
	if err != nil {
		t.Fatalf("GenerateReviewLibrary() error = %v", err)
	}
	if strings.Contains(strings.Join(result.Paths, "\n"), "domain-") {
		t.Fatalf("generated paths should be readable, got %+v", result.Paths)
	}
	bankPath := filepath.Join(root, "experiences", "marketing-new-media-operations-intern", "业务与策略题库.md")
	bank, err := os.ReadFile(bankPath)
	if err != nil {
		t.Fatalf("expected question bank %s: %v\npaths=%+v", bankPath, err, result.Paths)
	}
	for _, expected := range []string{"给你 5 分钟看一个账号", "Wonderlab", "8.3W", "证据边界"} {
		if !strings.Contains(string(bank), expected) {
			t.Fatalf("question bank missing %q:\n%s", expected, bank)
		}
	}
	rolePath := filepath.Join(root, "my-interviews", "市场营销新媒体运营实习生JD", "01-临阵抗拷打主文档.md")
	if _, err := os.Stat(rolePath); err != nil {
		t.Fatalf("expected role cramming doc %s: %v\npaths=%+v", rolePath, err, result.Paths)
	}
	projectPath := filepath.Join(root, "prepare", "wonderlab-interview-qa.md")
	project, err := os.ReadFile(projectPath)
	if err != nil {
		t.Fatalf("expected project QA %s: %v\npaths=%+v", projectPath, err, result.Paths)
	}
	if !strings.Contains(string(project), "累计曝光预计超500万") {
		t.Fatalf("project QA should include resume evidence:\n%s", project)
	}
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
	result, err := ws.GenerateReviewLibrary(now)
	if err != nil {
		t.Fatalf("GenerateReviewLibrary() error = %v", err)
	}
	if len(result.Paths) == 0 {
		t.Fatalf("expected generated review library paths")
	}
	if _, err := os.Stat(filepath.Join(root, "experiences", "general", "通用 面经资料包.md")); err != nil {
		t.Fatalf("expected general review package: %v", err)
	}
}
