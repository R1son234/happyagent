package career

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"happyagent/internal/config"
)

type CopilotService struct {
	WorkspaceRoot string
	App           Application
	Config        config.Config
	Now           func() time.Time
	TaskRunner    StructuredTaskRunner
}

type InboxView struct {
	Files        []InboxFileView    `json:"files"`
	PendingItems []PendingInboxItem `json:"pending_items"`
	Counts       map[string]int     `json:"counts"`
}

type InboxFileView struct {
	Path     string    `json:"path"`
	Name     string    `json:"name"`
	Size     int64     `json:"size"`
	Modified time.Time `json:"modified"`
	Status   string    `json:"status"`
}

type ClassifyInboxRequest struct {
	Paths []string `json:"paths"`
}

type ClassifyInboxResult struct {
	Items    []PendingInboxItem `json:"items"`
	Warnings []string           `json:"warnings"`
}

type ConfirmMaterialRequest struct {
	ID           string `json:"id"`
	MaterialType string `json:"material_type"`
	Destination  string `json:"destination"`
}

type ConfirmMaterialResult struct {
	Item         WorkspaceItem `json:"item"`
	RecordPath   string        `json:"record_path"`
	RemovedInbox bool          `json:"removed_inbox"`
}

type SplitJDRequest struct {
	SourcePath string `json:"source_path"`
}

type SplitJDResult struct {
	Items        []WorkspaceItem    `json:"items"`
	PendingItems []PendingInboxItem `json:"pending_items"`
	Warnings     []string           `json:"warnings"`
}

type GenerateReviewLibraryRequest struct {
	SourcePaths []string `json:"source_paths"`
}

type GenerateProjectPackRequest struct {
	ProjectName string   `json:"project_name"`
	SourcePaths []string `json:"source_paths"`
}

type GenerateBattlePackRequest struct {
	JDPath      string   `json:"jd_path"`
	SourcePaths []string `json:"source_paths"`
}

type ExtractInterviewReviewRequest struct {
	Target      string   `json:"target"`
	SourcePaths []string `json:"source_paths"`
}

type GeneratedDocumentResult struct {
	Path     string                  `json:"path"`
	Record   GeneratedArtifactRecord `json:"record"`
	Warnings []string                `json:"warnings"`
}

type ListDiagnosticsRequest struct {
	Limit int `json:"limit"`
}

type ListDiagnosticsResult struct {
	Items []DiagnosticRecord `json:"items"`
}

func (s *CopilotService) ListInbox(ctx context.Context) (InboxView, error) {
	_ = ctx
	ws, err := OpenWorkspace(s.workspaceRoot(), s.now())
	if err != nil {
		return InboxView{}, err
	}
	state, err := ws.ReadInboxState()
	if err != nil {
		return InboxView{}, err
	}
	statusByPath := map[string]string{}
	filePathSeen := map[string]bool{}
	for _, item := range state.Items {
		statusByPath[filepath.ToSlash(item.SourcePath)] = item.Status
	}
	inboxRoot := filepath.Join(ws.Root, "inbox")
	entries, err := os.ReadDir(inboxRoot)
	if err != nil {
		if os.IsNotExist(err) {
			err = nil
		} else {
			return InboxView{}, err
		}
	}
	files := make([]InboxFileView, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == "" || entry.Name()[0] == '.' {
			continue
		}
		info, statErr := entry.Info()
		if statErr != nil {
			continue
		}
		rel := filepath.ToSlash(filepath.Join("inbox", entry.Name()))
		status := statusByPath[rel]
		if status == "" {
			status = InboxItemStatusUnclassified
		}
		files = append(files, InboxFileView{
			Path:     rel,
			Name:     entry.Name(),
			Size:     info.Size(),
			Modified: info.ModTime(),
			Status:   status,
		})
		filePathSeen[rel] = true
	}
	counts := map[string]int{}
	for _, file := range files {
		counts[file.Status]++
	}
	for _, item := range state.Items {
		if filePathSeen[filepath.ToSlash(item.SourcePath)] {
			continue
		}
		counts[item.Status]++
	}
	return InboxView{Files: files, PendingItems: state.Items, Counts: counts}, nil
}

