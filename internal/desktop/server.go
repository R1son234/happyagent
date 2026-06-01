package desktop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"happyagent/internal/app"
	"happyagent/internal/career"
	"happyagent/internal/config"
	"happyagent/internal/observe"
	"happyagent/internal/runlog"
	"happyagent/internal/runtime"
	"happyagent/internal/store"
)

const defaultProfile = "career-copilot"

type Server struct {
	cfg           config.Config
	app           *app.Application
	careerService *career.CopilotService
	workspaceRoot string
	staticDir     string
	mux           *http.ServeMux
	eventBroker   *runEventBroker

	mu       sync.Mutex
	sessions map[string]string
}

type Options struct {
	Config        config.Config
	Runtime       *runtime.Runtime
	WorkspaceRoot string
	StaticDir     string
}

func NewServer(opts Options) (*Server, error) {
	if opts.Runtime == nil {
		return nil, fmt.Errorf("runtime must not be nil")
	}
	workspaceRoot := strings.TrimSpace(opts.WorkspaceRoot)
	if workspaceRoot == "" {
		workspaceRoot = career.DefaultWorkspaceRoot
	}
	if _, err := career.OpenWorkspace(workspaceRoot, time.Now()); err != nil {
		return nil, err
	}
	application, err := buildApplication(opts.Runtime)
	if err != nil {
		return nil, err
	}
	s := &Server{
		cfg:           opts.Config,
		app:           application,
		workspaceRoot: workspaceRoot,
		staticDir:     opts.StaticDir,
		mux:           http.NewServeMux(),
		eventBroker:   newRunEventBroker(),
		sessions:      map[string]string{},
	}
	s.routes()
	return s, nil
}

func (s *Server) Handler() http.Handler {
	return requestLogger(s.mux)
}

func (s *Server) ListenAndServe(ctx context.Context, addr string) (string, error) {
	if strings.TrimSpace(addr) == "" {
		addr = "127.0.0.1:0"
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return "", err
	}
	server := &http.Server{Handler: s.Handler()}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	go func() {
		if err := server.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintf(os.Stderr, "desktop server: %v\n", err)
		}
	}()
	return "http://" + ln.Addr().String(), nil
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /api/health", s.handleHealth)
	s.mux.HandleFunc("GET /api/workspace/status", s.handleWorkspaceStatus)
	s.mux.HandleFunc("GET /api/files/tree", s.handleFileTree)
	s.mux.HandleFunc("GET /api/files/preview", s.handleFilePreview)
	s.mux.HandleFunc("POST /api/files/import", s.handleFileImport)
	s.mux.HandleFunc("POST /api/files/upload", s.handleFileUpload)
	s.mux.HandleFunc("GET /api/inbox", s.handleInbox)
	s.mux.HandleFunc("POST /api/inbox/classify", s.handleInboxClassify)
	s.mux.HandleFunc("POST /api/inbox/confirm", s.handleInboxConfirm)
	s.mux.HandleFunc("GET /api/runs/events", s.handleRunEvents)
	s.mux.HandleFunc("POST /api/workspace/target-jd", s.handleSelectTargetJD)
	s.mux.HandleFunc("POST /api/jd/split", s.handleJDSplit)
	s.mux.HandleFunc("POST /api/review-library/generate", s.handleReviewLibraryGenerate)
	s.mux.HandleFunc("POST /api/project-pack/generate", s.handleProjectPackGenerate)
	s.mux.HandleFunc("POST /api/battle-pack/generate", s.handleBattlePackGenerate)
	s.mux.HandleFunc("POST /api/interview-review/extract", s.handleInterviewReviewExtract)
	s.mux.HandleFunc("POST /api/workspace/cleanup-generated", s.handleCleanupGeneratedArtifacts)
	s.mux.HandleFunc("POST /api/workspace/rebuild", s.handleRebuildGeneratedArtifacts)
	s.mux.HandleFunc("GET /api/diagnostics", s.handleDiagnostics)
	s.mux.HandleFunc("GET /api/graph", s.handleGraph)
	s.mux.HandleFunc("POST /api/chat/sessions", s.handleCreateChatSession)
	s.mux.HandleFunc("POST /api/chat/runs", s.handleChatRun)
	s.mux.HandleFunc("GET /api/settings", s.handleSettings)
	s.mux.HandleFunc("PUT /api/settings", s.handleUpdateSettings)
	s.mux.HandleFunc("/", s.handleStatic)
}

func buildApplication(rt *runtime.Runtime) (*app.Application, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	dataStore, err := store.New(filepath.Join(cwd, ".happyagent", "store"))
	if err != nil {
		return nil, err
	}
	return app.New(rt, dataStore, observe.NewMetrics())
}

func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
	})
}

type streamProgressReporter struct {
	streamID string
	broker   *runEventBroker
	taskName string
}

func (r streamProgressReporter) publish(event RunEvent) {
	if r.broker == nil || strings.TrimSpace(r.streamID) == "" {
		return
	}
	if strings.TrimSpace(event.TaskName) == "" {
		event.TaskName = r.taskName
	}
	r.broker.Publish(r.streamID, event)
}

func (r streamProgressReporter) Status(message string) {
	if strings.TrimSpace(message) == "" {
		return
	}
	r.publish(RunEvent{Type: "status_text", Message: message})
}

func (r streamProgressReporter) StepStart(stepIndex int) {
	r.publish(RunEvent{
		Type:      "step_started",
		StepIndex: stepIndex,
		Message:   fmt.Sprintf("模型第 %d 轮处理中", stepIndex),
	})
}

func (r streamProgressReporter) ToolStart(toolName string, arguments []byte) {
	r.publish(RunEvent{
		Type:     "tool_started",
		ToolName: toolName,
		Message:  humanToolMessage(toolName, arguments, true),
	})
}

