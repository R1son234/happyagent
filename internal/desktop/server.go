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
	"happyagent/internal/tools"
)

const defaultProfile = "career-copilot"

type Server struct {
	cfg           config.Config
	app           *app.Application
	workspaceRoot string
	staticDir     string
	mux           *http.ServeMux

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
	writeJSON(w, http.StatusOK, map[string]any{
		"root":   s.workspaceRoot,
		"meta":   meta,
		"index":  index,
		"counts": counts,
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
		case "resume", "jd", "experiences", "prepare", "my-interviews", "outputs":
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
	ws, err := career.OpenWorkspace(s.workspaceRoot, time.Now())
	if err != nil {
		writeError(w, err)
		return
	}
	var items []career.WorkspaceItem
	var warnings []string
	for _, path := range req.Paths {
		result, err := career.IngestFile(r.Context(), ws, career.IngestRequest{
			Path:      path,
			HintType:  req.HintType,
			UserInput: filepath.Base(path),
			Now:       time.Now(),
		})
		if result.Item.ID != "" {
			items = append(items, result.Item)
		}
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %v", path, err))
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":    items,
		"warnings": warnings,
	})
}

func (s *Server) handleFileUpload(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		writeError(w, err)
		return
	}
	ws, err := career.OpenWorkspace(s.workspaceRoot, time.Now())
	if err != nil {
		writeError(w, err)
		return
	}
	inboxRoot := filepath.Join(s.workspaceRoot, "inbox")
	if err := os.MkdirAll(inboxRoot, 0o755); err != nil {
		writeError(w, err)
		return
	}
	var savedPaths []string
	var items []career.WorkspaceItem
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
			result, err := career.IngestFile(r.Context(), ws, career.IngestRequest{
				Path:      dstPath,
				UserInput: filepath.Base(dstPath),
				Now:       time.Now(),
			})
			if result.Item.ID != "" {
				items = append(items, result.Item)
			}
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("%s: %v", header.Filename, err))
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"saved_paths": savedPaths,
		"items":       items,
		"warnings":    warnings,
	})
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
	var events []map[string]any
	logSession, logPath := initDesktopRunLog(cfg, profile, sessionID, req.Input)
	if logSession != nil {
		defer func() {
			runlog.Disable()
			_ = logSession.Close()
		}()
	}
	record, err := s.app.AppendUserTurn(ctx, app.AppendTurnRequest{
		SessionID:     sessionID,
		ProfileName:   profile,
		Input:         req.Input,
		SystemPrompt:  cfg.Engine.SystemPrompt,
		ApprovedTools: cfg.Tools.ApprovedTools,
		OnStepStart: func(stepIndex int) {
			events = append(events, map[string]any{"type": "step_started", "step": stepIndex})
		},
		OnToolCallStart: func(toolName string) {
			events = append(events, map[string]any{"type": "tool_started", "tool": toolName})
		},
		OnToolCallEnd: func(toolName string, succeeded bool) {
			events = append(events, map[string]any{"type": "tool_finished", "tool": toolName, "succeeded": succeeded})
		},
		OnTodosUpdated: func(todos []tools.TodoItem) {
			events = append(events, map[string]any{"type": "todos_updated", "todos": todos})
		},
	})
	if err != nil {
		runlog.Section("Error", err.Error())
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error":      err.Error(),
			"record":     record,
			"events":     events,
			"session_id": sessionID,
			"log_path":   logPath,
		})
		return
	}
	runlog.Section("Final Output", record.Output)
	writeJSON(w, http.StatusOK, map[string]any{
		"session_id": sessionID,
		"record":     record,
		"events":     events,
		"log_path":   logPath,
	})
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
