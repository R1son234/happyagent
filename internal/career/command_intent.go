package career

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"happyagent/internal/store"
)

type NaturalLanguageResult struct {
	Output         string
	GeneratedPaths []string
	PrimaryPath    string
	OutputPaths    UserOutputPaths
	RunSummaryPath string
}

func handleNaturalLanguageInput(deps Dependencies, workspace *Workspace, sessionID string, input string) error {
	_, err := executeNaturalLanguageInput(context.Background(), deps, workspace, sessionID, input)
	return err
}

func executeNaturalLanguageInput(ctx context.Context, deps Dependencies, workspace *Workspace, sessionID string, input string) (NaturalLanguageResult, error) {
	decision, candidates, err := decideNaturalLanguageInput(ctx, deps, workspace, input)
	if err != nil {
		return NaturalLanguageResult{}, err
	}
	if decision.Intent == CareerIntentStatus {
		return NaturalLanguageResult{}, printWorkspaceStatus(deps.Stdout, workspace)
	}
	if decision.NeedsUserConfirmation || decision.Confidence != ConfidenceHigh {
		message := confirmationMessage(decision)
		fmt.Fprintln(deps.Stdout, "assistant> "+message)
		return NaturalLanguageResult{Output: message}, nil
	}

	autoArchived, ingestErrors, err := executeSemanticMaterialActions(ctx, deps.Stdout, workspace, input, decision, candidates)
	if err != nil {
		return NaturalLanguageResult{}, err
	}
	if decision.ShouldScanInbox {
		paths, inboxErr := DiscoverInboxFiles(workspace)
		if inboxErr != nil {
			return NaturalLanguageResult{}, inboxErr
		}
		if len(paths) > 0 {
			ingestErrors = append(ingestErrors, fmt.Sprintf("发现 %d 个 inbox 文件；请在桌面端或 inbox 分类流程中确认整理。", len(paths)))
		}
	}
	if decision.Intent == CareerIntentIngest && len(decision.RequestedOutputs) == 0 {
		return NaturalLanguageResult{}, printIngestSummary(deps.Stdout, workspace, autoArchived, ingestErrors)
	}
	return handleDecisionWithModelTurn(ctx, deps, workspace, sessionID, input, decision, autoArchived, ingestErrors)
}

func decideNaturalLanguageInput(ctx context.Context, deps Dependencies, workspace *Workspace, input string) (UserInputSemanticDecision, []SemanticFileCandidate, error) {
	guide, err := workspace.LoadGuide()
	if err != nil {
		return UserInputSemanticDecision{}, nil, err
	}
	meta, err := workspace.ReadMetadata()
	if err != nil {
		return UserInputSemanticDecision{}, nil, err
	}
	inbox, err := workspaceInboxView(workspace)
	if err != nil {
		return UserInputSemanticDecision{}, nil, err
	}
	candidates, warnings := collectSemanticFileCandidates(ctx, workspace, input)
	if len(warnings) > 0 {
		fmt.Fprintln(deps.Stderr, strings.Join(warnings, "\n"))
	}
	runner := &appStructuredTaskRunner{
		App:       deps.App,
		Config:    deps.Config,
		SessionID: "",
	}
	decision, err := ClassifyUserInputWithLLM(ctx, runner, SemanticDecisionRequest{
		Input:         input,
		Guide:         guide,
		Metadata:      meta,
		InboxCounts:   inbox.Counts,
		PendingItems:  inbox.PendingItems,
		Candidates:    candidates,
		WorkspaceRoot: workspace.Root,
	})
	if err != nil {
		return UserInputSemanticDecision{}, nil, fmt.Errorf("classify user input with LLM: %w", err)
	}
	return decision, candidates, nil
}

func workspaceInboxView(workspace *Workspace) (InboxView, error) {
	state, err := workspace.ReadInboxState()
	if err != nil {
		return InboxView{}, err
	}
	counts := map[string]int{}
	for _, item := range state.Items {
		counts[item.Status]++
	}
	paths, err := DiscoverInboxFiles(workspace)
	if err != nil {
		return InboxView{}, err
	}
	counts[InboxItemStatusUnclassified] += len(paths)
	return InboxView{PendingItems: state.Items, Counts: counts}, nil
}

