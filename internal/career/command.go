package career

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"happyagent/internal/app"
	"happyagent/internal/config"
	"happyagent/internal/observe"
	"happyagent/internal/report"
	"happyagent/internal/store"
	"happyagent/internal/terminal"
)

const ProfileName = "career-copilot"
const MinAnalyzeLoopSteps = 20
const MinAnalyzeTimeoutSeconds = 180

type Application interface {
	CreateSession(profileName string) (store.SessionRecord, error)
	AppendUserTurn(ctx context.Context, req app.AppendTurnRequest) (store.RunRecord, error)
}

type AnalyzeOptions struct {
	JDPath        string
	ResumePath    string
	TargetPath    string
	RepoPath      string
	MarkdownPath  string
	JSONPath      string
	TraceJSONPath string
	TemplatePath  string
}

type Dependencies struct {
	App           Application
	Config        config.Config
	Stdin         io.Reader
	Stdout        io.Writer
	Stderr        io.Writer
	WorkspaceRoot string
}

func RunCLI(ctx context.Context, args []string, deps Dependencies) error {
	if len(args) == 0 {
		return RunInteractive(deps)
	}
	switch args[0] {
	case "analyze":
		options, err := parseAnalyzeOptions(args[1:])
		if err != nil {
			return err
		}
		return Analyze(ctx, options, deps)
	case "rewrite-resume":
		return runReportTransform(args[1:], deps.Stdout, RenderResumeBullets)
	case "interview-brief":
		return runReportTransform(args[1:], deps.Stdout, RenderInterviewBrief)
	case "gap-plan":
		return runReportTransform(args[1:], deps.Stdout, RenderGapPlan)
	default:
		return fmt.Errorf("unknown career subcommand %q", args[0])
	}
}

func RunInteractive(deps Dependencies) error {
	if deps.App == nil {
		return fmt.Errorf("career interactive requires an application")
	}
	if deps.Stdin == nil {
		deps.Stdin = os.Stdin
	}
	if deps.Stdout == nil {
		deps.Stdout = os.Stdout
	}
	if deps.Stderr == nil {
		deps.Stderr = os.Stderr
	}
	workspaceRoot := deps.WorkspaceRoot
	if strings.TrimSpace(workspaceRoot) == "" {
		workspaceRoot = DefaultWorkspaceRoot
	}
	workspace, err := OpenWorkspace(workspaceRoot, time.Now())
	if err != nil {
		return err
	}
	session, err := deps.App.CreateSession(ProfileName)
	if err != nil {
		return fmt.Errorf("create career session: %w", err)
	}

	printCareerWelcome(deps.Stdout, workspace.Root, session.ID)
	lineReader, err := terminal.NewLineReader(deps.Stdin, deps.Stdout)
	if err != nil {
		return err
	}
	defer lineReader.Close()
	for {
		rawInput, err := lineReader.ReadLine("career> ")
		if err != nil {
			if err == io.EOF {
				fmt.Fprintln(deps.Stdout)
				return nil
			}
			fmt.Fprintln(deps.Stdout)
			return err
		}
		input := strings.TrimSpace(rawInput)
		if input == "" {
			continue
		}
		switch input {
		case "/exit", "/quit":
			return nil
		case "/help":
			printCareerHelp(deps.Stdout)
			continue
		case "/status":
			if err := printWorkspaceStatus(deps.Stdout, workspace); err != nil {
				return err
			}
			continue
		}
		if isCommandHelpQuestion(input) {
			printCareerHelp(deps.Stdout)
			continue
		}
		if strings.HasPrefix(input, "/export") {
			if err := handleExportCommand(deps.Stdout, workspace, input); err != nil {
				return err
			}
			continue
		}
		if strings.HasPrefix(input, "/add") {
			if err := handleAddCommand(deps.Stdout, lineReader, workspace, input); err != nil {
				return err
			}
			continue
		}
		autoArchived, ingestErrors, err := autoArchiveReferencedFiles(context.Background(), deps.Stdout, workspace, input)
		if err != nil {
			return err
		}
		classification := ClassifyInput(input)
		if classification.ShouldSave && len(autoArchived) == 0 {
			if err := saveMaterialAndPrint(deps.Stdout, workspace, classification.Type, input, "已识别并归档为 "+displayWorkspaceType(classification.Type)); err != nil {
				return err
			}
			continue
		}
		analysisRequested := shouldAnalyzeInput(input)
		if len(autoArchived) > 0 && !analysisRequested {
			continue
		}

		meta, err := workspace.ReadMetadata()
		if err != nil {
			return err
		}
		record, err := runCareerTurn(deps, session.ID, BuildInteractivePromptWithAutoSaved(input, classification, autoArchived, ingestErrors, meta, analysisRequested, workspace.Root), classification)
		if err != nil {
			if record.ID != "" {
				fmt.Fprintf(deps.Stderr, "run_id=%s session_id=%s\n", record.ID, record.SessionID)
			}
			return fmt.Errorf("run career turn: %w", err)
		}
		fmt.Fprintf(deps.Stderr, "run_id=%s session_id=%s\n", record.ID, record.SessionID)
		fmt.Fprintf(deps.Stdout, "assistant> %s\n", record.Output)
		if analysisRequested {
			if reportPath, writeErr := workspace.WriteArtifact("career-report", "Career Analysis", record.Output, time.Now()); writeErr == nil {
				fmt.Fprintf(deps.Stdout, "assistant> 已保存本轮分析报告：%s\n", reportPath)
			}
		}
	}
}

