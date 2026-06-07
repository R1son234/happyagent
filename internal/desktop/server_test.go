package desktop

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"happyagent/internal/app"
	"happyagent/internal/career"
	"happyagent/internal/config"
	"happyagent/internal/store"
)

func TestIsWithin(t *testing.T) {
	root := t.TempDir()
	if !isWithin(root, root) {
		t.Fatalf("root should be within itself")
	}
	if !isWithin(root, root+"/child/file.md") {
		t.Fatalf("child path should be within root")
	}
	if isWithin(root, root+"/../outside.md") {
		t.Fatalf("parent escape should not be within root")
	}
}

func TestPreviewKind(t *testing.T) {
	tests := map[string]string{
		".md":   "markdown",
		".txt":  "text",
		".json": "json",
		".pdf":  "pdf",
		".docx": "docx",
		".bin":  "unsupported",
	}
	for ext, want := range tests {
		if got := previewKind(ext); got != want {
			t.Fatalf("previewKind(%q) = %q, want %q", ext, got, want)
		}
	}
}

func TestBuildTreeShowsUserVisibleMaterialDirectories(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, career.WorkspaceDirResume, "resume.md"), "# Resume\n")
	mustWriteFile(t, filepath.Join(root, career.WorkspaceDirJD, "jd.md"), "# JD\n")
	mustWriteFile(t, filepath.Join(root, career.WorkspaceDirExperiences, "experience.md"), "# Experience\n")
	mustWriteFile(t, filepath.Join(root, career.WorkspaceDirResume, "metadata.json"), "{}\n")
	mustWriteFile(t, filepath.Join(root, career.WorkspaceDirResume, "extracted.md"), "internal\n")
	mustWriteFile(t, filepath.Join(root, career.WorkspaceInternalDir, "index.json"), "{}\n")
	mustWriteFile(t, filepath.Join(root, career.WorkspaceDirResume, "item-dir", "metadata.json"), "{}\n")
	mustWriteFile(t, filepath.Join(root, career.WorkspaceDirResume, "item-dir", "extracted.md"), "internal\n")

	materialDirs := map[string]bool{
		career.WorkspaceDirResume:      true,
		career.WorkspaceDirJD:          true,
		career.WorkspaceDirExperiences: true,
	}
	tree, err := buildTree(root, root, 5, materialDirs)
	if err != nil {
		t.Fatalf("buildTree() error = %v", err)
	}

	resumeDir, ok := findChild(tree, career.WorkspaceDirResume)
	if !ok || resumeDir.Kind != "directory" {
		t.Fatalf("expected visible resume directory, got %+v", tree.Children)
	}
	if _, ok := findChild(tree, career.WorkspaceDirJD); !ok {
		t.Fatalf("expected visible JD directory, got %+v", tree.Children)
	}
	if _, ok := findChild(tree, career.WorkspaceDirExperiences); !ok {
		t.Fatalf("expected visible experiences directory, got %+v", tree.Children)
	}
	if _, ok := findChild(resumeDir, "resume.md"); !ok {
		t.Fatalf("expected user-visible resume file, got %+v", resumeDir.Children)
	}
	for _, hidden := range []string{"metadata.json", "extracted.md", "item-dir"} {
		if _, ok := findChild(resumeDir, hidden); ok {
			t.Fatalf("expected %s to be hidden, got %+v", hidden, resumeDir.Children)
		}
	}
	if _, ok := findChild(tree, ".happyagent"); ok {
		t.Fatalf("expected internal .happyagent directory to be hidden")
	}
}