func executeSemanticMaterialActions(ctx context.Context, output anyWriter, workspace *Workspace, input string, decision UserInputSemanticDecision, candidates []SemanticFileCandidate) ([]WorkspaceItem, []string, error) {
	var archived []WorkspaceItem
	var warnings []string
	if decision.ShouldSaveUserInput {
		itemType, ok := workspaceTypeForDecisionMaterial(decision.UserInputMaterialType)
		if !ok || itemType == WorkspaceTypeRecord {
			warnings = append(warnings, "用户输入材料类型不明确，未自动保存。")
		} else {
			item, err := saveMaterialFromDecision(workspace, itemType, input, decision)
			if err != nil {
				return archived, warnings, err
			}
			archived = append(archived, item)
			fmt.Fprintf(output, "assistant> 已识别并归档为 %s：%s\n", displayWorkspaceType(item.Type), item.Path)
		}
	}
	candidatesByID := map[string]SemanticFileCandidate{}
	for _, candidate := range candidates {
		candidatesByID[candidate.ID] = candidate
	}
	for _, fileDecision := range decision.ReferencedFiles {
		if fileDecision.Action != SemanticFileActionInclude {
			continue
		}
		if fileDecision.NeedsUserConfirmation || fileDecision.Confidence != ConfidenceHigh {
			warnings = append(warnings, fmt.Sprintf("%s 需要确认后归档：%s", fileDecision.SourcePath, fileDecision.Reason))
			continue
		}
		candidate, ok := candidatesByID[fileDecision.CandidateID]
		if !ok {
			warnings = append(warnings, fmt.Sprintf("%s 不是有效候选文件，已跳过。", fileDecision.SourcePath))
			continue
		}
		result, err := IngestFile(ctx, workspace, IngestRequest{
			Path:      firstNonEmpty(candidate.ReadPath, candidate.SourcePath),
			Decision:  fileDecision,
			UserInput: input,
			Now:       time.Now(),
		})
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %s", fileDecision.SourcePath, err.Error()))
			continue
		}
		archived = append(archived, result.Item)
		fmt.Fprintf(output, "assistant> 已自动归档 %s：%s -> %s\n", displayWorkspaceType(result.ItemType), fileDecision.SourcePath, result.Item.Path)
	}
	return archived, warnings, nil
}

type anyWriter interface {
	Write(p []byte) (n int, err error)
}

func handleDecisionWithModelTurn(ctx context.Context, deps Dependencies, workspace *Workspace, sessionID string, input string, decision UserInputSemanticDecision, autoArchived []WorkspaceItem, ingestErrors []string) (NaturalLanguageResult, error) {
	meta, err := workspace.ReadMetadata()
	if err != nil {
		return NaturalLanguageResult{}, err
	}
	if message := readinessMessageForDecision(workspace, meta, decision); message != "" {
		fmt.Fprintln(deps.Stdout, message)
		return NaturalLanguageResult{Output: strings.TrimPrefix(message, "assistant> ")}, nil
	}
	guide, err := workspace.LoadGuide()
	if err != nil {
		return NaturalLanguageResult{}, err
	}
	record, err := runCareerTurn(ctx, deps, sessionID, BuildInteractivePromptWithDecision(input, decision, autoArchived, ingestErrors, meta, workspace.Root, guide), decision)
	if err != nil {
		if record.ID != "" {
			fmt.Fprintf(deps.Stderr, "run_id=%s session_id=%s\n", record.ID, record.SessionID)
		}
		return NaturalLanguageResult{}, fmt.Errorf("run career turn: %w", err)
	}
	fmt.Fprintf(deps.Stderr, "run_id=%s session_id=%s\n", record.ID, record.SessionID)
	fmt.Fprintf(deps.Stdout, "assistant> %s\n", record.Output)

	outputs := executableOutputs(decision)
	if len(outputs) == 0 {
		return NaturalLanguageResult{Output: record.Output}, nil
	}
	return persistDecisionOutputs(deps, workspace, sessionID, decision, outputs, record, meta, autoArchived)
}