func (s *CopilotService) ClassifyInbox(ctx context.Context, req ClassifyInboxRequest) (ClassifyInboxResult, error) {
	ws, err := OpenWorkspace(s.workspaceRoot(), s.now())
	if err != nil {
		return ClassifyInboxResult{}, err
	}
	targets, warnings, err := s.prepareInboxClassificationFiles(ctx, ws, req.Paths)
	if err != nil {
		return ClassifyInboxResult{}, err
	}
	if len(targets) == 0 {
		return ClassifyInboxResult{Warnings: warnings}, nil
	}
	runner := s.structuredTaskRunner()
	result, err := runner.RunStructuredTask(ctx, StructuredTaskRequest{
		TaskName:      "classify_inbox",
		PromptVersion: PromptVersionFileClassification,
		Input:         buildFileClassificationPrompt(targets),
		SourcePaths:   inboxClassificationSourcePaths(targets),
	})
	if err != nil {
		return ClassifyInboxResult{}, err
	}
	parsed, err := parseFileClassificationOutput(result.Output)
	if err != nil {
		return ClassifyInboxResult{}, err
	}
	byPath := map[string]InboxFileForClassification{}
	for _, target := range targets {
		byPath[filepath.ToSlash(target.SourcePath)] = target
	}
	state, err := ws.ReadInboxState()
	if err != nil {
		return ClassifyInboxResult{}, err
	}
	var items []PendingInboxItem
	for _, classified := range parsed.Files {
		sourcePath := filepath.ToSlash(classified.SourcePath)
		source, ok := byPath[sourcePath]
		if !ok {
			return ClassifyInboxResult{}, fmt.Errorf("classification returned unknown source_path %q", classified.SourcePath)
		}
		item := PendingInboxItem{
			ID:                    pendingInboxID(sourcePath, source.SourceHash),
			SourcePath:            sourcePath,
			SourceHash:            source.SourceHash,
			OriginalName:          filepath.Base(sourcePath),
			MaterialType:          strings.TrimSpace(classified.MaterialType),
			Destination:           strings.TrimSpace(classified.Destination),
			Confidence:            classified.Confidence,
			Reason:                strings.TrimSpace(classified.Reason),
			SourceExcerpt:         strings.TrimSpace(classified.SourceExcerpt),
			NeedsUserConfirmation: classified.NeedsUserConfirmation || classified.Confidence != ConfidenceHigh,
			QuestionsForUser:      classified.QuestionsForUser,
			Status:                InboxItemStatusPending,
			Meta: LLMTraceMeta{
				GeneratedAt:   result.GeneratedAt,
				Model:         result.Model,
				PromptVersion: PromptVersionFileClassification,
			},
		}
		if item.Confidence == ConfidenceHigh && !item.NeedsUserConfirmation {
			confirmed, _, confirmErr := s.confirmClassifiedInboxItem(ctx, ws, item, source.Content)
			if confirmErr != nil {
				return ClassifyInboxResult{}, confirmErr
			}
			item = confirmed
		}
		state = upsertInboxStateItem(state, item)
		items = append(items, item)
	}
	if err := ws.WriteInboxState(state); err != nil {
		return ClassifyInboxResult{}, err
	}
	return ClassifyInboxResult{Items: items, Warnings: warnings}, nil
}