func (r streamProgressReporter) ToolEnd(toolName string, arguments []byte, succeeded bool) {
	status := "completed"
	message := ""
	if !succeeded {
		status = "failed"
		message = humanToolMessage(toolName, arguments, false)
	}
	r.publish(RunEvent{
		Type:     "tool_finished",
		ToolName: toolName,
		Status:   status,
		Message:  message,
	})
}

func humanToolMessage(toolName string, arguments []byte, starting bool) string {
	switch toolName {
	case "file_read":
		var input struct {
			Path string `json:"path"`
		}
		_ = json.Unmarshal(arguments, &input)
		path := strings.TrimSpace(input.Path)
		if path == "" {
			if starting {
				return "正在读取资料"
			}
			return "读取资料失败"
		}
		if starting {
			return "正在读取资料：" + path
		}
		return "读取资料失败：" + path
	case "agent_task":
		var input struct {
			Name string `json:"name"`
			Role string `json:"role"`
		}
		_ = json.Unmarshal(arguments, &input)
		label := firstNonEmpty(strings.TrimSpace(input.Role), strings.TrimSpace(input.Name), "子分析任务")
		if starting {
			return "正在委派子分析任务：" + label
		}
		return "子分析任务失败：" + label
	default:
		if starting {
			return "正在执行：" + toolName
		}
		return toolName + " 执行失败"
	}
}

func (s *Server) publishRunEvent(streamID string, event RunEvent) {
	if s.eventBroker == nil || strings.TrimSpace(streamID) == "" {
		return
	}
	s.eventBroker.Publish(streamID, event)
}

func (s *Server) finalizeRunStream(streamID string) {
	if s.eventBroker == nil || strings.TrimSpace(streamID) == "" {
		return
	}
	s.eventBroker.Finalize(streamID)
}

func (s *Server) copilotService() *career.CopilotService {
	if s.careerService != nil {
		return s.careerService
	}
	s.mu.Lock()
	cfg := s.cfg
	s.mu.Unlock()
	return &career.CopilotService{
		WorkspaceRoot: s.workspaceRoot,
		App:           s.app,
		Config:        cfg,
	}
}

func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	if s.staticDir == "" {
		http.NotFound(w, r)
		return
	}
	path := filepath.Clean(strings.TrimPrefix(r.URL.Path, "/"))
	if path == "." || path == "" {
		path = "index.html"
	}
	abs := filepath.Join(s.staticDir, path)
	if !isWithin(s.staticDir, abs) {
		http.Error(w, "path escapes static root", http.StatusBadRequest)
		return
	}
	info, err := os.Stat(abs)
	if err != nil || info.IsDir() {
		abs = filepath.Join(s.staticDir, "index.html")
	}
	http.ServeFile(w, r, abs)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	cfg := s.cfg
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":               true,
		"workspace_root":   s.workspaceRoot,
		"model":            cfg.LLM.Model,
		"model_configured": strings.TrimSpace(cfg.LLM.APIKey) != "",
	})
}

func (s *Server) handleWorkspaceStatus(w http.ResponseWriter, r *http.Request) {
	ws, err := career.OpenWorkspace(s.workspaceRoot, time.Now())
	if err != nil {
		writeError(w, err)
		return
	}
	meta, index, err := ws.Status()
	if err != nil {
		writeError(w, err)
		return
	}
	counts := map[string]int{}
	for _, item := range index.Items {
		counts[item.Type]++
	}
	inboxState, _ := ws.ReadInboxState()
	pendingCount := 0
	for _, item := range inboxState.Items {
		if item.Status == career.InboxItemStatusPending {
			pendingCount++
		}
	}
	generatedState, _ := ws.ReadGeneratedState()
	var staleItems []map[string]any
	for _, item := range generatedState.Items {
		if item.Stale {
			staleItems = append(staleItems, map[string]any{
				"path":   item.Path,
				"kind":   item.Kind,
				"reason": item.StaleReason,
			})
		}
	}
	runSummaries, _ := ws.ReadRunSummaryState()
	currentTargetJD := resolveCurrentTargetJD(meta, index)
	writeJSON(w, http.StatusOK, map[string]any{
		"root":               s.workspaceRoot,
		"meta":               meta,
		"index":              index,
		"counts":             counts,
		"pending_count":      pendingCount,
		"current_target_jd":  currentTargetJD,
		"stale_items":        staleItems,
		"latest_run_summary": firstRunSummary(runSummaries.Items),
	})
}

type fileNode struct {
	Name     string     `json:"name"`
	Path     string     `json:"path"`
	Kind     string     `json:"kind"`
	Size     int64      `json:"size,omitempty"`
	Modified time.Time  `json:"modified,omitempty"`
	Children []fileNode `json:"children,omitempty"`
}

func (s *Server) handleFileTree(w http.ResponseWriter, r *http.Request) {
	root, err := filepath.Abs(s.workspaceRoot)
	if err != nil {
		writeError(w, err)
		return
	}
	ws, err := career.OpenWorkspace(s.workspaceRoot, time.Now())
	if err != nil {
		writeError(w, err)
		return
	}
	guide, err := ws.LoadGuide()
	if err != nil {
		writeError(w, err)
		return
	}
	materialDirs := map[string]bool{}
	for _, rule := range guide.Directories {
		if rule.SaveMode == career.SaveModeMaterialDir {
			materialDirs[rule.Path] = true
		}
	}
	node, err := buildTree(root, root, 5, materialDirs)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, node)
}