func persistDecisionOutputs(deps Dependencies, workspace *Workspace, sessionID string, decision UserInputSemanticDecision, outputs []RequestedOutputDecision, record store.RunRecord, meta WorkspaceMetadata, autoArchived []WorkspaceItem) (NaturalLanguageResult, error) {
	now := time.Now()
	var generatedPaths []string
	var primaryPath string
	var outputPaths UserOutputPaths
	for _, requested := range outputs {
		switch requested.Kind {
		case OutputKindReviewLibrary:
			result, err := generateReviewLibraryWithLLM(deps, workspace, sessionID, now)
			if err != nil {
				return NaturalLanguageResult{}, err
			}
			generatedPaths = append(generatedPaths, result.Paths...)
			primaryPath = firstNonEmpty(primaryPath, firstString(result.Paths))
		case OutputKindProjectPack, OutputKindBattlePack:
			generated, err := runServiceGeneratedOutput(context.Background(), deps, workspace, requested)
			if err != nil {
				return NaturalLanguageResult{}, err
			}
			generatedPaths = append(generatedPaths, generated.GeneratedPaths...)
			primaryPath = firstNonEmpty(primaryPath, generated.PrimaryPath, generated.Path)
		case OutputKindReport, OutputKindResumeReview, OutputKindInterviewBrief, OutputKindGapPlan, OutputKindInterviewReview:
			jsonContent := []byte(nil)
			if requested.Kind == OutputKindReport {
				payload := map[string]string{
					"run_id":         recordID(record),
					"session_id":     recordSessionID(record),
					"intent":         string(decision.Intent),
					"output":         record.Output,
					"current_jd":     meta.ActiveJD,
					"current_resume": meta.CurrentResume,
				}
				data, marshalErr := json.MarshalIndent(payload, "", "  ")
				if marshalErr == nil {
					jsonContent = data
				}
			}
			title := firstNonEmpty(requested.Title, defaultOutputTitle(requested.Kind))
			paths, err := workspace.WriteUserOutput(requested.Kind, title, record.Output, jsonContent, now)
			if err != nil {
				return NaturalLanguageResult{}, err
			}
			outputPaths = paths
			generatedPaths = appendGeneratedPaths(paths, generatedPaths)
			primaryPath = firstNonEmpty(primaryPath, paths.LatestMarkdown)
			printCompletionSummary(deps.Stdout, title, collectedInputPaths(workspace.Root, meta, autoArchived), paths)
		}
	}
	generatedPaths = uniqueStrings(generatedPaths)
	runSummaryPath, _ := workspace.WriteRunSummary(RunSummaryRecord{
		TaskName:    string(decision.Intent),
		Status:      RunSummaryStatusSuccess,
		CreatedAt:   now,
		InputPaths:  collectedInputPaths(workspace.Root, meta, autoArchived),
		Generated:   generatedPaths,
		PrimaryPath: primaryPath,
		NextActions: []string{"检查输出报告和新增资料", "如资料不足，可继续补充简历、JD、面经或面试复盘"},
	})
	generatedPaths = uniqueStrings(append(generatedPaths, runSummaryPath))
	return NaturalLanguageResult{
		Output:         record.Output,
		GeneratedPaths: generatedPaths,
		PrimaryPath:    firstNonEmpty(runSummaryPath, primaryPath),
		OutputPaths:    outputPaths,
		RunSummaryPath: runSummaryPath,
	}, nil
}

func recordID(record store.RunRecord) string {
	return record.ID
}

func recordSessionID(record store.RunRecord) string {
	return record.SessionID
}