func (s *CopilotService) ConfirmMaterialClassification(ctx context.Context, req ConfirmMaterialRequest) (ConfirmMaterialResult, error) {
	ws, err := OpenWorkspace(s.workspaceRoot(), s.now())
	if err != nil {
		return ConfirmMaterialResult{}, err
	}
	state, err := ws.ReadInboxState()
	if err != nil {
		return ConfirmMaterialResult{}, err
	}
	req.ID = strings.TrimSpace(req.ID)
	if req.ID == "" {
		return ConfirmMaterialResult{}, fmt.Errorf("confirm material id must not be empty")
	}
	itemIndex := -1
	var item PendingInboxItem
	for i, candidate := range state.Items {
		if candidate.ID == req.ID {
			itemIndex = i
			item = candidate
			break
		}
	}
	if itemIndex < 0 {
		return ConfirmMaterialResult{}, fmt.Errorf("pending inbox item %q not found", req.ID)
	}
	if item.Status == InboxItemStatusConfirmed {
		return ConfirmMaterialResult{}, fmt.Errorf("pending inbox item %q is already confirmed", req.ID)
	}
	if req.MaterialType != "" {
		item.MaterialType = strings.TrimSpace(req.MaterialType)
	}
	if req.Destination != "" {
		item.Destination = strings.TrimSpace(req.Destination)
	}
	if _, ok := workspaceTypeForMaterialType(item.MaterialType); !ok {
		return ConfirmMaterialResult{}, fmt.Errorf("material_type %q is not supported", item.MaterialType)
	}
	sourcePath := filepath.ToSlash(strings.TrimSpace(item.SourcePath))
	if err := validateWorkspaceRelPath(sourcePath); err != nil {
		return ConfirmMaterialResult{}, fmt.Errorf("confirm source path: %w", err)
	}
	if !strings.HasPrefix(sourcePath, "inbox/") {
		return ConfirmMaterialResult{}, fmt.Errorf("confirm source path %q must be inside inbox", sourcePath)
	}
	absSource := filepath.Join(ws.Root, filepath.FromSlash(sourcePath))
	extracted, err := extractDocument(ctx, absSource)
	if err != nil {
		item.Status = InboxItemStatusFailed
		item.Meta.GeneratedAt = s.now()
		state.Items[itemIndex] = item
		_ = ws.WriteInboxState(state)
		return ConfirmMaterialResult{}, err
	}
	confirmed, guidedResult, err := s.confirmClassifiedInboxItem(ctx, ws, item, extracted.Text)
	if err != nil {
		item.Status = InboxItemStatusFailed
		item.Meta.GeneratedAt = s.now()
		state.Items[itemIndex] = item
		_ = ws.WriteInboxState(state)
		return ConfirmMaterialResult{}, err
	}
	state.Items[itemIndex] = confirmed
	if err := ws.WriteInboxState(state); err != nil {
		return ConfirmMaterialResult{}, err
	}
	return ConfirmMaterialResult{
		Item:         guidedResult.Item,
		RecordPath:   guidedResult.RecordRel,
		RemovedInbox: true,
	}, nil
}

func (s *CopilotService) SplitJobDescriptions(ctx context.Context, req SplitJDRequest) (SplitJDResult, error) {
	ws, err := OpenWorkspace(s.workspaceRoot(), s.now())
	if err != nil {
		return SplitJDResult{}, err
	}
	sourcePath := filepath.ToSlash(strings.TrimSpace(req.SourcePath))
	if err := validateWorkspaceRelPath(sourcePath); err != nil {
		return SplitJDResult{}, err
	}
	content, err := readWorkspaceText(ws, sourcePath)
	if err != nil {
		_ = s.writeDiagnostic(ws, DiagnosticRecord{TaskName: "split_jd", PromptVersion: PromptVersionJDSplit, SourcePaths: []string{sourcePath}, Error: err.Error()})
		return SplitJDResult{}, err
	}
	result, err := s.structuredTaskRunner().RunStructuredTask(ctx, StructuredTaskRequest{
		TaskName:      "split_jd",
		PromptVersion: PromptVersionJDSplit,
		Input:         buildJDSplitPrompt(sourcePath, content),
		SourcePaths:   []string{sourcePath},
	})
	if err != nil {
		_ = s.writeDiagnostic(ws, DiagnosticRecord{TaskName: "split_jd", PromptVersion: PromptVersionJDSplit, SourcePaths: []string{sourcePath}, Error: err.Error()})
		return SplitJDResult{}, err
	}
	parsed, err := parseJDSplitOutput(result.Output)
	if err != nil {
		_ = s.writeDiagnostic(ws, DiagnosticRecord{TaskName: "split_jd", PromptVersion: PromptVersionJDSplit, SourcePaths: []string{sourcePath}, Error: err.Error(), RawOutput: result.Output})
		return SplitJDResult{}, err
	}
	state, err := ws.ReadInboxState()
	if err != nil {
		return SplitJDResult{}, err
	}
	var items []WorkspaceItem
	var pending []PendingInboxItem
	for _, jd := range parsed.JobDescriptions {
		body := renderSplitJDMarkdown(jd, sourcePath)
		hash := "sha256:" + ContentFingerprint(body)
		pendingItem := PendingInboxItem{
			ID:                    pendingInboxID(sourcePath+"-"+jd.Title, hash),
			SourcePath:            sourcePath,
			SourceHash:            hash,
			OriginalName:          filepath.Base(sourcePath),
			MaterialType:          "jd",
			Destination:           WorkspaceDirJD,
			Confidence:            jd.Confidence,
			Reason:                "LLM 拆分 JD：" + jd.Title,
			SourceExcerpt:         jd.SourceExcerpt,
			NeedsUserConfirmation: jd.Confidence != ConfidenceHigh,
			Status:                InboxItemStatusPending,
			Meta:                  LLMTraceMeta{GeneratedAt: result.GeneratedAt, Model: result.Model, PromptVersion: PromptVersionJDSplit},
		}
		if jd.Confidence == ConfidenceHigh {
			item, addErr := ws.AddMaterialFromFile(WorkspaceFileInput{
				ItemType:           WorkspaceTypeJD,
				Title:              jd.Title,
				Text:               body,
				OriginalName:       filepath.Base(sourcePath),
				Now:                s.now(),
				Extractor:          "llm_jd_split",
				MIMEType:           "text/markdown",
				ExtractStatus:      "ok",
				ContentFingerprint: strings.TrimPrefix(hash, "sha256:"),
			})
			if addErr != nil {
				_ = s.writeDiagnostic(ws, DiagnosticRecord{TaskName: "split_jd", PromptVersion: PromptVersionJDSplit, SourcePaths: []string{sourcePath}, Error: addErr.Error(), RawOutput: result.Output})
				return SplitJDResult{}, addErr
			}
			pendingItem.Status = InboxItemStatusConfirmed
			pendingItem.NeedsUserConfirmation = false
			items = append(items, item)
		} else {
			pending = append(pending, pendingItem)
		}
		state = upsertInboxStateItem(state, pendingItem)
	}
	if err := ws.WriteInboxState(state); err != nil {
		return SplitJDResult{}, err
	}
	return SplitJDResult{Items: items, PendingItems: pending}, nil
}