func handleExportCommand(output io.Writer, workspace *Workspace, input string) error {
	kind := strings.TrimSpace(strings.TrimPrefix(input, "/export"))
	if kind == "" {
		fmt.Fprintln(output, "assistant> 用法：/export <类型>。支持 jd-match、resume-review、project-pitch、interview-review、review-material。")
		return nil
	}
	title, content, err := RenderWorkspaceArtifact(workspace, kind)
	if err != nil {
		return err
	}
	path, err := workspace.WriteArtifact(kind, title, content, time.Now())
	if err != nil {
		return err
	}
	fmt.Fprintf(output, "assistant> 已导出 %s：%s\n", title, path)
	return nil
}

func handleAddCommand(output io.Writer, lineReader terminal.LineReader, workspace *Workspace, input string) error {
	fields := strings.Fields(input)
	if len(fields) < 2 {
		fmt.Fprintln(output, "assistant> 用法：/add <类型> <内容>。支持 jd、resume、project、interview_experience、interview_record、review_note。多行内容可输入 /add <类型> 后粘贴，单独一行 . 结束。")
		return nil
	}
	itemType := normalizeWorkspaceType(fields[1])
	inline := strings.TrimSpace(strings.TrimPrefix(input, strings.Join(fields[:2], " ")))
	if !IsSupportedWorkspaceType(itemType) {
		fmt.Fprintf(output, "assistant> 暂不支持归档类型 %q。可用类型：jd、resume、project、interview_experience、interview_record、review_note。\n", fields[1])
		return nil
	}
	content := inline
	if content != "" && isExistingFile(content) {
		fileContent, err := extractDocument(context.Background(), content)
		if err != nil {
			return err
		}
		return saveMaterialFileAndPrint(output, workspace, WorkspaceFileInput{
			ItemType:      itemType,
			Text:          fileContent.Text,
			OriginalPath:  content,
			OriginalName:  filepath.Base(content),
			Now:           time.Now(),
			Extractor:     fileContent.Extractor,
			MIMEType:      fileContent.MIMEType,
			ExtractStatus: fileContent.ExtractStatus,
			ExtractError:  fileContent.ExtractError,
		}, "已从文件添加 "+displayWorkspaceType(itemType))
	}
	if content == "" {
		fmt.Fprintf(output, "assistant> 请粘贴 %s 内容，单独一行 . 结束。\n", displayWorkspaceType(itemType))
		var lines []string
		for {
			line, err := lineReader.ReadLine("")
			if err != nil {
				if err == io.EOF {
					break
				}
				return err
			}
			if strings.TrimSpace(line) == "." {
				break
			}
			lines = append(lines, line)
		}
		content = strings.TrimSpace(strings.Join(lines, "\n"))
	}
	if content == "" {
		fmt.Fprintf(output, "assistant> 没有收到 %s 内容，未写入工作区。\n", displayWorkspaceType(itemType))
		return nil
	}
	return saveMaterialAndPrint(output, workspace, itemType, content, "已添加 "+displayWorkspaceType(itemType))
}

func isExistingFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func saveMaterialAndPrint(output io.Writer, workspace *Workspace, itemType string, content string, prefix string) error {
	item, err := workspace.AddMaterial(itemType, content, time.Now())
	if err != nil {
		return err
	}
	fmt.Fprintf(output, "assistant> %s，并保存到 %s。我已经更新工作区索引，可以基于这些资料做匹配分析、简历优化和面试准备。\n", prefix, item.Path)
	return nil
}

func saveMaterialFileAndPrint(output io.Writer, workspace *Workspace, input WorkspaceFileInput, prefix string) error {
	item, err := workspace.AddMaterialFromFile(input)
	if err != nil {
		return err
	}
	fmt.Fprintf(output, "assistant> %s，并保存到 %s。我已经更新工作区索引，可以基于这些资料做匹配分析、简历优化和面试准备。\n", prefix, item.Path)
	return nil
}

func autoArchiveReferencedFiles(ctx context.Context, output io.Writer, workspace *Workspace, input string) ([]WorkspaceItem, []string, error) {
	paths := extractReferencedFiles(input)
	if len(paths) == 0 {
		paths = discoverFilesInReferencedDirectories(input)
	}
	if len(paths) == 0 {
		return nil, nil, nil
	}
	archived := make([]WorkspaceItem, 0, len(paths))
	ingestErrors := make([]string, 0)
	for _, path := range paths {
		result, err := IngestFile(ctx, workspace, IngestRequest{
			Path:      path,
			HintType:  detectWorkspaceTypeHintNearPath(input, path),
			UserInput: input,
			Now:       time.Now(),
		})
		if err != nil {
			ingestErrors = append(ingestErrors, fmt.Sprintf("%s: %s", path, err.Error()))
			if result.Item.ID != "" {
				fmt.Fprintf(output, "assistant> 已保存源文件，但提取失败：%s\n", err.Error())
			} else {
				fmt.Fprintf(output, "assistant> 无法自动归档 %s：%s\n", path, err.Error())
			}
		} else {
			fmt.Fprintf(output, "assistant> 已自动归档 %s：%s -> %s\n", displayWorkspaceType(result.ItemType), path, result.Item.Path)
		}
		if result.Item.ID != "" {
			archived = append(archived, result.Item)
		}
	}
	return archived, ingestErrors, nil
}

func detectWorkspaceTypeHint(input string) string {
	normalized := strings.ToLower(input)
	switch {
	case strings.Contains(normalized, "jd"), strings.Contains(normalized, "job description"), strings.Contains(input, "职位"), strings.Contains(input, "岗位"):
		return WorkspaceTypeJD
	case strings.Contains(normalized, "resume"), strings.Contains(normalized, "cv"), strings.Contains(input, "简历"):
		return WorkspaceTypeResume
	case strings.Contains(normalized, "project"), strings.Contains(input, "项目"):
		return WorkspaceTypeProject
	case strings.Contains(normalized, "interview experience"), strings.Contains(input, "面经"):
		return WorkspaceTypeInterviewExperience
	case strings.Contains(normalized, "interview record"), strings.Contains(input, "面试记录"), strings.Contains(input, "复盘"):
		return WorkspaceTypeInterviewRecord
	case strings.Contains(normalized, "review note"), strings.Contains(normalized, "study note"), strings.Contains(input, "笔记"), strings.Contains(input, "复习"):
		return WorkspaceTypeReviewNote
	default:
		return ""
	}
}

func detectWorkspaceTypeHintNearPath(input string, path string) string {
	if strings.TrimSpace(path) == "" {
		return detectWorkspaceTypeHint(input)
	}
	index := strings.Index(input, path)
	if index < 0 {
		return detectWorkspaceTypeHint(input)
	}
	start := index - 24
	if start < 0 {
		start = 0
	}
	end := index + len(path) + 24
	if end > len(input) {
		end = len(input)
	}
	window := input[start:end]
	if hinted := detectWorkspaceTypeHint(window); hinted != "" {
		return hinted
	}
	return detectWorkspaceTypeHint(input)
}