func buildTree(root, abs string, depth int, materialDirs map[string]bool) (fileNode, error) {
	info, err := os.Stat(abs)
	if err != nil {
		return fileNode{}, err
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return fileNode{}, err
	}
	if rel == "." {
		rel = ""
	}
	node := fileNode{
		Name:     info.Name(),
		Path:     filepath.ToSlash(rel),
		Kind:     "file",
		Size:     info.Size(),
		Modified: info.ModTime(),
	}
	if info.IsDir() {
		node.Kind = "directory"
		if rel == "" {
			node.Name = filepath.Base(root)
		}
		if depth <= 0 {
			return node, nil
		}
		entries, err := os.ReadDir(abs)
		if err != nil {
			return fileNode{}, err
		}
		sort.Slice(entries, func(i, j int) bool {
			if entries[i].IsDir() != entries[j].IsDir() {
				return entries[i].IsDir()
			}
			return strings.ToLower(entries[i].Name()) < strings.ToLower(entries[j].Name())
		})
		isMaterialDir := materialDirs[filepath.ToSlash(rel)]
		for _, entry := range entries {
			if shouldHideWorkspaceEntry(rel, entry.Name()) {
				continue
			}
			if isMaterialDir && entry.IsDir() && isItemDir(filepath.Join(abs, entry.Name())) {
				continue
			}
			child, err := buildTree(root, filepath.Join(abs, entry.Name()), depth-1, materialDirs)
			if err != nil {
				continue
			}
			node.Children = append(node.Children, child)
		}
	}
	return node, nil
}

func shouldHideWorkspaceEntry(parentRel string, name string) bool {
	if strings.HasPrefix(name, ".") {
		return true
	}
	lower := strings.ToLower(name)
	switch lower {
	case "record", "logs", "workspace.json", "index.json", strings.ToLower(career.WorkspaceGuideFileName), "metadata.json", "extracted.md":
		return true
	}
	if strings.HasPrefix(lower, "source.") || strings.HasSuffix(lower, ".trace.json") {
		return true
	}
	parentRel = filepath.ToSlash(parentRel)
	if parentRel == "" {
		switch name {
		case "resume", "jd", "experiences", "prepare", "project", "my-interviews", "outputs":
			return true
		}
	}
	return false
}

func isItemDir(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	hasExtracted, hasMetadata := false, false
	for _, e := range entries {
		base := strings.ToLower(e.Name())
		if strings.HasPrefix(base, "extracted.") {
			hasExtracted = true
		}
		if strings.HasPrefix(base, "metadata.") {
			hasMetadata = true
		}
	}
	return hasExtracted && hasMetadata
}

func (s *Server) handleFilePreview(w http.ResponseWriter, r *http.Request) {
	rel := r.URL.Query().Get("path")
	abs, err := s.workspacePath(rel)
	if err != nil {
		writeError(w, err)
		return
	}
	info, err := os.Stat(abs)
	if err != nil {
		writeError(w, err)
		return
	}
	if info.IsDir() {
		writeJSON(w, http.StatusOK, map[string]any{
			"path": rel,
			"kind": "directory",
		})
		return
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		writeError(w, err)
		return
	}
	const maxPreview = 120 * 1024
	truncated := false
	if len(data) > maxPreview {
		data = data[:maxPreview]
		truncated = true
	}
	ext := strings.ToLower(filepath.Ext(abs))
	kind := previewKind(ext)
	content := ""
	if kind == "text" || kind == "markdown" || kind == "json" {
		content = string(data)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"path":      filepath.ToSlash(rel),
		"name":      filepath.Base(abs),
		"kind":      kind,
		"size":      info.Size(),
		"modified":  info.ModTime(),
		"content":   content,
		"truncated": truncated,
	})
}

func previewKind(ext string) string {
	switch ext {
	case ".md", ".markdown":
		return "markdown"
	case ".txt", ".log":
		return "text"
	case ".json":
		return "json"
	case ".png", ".jpg", ".jpeg", ".gif", ".webp":
		return "image"
	case ".pdf":
		return "pdf"
	case ".docx":
		return "docx"
	default:
		return "unsupported"
	}
}

func (s *Server) handleFileImport(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Paths    []string `json:"paths"`
		HintType string   `json:"hint_type"`
	}
	if err := readJSON(r.Body, &req); err != nil {
		writeError(w, err)
		return
	}
	inboxRoot := filepath.Join(s.workspaceRoot, "inbox")
	if err := os.MkdirAll(inboxRoot, 0o755); err != nil {
		writeError(w, err)
		return
	}
	var savedPaths []string
	var warnings []string
	for _, path := range req.Paths {
		absPath, err := filepath.Abs(strings.TrimSpace(path))
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %v", path, err))
			continue
		}
		info, err := os.Stat(absPath)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %v", path, err))
			continue
		}
		if info.IsDir() {
			warnings = append(warnings, fmt.Sprintf("%s: is a directory, expected a file", path))
			continue
		}
		name := safeUploadName(filepath.Base(absPath))
		dstPath := uniqueInboxPath(inboxRoot, name)
		if err := copyFileToPath(absPath, dstPath); err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %v", path, err))
			continue
		}
		savedPaths = append(savedPaths, dstPath)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"saved_paths":     savedPaths,
		"items":           []career.WorkspaceItem{},
		"generated_paths": []string{},
		"warnings":        warnings,
	})
}

func (s *Server) handleFileUpload(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		writeError(w, err)
		return
	}
	inboxRoot := filepath.Join(s.workspaceRoot, "inbox")
	if err := os.MkdirAll(inboxRoot, 0o755); err != nil {
		writeError(w, err)
		return
	}
	var savedPaths []string
	var warnings []string
	for _, headers := range r.MultipartForm.File {
		for _, header := range headers {
			src, err := header.Open()
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("%s: %v", header.Filename, err))
				continue
			}
			name := safeUploadName(header.Filename)
			dstPath := uniqueInboxPath(inboxRoot, name)
			dst, err := os.OpenFile(dstPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
			if err != nil {
				src.Close()
				warnings = append(warnings, fmt.Sprintf("%s: %v", header.Filename, err))
				continue
			}
			_, copyErr := io.Copy(dst, io.LimitReader(src, 64<<20))
			closeErr := errors.Join(src.Close(), dst.Close())
			if copyErr != nil || closeErr != nil {
				warnings = append(warnings, fmt.Sprintf("%s: %v", header.Filename, errors.Join(copyErr, closeErr)))
				continue
			}
			savedPaths = append(savedPaths, dstPath)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"saved_paths": savedPaths,
		"items":       []career.WorkspaceItem{},
		"warnings":    warnings,
	})
}