func (s *CopilotService) GenerateReviewLibrary(ctx context.Context, req GenerateReviewLibraryRequest) (GeneratedDocumentResult, error) {
	return s.generateMarkdownDocument(ctx, generateMarkdownSpec{
		TaskName:      "generate_review_library",
		PromptVersion: PromptVersionReviewLibrary,
		Kind:          "question_bank",
		OutputDir:     filepath.Join(WorkspaceDirPrepare, "LLM题库"),
		FallbackName:  "复习资料库",
		SourcePaths:   req.SourcePaths,
		DefaultTypes:  []string{WorkspaceTypeExperiences, WorkspaceTypeJD, WorkspaceTypeResume, WorkspaceTypeProject, WorkspaceTypePrepare},
		Instructions:  "生成可复习的题库与增量复习资料。必须包含考点、标准答案、结合简历回答、追问问答、风险点和来源证据。",
	})
}

func (s *CopilotService) GenerateProjectPack(ctx context.Context, req GenerateProjectPackRequest) (GeneratedDocumentResult, error) {
	name := strings.TrimSpace(req.ProjectName)
	if name == "" {
		name = "项目专项"
	}
	return s.generateMarkdownDocument(ctx, generateMarkdownSpec{
		TaskName:      "generate_project_pack",
		PromptVersion: PromptVersionProjectPack,
		Kind:          "project_pack",
		OutputDir:     WorkspaceDirProjectPack,
		FallbackName:  name + "-interview-qa",
		SourcePaths:   req.SourcePaths,
		DefaultTypes:  []string{WorkspaceTypeResume, WorkspaceTypePrepare, WorkspaceTypeProject},
		Instructions:  "生成项目专项面试材料。必须包含一句话介绍、一分钟版本、三分钟版本、架构链路、关键模块、取舍、失败模式、数据证据、迁移讲法、高频追问；缺少证据必须写待补证据。",
	})
}

func (s *CopilotService) GenerateBattlePack(ctx context.Context, req GenerateBattlePackRequest) (GeneratedDocumentResult, error) {
	sourcePaths := append([]string{}, req.SourcePaths...)
	if strings.TrimSpace(req.JDPath) != "" {
		sourcePaths = append(sourcePaths, req.JDPath)
	}
	return s.generateMarkdownDocument(ctx, generateMarkdownSpec{
		TaskName:      "generate_battle_pack",
		PromptVersion: PromptVersionBattlePack,
		Kind:          "battle_pack",
		OutputDir:     WorkspaceDirMyInterviews,
		FallbackName:  "面试作战包",
		SourcePaths:   sourcePaths,
		DefaultTypes:  []string{WorkspaceTypeResume, WorkspaceTypeJD, WorkspaceTypeExperiences, WorkspaceTypeProject},
		RequireTypes:  []string{WorkspaceTypeResume, WorkspaceTypeJD},
		Instructions:  "生成岗位面试作战包。必须包含 JD 画像、候选人主线、证据矩阵、风险清单、临阵复习顺序、项目映射、反问问题和来源证据。",
	})
}