func Analyze(ctx context.Context, options AnalyzeOptions, deps Dependencies) error {
	if deps.App == nil {
		return fmt.Errorf("career analyze requires an application")
	}
	if deps.Stdout == nil {
		deps.Stdout = io.Discard
	}
	if deps.Stderr == nil {
		deps.Stderr = io.Discard
	}
	normalized, err := normalizeAnalyzeOptions(options)
	if err != nil {
		return err
	}
	if err := validateInputs(normalized); err != nil {
		return err
	}

	session, err := deps.App.CreateSession(ProfileName)
	if err != nil {
		return fmt.Errorf("create career session: %w", err)
	}

	prompt := BuildAnalyzePrompt(normalized)
	record, err := deps.App.AppendUserTurn(ctx, app.AppendTurnRequest{
		SessionID:    session.ID,
		ProfileName:  ProfileName,
		Input:        prompt,
		SystemPrompt: deps.Config.Engine.SystemPrompt,
	})
	if err != nil {
		return fmt.Errorf("run career analysis: %w", err)
	}

	careerReport, record, err := parseOrRepairReport(ctx, deps, session.ID, record)
	if err != nil {
		return err
	}
	careerReport.Appendix.RunID = record.ID
	careerReport.Appendix.SessionID = record.SessionID

	markdown, err := RenderMarkdown(careerReport, normalized.TemplatePath)
	if err != nil {
		return err
	}
	if err := WriteText(normalized.MarkdownPath, markdown); err != nil {
		return err
	}
	if err := WriteReportJSON(normalized.JSONPath, careerReport); err != nil {
		return err
	}
	traceReport := report.RunReport{
		RunID:         record.ID,
		SessionID:     record.SessionID,
		Profile:       record.Profile,
		Model:         deps.Config.LLM.Model,
		Input:         record.Input,
		Output:        record.Output,
		Status:        record.Status,
		ErrorCategory: record.ErrorCategory,
		Trace:         record.Trace,
		Steps:         record.Steps,
		SystemPrompt:  record.SystemPrompt,
		Events:        record.Events,
	}
	if err := report.WriteJSON(normalized.TraceJSONPath, traceReport); err != nil {
		return err
	}

	printAnalyzeSummary(deps.Stdout, careerReport, record, normalized)
	return nil
}