func (s *Server) handleInbox(w http.ResponseWriter, r *http.Request) {
	view, err := s.copilotService().ListInbox(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) handleInboxClassify(w http.ResponseWriter, r *http.Request) {
	var req career.ClassifyInboxRequest
	if err := readJSON(r.Body, &req); err != nil {
		writeError(w, err)
		return
	}
	s.mu.Lock()
	cfg := s.cfg
	s.mu.Unlock()
	reporter := streamProgressReporter{streamID: strings.TrimSpace(req.StreamID), broker: s.eventBroker, taskName: "organize_inbox"}
	if req.StreamID != "" {
		s.publishRunEvent(req.StreamID, RunEvent{Type: "run_started", TaskName: "organize_inbox", Message: "开始整理 inbox"})
	}
	logSession, logPath := initDesktopRunLog(cfg, "organize_inbox", "inbox", "开始整理 inbox")
	ctx := career.WithProgressReporter(r.Context(), reporter)
	reporter.Status("正在扫描和抽取 inbox 资料")
	result, err := s.copilotService().ClassifyInbox(ctx, req)
	if err != nil {
		runlog.Section("Error", err.Error())
		if logSession != nil {
			runlog.Disable()
			_ = logSession.Close()
		}
		writeError(w, err)
		if req.StreamID != "" {
			s.publishRunEvent(req.StreamID, RunEvent{Type: "run_failed", TaskName: "organize_inbox", Message: err.Error(), Status: "failed"})
			s.finalizeRunStream(req.StreamID)
		}
		return
	}
	if logSession != nil {
		runlog.Disable()
		_ = logSession.Close()
	}
	ws, err := career.OpenWorkspace(s.workspaceRoot, time.Now())
	if err != nil {
		writeError(w, err)
		if req.StreamID != "" {
			s.publishRunEvent(req.StreamID, RunEvent{Type: "run_failed", TaskName: "organize_inbox", Message: err.Error(), Status: "failed"})
			s.finalizeRunStream(req.StreamID)
		}
		return
	}
	reporter.Status("分类完成，正在并发生成后续资料")
	generatedPaths, postProcessTasks, warnings := s.postProcessInboxClassification(ctx, ws, result.Items, strings.TrimSpace(req.StreamID))
	status := career.RunSummaryStatusSuccess
	if len(warnings) > 0 {
		status = career.RunSummaryStatusPartialSuccess
	}
	runSummaryPath, _ := ws.WriteRunSummary(career.RunSummaryRecord{
		TaskName:    "organize_inbox",
		Status:      status,
		CreatedAt:   time.Now(),
		InputPaths:  req.Paths,
		Generated:   generatedPaths,
		PrimaryPath: firstPath(generatedPaths),
		LogPath:     logPath,
		Warnings:    warnings,
		NextActions: []string{"确认待确认资料分类", "如目标岗位已明确，可选择 JD 后生成作战包"},
	})
	generatedPaths = uniquePaths(append(generatedPaths, runSummaryPath))
	writeJSON(w, http.StatusOK, map[string]any{
		"items":             result.Items,
		"warnings":          append(result.Warnings, warnings...),
		"generated_paths":   generatedPaths,
		"postprocess_tasks": postProcessTasks,
		"primary_path":      firstNonEmpty(runSummaryPath, firstPath(generatedPaths)),
		"log_path":          logPath,
		"run_summary_path":  runSummaryPath,
	})
	if req.StreamID != "" {
		s.publishRunEvent(req.StreamID, RunEvent{Type: "run_completed", TaskName: "organize_inbox", Message: "整理与后续生成已完成", Status: string(status)})
		s.finalizeRunStream(req.StreamID)
	}
}

func (s *Server) handleInboxConfirm(w http.ResponseWriter, r *http.Request) {
	var req career.ConfirmMaterialRequest
	if err := readJSON(r.Body, &req); err != nil {
		writeError(w, err)
		return
	}
	result, err := s.copilotService().ConfirmMaterialClassification(r.Context(), req)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleRunEvents(w http.ResponseWriter, r *http.Request) {
	streamID := strings.TrimSpace(r.URL.Query().Get("stream_id"))
	if streamID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "stream_id is required"})
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "streaming is unavailable"})
		return
	}
	backlog, ch, unsubscribe := s.eventBroker.Subscribe(streamID)
	defer unsubscribe()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	for _, event := range backlog {
		if err := writeSSEEvent(w, event); err != nil {
			return
		}
		flusher.Flush()
	}
	for {
		select {
		case <-r.Context().Done():
			return
		case event, ok := <-ch:
			if !ok {
				return
			}
			if err := writeSSEEvent(w, event); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (s *Server) handleSelectTargetJD(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if err := readJSON(r.Body, &req); err != nil {
		writeError(w, err)
		return
	}
	ws, err := career.OpenWorkspace(s.workspaceRoot, time.Now())
	if err != nil {
		writeError(w, err)
		return
	}
	if err := ws.SetActiveJD(req.Path, time.Now()); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleJDSplit(w http.ResponseWriter, r *http.Request) {
	var req career.SplitJDRequest
	if err := readJSON(r.Body, &req); err != nil {
		writeError(w, err)
		return
	}
	result, err := s.copilotService().SplitJobDescriptions(r.Context(), req)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleReviewLibraryGenerate(w http.ResponseWriter, r *http.Request) {
	var req career.GenerateReviewLibraryRequest
	if err := readJSON(r.Body, &req); err != nil {
		writeError(w, err)
		return
	}
	reporter := streamProgressReporter{streamID: strings.TrimSpace(req.StreamID), broker: s.eventBroker, taskName: "generate_review_library"}
	ctx := career.WithProgressReporter(r.Context(), reporter)
	if req.StreamID != "" {
		s.publishRunEvent(req.StreamID, RunEvent{Type: "run_started", TaskName: "generate_review_library", Message: "开始生成复习资料库"})
	}
	result, err := s.copilotService().GenerateReviewLibrary(ctx, req)
	if err != nil {
		writeError(w, err)
		if req.StreamID != "" {
			s.publishRunEvent(req.StreamID, RunEvent{Type: "run_failed", TaskName: "generate_review_library", Message: err.Error(), Status: "failed"})
			s.finalizeRunStream(req.StreamID)
		}
		return
	}
	writeJSON(w, http.StatusOK, result)
	if req.StreamID != "" {
		s.publishRunEvent(req.StreamID, RunEvent{Type: "run_completed", TaskName: "generate_review_library", Message: "复习资料库生成完成", Status: "success"})
		s.finalizeRunStream(req.StreamID)
	}
}

func (s *Server) handleProjectPackGenerate(w http.ResponseWriter, r *http.Request) {
	var req career.GenerateProjectPackRequest
	if err := readJSON(r.Body, &req); err != nil {
		writeError(w, err)
		return
	}
	reporter := streamProgressReporter{streamID: strings.TrimSpace(req.StreamID), broker: s.eventBroker, taskName: "generate_project_pack"}
	ctx := career.WithProgressReporter(r.Context(), reporter)
	if req.StreamID != "" {
		s.publishRunEvent(req.StreamID, RunEvent{Type: "run_started", TaskName: "generate_project_pack", Message: "开始生成项目专项"})
	}
	result, err := s.copilotService().GenerateProjectPack(ctx, req)
	if err != nil {
		writeError(w, err)
		if req.StreamID != "" {
			s.publishRunEvent(req.StreamID, RunEvent{Type: "run_failed", TaskName: "generate_project_pack", Message: err.Error(), Status: "failed"})
			s.finalizeRunStream(req.StreamID)
		}
		return
	}
	writeJSON(w, http.StatusOK, result)
	if req.StreamID != "" {
		s.publishRunEvent(req.StreamID, RunEvent{Type: "run_completed", TaskName: "generate_project_pack", Message: "项目专项生成完成", Status: "success"})
		s.finalizeRunStream(req.StreamID)
	}
}

func (s *Server) handleBattlePackGenerate(w http.ResponseWriter, r *http.Request) {
	var req career.GenerateBattlePackRequest
	if err := readJSON(r.Body, &req); err != nil {
		writeError(w, err)
		return
	}
	reporter := streamProgressReporter{streamID: strings.TrimSpace(req.StreamID), broker: s.eventBroker, taskName: "generate_battle_pack"}
	ctx := career.WithProgressReporter(r.Context(), reporter)
	if req.StreamID != "" {
		s.publishRunEvent(req.StreamID, RunEvent{Type: "run_started", TaskName: "generate_battle_pack", Message: "开始生成面试作战包"})
	}
	result, err := s.copilotService().GenerateBattlePack(ctx, req)
	if err != nil {
		writeError(w, err)
		if req.StreamID != "" {
			s.publishRunEvent(req.StreamID, RunEvent{Type: "run_failed", TaskName: "generate_battle_pack", Message: err.Error(), Status: "failed"})
			s.finalizeRunStream(req.StreamID)
		}
		return
	}
	writeJSON(w, http.StatusOK, result)
	if req.StreamID != "" {
		s.publishRunEvent(req.StreamID, RunEvent{Type: "run_completed", TaskName: "generate_battle_pack", Message: "面试作战包生成完成", Status: "success"})
		s.finalizeRunStream(req.StreamID)
	}
}

func (s *Server) handleInterviewReviewExtract(w http.ResponseWriter, r *http.Request) {
	var req career.ExtractInterviewReviewRequest
	if err := readJSON(r.Body, &req); err != nil {
		writeError(w, err)
		return
	}
	reporter := streamProgressReporter{streamID: strings.TrimSpace(req.StreamID), broker: s.eventBroker, taskName: "extract_interview_review"}
	ctx := career.WithProgressReporter(r.Context(), reporter)
	if req.StreamID != "" {
		s.publishRunEvent(req.StreamID, RunEvent{Type: "run_started", TaskName: "extract_interview_review", Message: "开始抽取真实面试复盘"})
	}
	result, err := s.copilotService().ExtractInterviewReview(ctx, req)
	if err != nil {
		writeError(w, err)
		if req.StreamID != "" {
			s.publishRunEvent(req.StreamID, RunEvent{Type: "run_failed", TaskName: "extract_interview_review", Message: err.Error(), Status: "failed"})
			s.finalizeRunStream(req.StreamID)
		}
		return
	}
	writeJSON(w, http.StatusOK, result)
	if req.StreamID != "" {
		s.publishRunEvent(req.StreamID, RunEvent{Type: "run_completed", TaskName: "extract_interview_review", Message: "真实面试复盘抽取完成", Status: "success"})
		s.finalizeRunStream(req.StreamID)
	}
}

func (s *Server) handleCleanupGeneratedArtifacts(w http.ResponseWriter, r *http.Request) {
	result, err := s.copilotService().CleanupGeneratedArtifacts(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleRebuildGeneratedArtifacts(w http.ResponseWriter, r *http.Request) {
	result, err := s.copilotService().RebuildGeneratedArtifacts(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleDiagnostics(w http.ResponseWriter, r *http.Request) {
	result, err := s.copilotService().ListDiagnostics(r.Context(), career.ListDiagnosticsRequest{Limit: 10})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

type postProcessTaskResult struct {
	TaskName       string   `json:"task_name"`
	Status         string   `json:"status"`
	Message        string   `json:"message,omitempty"`
	GeneratedPaths []string `json:"generated_paths,omitempty"`
}

func safeUploadName(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	if name == "." || name == "" {
		return fmt.Sprintf("upload-%d.txt", time.Now().UnixNano())
	}
	name = strings.Map(func(r rune) rune {
		switch {
		case r == '/' || r == '\\' || r == ':':
			return '-'
		case r < 32:
			return -1
		default:
			return r
		}
	}, name)
	return name
}

func uniqueInboxPath(inboxRoot string, name string) string {
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	candidate := filepath.Join(inboxRoot, name)
	for i := 1; ; i++ {
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
		candidate = filepath.Join(inboxRoot, fmt.Sprintf("%s-%d%s", base, i, ext))
	}
}

func copyFileToPath(srcPath string, dstPath string) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := os.OpenFile(dstPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(dst, src)
	closeErr := dst.Close()
	return errors.Join(copyErr, closeErr)
}

func (s *Server) handleGraph(w http.ResponseWriter, r *http.Request) {
	ws, err := career.OpenWorkspace(s.workspaceRoot, time.Now())
	if err != nil {
		writeError(w, err)
		return
	}
	_, index, err := ws.Status()
	if err != nil {
		writeError(w, err)
		return
	}
	nodes := make([]map[string]any, 0, len(index.Items))
	for _, item := range index.Items {
		nodes = append(nodes, map[string]any{
			"id":    item.ID,
			"label": item.Title,
			"type":  item.Type,
			"path":  item.Path,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"nodes": nodes, "edges": []any{}})
}

func (s *Server) handleCreateChatSession(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Profile string `json:"profile"`
	}
	_ = readJSON(r.Body, &req)
	profile := strings.TrimSpace(req.Profile)
	if profile == "" {
		profile = defaultProfile
	}
	session, err := s.app.CreateSession(profile)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, session)
}

func (s *Server) handleChatRun(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SessionID string `json:"session_id"`
		Profile   string `json:"profile"`
		Input     string `json:"input"`
		StreamID  string `json:"stream_id"`
	}
	if err := readJSON(r.Body, &req); err != nil {
		writeError(w, err)
		return
	}
	profile := strings.TrimSpace(req.Profile)
	if profile == "" {
		profile = defaultProfile
	}
	sessionID := strings.TrimSpace(req.SessionID)
	if sessionID == "" {
		session, err := s.app.CreateSession(profile)
		if err != nil {
			writeError(w, err)
			return
		}
		sessionID = session.ID
	}
	s.mu.Lock()
	cfg := s.cfg
	s.mu.Unlock()
	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(cfg.Engine.RunTimeoutSeconds)*time.Second)
	defer cancel()
	reporter := streamProgressReporter{streamID: strings.TrimSpace(req.StreamID), broker: s.eventBroker, taskName: "chat_generate"}
	ctx = career.WithProgressReporter(ctx, reporter)
	if req.StreamID != "" {
		s.publishRunEvent(req.StreamID, RunEvent{Type: "run_started", TaskName: "chat_generate", Message: "开始处理请求"})
	}
	logSession, logPath := initDesktopRunLog(cfg, profile, sessionID, req.Input)
	if logSession != nil {
		defer func() {
			runlog.Disable()
			_ = logSession.Close()
		}()
	}
	chat, err := s.copilotService().RunChatTurn(ctx, career.ChatTurnRequest{
		SessionID: sessionID,
		Input:     req.Input,
	})
	if err != nil {
		runlog.Section("Error", err.Error())
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error":      err.Error(),
			"session_id": sessionID,
			"log_path":   logPath,
		})
		if req.StreamID != "" {
			s.publishRunEvent(req.StreamID, RunEvent{Type: "run_failed", TaskName: "chat_generate", Message: err.Error(), Status: "failed"})
			s.finalizeRunStream(req.StreamID)
		}
		return
	}
	runlog.Section("Final Output", chat.Output)
	runSummaryPath := chat.RunSummaryPath
	if runSummaryPath == "" {
		ws, wsErr := career.OpenWorkspace(s.workspaceRoot, time.Now())
		if wsErr == nil {
			runSummaryPath, _ = ws.WriteRunSummary(career.RunSummaryRecord{
				TaskName:    "chat_generate",
				Status:      career.RunSummaryStatusSuccess,
				CreatedAt:   time.Now(),
				Generated:   chat.GeneratedPaths,
				PrimaryPath: chat.PrimaryPath,
				LogPath:     logPath,
			})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"session_id":       chat.SessionID,
		"record":           map[string]any{"output": chat.Output},
		"generated_paths":  uniquePaths(append(chat.GeneratedPaths, runSummaryPath)),
		"primary_path":     firstNonEmpty(runSummaryPath, chat.PrimaryPath),
		"log_path":         logPath,
		"run_summary_path": runSummaryPath,
	})
	if req.StreamID != "" {
		s.publishRunEvent(req.StreamID, RunEvent{Type: "run_completed", TaskName: "chat_generate", Message: "处理完成", Status: "success"})
		s.finalizeRunStream(req.StreamID)
	}
}

func initDesktopRunLog(cfg config.Config, profile string, sessionID string, input string) (*runlog.Session, string) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, ""
	}
	session, err := runlog.NewSession(cwd)
	if err != nil {
		return nil, ""
	}
	session.Enable()
	runlog.Section("Run Input", input)
	runlog.Linef("Model: `%s`", cfg.LLM.Model)
	runlog.Linef("Timeout: `%ds`", cfg.Engine.RunTimeoutSeconds)
	runlog.Linef("Profile: `%s`", profile)
	runlog.Linef("Session: `%s`", sessionID)
	runlog.Linef("")
	return session, session.Path()
}

func (s *Server) postProcessInboxClassification(ctx context.Context, ws *career.Workspace, items []career.PendingInboxItem, streamID string) ([]string, []postProcessTaskResult, []string) {
	hasJD := false
	hasResume := false
	hasExperience := false
	for _, item := range items {
		if item.Status != career.InboxItemStatusConfirmed {
			continue
		}
		switch item.MaterialType {
		case "jd":
			hasJD = true
		case "resume":
			hasResume = true
		case "public_interview_experience":
			hasExperience = true
		}
	}
	meta, err := ws.ReadMetadata()
	if err != nil {
		meta = career.WorkspaceMetadata{}
	}

	var (
		mu             sync.Mutex
		wg             sync.WaitGroup
		generatedPaths []string
		taskResults    []postProcessTaskResult
		warnings       []string
	)
	recordTask := func(result postProcessTaskResult) {
		mu.Lock()
		defer mu.Unlock()
		taskResults = append(taskResults, result)
		generatedPaths = append(generatedPaths, result.GeneratedPaths...)
		if result.Status == "failed" || result.Status == "skipped" {
			warnings = append(warnings, result.Message)
		}
	}
	runTask := func(taskName string, startMessage string, fn func(context.Context) (postProcessTaskResult, error)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if streamID != "" {
				s.publishRunEvent(streamID, RunEvent{
					Type:     "subtask_started",
					TaskName: "organize_inbox",
					Subtask:  taskName,
					Status:   "running",
					Message:  startMessage,
				})
			}
			taskCtx := career.WithProgressReporter(ctx, streamProgressReporter{
				streamID: streamID,
				broker:   s.eventBroker,
				taskName: taskName,
			})
			result, taskErr := fn(taskCtx)
			if taskErr != nil {
				result = postProcessTaskResult{
					TaskName: taskName,
					Status:   "failed",
					Message:  startMessage + "失败：" + taskErr.Error(),
				}
			}
			recordTask(result)
			if streamID != "" {
				s.publishRunEvent(streamID, RunEvent{
					Type:     "subtask_finished",
					TaskName: "organize_inbox",
					Subtask:  taskName,
					Status:   result.Status,
					Message:  result.Message,
				})
			}
		}()
	}

	if hasExperience {
		runTask("generate_review_library", "正在生成复习资料库", func(taskCtx context.Context) (postProcessTaskResult, error) {
			result, err := s.copilotService().GenerateReviewLibrary(taskCtx, career.GenerateReviewLibraryRequest{})
			if err != nil {
				return postProcessTaskResult{}, err
			}
			return postProcessTaskResult{
				TaskName:       "generate_review_library",
				Status:         "completed",
				Message:        "复习资料库生成完成",
				GeneratedPaths: result.GeneratedPaths,
			}, nil
		})
	} else {
		skipped := postProcessTaskResult{
			TaskName: "generate_review_library",
			Status:   "skipped",
			Message:  "复习资料库已跳过：未发现新归档的公开面经资料",
		}
		recordTask(skipped)
		if streamID != "" {
			s.publishRunEvent(streamID, RunEvent{Type: "subtask_finished", TaskName: "organize_inbox", Subtask: skipped.TaskName, Status: skipped.Status, Message: skipped.Message})
		}
	}

	if meta.CurrentResume != "" && (hasResume || hasJD || hasExperience) {
		projectName := "项目专项"
		if strings.TrimSpace(meta.ActiveProject) != "" {
			projectName = strings.TrimSuffix(filepath.Base(meta.ActiveProject), filepath.Ext(meta.ActiveProject))
		}
		runTask("generate_project_pack", "正在生成项目专项", func(taskCtx context.Context) (postProcessTaskResult, error) {
			result, err := s.copilotService().GenerateProjectPack(taskCtx, career.GenerateProjectPackRequest{
				ProjectName: projectName,
			})
			if err != nil {
				return postProcessTaskResult{}, err
			}
			return postProcessTaskResult{
				TaskName:       "generate_project_pack",
				Status:         "completed",
				Message:        "项目专项生成完成",
				GeneratedPaths: result.GeneratedPaths,
			}, nil
		})
	} else {
		skipped := postProcessTaskResult{
			TaskName: "generate_project_pack",
			Status:   "skipped",
			Message:  "项目专项已跳过：当前简历不可用",
		}
		recordTask(skipped)
		if streamID != "" {
			s.publishRunEvent(streamID, RunEvent{Type: "subtask_finished", TaskName: "organize_inbox", Subtask: skipped.TaskName, Status: skipped.Status, Message: skipped.Message})
		}
	}

	if meta.CurrentResume != "" && meta.ActiveJD != "" && (hasResume || hasJD || hasExperience) {
		runTask("generate_battle_pack", "正在生成面试作战包", func(taskCtx context.Context) (postProcessTaskResult, error) {
			result, err := s.copilotService().GenerateBattlePack(taskCtx, career.GenerateBattlePackRequest{
				JDPath:      meta.ActiveJD,
				SourcePaths: []string{meta.CurrentResume},
			})
			if err != nil {
				return postProcessTaskResult{}, err
			}
			return postProcessTaskResult{
				TaskName:       "generate_battle_pack",
				Status:         "completed",
				Message:        "面试作战包生成完成",
				GeneratedPaths: result.GeneratedPaths,
			}, nil
		})
	} else {
		message := "面试作战包已跳过：缺少当前简历或目标 JD"
		if meta.ActiveJD == "" {
			message = "面试作战包已跳过：尚未设置目标 JD"
		}
		skipped := postProcessTaskResult{
			TaskName: "generate_battle_pack",
			Status:   "skipped",
			Message:  message,
		}
		recordTask(skipped)
		if streamID != "" {
			s.publishRunEvent(streamID, RunEvent{Type: "subtask_finished", TaskName: "organize_inbox", Subtask: skipped.TaskName, Status: skipped.Status, Message: skipped.Message})
		}
	}

	wg.Wait()
	return uniquePaths(generatedPaths), taskResults, uniquePaths(warnings)
}

func resolveCurrentTargetJD(meta career.WorkspaceMetadata, index career.WorkspaceIndex) map[string]any {
	active := filepath.ToSlash(strings.TrimSpace(meta.ActiveJD))
	if active == "" {
		return map[string]any{}
	}
	for _, item := range index.Items {
		if item.Type != career.WorkspaceTypeJD {
			continue
		}
		if item.Metadata.Source != "" && filepath.ToSlash(item.Metadata.Source) == active {
			return map[string]any{"path": item.Path, "title": item.Title}
		}
	}
	return map[string]any{"path": active}
}

func firstRunSummary(items []career.RunSummaryRecord) map[string]any {
	if len(items) == 0 {
		return map[string]any{}
	}
	item := items[0]
	return map[string]any{
		"id":              item.ID,
		"task_name":       item.TaskName,
		"status":          item.Status,
		"created_at":      item.CreatedAt,
		"primary_path":    item.PrimaryPath,
		"generated_paths": item.Generated,
		"log_path":        item.LogPath,
		"warnings":        item.Warnings,
	}
}

func relWorkspaceLogPath(workspaceRoot string, path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	absWorkspace, err := filepath.Abs(workspaceRoot)
	if err == nil {
		if rel, relErr := filepath.Rel(absWorkspace, path); relErr == nil && !strings.HasPrefix(rel, "..") {
			return filepath.ToSlash(rel)
		}
	}
	absCwd, err := os.Getwd()
	if err == nil {
		if rel, relErr := filepath.Rel(absCwd, path); relErr == nil {
			return filepath.ToSlash(rel)
		}
	}
	return filepath.ToSlash(path)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func uniquePaths(paths []string) []string {
	seen := map[string]bool{}
	var result []string
	for _, path := range paths {
		path = filepath.ToSlash(strings.TrimSpace(path))
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		result = append(result, path)
	}
	sort.Strings(result)
	return result
}

func firstPath(paths []string) string {
	for _, path := range paths {
		if strings.TrimSpace(path) != "" {
			return path
		}
	}
	return ""
}

func writeSSEEvent(w http.ResponseWriter, event RunEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "data: %s\n\n", data)
	return err
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	content, err := os.ReadFile(config.ConfigPath())
	if err != nil {
		writeError(w, err)
		return
	}
	s.mu.Lock()
	cfg := s.cfg
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"path":             config.ConfigPath(),
		"content":          string(content),
		"model":            cfg.LLM.Model,
		"model_configured": cfg.LLM.APIKey != "",
		"workspace_root":   s.workspaceRoot,
		"tools_root":       cfg.Tools.RootDir,
		"write_enabled":    cfg.Tools.WriteEnabled,
		"delete_enabled":   cfg.Tools.DeleteEnabled,
		"restart_required": false,
	})
}