func (s *CopilotService) ExtractInterviewReview(ctx context.Context, req ExtractInterviewReviewRequest) (GeneratedDocumentResult, error) {
	name := strings.TrimSpace(req.Target)
	if name == "" {
		name = "真实面试复盘"
	}
	return s.generateMarkdownDocument(ctx, generateMarkdownSpec{
		TaskName:      "extract_interview_review",
		PromptVersion: PromptVersionInterviewReview,
		Kind:          "interview_review",
		OutputDir:     WorkspaceDirMyInterviews,
		FallbackName:  name,
		SourcePaths:   req.SourcePaths,
		DefaultTypes:  []string{WorkspaceTypeMyInterviews, WorkspaceTypeExperiences},
		Instructions:  "抽取真实面试复盘。必须区分用户原始回答和建议改写，包含公司岗位线索、轮次、问题、卡点、风险、下次建议回答和来源证据。",
	})
}

func (s *CopilotService) ListDiagnostics(ctx context.Context, req ListDiagnosticsRequest) (ListDiagnosticsResult, error) {
	_ = ctx
	ws, err := OpenWorkspace(s.workspaceRoot(), s.now())
	if err != nil {
		return ListDiagnosticsResult{}, err
	}
	limit := req.Limit
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	root := filepath.Join(ws.Root, WorkspaceInternalDir, WorkspaceDiagnosticsDir)
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return ListDiagnosticsResult{Items: []DiagnosticRecord{}}, nil
		}
		return ListDiagnosticsResult{}, err
	}
	var records []DiagnosticRecord
	for i := len(entries) - 1; i >= 0 && len(records) < limit; i-- {
		entry := entries[i]
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
			continue
		}
		var record DiagnosticRecord
		if err := ws.readJSON(filepath.Join(root, entry.Name()), &record); err != nil {
			continue
		}
		records = append(records, record)
	}
	return ListDiagnosticsResult{Items: records}, nil
}

func (s *CopilotService) prepareInboxClassificationFiles(ctx context.Context, ws *Workspace, paths []string) ([]InboxFileForClassification, []string, error) {
	_ = ctx
	discovered, err := DiscoverInboxFiles(ws)
	if err != nil {
		return nil, nil, err
	}
	allowed := map[string]bool{}
	if len(paths) > 0 {
		for _, path := range paths {
			rel := filepath.ToSlash(strings.TrimSpace(path))
			if err := validateWorkspaceRelPath(rel); err != nil {
				return nil, nil, fmt.Errorf("classify path %q: %w", path, err)
			}
			if !strings.HasPrefix(rel, "inbox/") {
				return nil, nil, fmt.Errorf("classify path %q must be inside inbox", path)
			}
			allowed[rel] = true
		}
	}
	var files []InboxFileForClassification
	var warnings []string
	for _, absPath := range discovered {
		rel, err := filepath.Rel(ws.Root, absPath)
		if err != nil {
			return nil, nil, err
		}
		rel = filepath.ToSlash(rel)
		if len(allowed) > 0 && !allowed[rel] {
			continue
		}
		extracted, err := extractDocument(ctx, absPath)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %v", rel, err))
			continue
		}
		files = append(files, InboxFileForClassification{
			SourcePath: rel,
			SourceHash: "sha256:" + ContentFingerprint(extracted.Text),
			Content:    extracted.Text,
		})
	}
	return files, warnings, nil
}

func (s *CopilotService) structuredTaskRunner() StructuredTaskRunner {
	if s.TaskRunner != nil {
		return s.TaskRunner
	}
	return &appStructuredTaskRunner{App: s.App, Config: s.Config, Now: s.Now}
}