func runCareerTurn(deps Dependencies, sessionID string, prompt string, classification InputClassification) (store.RunRecord, error) {
	timeout := time.Duration(deps.Config.Engine.RunTimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return deps.App.AppendUserTurn(ctx, app.AppendTurnRequest{
		SessionID:    sessionID,
		ProfileName:  ProfileName,
		Input:        prompt,
		SystemPrompt: deps.Config.Engine.SystemPrompt,
		Events:       []observe.Event{classificationEvent(classification)},
	})
}

func parseOrRepairReport(ctx context.Context, deps Dependencies, sessionID string, record store.RunRecord) (Report, store.RunRecord, error) {
	careerReport, err := ParseReportString(record.Output)
	if err == nil {
		return careerReport, record, nil
	}
	repairPrompt := BuildReportRepairPrompt(record.Output, err)
	repairedRecord, repairErr := deps.App.AppendUserTurn(ctx, app.AppendTurnRequest{
		SessionID:    sessionID,
		ProfileName:  ProfileName,
		Input:        repairPrompt,
		SystemPrompt: deps.Config.Engine.SystemPrompt,
	})
	if repairErr != nil {
		return Report{}, record, fmt.Errorf("parse career report json: %w; repair run failed: %v", err, repairErr)
	}
	careerReport, repairParseErr := ParseReportString(repairedRecord.Output)
	if repairParseErr != nil {
		return Report{}, repairedRecord, fmt.Errorf("parse career report json: %w; repair parse failed: %v", err, repairParseErr)
	}
	return careerReport, repairedRecord, nil
}

func BuildReportRepairPrompt(output string, parseErr error) string {
	return fmt.Sprintf(`The previous final_answer content was not valid career_report JSON.

JSON parse error:
%v

Repair task:
- Return only one valid JSON object for the career_report schema.
- Do not use Markdown fences.
- Do not include comments.
- Do not put raw line breaks inside JSON string values; use short single-line strings or arrays.
- Keep all fields required by the career_report schema: summary, jd_analysis, project_evidence, resume_rewrite, interview_brief, gap_plan, risk_flags, appendix.
- Preserve the same facts and evidence boundaries as much as possible.

Invalid previous content:
%s`, parseErr, output)
}

func BuildInteractivePrompt(input string, classification InputClassification) string {
	return BuildInteractivePromptWithAutoSaved(input, classification, nil, nil, WorkspaceMetadata{}, false, "")
}

func BuildInteractivePromptWithAutoSaved(input string, classification InputClassification, autoSaved []WorkspaceItem, ingestErrors []string, meta WorkspaceMetadata, analysisRequested bool, workspaceRoot string) string {
	autoSavedSection := ""
	if len(autoSaved) > 0 {
		var lines []string
		for _, item := range autoSaved {
			lines = append(lines, fmt.Sprintf("- type: %s, title: %s, stored_path: %s", item.Type, item.Title, promptWorkspacePath(workspaceRoot, item.Path)))
		}
		autoSavedSection = "\n\nAuto-saved workspace assets:\n" + strings.Join(lines, "\n")
	}
	ingestErrorSection := ""
	if len(ingestErrors) > 0 {
		ingestErrorSection = "\n\nIngest warnings:\n- " + strings.Join(ingestErrors, "\n- ")
	}
	workspaceSection := ""
	if meta.CurrentResume != "" || meta.ActiveJD != "" || meta.ActiveProject != "" {
		workspaceSection = fmt.Sprintf("\n\nWorkspace pointers:\n- current_resume: %s\n- active_jd: %s\n- active_project: %s", emptyIfBlank(promptWorkspacePath(workspaceRoot, meta.CurrentResume)), emptyIfBlank(promptWorkspacePath(workspaceRoot, meta.ActiveJD)), emptyIfBlank(promptWorkspacePath(workspaceRoot, meta.ActiveProject)))
	}
	analysisSection := ""
	if analysisRequested {
		analysisSection = "\n\nAnalysis priority:\n- Use the newly saved resume and active JD for matching analysis when both exist.\n- Read the extracted workspace files before concluding.\n- If auto-saved workspace assets already exist, do not ask the user to choose storage paths, extraction tools, or workflow options.\n- Treat DOCX/PDF extraction as already handled by the application layer unless an explicit ingest warning says extraction failed.\n- Keep user-provided facts separate from suggestions."
	}
	return fmt.Sprintf(`You are running inside the Career Copilot continuous conversation workspace.

Input classification:
- type: %s
- confidence: %.2f
- signals: %s

User input:
%s%s%s%s%s`, classification.Type, classification.Confidence, strings.Join(classification.Signals, ", "), input, autoSavedSection, ingestErrorSection, workspaceSection, analysisSection)
}

func promptWorkspacePath(workspaceRoot string, storedPath string) string {
	storedPath = strings.TrimSpace(storedPath)
	if storedPath == "" {
		return ""
	}
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	if workspaceRoot == "" {
		return filepath.ToSlash(storedPath)
	}
	return filepath.ToSlash(filepath.Join(workspaceRoot, storedPath))
}

func shouldAnalyzeInput(input string) bool {
	normalized := strings.ToLower(strings.TrimSpace(input))
	if normalized == "" {
		return false
	}
	signals := []string{
		"分析", "看看", "建议", "优化", "匹配", "评估", "review", "analy", "match", "gap", "rewrite",
	}
	for _, signal := range signals {
		if strings.Contains(normalized, signal) {
			return true
		}
	}
	return false
}

func emptyIfBlank(value string) string {
	if strings.TrimSpace(value) == "" {
		return "(none)"
	}
	return value
}

func printCareerWelcome(output io.Writer, workspaceRoot string, sessionID string) {
	fmt.Fprintln(output, "Career Copilot")
	fmt.Fprintln(output)
	fmt.Fprintln(output, "我是你的智能求职助手，可以帮你分析 JD、优化简历、匹配项目经历、推荐面试问题、模拟面试、记录面试过程，并整理复习资料。")
	fmt.Fprintln(output)
	fmt.Fprintln(output, "我会把与你求职相关的资料保存在本地工作区，包含 JD、简历版本、项目素材、面经、面试记录、复习资料和生成报告。你可以随时让我查看、整理、更新或导出这些资料。")
	fmt.Fprintln(output)
	fmt.Fprintf(output, "Workspace: %s\n", workspaceRoot)
	fmt.Fprintf(output, "Session: %s\n", sessionID)
	fmt.Fprintln(output)
	fmt.Fprintln(output, "你可以直接粘贴 JD、简历、面经或面试记录，也可以输入 /help 查看命令。")
}

func printCareerHelp(output io.Writer) {
	fmt.Fprintln(output, "Commands:")
	fmt.Fprintln(output, "  /help     查看可用命令")
	fmt.Fprintln(output, "  /status   查看当前求职工作区")
	fmt.Fprintln(output, "  /export   导出 jd-match、resume-review、project-pitch、interview-review、review-material")
	fmt.Fprintln(output, "  /add jd   添加 JD；多行内容用单独一行 . 结束")
	fmt.Fprintln(output, "  /add resume | project | interview_experience | interview_record | review_note")
	fmt.Fprintln(output, "  /exit     退出")
	fmt.Fprintln(output)
	fmt.Fprintln(output, "你也可以直接输入自然语言，例如：这是一个新的 JD，帮我分析一下。")
}

func isCommandHelpQuestion(input string) bool {
	normalized := strings.ToLower(strings.TrimSpace(input))
	if normalized == "" {
		return false
	}
	if strings.Contains(normalized, "command") || strings.Contains(normalized, "commands") {
		return strings.Contains(normalized, "available") || strings.Contains(normalized, "what") || strings.Contains(normalized, "list") || strings.Contains(normalized, "help")
	}
	if strings.Contains(normalized, "命令") {
		return strings.Contains(normalized, "可用") ||
			strings.Contains(normalized, "哪些") ||
			strings.Contains(normalized, "什么") ||
			strings.Contains(normalized, "帮助") ||
			strings.Contains(normalized, "help")
	}
	return strings.Contains(normalized, "有哪些命令") ||
		strings.Contains(normalized, "可用命令") ||
		strings.Contains(normalized, "命令有哪些") ||
		strings.Contains(normalized, "能用什么命令")
}

func printWorkspaceStatus(output io.Writer, workspace *Workspace) error {
	meta, index, err := workspace.Status()
	if err != nil {
		return err
	}
	counts := map[string]int{}
	for _, item := range index.Items {
		counts[item.Type]++
	}
	fmt.Fprintln(output, "Workspace Status")
	fmt.Fprintf(output, "  Root: %s\n", workspace.Root)
	fmt.Fprintf(output, "  Items: %d\n", len(index.Items))
	fmt.Fprintf(output, "  JDs: %d\n", counts["jd"])
	fmt.Fprintf(output, "  Resumes: %d\n", counts["resume"])
	fmt.Fprintf(output, "  Projects: %d\n", counts["project"])
	fmt.Fprintf(output, "  Interview Experience: %d\n", counts["interview_experience"])
	fmt.Fprintf(output, "  Interview Records: %d\n", counts["interview_record"])
	fmt.Fprintf(output, "  Review Notes: %d\n", counts["review_note"])
	if meta.ActiveJD != "" {
		fmt.Fprintf(output, "  Active JD: %s\n", meta.ActiveJD)
	}
	if meta.CurrentResume != "" {
		fmt.Fprintf(output, "  Current Resume: %s\n", meta.CurrentResume)
	}
	if meta.ActiveProject != "" {
		fmt.Fprintf(output, "  Active Project: %s\n", meta.ActiveProject)
	}
	return nil
}

func classificationEvent(classification InputClassification) observe.Event {
	return observe.Event{
		Time:    time.Now(),
		Type:    "career_input_classified",
		Message: "career input classified",
		Data: map[string]string{
			"type":        classification.Type,
			"confidence":  fmt.Sprintf("%.2f", classification.Confidence),
			"should_save": fmt.Sprintf("%t", classification.ShouldSave),
			"signals":     strings.Join(classification.Signals, ","),
		},
	}
}

func normalizeWorkspaceType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "jd", "job", "job_description", "job-description", "岗位":
		return WorkspaceTypeJD
	case "resume", "cv", "简历":
		return WorkspaceTypeResume
	case "project", "项目":
		return WorkspaceTypeProject
	case "interview_experience", "interview-experience", "experience", "面经":
		return WorkspaceTypeInterviewExperience
	case "interview_record", "interview-record", "record", "面试记录":
		return WorkspaceTypeInterviewRecord
	case "review_note", "review-note", "note", "notes", "笔记", "复习":
		return WorkspaceTypeReviewNote
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func displayWorkspaceType(itemType string) string {
	switch itemType {
	case WorkspaceTypeJD:
		return "JD"
	case WorkspaceTypeResume:
		return "简历"
	case WorkspaceTypeProject:
		return "项目素材"
	case WorkspaceTypeInterviewExperience:
		return "面经"
	case WorkspaceTypeInterviewRecord:
		return "面试记录"
	case WorkspaceTypeReviewNote:
		return "复习笔记"
	default:
		return itemType
	}
}

func BuildAnalyzePrompt(options AnalyzeOptions) string {
	return fmt.Sprintf(`Run Career Copilot analysis for the target role described by the input files.

Use the career-copilot profile behavior and inspect evidence before concluding.

Inputs:
- JD: %s
- Resume draft: %s
- Target statement: %s
- Repository root: %s

Required tool use:
- Read the JD, resume, and target files with file_read.
- Use file_search or search_docs for supporting evidence when useful.
- Prefer scoped reads of the most relevant example, README, docs, or project files. Do not scan unrelated source files unless the target role or resume explicitly needs them.

When complete, call final_answer with valid JSON only, with this exact shape:
{
  "summary": {"target_role": "...", "match_score": 0, "verdict": "..."},
  "jd_analysis": {"required_capabilities": [{"name": "...", "importance": "high|medium|low", "evidence_needed": "..."}]},
  "project_evidence": [{"claim": "...", "evidence": [{"path": "...", "reason": "..."}], "confidence": "high|medium|low"}],
  "resume_rewrite": {"bullets": [{"original": "...", "recommended": "...", "why": "..."}]},
  "interview_brief": {"project_pitch": "...", "architecture_talk_track": "...", "tradeoffs": ["..."], "questions_to_expect": ["..."]},
  "gap_plan": [{"priority": "P0|P1|P2", "item": "...", "why_it_matters": "...", "acceptance": "..."}],
  "risk_flags": [{"statement": "...", "reason": "..."}],
  "appendix": {"files_reviewed": ["..."]}
}

Constraints:
- Adapt the analysis to the target role in the JD and target files; do not assume an engineering role.
- Do not invent employment history, production usage, metrics, financial impact, business impact, or other domain-specific outcomes.
- Do not strengthen vague claims into specific numbers, scale, tools, budgets, user counts, revenue, performance, conversion, accuracy, latency, adoption, efficiency, awards, or business outcomes unless reviewed evidence explicitly provides them.
- Resume rewrites may improve clarity, structure, seniority framing, and role alignment, but must not add new facts, responsibilities, metrics, technologies, domains, or outcomes.
- Resume rewrites must not add unstated delivery qualities, timing claims, adoption claims, collaboration scope, decision authority, iteration cadence, or impact language such as on schedule, real-time, team-wide, led, owned, improved, accelerated, or established unless reviewed evidence explicitly supports that exact claim.
- If a stronger bullet requires missing evidence, place it in gap_plan or mark it as needing user confirmation instead of writing it as fact.
- Treat resume-only claims as candidate-provided evidence, not independently verified evidence.
- Use high confidence only for claims directly supported by reviewed files; use medium or low confidence for inferred, self-reported, or partially supported claims.
- Every project_evidence item must cite at least one repository path.
- Include at least three resume bullets, three project evidence claims, three gap items, and one risk flag.
- Keep every JSON string value on one line. Do not place raw line breaks inside quoted strings.
- Return only the JSON object. Do not wrap it in Markdown fences.
- Keep the report actionable for editing a resume and preparing for interviews.`,
		options.JDPath,
		options.ResumePath,
		options.TargetPath,
		options.RepoPath,
	)
}

func parseAnalyzeOptions(args []string) (AnalyzeOptions, error) {
	fs := flag.NewFlagSet("career analyze", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	options := AnalyzeOptions{}
	fs.StringVar(&options.JDPath, "jd", "", "path to target job description markdown")
	fs.StringVar(&options.ResumePath, "resume", "", "path to resume draft markdown")
	fs.StringVar(&options.TargetPath, "target", "", "path to career target markdown")
	fs.StringVar(&options.RepoPath, "repo", ".", "repository root to inspect")
	fs.StringVar(&options.MarkdownPath, "out", "outputs/career-report.md", "markdown report output path")
	fs.StringVar(&options.JSONPath, "json", "outputs/career-report.json", "structured career report JSON output path")
	fs.StringVar(&options.TraceJSONPath, "trace-json", "logs/career/latest-trace.json", "runtime trace JSON output path")
	fs.StringVar(&options.TemplatePath, "template", DefaultReportTemplatePath, "markdown template path")
	if err := fs.Parse(args); err != nil {
		return AnalyzeOptions{}, err
	}
	return options, nil
}

func normalizeAnalyzeOptions(options AnalyzeOptions) (AnalyzeOptions, error) {
	if strings.TrimSpace(options.RepoPath) == "" {
		options.RepoPath = "."
	}
	repo, err := filepath.Abs(options.RepoPath)
	if err != nil {
		return AnalyzeOptions{}, fmt.Errorf("resolve repo path: %w", err)
	}
	options.RepoPath = repo
	if strings.TrimSpace(options.MarkdownPath) == "" {
		options.MarkdownPath = "outputs/career-report.md"
	}
	if strings.TrimSpace(options.JSONPath) == "" {
		options.JSONPath = "outputs/career-report.json"
	}
	if strings.TrimSpace(options.TraceJSONPath) == "" {
		options.TraceJSONPath = "logs/career/latest-trace.json"
	}
	if strings.TrimSpace(options.TemplatePath) == "" {
		options.TemplatePath = DefaultReportTemplatePath
	}
	return options, nil
}

func validateInputs(options AnalyzeOptions) error {
	required := map[string]string{
		"--jd":     options.JDPath,
		"--resume": options.ResumePath,
		"--target": options.TargetPath,
	}
	for name, path := range required {
		if strings.TrimSpace(path) == "" {
			return fmt.Errorf("%s is required", name)
		}
		if err := ensureReadableFile(path); err != nil {
			return fmt.Errorf("%s %q: %w", name, path, err)
		}
	}
	info, err := os.Stat(options.RepoPath)
	if err != nil {
		return fmt.Errorf("--repo %q: %w", options.RepoPath, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("--repo %q is not a directory", options.RepoPath)
	}
	if err := ensureReadableFile(options.TemplatePath); err != nil {
		return fmt.Errorf("--template %q: %w", options.TemplatePath, err)
	}
	return nil
}

func ensureReadableFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("is a directory")
	}
	return nil
}

func printAnalyzeSummary(output io.Writer, report Report, record store.RunRecord, options AnalyzeOptions) {
	fmt.Fprintln(output, "Career Copilot Report")
	fmt.Fprintln(output)
	fmt.Fprintf(output, "Profile: %s\n", ProfileName)
	fmt.Fprintf(output, "Run: %s\n", record.ID)
	fmt.Fprintf(output, "Session: %s\n", record.SessionID)
	fmt.Fprintln(output)
	fmt.Fprintf(output, "Match: %d/100\n", report.Summary.MatchScore)
	fmt.Fprintln(output, "Strong signals:")
	for _, claim := range TopEvidenceClaims(report, 3) {
		fmt.Fprintf(output, "  - %s\n", claim)
	}
	fmt.Fprintln(output)
	fmt.Fprintln(output, "Top gaps:")
	for _, gap := range TopGapItems(report, 3) {
		fmt.Fprintf(output, "  - %s\n", gap)
	}
	fmt.Fprintln(output)
	fmt.Fprintf(output, "Report: %s\n", options.MarkdownPath)
	fmt.Fprintf(output, "JSON: %s\n", options.JSONPath)
	fmt.Fprintf(output, "Trace: %s\n", options.TraceJSONPath)
}

func runReportTransform(args []string, stdout io.Writer, render func(Report) string) error {
	fs := flag.NewFlagSet("career report transform", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var reportPath string
	var outPath string
	fs.StringVar(&reportPath, "report", "", "career report JSON path")
	fs.StringVar(&outPath, "out", "", "markdown output path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(reportPath) == "" {
		return fmt.Errorf("--report is required")
	}
	careerReport, err := ReadReport(reportPath)
	if err != nil {
		return err
	}
	content := render(careerReport)
	if strings.TrimSpace(outPath) == "" {
		if stdout == nil {
			stdout = io.Discard
		}
		fmt.Fprint(stdout, content)
		return nil
	}
	if err := WriteText(outPath, content); err != nil {
		return err
	}
	if stdout != nil {
		fmt.Fprintf(stdout, "Wrote: %s\n", outPath)
	}
	return nil
}

func ContextWithConfiguredTimeout(parent context.Context, cfg config.Config) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, time.Duration(cfg.Engine.RunTimeoutSeconds)*time.Second)
}