func executableOutputs(decision UserInputSemanticDecision) []RequestedOutputDecision {
	var outputs []RequestedOutputDecision
	for _, output := range decision.RequestedOutputs {
		if output.Kind == OutputKindChat || strings.TrimSpace(output.Kind) == "" {
			continue
		}
		outputs = append(outputs, output)
	}
	return outputs
}

func runServiceGeneratedOutput(ctx context.Context, deps Dependencies, workspace *Workspace, requested RequestedOutputDecision) (GeneratedDocumentResult, error) {
	service := &CopilotService{WorkspaceRoot: workspace.Root, App: deps.App, Config: deps.Config}
	switch requested.Kind {
	case OutputKindProjectPack:
		return service.GenerateProjectPack(ctx, GenerateProjectPackRequest{})
	case OutputKindBattlePack:
		meta, err := workspace.ReadMetadata()
		if err != nil {
			return GeneratedDocumentResult{}, err
		}
		return service.GenerateBattlePack(ctx, GenerateBattlePackRequest{JDPath: meta.ActiveJD, SourcePaths: []string{meta.CurrentResume}})
	default:
		return GeneratedDocumentResult{}, fmt.Errorf("unsupported generated output kind %q", requested.Kind)
	}
}

func saveMaterialFromDecision(workspace *Workspace, itemType string, content string, decision UserInputSemanticDecision) (WorkspaceItem, error) {
	if itemType == WorkspaceTypeExperiences {
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
	classification := InputClassification{
		Type:       itemType,
		Confidence: confidenceFloat(decision.Confidence),
		ShouldSave: true,
		Reason:     decision.Reason,
		RulePath:   classificationRulePath(guide, itemType),
		Meta:       decision.Meta,
	}
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

func readinessMessageForDecision(workspace *Workspace, meta WorkspaceMetadata, decision UserInputSemanticDecision) string {
	required := map[string]bool{}
	for _, value := range decision.RequiredState {
		required[strings.TrimSpace(value)] = true
	}
	for _, output := range decision.RequestedOutputs {
		switch output.Kind {
		case OutputKindReport:
			required["current_resume"] = true
			required["active_jd"] = true
		case OutputKindResumeReview:
			required["current_resume"] = true
		case OutputKindInterviewBrief, OutputKindGapPlan, OutputKindBattlePack:
			required["active_jd"] = true
		case OutputKindProjectPack:
			required["current_resume"] = true
		}
	}
	inbox := filepath.ToSlash(filepath.Join(workspace.Root, "inbox"))
	if required["current_resume"] && strings.TrimSpace(meta.CurrentResume) == "" && required["active_jd"] && strings.TrimSpace(meta.ActiveJD) == "" {
		return fmt.Sprintf("assistant> 现在还缺少简历和 JD。请把资料放到 %s，然后直接说你希望我怎么整理或分析。", inbox)
	}
	if required["current_resume"] && strings.TrimSpace(meta.CurrentResume) == "" {
		return fmt.Sprintf("assistant> 现在还缺少简历。请把资料放到 %s，然后直接说你希望我怎么整理或分析。", inbox)
	}
	if required["active_jd"] && strings.TrimSpace(meta.ActiveJD) == "" {
		return fmt.Sprintf("assistant> 现在还缺少 JD。请把资料放到 %s，然后直接说你希望我怎么整理或分析。", inbox)
	}
	return ""
}

func confirmationMessage(decision UserInputSemanticDecision) string {
	if len(decision.QuestionsForUser) > 0 {
		return strings.Join(decision.QuestionsForUser, " ")
	}
	if strings.TrimSpace(decision.Reason) != "" {
		return "我还需要你确认一下：" + decision.Reason
	}
	return "我还需要你确认材料类型或下一步动作。"
}

func workspaceTypeForDecisionMaterial(materialType string) (string, bool) {
	if itemType, ok := workspaceTypeForMaterialType(materialType); ok {
		return itemType, true
	}
	itemType := strings.ToLower(strings.TrimSpace(materialType))
	if IsSupportedWorkspaceType(itemType) && itemType != WorkspaceTypeGeneral {
		return itemType, true
	}
	return "", false
}

func defaultOutputTitle(kind string) string {
	switch kind {
	case OutputKindReport:
		return "完整匹配报告"
	case OutputKindResumeReview:
		return "简历优化建议"
	case OutputKindInterviewBrief:
		return "面试准备材料"
	case OutputKindGapPlan:
		return "能力差距计划"
	case OutputKindInterviewReview:
		return "面试复盘"
	default:
		return "求职助手输出"
	}
}

func confidenceFloat(confidence ConfidenceLevel) float64 {
	switch confidence {
	case ConfidenceHigh:
		return 0.95
	case ConfidenceMedium:
		return 0.65
	case ConfidenceLow:
		return 0.35
	default:
		return 0
	}
}

func normalizeWorkspaceType(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func displayWorkspaceType(itemType string) string {
	return workspaceTypeDisplayName(itemType)
}

func collectSemanticFileCandidates(ctx context.Context, workspace *Workspace, input string) ([]SemanticFileCandidate, []string) {
	_ = ctx
	seen := map[string]bool{}
	var candidates []SemanticFileCandidate
	var warnings []string
	addPath := func(path string) {
		path = strings.TrimSpace(path)
		if path == "" || seen[path] {
			return
		}
		seen[path] = true
		candidate, err := buildSemanticFileCandidate(len(candidates)+1, path)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %v", path, err))
			return
		}
		candidates = append(candidates, candidate)
	}
	for _, path := range extractReferencedFiles(input) {
		addPath(path)
	}
	for _, dir := range extractReferencedDirectories(input) {
		paths, err := listDirectoryCandidateFiles(dir, 30)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %v", dir, err))
			continue
		}
		for _, path := range paths {
			addPath(path)
		}
	}
	return candidates, warnings
}

func listDirectoryCandidateFiles(dir string, limit int) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	type fileEntry struct {
		path    string
		modTime time.Time
	}
	var files []fileEntry
	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if !isSupportedIngestExt(ext) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, fileEntry{path: filepath.Join(dir, entry.Name()), modTime: info.ModTime()})
	}
	sort.SliceStable(files, func(i, j int) bool {
		return files[i].modTime.After(files[j].modTime)
	})
	if limit > 0 && len(files) > limit {
		files = files[:limit]
	}
	paths := make([]string, 0, len(files))
	for _, file := range files {
		paths = append(paths, file.path)
	}
	return paths, nil
}