func (s *CopilotService) confirmClassifiedInboxItem(ctx context.Context, ws *Workspace, item PendingInboxItem, content string) (PendingInboxItem, GuidedMaterialResult, error) {
	_ = ctx
	itemType, ok := workspaceTypeForMaterialType(item.MaterialType)
	if !ok {
		item.Status = InboxItemStatusPending
		item.NeedsUserConfirmation = true
		return item, GuidedMaterialResult{}, nil
	}
	absSource := filepath.Join(ws.Root, filepath.FromSlash(item.SourcePath))
	input := WorkspaceFileInput{
		ItemType:           itemType,
		Title:              strings.TrimSuffix(item.OriginalName, filepath.Ext(item.OriginalName)),
		Text:               content,
		OriginalPath:       absSource,
		OriginalName:       item.OriginalName,
		Now:                s.now(),
		Extractor:          "plain_text",
		MIMEType:           mimeTypeForExt(strings.ToLower(filepath.Ext(absSource))),
		ExtractStatus:      "ok",
		ContentFingerprint: strings.TrimPrefix(item.SourceHash, "sha256:"),
	}
	classification := InputClassification{
		Type:       itemType,
		Confidence: 0.95,
		ShouldSave: true,
		Reason:     firstNonEmpty(item.Reason, "LLM high confidence classification"),
	}
	result, err := ws.AddGuidedMaterial(GuidedMaterialInput{
		ItemType:       itemType,
		Classification: classification,
		File:           input,
		SourceLabel:    item.SourcePath,
		Now:            s.now(),
	})
	if err != nil {
		return PendingInboxItem{}, GuidedMaterialResult{}, err
	}
	if err := os.Remove(absSource); err != nil && !os.IsNotExist(err) {
		return PendingInboxItem{}, GuidedMaterialResult{}, err
	}
	if result.Item.Type == WorkspaceTypeResume {
		if err := markBattlePacksStaleForNewResume(ws, result.Item, s.now()); err != nil {
			return PendingInboxItem{}, GuidedMaterialResult{}, err
		}
	}
	item.Status = InboxItemStatusConfirmed
	item.NeedsUserConfirmation = false
	return item, result, nil
}

func workspaceTypeForMaterialType(materialType string) (string, bool) {
	switch strings.TrimSpace(materialType) {
	case "resume":
		return WorkspaceTypeResume, true
	case "jd":
		return WorkspaceTypeJD, true
	case "public_interview_experience":
		return WorkspaceTypeExperiences, true
	case "project_material":
		return WorkspaceTypeProject, true
	case "real_interview_record":
		return WorkspaceTypeMyInterviews, true
	case "review_note":
		return WorkspaceTypeRecord, true
	default:
		return "", false
	}
}

func upsertInboxStateItem(state InboxState, item PendingInboxItem) InboxState {
	for i, existing := range state.Items {
		if existing.ID == item.ID {
			state.Items[i] = item
			return state
		}
	}
	state.Items = append(state.Items, item)
	return state
}

func pendingInboxID(sourcePath string, sourceHash string) string {
	base := slugForPath(strings.TrimSuffix(filepath.Base(sourcePath), filepath.Ext(sourcePath)))
	hash := strings.TrimPrefix(sourceHash, "sha256:")
	if len(hash) > 12 {
		hash = hash[:12]
	}
	return "inbox-" + firstNonEmpty(base, "item") + "-" + hash
}

func inboxClassificationSourcePaths(files []InboxFileForClassification) []string {
	paths := make([]string, 0, len(files))
	for _, file := range files {
		paths = append(paths, file.SourcePath)
	}
	return paths
}

type generateMarkdownSpec struct {
	TaskName      string
	PromptVersion string
	Kind          string
	OutputDir     string
	FallbackName  string
	SourcePaths   []string
	DefaultTypes  []string
	RequireTypes  []string
	Instructions  string
}