func (s *Server) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Content string `json:"content"`
	}
	if err := readJSON(r.Body, &req); err != nil {
		writeError(w, err)
		return
	}
	formatted, err := normalizeConfigJSON(req.Content)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	loaded, err := validateConfigContent(formatted)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	if err := os.WriteFile(config.ConfigPath(), formatted, 0o600); err != nil {
		writeError(w, err)
		return
	}
	career.PrepareConfig(&loaded, []string{"career"})
	s.mu.Lock()
	s.cfg = loaded
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"path":             config.ConfigPath(),
		"content":          string(formatted),
		"model":            loaded.LLM.Model,
		"model_configured": loaded.LLM.APIKey != "",
		"workspace_root":   s.workspaceRoot,
		"tools_root":       loaded.Tools.RootDir,
		"write_enabled":    loaded.Tools.WriteEnabled,
		"delete_enabled":   loaded.Tools.DeleteEnabled,
		"restart_required": true,
	})
}

func normalizeConfigJSON(content string) ([]byte, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, fmt.Errorf("settings content must not be empty")
	}
	var raw any
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return nil, fmt.Errorf("parse settings JSON: %w", err)
	}
	formatted, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("format settings JSON: %w", err)
	}
	return append(formatted, '\n'), nil
}

func validateConfigContent(content []byte) (config.Config, error) {
	tmp, err := os.CreateTemp(".", ".happyagent-settings-*.json")
	if err != nil {
		return config.Config{}, err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		return config.Config{}, err
	}
	if err := tmp.Close(); err != nil {
		return config.Config{}, err
	}
	return config.LoadFromPath(tmpPath)
}