func TestHandleFileUploadOnlySavesToInbox(t *testing.T) {
	root := t.TempDir()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("files", "sample.md")
	if err != nil {
		t.Fatalf("CreateFormFile() error = %v", err)
	}
	if _, err := part.Write([]byte("# Sample\n公开面经：一面问示例问题。")); err != nil {
		t.Fatalf("Write multipart file error = %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close multipart writer error = %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/files/upload", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	server := &Server{workspaceRoot: root}
	server.handleFileUpload(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		SavedPaths []string               `json:"saved_paths"`
		Items      []career.WorkspaceItem `json:"items"`
		Warnings   []string               `json:"warnings"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("parse response: %v", err)
	}
	if len(resp.SavedPaths) != 1 || len(resp.Items) != 0 || len(resp.Warnings) != 0 {
		t.Fatalf("unexpected upload response: %+v", resp)
	}
	if _, err := os.Stat(filepath.Join(root, "inbox", "sample.md")); err != nil {
		t.Fatalf("expected uploaded file in inbox: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, career.WorkspaceInternalDir, "index.json")); !os.IsNotExist(err) {
		t.Fatalf("upload should not ingest or create workspace index, stat err=%v", err)
	}
}

func TestHandleFileImportOnlySavesToInbox(t *testing.T) {
	root := t.TempDir()
	sourceDir := t.TempDir()
	sourcePath := filepath.Join(sourceDir, "experience.md")
	mustWriteFile(t, sourcePath, "# 示例面经\n\n一面问示例问题。")

	body, err := json.Marshal(map[string]any{"paths": []string{sourcePath}})
	if err != nil {
		t.Fatalf("Marshal request error = %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/files/import", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server := &Server{workspaceRoot: root}
	server.handleFileImport(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		SavedPaths     []string               `json:"saved_paths"`
		Items          []career.WorkspaceItem `json:"items"`
		GeneratedPaths []string               `json:"generated_paths"`
		Warnings       []string               `json:"warnings"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("parse response: %v", err)
	}
	if len(resp.SavedPaths) != 1 || len(resp.Items) != 0 || len(resp.GeneratedPaths) != 0 || len(resp.Warnings) != 0 {
		t.Fatalf("unexpected import response: %+v", resp)
	}
	if _, err := os.Stat(filepath.Join(root, "inbox", "experience.md")); err != nil {
		t.Fatalf("expected imported file in inbox: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, career.WorkspaceInternalDir, "index.json")); !os.IsNotExist(err) {
		t.Fatalf("import should not ingest or create workspace index, stat err=%v", err)
	}
}

func TestHandleInboxListsFilesAndPendingState(t *testing.T) {
	now := time.Date(2026, 5, 24, 14, 0, 0, 0, time.UTC)
	root := t.TempDir()
	ws, err := career.OpenWorkspace(root, now)
	if err != nil {
		t.Fatalf("OpenWorkspace() error = %v", err)
	}
	mustWriteFile(t, filepath.Join(root, "inbox", "resume.md"), "# Resume\n")
	if err := ws.WriteInboxState(career.InboxState{Items: []career.PendingInboxItem{{
		ID:         "pending-resume",
		SourcePath: "inbox/resume.md",
		Status:     career.InboxItemStatusPending,
		Confidence: career.ConfidenceMedium,
	}}}); err != nil {
		t.Fatalf("WriteInboxState() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/inbox", nil)
	rec := httptest.NewRecorder()
	server := &Server{workspaceRoot: root}
	server.handleInbox(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp career.InboxView
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("parse response: %v", err)
	}
	if len(resp.Files) != 1 || resp.Files[0].Path != "inbox/resume.md" || resp.Files[0].Status != career.InboxItemStatusPending {
		t.Fatalf("unexpected inbox files: %+v", resp.Files)
	}
	if len(resp.PendingItems) != 1 || resp.Counts[career.InboxItemStatusPending] != 1 {
		t.Fatalf("unexpected pending state response: %+v", resp)
	}
}

func TestHandleInboxClassifyUsesCopilotService(t *testing.T) {
	now := time.Date(2026, 5, 24, 14, 30, 0, 0, time.UTC)
	root := t.TempDir()
	if _, err := career.OpenWorkspace(root, now); err != nil {
		t.Fatalf("OpenWorkspace() error = %v", err)
	}
	mustWriteFile(t, filepath.Join(root, "inbox", "jd.md"), "# 示例 JD\n\n岗位职责：负责示例系统。\n")
	body := bytes.NewReader([]byte(`{"paths":["inbox/jd.md"]}`))
	req := httptest.NewRequest(http.MethodPost, "/api/inbox/classify", body)
	rec := httptest.NewRecorder()
	server := &Server{
		workspaceRoot: root,
		careerService: &career.CopilotService{
			WorkspaceRoot: root,
			Now:           func() time.Time { return now },
			TaskRunner: fakeDesktopStructuredTaskRunner{
				output: `{"files":[{"source_path":"inbox/jd.md","source_hash":"sha256:fake","material_type":"jd","confidence":"high","reason":"包含岗位职责。","source_excerpt":"岗位职责：负责示例系统。","destination":"岗位明细","needs_user_confirmation":false,"questions_for_user":[]}]}`,
			},
		},
	}
	server.handleInboxClassify(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp career.ClassifyInboxResult
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("parse response: %v", err)
	}
	if len(resp.Items) != 1 || resp.Items[0].Status != career.InboxItemStatusConfirmed {
		t.Fatalf("expected confirmed classification, got %+v", resp.Items)
	}
	if _, err := os.Stat(filepath.Join(root, "inbox", "jd.md")); !os.IsNotExist(err) {
		t.Fatalf("expected inbox source removed, err=%v", err)
	}
	ws, err := career.OpenWorkspace(root, now)
	if err != nil {
		t.Fatalf("OpenWorkspace() error = %v", err)
	}
	_, index, err := ws.Status()
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if len(index.Items) != 1 || index.Items[0].Type != career.WorkspaceTypeJD {
		t.Fatalf("expected one JD item, got %+v", index.Items)
	}
}

func TestRunEventBrokerReplaysHistoryAndStreamsLiveEvents(t *testing.T) {
	broker := newRunEventBroker()
	broker.Publish("stream-1", RunEvent{Type: "status_text", Message: "started"})

	backlog, ch, unsubscribe := broker.Subscribe("stream-1")
	defer unsubscribe()

	if len(backlog) != 1 || backlog[0].Message != "started" {
		t.Fatalf("unexpected backlog: %+v", backlog)
	}

	broker.Publish("stream-1", RunEvent{Type: "status_text", Message: "running"})
	select {
	case event := <-ch:
		if event.Message != "running" {
			t.Fatalf("unexpected live event: %+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for live event")
	}
}

func TestHandleInboxConfirmUsesCopilotService(t *testing.T) {
	now := time.Date(2026, 5, 24, 15, 0, 0, 0, time.UTC)
	root := t.TempDir()
	ws, err := career.OpenWorkspace(root, now)
	if err != nil {
		t.Fatalf("OpenWorkspace() error = %v", err)
	}
	mustWriteFile(t, filepath.Join(root, "inbox", "note.md"), "# 复习笔记\n\n示例内容。")
	if err := ws.WriteInboxState(career.InboxState{Items: []career.PendingInboxItem{{
		ID:                    "pending-note",
		SourcePath:            "inbox/note.md",
		SourceHash:            "sha256:abc",
		OriginalName:          "note.md",
		MaterialType:          "unknown",
		Confidence:            career.ConfidenceMedium,
		NeedsUserConfirmation: true,
		Status:                career.InboxItemStatusPending,
	}}}); err != nil {
		t.Fatalf("WriteInboxState() error = %v", err)
	}
	body := bytes.NewReader([]byte(`{"id":"pending-note","material_type":"review_note","destination":"复习资料库"}`))
	req := httptest.NewRequest(http.MethodPost, "/api/inbox/confirm", body)
	rec := httptest.NewRecorder()
	server := &Server{
		workspaceRoot: root,
		careerService: &career.CopilotService{
			WorkspaceRoot: root,
			Now:           func() time.Time { return now },
		},
	}
	server.handleInboxConfirm(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp career.ConfirmMaterialResult
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("parse response: %v", err)
	}
	if resp.Item.Type != career.WorkspaceTypeRecord || resp.RecordPath == "" || !resp.RemovedInbox {
		t.Fatalf("unexpected confirm response: %+v", resp)
	}
	if _, err := os.Stat(filepath.Join(root, "inbox", "note.md")); !os.IsNotExist(err) {
		t.Fatalf("expected inbox source removed, err=%v", err)
	}
}

func TestNormalizeConfigJSONFormatsValidJSON(t *testing.T) {
	formatted, err := normalizeConfigJSON(`{"llm":{"model":"test"}}`)
	if err != nil {
		t.Fatalf("normalizeConfigJSON() error = %v", err)
	}
	got := string(formatted)
	if !strings.Contains(got, "\n  \"llm\": {") {
		t.Fatalf("expected indented JSON, got:\n%s", got)
	}
	if !strings.HasSuffix(got, "\n") {
		t.Fatalf("expected trailing newline, got %q", got)
	}
}

func TestNormalizeConfigJSONRejectsInvalidJSON(t *testing.T) {
	if _, err := normalizeConfigJSON(`{"llm":`); err == nil {
		t.Fatalf("expected invalid JSON to be rejected")
	}
}

func TestWorkspacePathAllowsAbsoluteLogFile(t *testing.T) {
	root := t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	logPath := filepath.Join(cwd, "logs", "test-preview-log.md")
	t.Cleanup(func() { _ = os.Remove(logPath) })
	mustWriteFile(t, logPath, "# Test Log\n")
	server := &Server{workspaceRoot: root}
	abs, err := server.workspacePath(logPath)
	if err != nil {
		t.Fatalf("workspacePath() error = %v", err)
	}
	if abs != logPath {
		t.Fatalf("workspacePath() = %q, want %q", abs, logPath)
	}
}

func TestHandleChatRunReturnsGeneratedPathsAndLogPath(t *testing.T) {
	now := time.Date(2026, 5, 25, 10, 0, 0, 0, time.UTC)
	root := t.TempDir()
	ws, err := career.OpenWorkspace(root, now)
	if err != nil {
		t.Fatalf("OpenWorkspace() error = %v", err)
	}
	if _, err := ws.AddMaterial(career.WorkspaceTypeJD, "# 示例 JD\n\n岗位职责：负责示例 Agent 系统。", now); err != nil {
		t.Fatalf("AddMaterial(jd) error = %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/chat/runs", bytes.NewReader([]byte(`{"session_id":"session-test","profile":"career-copilot","input":"帮我生成面试准备材料"}`)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server := &Server{
		cfg:           config.Default(),
		workspaceRoot: root,
		careerService: &career.CopilotService{
			WorkspaceRoot: root,
			App: fakeDesktopCareerApplication{
				sessionID: "session-test",
				output:    "# 面试准备材料\n\n## 复习计划\n- 先看 JD 关键词。\n",
			},
			Now: func() time.Time { return now },
		},
	}
	server.cfg.Engine.RunTimeoutSeconds = 60
	server.handleChatRun(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		SessionID      string   `json:"session_id"`
		GeneratedPaths []string `json:"generated_paths"`
		PrimaryPath    string   `json:"primary_path"`
		LogPath        string   `json:"log_path"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("parse response: %v", err)
	}
	if resp.SessionID != "session-test" {
		t.Fatalf("unexpected session id: %+v", resp)
	}
	if resp.PrimaryPath == "" || len(resp.GeneratedPaths) == 0 {
		t.Fatalf("expected generated paths, got %+v", resp)
	}
	if resp.LogPath == "" {
		t.Fatalf("expected log path, got %+v", resp)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(resp.PrimaryPath))); err != nil {
		t.Fatalf("expected generated primary file: %v", err)
	}
	if _, err := os.Stat(resp.LogPath); err != nil {
		t.Fatalf("expected chat log file: %v", err)
	}
}

func mustWriteFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}

func findChild(node fileNode, name string) (fileNode, bool) {
	for _, child := range node.Children {
		if child.Name == name {
			return child, true
		}
	}
	return fileNode{}, false
}

type fakeDesktopStructuredTaskRunner struct {
	output string
}

func (r fakeDesktopStructuredTaskRunner) RunStructuredTask(ctx context.Context, req career.StructuredTaskRequest) (career.StructuredTaskResult, error) {
	_ = ctx
	_ = req
	return career.StructuredTaskResult{
		Output:      r.output,
		Model:       "fake-model",
		RunID:       "fake-run",
		SessionID:   "fake-session",
		GeneratedAt: time.Date(2026, 5, 24, 14, 30, 0, 0, time.UTC),
	}, nil
}

type fakeDesktopCareerApplication struct {
	sessionID string
	output    string
}

func (f fakeDesktopCareerApplication) CreateSession(profileName string) (store.SessionRecord, error) {
	_ = profileName
	return store.SessionRecord{ID: f.sessionID}, nil
}

func (f fakeDesktopCareerApplication) AppendUserTurn(ctx context.Context, req app.AppendTurnRequest) (store.RunRecord, error) {
	_ = ctx
	output := f.output
	if strings.Contains(req.Input, "<career_user_input_semantic_decision>") {
		output = `{"intent":"interview_brief","confidence":"high","reason":"test semantic decision","should_scan_inbox":false,"should_save_user_input":false,"user_input_material_type":"unknown","user_input_destination":"","needs_user_confirmation":false,"questions_for_user":[],"referenced_files":[],"requested_outputs":[{"kind":"interview-brief","title":"面试准备材料","reason":"test semantic decision","required_sources":[]}],"required_state":[],"risk_flags":[]}`
	}
	return store.RunRecord{
		ID:        "run-test",
		SessionID: firstNonEmptyTest(req.SessionID, f.sessionID, "session-test"),
		Profile:   req.ProfileName,
		Input:     req.Input,
		Output:    output,
	}, nil
}

func firstNonEmptyTest(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
