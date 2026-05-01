package career

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"happyagent/internal/app"
	"happyagent/internal/config"
	"happyagent/internal/store"
)

type stubCareerApp struct {
	session        store.SessionRecord
	runs           []store.RunRecord
	appendRequests []app.AppendTurnRequest
}

func (s *stubCareerApp) CreateSession(profileName string) (store.SessionRecord, error) {
	s.session.Profile = profileName
	return s.session, nil
}

func (s *stubCareerApp) AppendUserTurn(ctx context.Context, req app.AppendTurnRequest) (store.RunRecord, error) {
	s.appendRequests = append(s.appendRequests, req)
	if len(s.runs) == 0 {
		return store.RunRecord{ID: "run-1", SessionID: req.SessionID, Output: "ok"}, nil
	}
	index := len(s.appendRequests) - 1
	if index >= len(s.runs) {
		index = len(s.runs) - 1
	}
	return s.runs[index], nil
}

func TestRunInteractiveCreatesWorkspaceAndHandlesStatus(t *testing.T) {
	app := &stubCareerApp{
		session: store.SessionRecord{
			ID:        "session-career",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := RunInteractive(Dependencies{
		App:           app,
		Config:        config.Default(),
		Stdin:         strings.NewReader("/status\n/exit\n"),
		Stdout:        &stdout,
		Stderr:        &stderr,
		WorkspaceRoot: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("RunInteractive() error = %v", err)
	}
	output := stdout.String()
	for _, expected := range []string{"Career Copilot", "智能求职助手", "Workspace Status", "Session: session-career"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("output missing %q:\n%s", expected, output)
		}
	}
}

func TestRunInteractiveIngestsJDWithoutModelTurn(t *testing.T) {
	app := &stubCareerApp{
		session: store.SessionRecord{
			ID:        "session-career",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
	}
	workspaceRoot := t.TempDir()
	var stdout bytes.Buffer

	input := "# AI Agent Backend Engineer 岗位。岗位职责：负责 Agent runtime、RAG 和 MCP 工具平台。任职要求：熟悉 Go backend、LLM、observability 和 tool calling。"
	err := RunInteractive(Dependencies{
		App:           app,
		Config:        config.Default(),
		Stdin:         strings.NewReader(input + "\n/exit\n"),
		Stdout:        &stdout,
		Stderr:        &bytes.Buffer{},
		WorkspaceRoot: workspaceRoot,
	})
	if err != nil {
		t.Fatalf("RunInteractive() error = %v", err)
	}
	if len(app.appendRequests) != 0 {
		t.Fatalf("JD ingestion should not call model, got %d calls", len(app.appendRequests))
	}
	ws, err := OpenWorkspace(workspaceRoot, time.Now())
	if err != nil {
		t.Fatalf("OpenWorkspace() error = %v", err)
	}
	_, index, err := ws.Status()
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if len(index.Items) != 1 || index.Items[0].Type != "jd" {
		t.Fatalf("expected one jd item, got %+v", index.Items)
	}
	if !strings.Contains(stdout.String(), "已识别并归档为 JD") {
		t.Fatalf("missing JD confirmation:\n%s", stdout.String())
	}
}

func TestRunInteractiveAddJDCommandSupportsMultilineInput(t *testing.T) {
	app := &stubCareerApp{
		session: store.SessionRecord{
			ID:        "session-career",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
	}
	workspaceRoot := t.TempDir()
	var stdout bytes.Buffer

	err := RunInteractive(Dependencies{
		App:    app,
		Config: config.Default(),
		Stdin: strings.NewReader(`/add jd
# AI Agent Backend Engineer
岗位职责：负责 Agent runtime、RAG 和 MCP 工具平台。
任职要求：熟悉 Go backend、LLM、observability 和 tool calling。
.
/exit
`),
		Stdout:        &stdout,
		Stderr:        &bytes.Buffer{},
		WorkspaceRoot: workspaceRoot,
	})
	if err != nil {
		t.Fatalf("RunInteractive() error = %v", err)
	}
	ws, err := OpenWorkspace(workspaceRoot, time.Now())
	if err != nil {
		t.Fatalf("OpenWorkspace() error = %v", err)
	}
	_, index, err := ws.Status()
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if len(index.Items) != 1 || index.Items[0].Title != "AI Agent Backend Engineer" {
		t.Fatalf("expected multiline jd item, got %+v", index.Items)
	}
	if !strings.Contains(stdout.String(), "已添加 JD") {
		t.Fatalf("missing add confirmation:\n%s", stdout.String())
	}
}

func TestRunInteractiveAddResumeCommandArchivesResume(t *testing.T) {
	app := &stubCareerApp{
		session: store.SessionRecord{
			ID:        "session-career",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
	}
	workspaceRoot := t.TempDir()

	err := RunInteractive(Dependencies{
		App:           app,
		Config:        config.Default(),
		Stdin:         strings.NewReader("/add resume 简历：工作经历 Go 后端，项目经历 Agent runtime。\n/exit\n"),
		Stdout:        &bytes.Buffer{},
		Stderr:        &bytes.Buffer{},
		WorkspaceRoot: workspaceRoot,
	})
	if err != nil {
		t.Fatalf("RunInteractive() error = %v", err)
	}
	ws, err := OpenWorkspace(workspaceRoot, time.Now())
	if err != nil {
		t.Fatalf("OpenWorkspace() error = %v", err)
	}
	meta, index, err := ws.Status()
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if meta.CurrentResume == "" {
		t.Fatalf("expected current resume to be updated")
	}
	if len(index.Items) != 1 || index.Items[0].Type != WorkspaceTypeResume {
		t.Fatalf("expected one resume item, got %+v", index.Items)
	}
}

func TestRunInteractiveAddJDFileCommandArchivesOriginalFile(t *testing.T) {
	app := &stubCareerApp{
		session: store.SessionRecord{
			ID:        "session-career",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
	}
	workspaceRoot := t.TempDir()
	sourcePath := filepath.Join(t.TempDir(), "ai.txt")
	content := "# AI Agent Backend Engineer\n岗位职责：负责 Agent runtime、RAG 和 MCP。\n任职要求：熟悉 Go、LLM、observability。\n"
	if err := os.WriteFile(sourcePath, []byte(content), 0o644); err != nil {
		t.Fatalf("write source file: %v", err)
	}

	err := RunInteractive(Dependencies{
		App:           app,
		Config:        config.Default(),
		Stdin:         strings.NewReader("/add jd " + sourcePath + "\n/exit\n"),
		Stdout:        &bytes.Buffer{},
		Stderr:        &bytes.Buffer{},
		WorkspaceRoot: workspaceRoot,
	})
	if err != nil {
		t.Fatalf("RunInteractive() error = %v", err)
	}
	ws, err := OpenWorkspace(workspaceRoot, time.Now())
	if err != nil {
		t.Fatalf("OpenWorkspace() error = %v", err)
	}
	_, index, err := ws.Status()
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if len(index.Items) != 1 {
		t.Fatalf("expected one archived file, got %+v", index.Items)
	}
	item := index.Items[0]
	if item.Type != WorkspaceTypeJD {
		t.Fatalf("expected jd item, got %+v", item)
	}
	if !strings.HasSuffix(item.Path, "/extracted.md") {
		t.Fatalf("expected extracted markdown path, got %q", item.Path)
	}
	metadataPath := filepath.Join(workspaceRoot, "jds", item.ID, "metadata.json")
	data, err := os.ReadFile(metadataPath)
	if err != nil {
		t.Fatalf("read metadata: %v", err)
	}
	if !strings.Contains(string(data), `"original":`) || !strings.Contains(string(data), `"source":`) {
		t.Fatalf("metadata missing original/source paths: %s", data)
	}
}

func TestRunInteractiveRecordsClassificationEventForModelTurn(t *testing.T) {
	app := &stubCareerApp{
		session: store.SessionRecord{
			ID:        "session-career",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		runs: []store.RunRecord{{ID: "run-1", SessionID: "session-career", Output: "ok"}},
	}

	err := RunInteractive(Dependencies{
		App:           app,
		Config:        config.Default(),
		Stdin:         strings.NewReader("帮我优化简历\n/exit\n"),
		Stdout:        &bytes.Buffer{},
		Stderr:        &bytes.Buffer{},
		WorkspaceRoot: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("RunInteractive() error = %v", err)
	}
	if len(app.appendRequests) != 1 {
		t.Fatalf("expected one model turn, got %d", len(app.appendRequests))
	}
	events := app.appendRequests[0].Events
	if len(events) != 1 {
		t.Fatalf("expected one classification event, got %+v", events)
	}
	if events[0].Type != "career_input_classified" || events[0].Data["type"] != WorkspaceTypeResume {
		t.Fatalf("unexpected classification event: %+v", events[0])
	}
}

func TestRunInteractiveAutoArchivesReferencedJDFileBeforeModelTurn(t *testing.T) {
	app := &stubCareerApp{
		session: store.SessionRecord{
			ID:        "session-career",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		runs: []store.RunRecord{{ID: "run-1", SessionID: "session-career", Output: "ok"}},
	}
	workspaceRoot := t.TempDir()
	sourcePath := filepath.Join(t.TempDir(), "ai.txt")
	content := "# AI Agent Backend Engineer\n岗位职责：负责 Agent runtime、RAG 和 MCP。\n任职要求：熟悉 Go、LLM、observability。\n"
	if err := os.WriteFile(sourcePath, []byte(content), 0o644); err != nil {
		t.Fatalf("write source file: %v", err)
	}
	var stdout bytes.Buffer

	err := RunInteractive(Dependencies{
		App:           app,
		Config:        config.Default(),
		Stdin:         strings.NewReader("我在 " + sourcePath + " 放了一个jd，帮我分析一下并记录下来\n/exit\n"),
		Stdout:        &stdout,
		Stderr:        &bytes.Buffer{},
		WorkspaceRoot: workspaceRoot,
	})
	if err != nil {
		t.Fatalf("RunInteractive() error = %v", err)
	}
	if len(app.appendRequests) != 1 {
		t.Fatalf("expected one model turn, got %d", len(app.appendRequests))
	}
	if !strings.Contains(app.appendRequests[0].Input, "Auto-saved workspace assets") {
		t.Fatalf("expected auto-saved prompt context, got:\n%s", app.appendRequests[0].Input)
	}
	if !strings.Contains(stdout.String(), "已自动归档 JD") {
		t.Fatalf("expected auto-archive confirmation, got:\n%s", stdout.String())
	}
	ws, err := OpenWorkspace(workspaceRoot, time.Now())
	if err != nil {
		t.Fatalf("OpenWorkspace() error = %v", err)
	}
	_, index, err := ws.Status()
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if len(index.Items) < 1 || index.Items[0].Type != WorkspaceTypeJD {
		t.Fatalf("expected jd item, got %+v", index.Items)
	}
}

func TestRunInteractiveAutoArchivesReferencedDOCXResumeWithoutSavingPromptText(t *testing.T) {
	app := &stubCareerApp{
		session: store.SessionRecord{
			ID:        "session-career",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		runs: []store.RunRecord{{ID: "run-1", SessionID: "session-career", Output: "resume advice"}},
	}
	workspaceRoot := t.TempDir()
	var stdout bytes.Buffer

	resumePath, err := filepath.Abs(filepath.Join("testdata", "resume-sample.docx"))
	if err != nil {
		t.Fatalf("filepath.Abs() error = %v", err)
	}
	err = RunInteractive(Dependencies{
		App:           app,
		Config:        config.Default(),
		Stdin:         strings.NewReader("我在 " + resumePath + " 放了我的简历，你看看内容并给我建议\n/exit\n"),
		Stdout:        &stdout,
		Stderr:        &bytes.Buffer{},
		WorkspaceRoot: workspaceRoot,
	})
	if err != nil {
		t.Fatalf("RunInteractive() error = %v", err)
	}
	ws, err := OpenWorkspace(workspaceRoot, time.Now())
	if err != nil {
		t.Fatalf("OpenWorkspace() error = %v", err)
	}
	meta, index, err := ws.Status()
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if len(index.Items) == 0 || index.Items[0].Type != WorkspaceTypeResume {
		t.Fatalf("expected archived resume item, got %+v", index.Items)
	}
	if meta.CurrentResume == "" {
		t.Fatalf("expected current resume to be updated")
	}
	data, err := os.ReadFile(filepath.Join(workspaceRoot, filepath.FromSlash(index.Items[0].Path)))
	if err != nil {
		t.Fatalf("read extracted resume: %v", err)
	}
	text := string(data)
	if strings.Contains(text, "你看看内容并给我建议") {
		t.Fatalf("expected extracted content instead of user prompt: %s", text)
	}
	if !strings.Contains(text, "Sample Backend Engineer") {
		t.Fatalf("expected docx content in extracted text: %s", text)
	}
	if !strings.Contains(stdout.String(), "已保存本轮分析报告") {
		t.Fatalf("expected report confirmation, got:\n%s", stdout.String())
	}
}

func TestRunInteractiveAutoArchivesResumeFromReferencedDirectory(t *testing.T) {
	app := &stubCareerApp{
		session: store.SessionRecord{
			ID:        "session-career",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		runs: []store.RunRecord{{ID: "run-1", SessionID: "session-career", Output: "resume advice"}},
	}
	workspaceRoot := t.TempDir()
	var stdout bytes.Buffer
	testDir, err := filepath.Abs("testdata")
	if err != nil {
		t.Fatalf("filepath.Abs() error = %v", err)
	}

	err = RunInteractive(Dependencies{
		App:           app,
		Config:        config.Default(),
		Stdin:         strings.NewReader("我在" + testDir + "目录里存了我的简历,一个docx文件,你帮我分析一下,并且记录下来\n/exit\n"),
		Stdout:        &stdout,
		Stderr:        &bytes.Buffer{},
		WorkspaceRoot: workspaceRoot,
	})
	if err != nil {
		t.Fatalf("RunInteractive() error = %v", err)
	}
	if len(app.appendRequests) != 1 {
		t.Fatalf("expected one model turn, got %d", len(app.appendRequests))
	}
	if !strings.Contains(app.appendRequests[0].Input, "Auto-saved workspace assets") {
		t.Fatalf("expected auto-saved prompt context, got:\n%s", app.appendRequests[0].Input)
	}
	ws, err := OpenWorkspace(workspaceRoot, time.Now())
	if err != nil {
		t.Fatalf("OpenWorkspace() error = %v", err)
	}
	meta, index, err := ws.Status()
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if meta.CurrentResume == "" {
		t.Fatalf("expected current resume to be updated")
	}
	var resumeFound bool
	for _, item := range index.Items {
		if item.Type != WorkspaceTypeResume {
			continue
		}
		resumeFound = true
		data, readErr := os.ReadFile(filepath.Join(workspaceRoot, filepath.FromSlash(item.Path)))
		if readErr != nil {
			t.Fatalf("read archived resume: %v", readErr)
		}
		if !strings.Contains(string(data), "Sample Backend Engineer") {
			t.Fatalf("expected extracted docx content, got:\n%s", string(data))
		}
	}
	if !resumeFound {
		t.Fatalf("expected archived resume item, got %+v", index.Items)
	}
	if !strings.Contains(stdout.String(), "已自动归档 简历") {
		t.Fatalf("expected auto-archive confirmation, got:\n%s", stdout.String())
	}
}

func TestRunInteractiveAutoArchivesResumeAndJDInSameTurn(t *testing.T) {
	app := &stubCareerApp{
		session: store.SessionRecord{
			ID:        "session-career",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		runs: []store.RunRecord{{ID: "run-1", SessionID: "session-career", Output: "match analysis"}},
	}
	workspaceRoot := t.TempDir()

	resumePath, err := filepath.Abs(filepath.Join("testdata", "resume-sample.docx"))
	if err != nil {
		t.Fatalf("filepath.Abs() error = %v", err)
	}
	jdPath, err := filepath.Abs(filepath.Join("testdata", "jd-sample.txt"))
	if err != nil {
		t.Fatalf("filepath.Abs() error = %v", err)
	}
	err = RunInteractive(Dependencies{
		App:           app,
		Config:        config.Default(),
		Stdin:         strings.NewReader("我在 " + resumePath + " 放了简历，" + jdPath + " 是目标 JD，帮我分析匹配度并给优化建议\n/exit\n"),
		Stdout:        &bytes.Buffer{},
		Stderr:        &bytes.Buffer{},
		WorkspaceRoot: workspaceRoot,
	})
	if err != nil {
		t.Fatalf("RunInteractive() error = %v", err)
	}
	ws, err := OpenWorkspace(workspaceRoot, time.Now())
	if err != nil {
		t.Fatalf("OpenWorkspace() error = %v", err)
	}
	meta, index, err := ws.Status()
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if meta.CurrentResume == "" || meta.ActiveJD == "" {
		t.Fatalf("expected resume and jd pointers, got %+v", meta)
	}
	if len(index.Items) < 3 {
		t.Fatalf("expected archived assets and report, got %+v", index.Items)
	}
	var hasResume, hasJD, hasReport bool
	for _, item := range index.Items {
		switch item.Type {
		case WorkspaceTypeResume:
			hasResume = true
		case WorkspaceTypeJD:
			hasJD = true
		case "report":
			hasReport = true
		}
	}
	if !hasResume || !hasJD || !hasReport {
		t.Fatalf("expected resume, jd, and report items, got %+v", index.Items)
	}
}

func TestRunInteractiveExportCommand(t *testing.T) {
	app := &stubCareerApp{
		session: store.SessionRecord{
			ID:        "session-career",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
	}
	workspaceRoot := t.TempDir()
	var stdout bytes.Buffer

	err := RunInteractive(Dependencies{
		App:           app,
		Config:        config.Default(),
		Stdin:         strings.NewReader("/export review-material\n/exit\n"),
		Stdout:        &stdout,
		Stderr:        &bytes.Buffer{},
		WorkspaceRoot: workspaceRoot,
	})
	if err != nil {
		t.Fatalf("RunInteractive() error = %v", err)
	}
	output := stdout.String()
	if !strings.Contains(output, "已导出 Review Material") {
		t.Fatalf("unexpected output:\n%s", output)
	}
	ws, err := OpenWorkspace(workspaceRoot, time.Now())
	if err != nil {
		t.Fatalf("OpenWorkspace() error = %v", err)
	}
	_, index, err := ws.Status()
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	var hasExport bool
	for _, item := range index.Items {
		if item.Type == "export" {
			hasExport = true
		}
	}
	if !hasExport {
		t.Fatalf("expected export item: %+v", index.Items)
	}
}

func TestRunInteractiveNaturalLanguageCommandHelpDoesNotCallModel(t *testing.T) {
	app := &stubCareerApp{
		session: store.SessionRecord{
			ID:        "session-career",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
	}
	var stdout bytes.Buffer

	err := RunInteractive(Dependencies{
		App:           app,
		Config:        config.Default(),
		Stdin:         strings.NewReader("你有哪些可用的命令?\n/exit\n"),
		Stdout:        &stdout,
		Stderr:        &bytes.Buffer{},
		WorkspaceRoot: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("RunInteractive() error = %v", err)
	}
	if len(app.appendRequests) != 0 {
		t.Fatalf("command help should not call model, got %d calls", len(app.appendRequests))
	}
	output := stdout.String()
	for _, expected := range []string{"/help", "/status", "/export", "/add", "/exit"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("help output missing %q:\n%s", expected, output)
		}
	}
	if strings.Contains(output, "/search") || strings.Contains(output, "docx_read") || strings.Contains(output, "file_read") || strings.Contains(output, "web_search") {
		t.Fatalf("help output should not list internal tools:\n%s", output)
	}
}

func TestBuildInteractivePromptIncludesFactBoundary(t *testing.T) {
	prompt := BuildInteractivePrompt("帮我优化简历", ClassifyInput("帮我优化简历"))
	for _, expected := range []string{"Career Copilot continuous conversation workspace", "Input classification", "User input", "帮我优化简历"} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("prompt missing %q:\n%s", expected, prompt)
		}
	}
	for _, unexpected := range []string{"Do not invent user experience", "Do not present internal tools", "Respond as an intelligent job-search assistant"} {
		if strings.Contains(prompt, unexpected) {
			t.Fatalf("prompt should not contain system instruction %q:\n%s", unexpected, prompt)
		}
	}
}

func TestBuildInteractivePromptWithAutoSavedUsesWorkspaceRootRelativePaths(t *testing.T) {
	prompt := BuildInteractivePromptWithAutoSaved(
		"帮我分析一下",
		ClassifyInput("帮我分析一下简历"),
		[]WorkspaceItem{{
			Type:  WorkspaceTypeResume,
			Title: "resume-sample",
			Path:  "resumes/versions/resume-20260501-143151-resume-sample/extracted.md",
		}},
		nil,
		WorkspaceMetadata{
			CurrentResume: "resumes/versions/resume-20260501-143151-resume-sample/extracted.md",
		},
		true,
		".happyagent/career",
	)
	for _, expected := range []string{
		".happyagent/career/resumes/versions/resume-20260501-143151-resume-sample/extracted.md",
		"do not ask the user to choose storage paths",
		"Treat DOCX/PDF extraction as already handled by the application layer",
	} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("prompt missing %q:\n%s", expected, prompt)
		}
	}
}
