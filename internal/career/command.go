package career

import (
	"context"
	"encoding/json"
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
	"happyagent/internal/runlog"
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

	printCareerWelcome(deps.Stdout, workspace, session.ID)
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
		if err := handleNaturalLanguageInput(deps, workspace, session.ID, input); err != nil {
			return err
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
	paths, err := workspace.WriteUserOutput(kind, title, content, nil, time.Now())
	if err != nil {
		return err
	}
	fmt.Fprintf(output, "assistant> 已生成并保存 %s：%s\n", title, paths.LatestMarkdown)
	return nil
}

func handleAddCommand(output io.Writer, lineReader terminal.LineReader, workspace *Workspace, input string) error {
	fields := strings.Fields(input)
	if len(fields) < 2 {
		fmt.Fprintln(output, "assistant> 用法：/add <类型> <内容>。支持 jd、resume、prepare、experiences、my-interviews、record。多行内容可输入 /add <类型> 后粘贴，单独一行 . 结束。")
		return nil
	}
	itemType := normalizeWorkspaceType(fields[1])
	inline := strings.TrimSpace(strings.TrimPrefix(input, strings.Join(fields[:2], " ")))
	if !IsSupportedWorkspaceType(itemType) {
		fmt.Fprintf(output, "assistant> 暂不支持归档类型 %q。可用类型：jd、resume、prepare、experiences、my-interviews、record。\n", fields[1])
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
	if strings.ToLower(strings.TrimSpace(itemType)) == WorkspaceTypeExperiences {
		result, err := workspace.ArchivePublicInterviewExperience(content, time.Now())
		if err != nil {
			return err
		}
		fmt.Fprintf(output, "assistant> %s，并保存到 %s；已同步沉淀到 %s，并记录导入流程 %s。\n", prefix, result.ExperienceItem.Path, result.MyInterviewRel, result.RecordRel)
		return nil
	}
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
	explicitPaths := make(map[string]bool, len(paths))
	for _, path := range paths {
		explicitPaths[path] = true
	}
	seenPaths := make(map[string]bool, len(paths))
	for _, path := range paths {
		seenPaths[path] = true
	}
	for _, path := range discoverFilesInReferencedDirectories(input) {
		if !seenPaths[path] {
			paths = append(paths, path)
			seenPaths[path] = true
		}
	}
	if len(paths) == 0 {
		return nil, nil, nil
	}
	archived := make([]WorkspaceItem, 0, len(paths))
	ingestErrors := make([]string, 0)
	for _, path := range paths {
		hintType := ""
		if explicitPaths[path] {
			hintType = detectWorkspaceTypeHintNearPath(input, path)
		}
		result, err := IngestFile(ctx, workspace, IngestRequest{
			Path:      path,
			HintType:  hintType,
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
	return detectWorkspaceTypeBySignals(input)
}

func detectWorkspaceTypeHintNearPath(input string, path string) string {
	if strings.TrimSpace(path) == "" {
		return detectWorkspaceTypeHint(input)
	}
	index := strings.Index(input, path)
	if index < 0 {
		base := filepath.Base(path)
		if base == "." || base == string(filepath.Separator) || base == path {
			return ""
		}
		index = strings.Index(input, base)
		if index < 0 {
			return ""
		}
		path = base
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
	runlog.Section("Run Input", prompt)
	runlog.Linef("Model: `%s`", deps.Config.LLM.Model)
	runlog.Linef("Timeout: `%ds`", deps.Config.Engine.RunTimeoutSeconds)
	runlog.Linef("Profile: `%s`", ProfileName)
	runlog.Linef("Session: `%s`", sessionID)
	runlog.Linef("")
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	record, err := deps.App.AppendUserTurn(ctx, app.AppendTurnRequest{
		SessionID:    sessionID,
		ProfileName:  ProfileName,
		Input:        prompt,
		SystemPrompt: deps.Config.Engine.SystemPrompt,
		Events:       []observe.Event{classificationEvent(classification)},
	})
	if err != nil {
		return record, err
	}
	runlog.Section("Final Output", record.Output)
	return record, nil
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
		analysisSection = "\n\nAnalysis priority:\n- Use the newly saved resume and active JD for matching analysis when both exist.\n- Read all listed stored_path and current workspace pointer files directly, preferably in one multi-tool step when more than one file is needed.\n- Do not call file_list or file_search to rediscover files that are already listed in this prompt.\n- Do not inspect record directories just to verify saving; the application layer saves generated analysis artifacts after the model response.\n- If auto-saved workspace assets already exist, do not ask the user to choose storage paths, extraction tools, or workflow options.\n- Treat DOCX/PDF extraction as already handled by the application layer unless an explicit ingest warning says extraction failed.\n- Keep user-provided facts separate from suggestions."
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

func handleNaturalLanguageInput(deps Dependencies, workspace *Workspace, sessionID string, input string) error {
	intent := ClassifyIntent(input)
	classification := ClassifyInput(input)
	autoArchived, ingestErrors, err := autoArchiveReferencedFiles(context.Background(), deps.Stdout, workspace, input)
	if err != nil {
		return err
	}
	if shouldScanInbox(intent) {
		inboxResult, inboxErr := IngestInbox(context.Background(), workspace, time.Now())
		if inboxErr != nil {
			return inboxErr
		}
		for _, item := range inboxResult.Items {
			autoArchived = append(autoArchived, item)
			fmt.Fprintf(deps.Stdout, "assistant> 已整理 inbox 文件到 %s：%s（已保留 inbox 原件）\n", displayWorkspaceType(item.Type), item.Path)
		}
		ingestErrors = append(ingestErrors, inboxResult.Warnings...)
	}
	if classification.ShouldSave && len(autoArchived) == 0 {
		item, err := saveMaterial(workspace, classification.Type, input)
		if err != nil {
			return err
		}
		autoArchived = append(autoArchived, item)
		fmt.Fprintf(deps.Stdout, "assistant> 已识别并归档为 %s：%s\n", displayWorkspaceType(item.Type), item.Path)
		if intent.Intent == CareerIntentChat || intent.Intent == CareerIntentIngest {
			return printIngestSummary(deps.Stdout, workspace, autoArchived, ingestErrors)
		}
	}
	switch intent.Intent {
	case CareerIntentStatus:
		return printWorkspaceStatus(deps.Stdout, workspace)
	case CareerIntentIngest:
		return printIngestSummary(deps.Stdout, workspace, autoArchived, ingestErrors)
	case CareerIntentAnalyze, CareerIntentResumeReview, CareerIntentInterviewBrief, CareerIntentGapPlan, CareerIntentInterviewReview:
		return handleIntentWithModelTurn(deps, workspace, sessionID, input, intent, classification, autoArchived, ingestErrors)
	default:
		return handleIntentWithModelTurn(deps, workspace, sessionID, input, intent, classification, autoArchived, ingestErrors)
	}
}

func shouldScanInbox(intent IntentClassification) bool {
	switch intent.Intent {
	case CareerIntentIngest, CareerIntentAnalyze, CareerIntentResumeReview, CareerIntentInterviewBrief, CareerIntentGapPlan, CareerIntentInterviewReview:
		return true
	default:
		return false
	}
}

func saveMaterial(workspace *Workspace, itemType string, content string) (WorkspaceItem, error) {
	if strings.ToLower(strings.TrimSpace(itemType)) == WorkspaceTypeExperiences {
		result, err := workspace.ArchivePublicInterviewExperience(content, time.Now())
		if err != nil {
			return WorkspaceItem{}, err
		}
		return result.ExperienceItem, nil
	}
	return workspace.AddMaterial(itemType, content, time.Now())
}

func handleIntentWithModelTurn(deps Dependencies, workspace *Workspace, sessionID string, input string, intent IntentClassification, classification InputClassification, autoArchived []WorkspaceItem, ingestErrors []string) error {
	meta, err := workspace.ReadMetadata()
	if err != nil {
		return err
	}
	if message := readinessMessage(workspace, meta, intent.Intent); message != "" {
		fmt.Fprintln(deps.Stdout, message)
		return nil
	}
	record, err := runCareerTurn(deps, sessionID, BuildInteractivePromptWithAutoSaved(input, classification, autoArchived, ingestErrors, meta, shouldGenerateOutput(intent.Intent), workspace.Root), classification)
	if err != nil {
		if record.ID != "" {
			fmt.Fprintf(deps.Stderr, "run_id=%s session_id=%s\n", record.ID, record.SessionID)
		}
		return fmt.Errorf("run career turn: %w", err)
	}
	fmt.Fprintf(deps.Stderr, "run_id=%s session_id=%s\n", record.ID, record.SessionID)
	fmt.Fprintf(deps.Stdout, "assistant> %s\n", record.Output)
	if !shouldGenerateOutput(intent.Intent) {
		return nil
	}
	now := time.Now()
	jsonContent := []byte(nil)
	if intent.Intent == CareerIntentAnalyze {
		payload := map[string]string{
			"run_id":         record.ID,
			"session_id":     record.SessionID,
			"intent":         string(intent.Intent),
			"output":         record.Output,
			"current_jd":     meta.ActiveJD,
			"current_resume": meta.CurrentResume,
		}
		data, marshalErr := json.MarshalIndent(payload, "", "  ")
		if marshalErr == nil {
			jsonContent = data
		}
	}
	outputKind, outputTitle := outputSpec(intent.Intent)
	paths, err := workspace.WriteUserOutput(outputKind, outputTitle, record.Output, jsonContent, now)
	if err != nil {
		return err
	}
	printCompletionSummary(deps.Stdout, outputTitle, collectedInputPaths(workspace.Root, meta, autoArchived), paths)
	return nil
}

func shouldGenerateOutput(intent CareerIntent) bool {
	switch intent {
	case CareerIntentAnalyze, CareerIntentResumeReview, CareerIntentInterviewBrief, CareerIntentGapPlan, CareerIntentInterviewReview:
		return true
	default:
		return false
	}
}

func outputSpec(intent CareerIntent) (string, string) {
	switch intent {
	case CareerIntentAnalyze:
		return "report", "完整匹配报告"
	case CareerIntentResumeReview:
		return "resume-review", "简历优化建议"
	case CareerIntentInterviewBrief:
		return "interview-brief", "面试准备材料"
	case CareerIntentGapPlan:
		return "gap-plan", "能力差距计划"
	case CareerIntentInterviewReview:
		return "interview-review", "面试复盘"
	default:
		return "career-note", "求职助手输出"
	}
}

func readinessMessage(workspace *Workspace, meta WorkspaceMetadata, intent CareerIntent) string {
	switch intent {
	case CareerIntentAnalyze:
		if meta.CurrentResume == "" && meta.ActiveJD == "" {
			return fmt.Sprintf("assistant> 现在还缺少简历和 JD。请把你准备好的内容放到 %s，然后直接说：我放好了，帮我分析。", filepath.ToSlash(filepath.Join(workspace.Root, "inbox")))
		}
		if meta.CurrentResume == "" {
			return fmt.Sprintf("assistant> 现在还缺少简历。请把你准备好的内容放到 %s，然后直接说：我放好了，帮我分析。", filepath.ToSlash(filepath.Join(workspace.Root, "inbox")))
		}
		if meta.ActiveJD == "" {
			return fmt.Sprintf("assistant> 现在还缺少 JD。请把你准备好的内容放到 %s，然后直接说：我放好了，帮我分析。", filepath.ToSlash(filepath.Join(workspace.Root, "inbox")))
		}
	case CareerIntentResumeReview:
		if meta.CurrentResume == "" {
			return fmt.Sprintf("assistant> 现在还缺少简历。请把你准备好的内容放到 %s，然后直接说：帮我优化简历。", filepath.ToSlash(filepath.Join(workspace.Root, "inbox")))
		}
	case CareerIntentInterviewBrief, CareerIntentGapPlan:
		if meta.ActiveJD == "" {
			return fmt.Sprintf("assistant> 现在还缺少 JD。请把你准备好的内容放到 %s，然后直接说：帮我生成面试准备材料。", filepath.ToSlash(filepath.Join(workspace.Root, "inbox")))
		}
	}
	return ""
}

func printIngestSummary(output io.Writer, workspace *Workspace, items []WorkspaceItem, warnings []string) error {
	if len(items) == 0 && len(warnings) == 0 {
		fmt.Fprintf(output, "assistant> 还没有发现新的可归档资料。请把你准备好的内容放到 %s。\n", filepath.ToSlash(filepath.Join(workspace.Root, "inbox")))
		return nil
	}
	if len(items) > 0 {
		fmt.Fprintln(output, "assistant> 已整理这些资料：")
		for _, item := range items {
			fmt.Fprintf(output, "  - %s：%s\n", displayWorkspaceType(item.Type), item.Path)
		}
	}
	for _, warning := range warnings {
		fmt.Fprintf(output, "assistant> 注意：%s\n", warning)
	}
	meta, _, err := workspace.Status()
	if err != nil {
		return err
	}
	if meta.CurrentResume != "" && meta.ActiveJD != "" {
		fmt.Fprintln(output, "assistant> 当前简历和 JD 都已就绪。你可以直接说：帮我分析一下匹配度。")
		return nil
	}
	fmt.Fprintf(output, "assistant> 请继续把你准备好的内容放到 %s。\n", filepath.ToSlash(filepath.Join(workspace.Root, "inbox")))
	return nil
}

func collectedInputPaths(workspaceRoot string, meta WorkspaceMetadata, autoArchived []WorkspaceItem) []string {
	seen := map[string]bool{}
	var paths []string
	for _, candidate := range []string{meta.CurrentResume, meta.ActiveJD, meta.ActiveProject} {
		if strings.TrimSpace(candidate) == "" || seen[candidate] {
			continue
		}
		paths = append(paths, promptWorkspacePath(workspaceRoot, candidate))
		seen[candidate] = true
	}
	for _, item := range autoArchived {
		if strings.TrimSpace(item.Path) == "" || seen[item.Path] {
			continue
		}
		paths = append(paths, promptWorkspacePath(workspaceRoot, item.Path))
		seen[item.Path] = true
	}
	return paths
}

func printCompletionSummary(output io.Writer, title string, inputs []string, paths UserOutputPaths) {
	fmt.Fprintf(output, "assistant> 完成：%s\n", title)
	for _, input := range inputs {
		fmt.Fprintf(output, "assistant> 读取：%s\n", input)
	}
	if paths.LatestMarkdown != "" {
		fmt.Fprintf(output, "assistant> 结果：%s\n", paths.LatestMarkdown)
	}
	if paths.LatestJSON != "" {
		fmt.Fprintf(output, "assistant> JSON：%s\n", paths.LatestJSON)
	}
}

func emptyIfBlank(value string) string {
	if strings.TrimSpace(value) == "" {
		return "(none)"
	}
	return value
}

func printCareerWelcome(output io.Writer, workspace *Workspace, sessionID string) {
	meta, index, err := workspace.Status()
	if err != nil {
		fmt.Fprintf(output, "求职助手 Career Copilot\n\n工作区：%s\n会话：%s\n", workspace.Root, sessionID)
		return
	}
	counts := workspaceCounts(index)
	fmt.Fprintln(output, careerBanner())
	fmt.Fprintln(output, "求职助手 Career Copilot")
	fmt.Fprintln(output)
	box := renderCareerBox("求职工作台", []string{
		"请把你准备好的内容放到：",
		"",
		"  " + filepath.ToSlash(filepath.Join(workspace.Root, "inbox")) + "/",
		"",
		"支持内容：简历、JD、项目经历、面经、面试记录、复习笔记",
	}, "当前资料", []string{
		"简历：" + displayPointer(meta.CurrentResume, "未发现"),
		"JD：" + displayPointer(meta.ActiveJD, "未发现"),
		fmt.Sprintf("项目素材：%d 份", counts[WorkspaceTypePrepare]),
		fmt.Sprintf("面经/面试记录：%d 份", counts[WorkspaceTypeExperiences]+counts[WorkspaceTypeMyInterviews]),
	}, "生成结果", []string{
		"所有报告和建议会保存到：",
		"",
		"  " + filepath.ToSlash(filepath.Join(workspace.Root, "outputs")) + "/",
	}, "你可以直接说", []string{
		"我把简历和 JD 放进 inbox 了，帮我分析一下",
		"帮我针对当前岗位优化简历",
		"帮我生成面试准备材料",
		"我刚面完，帮我复盘一下",
	})
	fmt.Fprintln(output, box)
	fmt.Fprintf(output, "会话：%s\n", sessionID)
}

func printCareerHelp(output io.Writer) {
	fmt.Fprintln(output, "你可以直接这样说：")
	fmt.Fprintln(output, "  我把简历和 JD 放进 inbox 了，帮我分析一下")
	fmt.Fprintln(output, "  帮我针对当前岗位优化简历")
	fmt.Fprintln(output, "  帮我生成面试准备材料")
	fmt.Fprintln(output, "  我刚面完，帮我复盘一下")
	fmt.Fprintln(output)
	fmt.Fprintln(output, "高级命令：")
	fmt.Fprintln(output, "  /help     查看帮助")
	fmt.Fprintln(output, "  /status   查看当前工作区状态")
	fmt.Fprintln(output, "  /export   生成 jd-match、resume-review、project-pitch、interview-review、review-material")
	fmt.Fprintln(output, "  /add jd   添加 JD；多行内容用单独一行 . 结束")
	fmt.Fprintln(output, "  /add resume | prepare | experiences | my-interviews | record")
	fmt.Fprintln(output, "  /exit     退出")
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
	counts := workspaceCounts(index)
	reportPath := outputPathIfExists(workspace, workspace.LatestOutputPath("report", ".md"))
	if reportPath == "" {
		reportPath = "未生成"
	}
	ready := "可直接生成完整匹配报告"
	if meta.CurrentResume == "" || meta.ActiveJD == "" {
		ready = "还不能生成完整匹配报告"
	}
	box := renderCareerBox("工作区状态", []string{
		"工作区：" + workspace.Root,
		"当前简历：" + displayPointer(meta.CurrentResume, "未发现"),
		"当前 JD：" + displayPointer(meta.ActiveJD, "未发现"),
		"当前项目：" + displayPointer(meta.ActiveProject, "未发现"),
	}, "资料统计", []string{
		fmt.Sprintf("简历：%d 份", counts[WorkspaceTypeResume]),
		fmt.Sprintf("JD：%d 份", counts[WorkspaceTypeJD]),
		fmt.Sprintf("项目素材：%d 份", counts[WorkspaceTypePrepare]),
		fmt.Sprintf("面经：%d 份", counts[WorkspaceTypeExperiences]),
		fmt.Sprintf("面试记录：%d 份", counts[WorkspaceTypeMyInterviews]),
		fmt.Sprintf("记录：%d 份", counts[WorkspaceTypeRecord]),
	}, "生成结果", []string{
		"最新报告：" + reportPath,
		"输出目录：" + filepath.ToSlash(filepath.Join(workspace.Root, "outputs")) + "/",
	}, "就绪情况", []string{
		ready,
		missingMaterialsHint(workspace, meta),
	})
	fmt.Fprintln(output, box)
	return nil
}

func workspaceCounts(index WorkspaceIndex) map[string]int {
	counts := map[string]int{}
	for _, item := range index.Items {
		counts[item.Type]++
	}
	return counts
}

func displayPointer(path string, fallback string) string {
	if strings.TrimSpace(path) == "" {
		return fallback
	}
	return path
}

func outputPathIfExists(workspace *Workspace, rel string) string {
	if strings.TrimSpace(rel) == "" {
		return ""
	}
	if _, err := os.Stat(filepath.Join(workspace.Root, filepath.FromSlash(rel))); err != nil {
		return ""
	}
	return rel
}

func missingMaterialsHint(workspace *Workspace, meta WorkspaceMetadata) string {
	var missing []string
	if meta.CurrentResume == "" {
		missing = append(missing, "简历")
	}
	if meta.ActiveJD == "" {
		missing = append(missing, "JD")
	}
	if len(missing) == 0 {
		return "你可以直接说：帮我分析一下匹配度"
	}
	return "缺少：" + strings.Join(missing, "、") + "。请放到 " + filepath.ToSlash(filepath.Join(workspace.Root, "inbox")) + "/"
}

func careerBanner() string {
	return strings.TrimRight(`
██╗  ██╗ █████╗ ██████╗ ██████╗ ██╗   ██╗
██║  ██║██╔══██╗██╔══██╗██╔══██╗╚██╗ ██╔╝
███████║███████║██████╔╝██████╔╝ ╚████╔╝
██╔══██║██╔══██║██╔═══╝ ██╔═══╝   ╚██╔╝
██║  ██║██║  ██║██║     ██║        ██║
╚═╝  ╚═╝╚═╝  ╚═╝╚═╝     ╚═╝        ╚═╝
`, "\n")
}

func renderCareerBox(title string, intro []string, section1 string, body1 []string, section2 string, body2 []string, section3 string, body3 []string) string {
	const innerWidth = 64
	var lines []string
	lines = append(lines, boxTop(title, innerWidth))
	for _, line := range intro {
		lines = append(lines, boxLine(line, innerWidth))
	}
	lines = append(lines, boxSection(section1, innerWidth))
	for _, line := range body1 {
		lines = append(lines, boxLine(line, innerWidth))
	}
	lines = append(lines, boxSection(section2, innerWidth))
	for _, line := range body2 {
		lines = append(lines, boxLine(line, innerWidth))
	}
	lines = append(lines, boxSection(section3, innerWidth))
	for _, line := range body3 {
		lines = append(lines, boxLine(line, innerWidth))
	}
	lines = append(lines, boxBottom(innerWidth))
	return strings.Join(lines, "\n")
}

func boxTop(title string, width int) string {
	return "╭" + centeredRule(title, width) + "╮"
}

func boxSection(title string, width int) string {
	return "├" + centeredRule(title, width) + "┤"
}

func boxBottom(width int) string {
	return "╰" + strings.Repeat("─", width+2) + "╯"
}

func centeredRule(title string, width int) string {
	label := " " + title + " "
	remaining := width + 2 - displayWidth(label)
	if remaining < 0 {
		remaining = 0
	}
	left := remaining / 2
	right := remaining - left
	return strings.Repeat("─", left) + label + strings.Repeat("─", right)
}

func boxLine(content string, width int) string {
	padding := width - displayWidth(content)
	if padding < 0 {
		padding = 0
	}
	return "│ " + content + strings.Repeat(" ", padding) + " │"
}

func displayWidth(value string) int {
	width := 0
	for _, r := range value {
		width += runeDisplayWidth(r)
	}
	return width
}

func runeDisplayWidth(r rune) int {
	switch {
	case r == 0:
		return 0
	case r < 0x20 || (r >= 0x7f && r < 0xa0):
		return 0
	case r >= 0x1100 && r <= 0x115f:
		return 2
	case r >= 0x2e80 && r <= 0xa4cf:
		return 2
	case r >= 0xac00 && r <= 0xd7a3:
		return 2
	case r >= 0xf900 && r <= 0xfaff:
		return 2
	case r >= 0xfe10 && r <= 0xfe19:
		return 2
	case r >= 0xfe30 && r <= 0xfe6f:
		return 2
	case r >= 0xff00 && r <= 0xff60:
		return 2
	case r >= 0xffe0 && r <= 0xffe6:
		return 2
	default:
		return 1
	}
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
	return strings.ToLower(strings.TrimSpace(value))
}

func displayWorkspaceType(itemType string) string {
	return workspaceTypeDisplayName(itemType)
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
	fs.StringVar(&options.MarkdownPath, "out", "", "markdown report output path")
	fs.StringVar(&options.JSONPath, "json", "", "structured career report JSON output path")
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
		options.MarkdownPath = filepath.Join(DefaultWorkspaceRoot, "outputs", "latest-report.md")
	}
	if strings.TrimSpace(options.JSONPath) == "" {
		options.JSONPath = filepath.Join(DefaultWorkspaceRoot, "outputs", "latest-report.json")
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
