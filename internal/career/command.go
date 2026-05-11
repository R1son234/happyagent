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
	guide, err := workspace.LoadGuide()
	if err != nil {
		return err
	}
	classification := ClassifyInputWithGuide(content, guide)
	classification.Type = itemType
	classification.ShouldSave = true
	classification.Reason = "explicit add command"
	classification.RulePath = classificationRulePath(guide, itemType)
	result, err := workspace.AddGuidedMaterial(GuidedMaterialInput{
		ItemType:       itemType,
		Classification: classification,
		Content:        content,
		SourceLabel:    "/add " + itemType,
		Now:            time.Now(),
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(output, "assistant> %s，并保存到 %s；分类记录写入 %s。我已经更新工作区索引，可以基于这些资料做匹配分析、简历优化和面试准备。\n", prefix, result.Item.Path, result.RecordRel)
	return nil
}

func saveMaterialFileAndPrint(output io.Writer, workspace *Workspace, input WorkspaceFileInput, prefix string) error {
	guide, err := workspace.LoadGuide()
	if err != nil {
		return err
	}
	classification := ClassifyInputWithSignals(input.Text, guide, input.OriginalName, input.ItemType, "")
	result, err := workspace.AddGuidedMaterial(GuidedMaterialInput{
		ItemType:       input.ItemType,
		Classification: classification,
		SourceLabel:    input.OriginalPath,
		Now:            input.Now,
		File:           input,
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(output, "assistant> %s，并保存到 %s；分类记录写入 %s。我已经更新工作区索引，可以基于这些资料做匹配分析、简历优化和面试准备。\n", prefix, result.Item.Path, result.RecordRel)
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
	return detectWorkspaceTypeHintNearPathWithGuide(input, path, DefaultWorkspaceGuide())
}

func detectWorkspaceTypeHintNearPathWithGuide(input string, path string, guide WorkspaceGuide) string {
	if strings.TrimSpace(path) == "" {
		return detectWorkspaceTypeBySignalsWithGuide(input, guide)
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
	if hinted := detectWorkspaceTypeBySignalsWithGuide(window, guide); hinted != "" {
		return hinted
	}
	return detectWorkspaceTypeBySignalsWithGuide(input, guide)
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

func handleNaturalLanguageInput(deps Dependencies, workspace *Workspace, sessionID string, input string) error {
	intent := ClassifyIntent(input)
	guide, err := workspace.LoadGuide()
	if err != nil {
		return err
	}
	classification := ClassifyInputWithGuide(input, guide)
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
	guide, err := workspace.LoadGuide()
	if err != nil {
		return WorkspaceItem{}, err
	}
	classification := ClassifyInputWithGuide(content, guide)
	classification.Type = itemType
	classification.RulePath = classificationRulePath(guide, itemType)
	result, err := workspace.AddGuidedMaterial(GuidedMaterialInput{
		ItemType:       itemType,
		Classification: classification,
		Content:        content,
		SourceLabel:    "natural_language_input",
		Now:            time.Now(),
	})
	if err != nil {
		return WorkspaceItem{}, err
	}
	return result.Item, nil
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
	guide, err := workspace.LoadGuide()
	if err != nil {
		return err
	}
	record, err := runCareerTurn(deps, sessionID, BuildInteractivePromptWithAutoSavedAndGuide(input, classification, autoArchived, ingestErrors, meta, shouldGenerateOutput(intent.Intent), workspace.Root, guide), classification)
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
