package career

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

func handleNaturalLanguageInput(deps Dependencies, workspace *Workspace, sessionID string, input string) error {
	intent := ClassifyIntent(input)

	// Memory intent: skip inbox scan and archive, route directly to model with memory-focused prompt.
	if isMemoryIntent(intent) {
		classification := ClassifyInputWithGuide(input, WorkspaceGuide{})
		classification.Type = string(CareerIntentMemory)
		return handleIntentWithModelTurn(deps, workspace, sessionID, input, intent, classification, nil, nil)
	}

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

func shouldScanInbox(intent IntentClassification) bool {
	if intent.Intent == CareerIntentIngest {
		return true
	}
	// Also scan when the input explicitly mentions inbox/import signals,
	// even if the classified intent is analyze or another type.
	for _, signal := range intent.Signals {
		switch strings.ToLower(signal) {
		case "inbox", "放进 inbox", "放进来了", "导入", "扫描", "scan", "记录下来", "存下来", "保存":
			return true
		}
	}
	return false
}

func shouldGenerateOutput(intent CareerIntent) bool {
	switch intent {
	case CareerIntentAnalyze, CareerIntentResumeReview, CareerIntentInterviewBrief, CareerIntentGapPlan, CareerIntentInterviewReview:
		return true
	default:
		return false
	}
}

func isMemoryIntent(intent IntentClassification) bool {
	return intent.Intent == CareerIntentMemory
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

func normalizeWorkspaceType(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func displayWorkspaceType(itemType string) string {
	return workspaceTypeDisplayName(itemType)
}