func (s *CopilotService) generateMarkdownDocument(ctx context.Context, spec generateMarkdownSpec) (GeneratedDocumentResult, error) {
	ws, err := OpenWorkspace(s.workspaceRoot(), s.now())
	if err != nil {
		return GeneratedDocumentResult{}, err
	}
	sources, contents, err := collectGenerationSources(ws, spec.SourcePaths, spec.DefaultTypes)
	if err != nil {
		_ = s.writeDiagnostic(ws, DiagnosticRecord{TaskName: spec.TaskName, PromptVersion: spec.PromptVersion, SourcePaths: spec.SourcePaths, Error: err.Error()})
		return GeneratedDocumentResult{}, err
	}
	if len(sources) == 0 {
		err := fmt.Errorf("%s requires at least one source", spec.TaskName)
		_ = s.writeDiagnostic(ws, DiagnosticRecord{TaskName: spec.TaskName, PromptVersion: spec.PromptVersion, Error: err.Error()})
		return GeneratedDocumentResult{}, err
	}
	if err := ensureRequiredSourceTypes(ws, sources, spec.RequireTypes); err != nil {
		_ = s.writeDiagnostic(ws, DiagnosticRecord{TaskName: spec.TaskName, PromptVersion: spec.PromptVersion, SourcePaths: sourceRefPaths(sources), Error: err.Error()})
		return GeneratedDocumentResult{}, err
	}
	result, err := s.structuredTaskRunner().RunStructuredTask(ctx, StructuredTaskRequest{
		TaskName:      spec.TaskName,
		PromptVersion: spec.PromptVersion,
		Input:         buildGeneratedMarkdownPrompt(spec.TaskName, spec.Instructions, sources, contents),
		SourcePaths:   sourceRefPaths(sources),
	})
	if err != nil {
		_ = s.writeDiagnostic(ws, DiagnosticRecord{TaskName: spec.TaskName, PromptVersion: spec.PromptVersion, SourcePaths: sourceRefPaths(sources), Error: err.Error()})
		return GeneratedDocumentResult{}, err
	}
	parsed, err := parseGeneratedMarkdownOutput(result.Output)
	if err != nil {
		_ = s.writeDiagnostic(ws, DiagnosticRecord{TaskName: spec.TaskName, PromptVersion: spec.PromptVersion, SourcePaths: sourceRefPaths(sources), Error: err.Error(), RawOutput: result.Output})
		return GeneratedDocumentResult{}, err
	}
	for _, ref := range parsed.SourceRefs {
		if _, ok := contents[filepath.ToSlash(ref.Path)]; !ok {
			err := fmt.Errorf("%s returned unknown source ref %q", spec.TaskName, ref.Path)
			_ = s.writeDiagnostic(ws, DiagnosticRecord{TaskName: spec.TaskName, PromptVersion: spec.PromptVersion, SourcePaths: sourceRefPaths(sources), Error: err.Error(), RawOutput: result.Output})
			return GeneratedDocumentResult{}, err
		}
	}
	outputName := firstNonEmpty(parsed.Title, spec.FallbackName, spec.Kind)
	rel := filepath.ToSlash(uniqueRelPath(ws.Root, filepath.Join(spec.OutputDir, safeFileNameWithExt(outputName, ".md"))))
	if err := ws.writeWorkspaceText(rel, parsed.Markdown); err != nil {
		_ = s.writeDiagnostic(ws, DiagnosticRecord{TaskName: spec.TaskName, PromptVersion: spec.PromptVersion, SourcePaths: sourceRefPaths(sources), Error: err.Error(), RawOutput: result.Output})
		return GeneratedDocumentResult{}, err
	}
	record := GeneratedArtifactRecord{
		Path:       rel,
		Kind:       spec.Kind,
		SourceRefs: parsed.SourceRefs,
		Meta:       LLMTraceMeta{GeneratedAt: result.GeneratedAt, Model: result.Model, PromptVersion: spec.PromptVersion},
	}
	if len(record.SourceRefs) == 0 {
		record.SourceRefs = sources
	}
	state, err := ws.ReadGeneratedState()
	if err != nil {
		return GeneratedDocumentResult{}, err
	}
	state = upsertGeneratedRecord(state, record)
	if err := ws.WriteGeneratedState(state); err != nil {
		return GeneratedDocumentResult{}, err
	}
	return GeneratedDocumentResult{Path: rel, Record: record}, nil
}

func collectGenerationSources(ws *Workspace, explicit []string, defaultTypes []string) ([]SourceRef, map[string]string, error) {
	contents := map[string]string{}
	var refs []SourceRef
	addPath := func(path string) error {
		rel := filepath.ToSlash(strings.TrimSpace(path))
		if err := validateWorkspaceRelPath(rel); err != nil {
			return err
		}
		if _, ok := contents[rel]; ok {
			return nil
		}
		content, err := readWorkspaceText(ws, rel)
		if err != nil {
			return err
		}
		contents[rel] = content
		refs = append(refs, SourceRef{Path: rel, Version: "sha256:" + ContentFingerprint(content)})
		return nil
	}
	for _, path := range explicit {
		if err := addPath(path); err != nil {
			return nil, nil, err
		}
	}
	if len(refs) > 0 {
		return refs, contents, nil
	}
	_, index, err := ws.Status()
	if err != nil {
		return nil, nil, err
	}
	allowed := map[string]bool{}
	for _, itemType := range defaultTypes {
		allowed[itemType] = true
	}
	for _, item := range index.Items {
		if len(allowed) > 0 && !allowed[item.Type] {
			continue
		}
		if err := addPath(item.Path); err != nil {
			continue
		}
	}
	return refs, contents, nil
}