func (s *Server) workspacePath(rel string) (string, error) {
	root, err := filepath.Abs(s.workspaceRoot)
	if err != nil {
		return "", err
	}
	rel = strings.TrimSpace(rel)
	if rel == "" {
		return root, nil
	}
	if filepath.IsAbs(rel) {
		abs := filepath.Clean(rel)
		cwd, cwdErr := os.Getwd()
		if cwdErr != nil {
			return "", cwdErr
		}
		logRoot := filepath.Join(cwd, "logs")
		if isWithin(root, abs) || isWithin(logRoot, abs) {
			return abs, nil
		}
		return "", fmt.Errorf("path escapes allowed roots")
	}
	rel = filepath.Clean(strings.TrimPrefix(rel, "/"))
	if rel == "." {
		rel = ""
	}
	abs := filepath.Join(root, rel)
	if !isWithin(root, abs) {
		return "", fmt.Errorf("path escapes workspace root")
	}
	return abs, nil
}

func isWithin(root, path string) bool {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(rootAbs, pathAbs)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != "..")
}

func readJSON(r io.Reader, dest any) error {
	if r == nil {
		return nil
	}
	dec := json.NewDecoder(io.LimitReader(r, 1<<20))
	if err := dec.Decode(dest); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	if errors.Is(err, fs.ErrNotExist) {
		status = http.StatusNotFound
	}
	writeJSON(w, status, map[string]any{"error": err.Error()})
}