func buildSemanticFileCandidate(index int, path string) (SemanticFileCandidate, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return SemanticFileCandidate{}, err
	}
	info, err := os.Stat(absPath)
	if err != nil {
		return SemanticFileCandidate{}, err
	}
	if info.IsDir() {
		return SemanticFileCandidate{}, fmt.Errorf("directory is not a file")
	}
	extracted, extractErr := extractDocument(context.Background(), absPath)
	excerpt := ""
	extractStatus := "ok"
	extractError := ""
	hash := ""
	if extractErr != nil {
		extractStatus = "failed"
		extractError = extractErr.Error()
	} else {
		excerpt = limitSemanticExcerpt(extracted.Text)
		hash = "sha256:" + ContentFingerprint(extracted.Text)
	}
	return SemanticFileCandidate{
		ID:            fmt.Sprintf("file-%d", index),
		SourcePath:    filepath.ToSlash(path),
		ReadPath:      filepath.ToSlash(absPath),
		SourceHash:    hash,
		Name:          filepath.Base(path),
		Ext:           strings.ToLower(filepath.Ext(path)),
		Size:          info.Size(),
		ModifiedUnix:  info.ModTime().Unix(),
		ExtractStatus: extractStatus,
		ExtractError:  extractError,
		Excerpt:       excerpt,
	}, nil
}

func limitSemanticExcerpt(content string) string {
	content = strings.TrimSpace(content)
	const maxRunes = 1200
	runes := []rune(content)
	if len(runes) <= maxRunes {
		return content
	}
	return string(runes[:maxRunes]) + "\n...[truncated]"
}