func ensureRequiredSourceTypes(ws *Workspace, sources []SourceRef, required []string) error {
	if len(required) == 0 {
		return nil
	}
	_, index, err := ws.Status()
	if err != nil {
		return err
	}
	typeByPath := map[string]string{}
	for _, item := range index.Items {
		typeByPath[filepath.ToSlash(item.Path)] = item.Type
	}
	have := map[string]bool{}
	for _, ref := range sources {
		have[typeByPath[filepath.ToSlash(ref.Path)]] = true
	}
	var missing []string
	for _, itemType := range required {
		if !have[itemType] {
			missing = append(missing, itemType)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required source types: %s", strings.Join(missing, ", "))
	}
	return nil
}

func sourceRefPaths(refs []SourceRef) []string {
	paths := make([]string, 0, len(refs))
	for _, ref := range refs {
		paths = append(paths, ref.Path)
	}
	return paths
}

func readWorkspaceText(ws *Workspace, rel string) (string, error) {
	rel = filepath.ToSlash(strings.TrimSpace(rel))
	if err := validateWorkspaceRelPath(rel); err != nil {
		return "", err
	}
	data, err := os.ReadFile(filepath.Join(ws.Root, filepath.FromSlash(rel)))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func renderSplitJDMarkdown(jd jdSplitItem, sourcePath string) string {
	var b strings.Builder
	b.WriteString("# " + strings.TrimSpace(jd.Title) + "\n\n")
	if jd.Company != "" || jd.Team != "" || jd.Role != "" {
		b.WriteString("## 基本信息\n\n")
		b.WriteString("- 公司：" + firstNonEmpty(jd.Company, "待补证据") + "\n")
		b.WriteString("- 团队：" + firstNonEmpty(jd.Team, "待补证据") + "\n")
		b.WriteString("- 岗位：" + firstNonEmpty(jd.Role, "待补证据") + "\n\n")
	}
	writeStringList(&b, "岗位职责", jd.Responsibilities)
	writeStringList(&b, "任职要求", jd.Requirements)
	writeStringList(&b, "关键词", jd.Keywords)
	b.WriteString("## 来源证据\n\n")
	b.WriteString("- 来源：" + filepath.ToSlash(sourcePath) + "\n")
	b.WriteString("- 片段：" + strings.TrimSpace(jd.SourceExcerpt) + "\n")
	return b.String()
}

func writeStringList(b *strings.Builder, title string, values []string) {
	b.WriteString("## " + title + "\n\n")
	if len(values) == 0 {
		b.WriteString("- 待补证据\n\n")
		return
	}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			b.WriteString("- " + value + "\n")
		}
	}
	b.WriteString("\n")
}

func upsertGeneratedRecord(state GeneratedState, record GeneratedArtifactRecord) GeneratedState {
	for i, existing := range state.Items {
		if existing.Path == record.Path || (existing.Kind == record.Kind && existing.Path == record.Path) {
			state.Items[i] = record
			return state
		}
	}
	state.Items = append(state.Items, record)
	return state
}

func markBattlePacksStaleForNewResume(ws *Workspace, resume WorkspaceItem, now time.Time) error {
	state, err := ws.ReadGeneratedState()
	if err != nil {
		return err
	}
	newVersion := resume.Metadata.ContentFingerprint
	if newVersion == "" {
		newVersion = resume.Metadata.SourceFingerprint
	}
	if newVersion != "" && !strings.HasPrefix(newVersion, "sha256:") {
		newVersion = "sha256:" + newVersion
	}
	changed := false
	for i, record := range state.Items {
		if record.Kind != "battle_pack" {
			continue
		}
		for _, ref := range record.SourceRefs {
			if filepath.ToSlash(ref.Path) == filepath.ToSlash(resume.Path) || strings.Contains(filepath.ToSlash(ref.Path), WorkspaceDirResume+"/") {
				if newVersion == "" || ref.Version != newVersion {
					state.Items[i].Stale = true
					state.Items[i].StaleReason = "基于旧简历生成；新简历已在 " + now.Format("2006-01-02 15:04") + " 确认。"
					changed = true
				}
				break
			}
		}
	}
	if !changed {
		return nil
	}
	return ws.WriteGeneratedState(state)
}

func (s *CopilotService) writeDiagnostic(ws *Workspace, record DiagnosticRecord) error {
	record.CreatedAt = s.now()
	returnError := strings.TrimSpace(record.Error)
	if returnError == "" {
		record.Error = "unknown error"
	}
	_, err := ws.WriteDiagnostic(record)
	return err
}

func (s *CopilotService) workspaceRoot() string {
	if s == nil || s.WorkspaceRoot == "" {
		return DefaultWorkspaceRoot
	}
	return s.WorkspaceRoot
}

func (s *CopilotService) now() time.Time {
	if s != nil && s.Now != nil {
		return s.Now()
	}
	return time.Now()
}
