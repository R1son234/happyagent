package career

import (
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

func TestInboxStateReadWriteRoundTrip(t *testing.T) {
	now := time.Date(2026, 5, 24, 10, 0, 0, 0, time.UTC)
	ws, err := OpenWorkspace(filepath.Join(t.TempDir(), "career"), now)
	if err != nil {
		t.Fatalf("OpenWorkspace() error = %v", err)
	}
	initial, err := ws.ReadInboxState()
	if err != nil {
		t.Fatalf("ReadInboxState() error = %v", err)
	}
	if initial.Version != inboxStateVersion || len(initial.Items) != 0 {
		t.Fatalf("unexpected initial state: %+v", initial)
	}
	state := InboxState{
		Items: []PendingInboxItem{
			{
				ID:         "pending-1",
				SourcePath: "inbox/resume.md",
				Status:     InboxItemStatusPending,
				Meta:       LLMTraceMeta{GeneratedAt: now, Model: "test", PromptVersion: "career-file-classification-v1"},
			},
		},
	}
	if err := ws.WriteInboxState(state); err != nil {
		t.Fatalf("WriteInboxState() error = %v", err)
	}
	roundTrip, err := ws.ReadInboxState()
	if err != nil {
		t.Fatalf("ReadInboxState() round trip error = %v", err)
	}
	if roundTrip.Version != inboxStateVersion || len(roundTrip.Items) != 1 || roundTrip.Items[0].ID != "pending-1" {
		t.Fatalf("unexpected round trip state: %+v", roundTrip)
	}
}

func TestInboxStateRejectsEscapingPath(t *testing.T) {
	ws, err := OpenWorkspace(filepath.Join(t.TempDir(), "career"), time.Now())
	if err != nil {
		t.Fatalf("OpenWorkspace() error = %v", err)
	}
	err = ws.WriteInboxState(InboxState{Items: []PendingInboxItem{{
		ID:         "bad",
		SourcePath: "../outside.md",
		Status:     InboxItemStatusPending,
	}}})
	if err == nil {
		t.Fatalf("expected escaping source path to be rejected")
	}
}

func TestGeneratedStateReadWriteRoundTrip(t *testing.T) {
	now := time.Date(2026, 5, 24, 11, 0, 0, 0, time.UTC)
	ws, err := OpenWorkspace(filepath.Join(t.TempDir(), "career"), now)
	if err != nil {
		t.Fatalf("OpenWorkspace() error = %v", err)
	}
	state := GeneratedState{Items: []GeneratedArtifactRecord{
		{
			Path: "我的面试/示例岗位/作战页.md",
			Kind: "battle_pack",
			SourceRefs: []SourceRef{{
				Path:    "我的简历/resume.md",
				Version: "sha256:example",
			}},
			Meta: LLMTraceMeta{GeneratedAt: now, Model: "test", PromptVersion: "career-battle-pack-v1"},
		},
	}}
	if err := ws.WriteGeneratedState(state); err != nil {
		t.Fatalf("WriteGeneratedState() error = %v", err)
	}
	roundTrip, err := ws.ReadGeneratedState()
	if err != nil {
		t.Fatalf("ReadGeneratedState() error = %v", err)
	}
	if roundTrip.Version != generatedStateVersion || len(roundTrip.Items) != 1 || roundTrip.Items[0].Kind != "battle_pack" {
		t.Fatalf("unexpected generated state: %+v", roundTrip)
	}
}

func TestCopilotServiceListInboxMergesFilesAndPendingState(t *testing.T) {
	now := time.Date(2026, 5, 24, 10, 30, 0, 0, time.UTC)
	root := filepath.Join(t.TempDir(), "career")
	ws, err := OpenWorkspace(root, now)
	if err != nil {
		t.Fatalf("OpenWorkspace() error = %v", err)
	}
	resumeRel := filepath.Join("inbox", "resume.md")
	jdRel := filepath.Join("inbox", "jd.md")
	if err := os.WriteFile(filepath.Join(root, resumeRel), []byte("# Resume\n"), 0o644); err != nil {
		t.Fatalf("write resume: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, jdRel), []byte("# JD\n"), 0o644); err != nil {
		t.Fatalf("write jd: %v", err)
	}
	if err := ws.WriteInboxState(InboxState{Items: []PendingInboxItem{
		{
			ID:         "pending-resume",
			SourcePath: filepath.ToSlash(resumeRel),
			Status:     InboxItemStatusPending,
			Confidence: ConfidenceMedium,
		},
	}}); err != nil {
		t.Fatalf("WriteInboxState() error = %v", err)
	}
	service := CopilotService{WorkspaceRoot: root, Now: func() time.Time { return now }}
	view, err := service.ListInbox(context.Background())
	if err != nil {
		t.Fatalf("ListInbox() error = %v", err)
	}
	if len(view.Files) != 2 {
		t.Fatalf("expected 2 inbox files, got %+v", view.Files)
	}
	if len(view.PendingItems) != 1 {
		t.Fatalf("expected 1 pending item, got %+v", view.PendingItems)
	}
	if view.Counts[InboxItemStatusPending] != 1 || view.Counts["unclassified"] != 1 {
		t.Fatalf("unexpected counts: %+v", view.Counts)
	}
	statusByPath := map[string]string{}
	for _, file := range view.Files {
		statusByPath[file.Path] = file.Status
	}
	if statusByPath[filepath.ToSlash(resumeRel)] != InboxItemStatusPending {
		t.Fatalf("expected resume pending status, got %+v", statusByPath)
	}
	if statusByPath[filepath.ToSlash(jdRel)] != "unclassified" {
		t.Fatalf("expected jd unclassified status, got %+v", statusByPath)
	}
}

func TestCopilotServiceClassifyInboxAutoConfirmsHighConfidence(t *testing.T) {
	now := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	root := filepath.Join(t.TempDir(), "career")
	if _, err := OpenWorkspace(root, now); err != nil {
		t.Fatalf("OpenWorkspace() error = %v", err)
	}
	sourceRel := filepath.Join("inbox", "jd.md")
	if err := os.WriteFile(filepath.Join(root, sourceRel), []byte("# 示例 JD\n岗位职责：负责示例系统。\n任职要求：熟悉示例工程。"), 0o644); err != nil {
		t.Fatalf("write jd: %v", err)
	}
	runner := &fakeStructuredTaskRunner{output: `{"files":[{"source_path":"inbox/jd.md","source_hash":"sha256:fake","material_type":"jd","confidence":"high","reason":"包含岗位职责和任职要求。","source_excerpt":"岗位职责：负责示例系统。","destination":"岗位明细","needs_user_confirmation":false,"questions_for_user":[]}]}`}
	service := CopilotService{WorkspaceRoot: root, TaskRunner: runner, Now: func() time.Time { return now }}
	result, err := service.ClassifyInbox(context.Background(), ClassifyInboxRequest{})
	if err != nil {
		t.Fatalf("ClassifyInbox() error = %v", err)
	}
	if len(result.Items) != 1 || result.Items[0].Status != InboxItemStatusConfirmed {
		t.Fatalf("expected confirmed high confidence item, got %+v", result.Items)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("expected one structured task call, got %d", len(runner.calls))
	}
	if !sameStrings(runner.calls[0].SourcePaths, []string{"inbox/jd.md"}) {
		t.Fatalf("source paths = %v, want workspace-relative inbox path", runner.calls[0].SourcePaths)
	}
	if strings.Contains(runner.calls[0].Input, DefaultWorkspaceRoot+"/inbox/jd.md") || !strings.Contains(runner.calls[0].Input, "<read_path>inbox/jd.md</read_path>") {
		t.Fatalf("classification prompt used wrong read_path:\n%s", runner.calls[0].Input)
	}
	if _, err := os.Stat(filepath.Join(root, sourceRel)); !os.IsNotExist(err) {
		t.Fatalf("expected inbox source removed after confirmed import, err=%v", err)
	}
	ws, err := OpenWorkspace(root, now)
	if err != nil {
		t.Fatalf("OpenWorkspace() error = %v", err)
	}
	_, index, err := ws.Status()
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if len(index.Items) != 1 || index.Items[0].Type != WorkspaceTypeJD {
		t.Fatalf("expected one JD item, got %+v", index.Items)
	}
}

func TestCopilotServiceClassifyInboxKeepsMediumPending(t *testing.T) {
	now := time.Date(2026, 5, 24, 12, 30, 0, 0, time.UTC)
	root := filepath.Join(t.TempDir(), "career")
	if _, err := OpenWorkspace(root, now); err != nil {
		t.Fatalf("OpenWorkspace() error = %v", err)
	}
	sourceRel := filepath.Join("inbox", "mixed.md")
	if err := os.WriteFile(filepath.Join(root, sourceRel), []byte("# 示例资料\n可能是岗位，也可能是复习笔记。"), 0o644); err != nil {
		t.Fatalf("write mixed: %v", err)
	}
	runner := &fakeStructuredTaskRunner{output: `{"files":[{"source_path":"inbox/mixed.md","source_hash":"sha256:fake","material_type":"review_note","confidence":"medium","reason":"资料类型不够明确。","source_excerpt":"可能是岗位，也可能是复习笔记。","destination":"待确认","needs_user_confirmation":true,"questions_for_user":["这份资料要作为复习笔记还是岗位资料？"]}]}`}
	service := CopilotService{WorkspaceRoot: root, TaskRunner: runner, Now: func() time.Time { return now }}
	result, err := service.ClassifyInbox(context.Background(), ClassifyInboxRequest{})
	if err != nil {
		t.Fatalf("ClassifyInbox() error = %v", err)
	}
	if len(result.Items) != 1 || result.Items[0].Status != InboxItemStatusPending || !result.Items[0].NeedsUserConfirmation {
		t.Fatalf("expected pending medium item, got %+v", result.Items)
	}
	if _, err := os.Stat(filepath.Join(root, sourceRel)); err != nil {
		t.Fatalf("expected medium source to stay in inbox: %v", err)
	}
	ws, err := OpenWorkspace(root, now)
	if err != nil {
		t.Fatalf("OpenWorkspace() error = %v", err)
	}
	_, index, err := ws.Status()
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if len(index.Items) != 0 {
		t.Fatalf("expected no indexed items before confirmation, got %+v", index.Items)
	}
	state, err := ws.ReadInboxState()
	if err != nil {
		t.Fatalf("ReadInboxState() error = %v", err)
	}
	if len(state.Items) != 1 || len(state.Items[0].QuestionsForUser) != 1 {
		t.Fatalf("expected pending state with question, got %+v", state)
	}
}

func TestCopilotServiceClassifyInboxNormalizesDestinationAliases(t *testing.T) {
	now := time.Date(2026, 5, 24, 13, 0, 0, 0, time.UTC)
	root := filepath.Join(t.TempDir(), "career")
	if _, err := OpenWorkspace(root, now); err != nil {
		t.Fatalf("OpenWorkspace() error = %v", err)
	}
	sourceRel := filepath.Join("inbox", "interview.md")
	if err := os.WriteFile(filepath.Join(root, sourceRel), []byte("# 面经\n候选人分享了面试题。"), 0o644); err != nil {
		t.Fatalf("write interview experience: %v", err)
	}
	runner := &fakeStructuredTaskRunner{output: `{"files":[{"source_path":"inbox/interview.md","source_hash":"sha256:fake","material_type":"public_interview_experience","confidence":"medium","reason":"包含公开面试经验。","source_excerpt":"候选人分享了面试题。","destination":"面经","needs_user_confirmation":true,"questions_for_user":[]}]}`}
	service := CopilotService{WorkspaceRoot: root, TaskRunner: runner, Now: func() time.Time { return now }}
	result, err := service.ClassifyInbox(context.Background(), ClassifyInboxRequest{})
	if err != nil {
		t.Fatalf("ClassifyInbox() error = %v", err)
	}
	if len(result.Items) != 1 || result.Items[0].Destination != WorkspaceDirExperiences {
		t.Fatalf("expected destination alias normalized to %q, got %+v", WorkspaceDirExperiences, result.Items)
	}
}

func TestCopilotServiceClassifyInboxRejectsUnknownSourceFromLLM(t *testing.T) {
	root := filepath.Join(t.TempDir(), "career")
	if _, err := OpenWorkspace(root, time.Now()); err != nil {
		t.Fatalf("OpenWorkspace() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "inbox", "jd.md"), []byte("# JD\n岗位职责：示例。"), 0o644); err != nil {
		t.Fatalf("write jd: %v", err)
	}
	runner := &fakeStructuredTaskRunner{output: `{"files":[{"source_path":"inbox/other.md","source_hash":"sha256:fake","material_type":"jd","confidence":"high","reason":"x","source_excerpt":"x","destination":"岗位明细","needs_user_confirmation":false,"questions_for_user":[]}]}`}
	service := CopilotService{WorkspaceRoot: root, TaskRunner: runner}
	_, err := service.ClassifyInbox(context.Background(), ClassifyInboxRequest{})
	if err == nil || !strings.Contains(err.Error(), "unknown source_path") {
		t.Fatalf("expected unknown source path error, got %v", err)
	}
}

func TestCopilotServiceConfirmMaterialClassificationWritesPendingItem(t *testing.T) {
	now := time.Date(2026, 5, 24, 13, 0, 0, 0, time.UTC)
	root := filepath.Join(t.TempDir(), "career")
	ws, err := OpenWorkspace(root, now)
	if err != nil {
		t.Fatalf("OpenWorkspace() error = %v", err)
	}
	sourceRel := filepath.ToSlash(filepath.Join("inbox", "resume.md"))
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(sourceRel)), []byte("# 示例简历\n\n项目经历：示例项目。"), 0o644); err != nil {
		t.Fatalf("write resume: %v", err)
	}
	state := InboxState{Items: []PendingInboxItem{{
		ID:                    "pending-resume",
		SourcePath:            sourceRel,
		SourceHash:            "sha256:abc",
		OriginalName:          "resume.md",
		MaterialType:          "unknown",
		Destination:           "待确认",
		Confidence:            ConfidenceMedium,
		NeedsUserConfirmation: true,
		Status:                InboxItemStatusPending,
	}}}
	if err := ws.WriteInboxState(state); err != nil {
		t.Fatalf("WriteInboxState() error = %v", err)
	}

	service := CopilotService{WorkspaceRoot: root, Now: func() time.Time { return now }}
	result, err := service.ConfirmMaterialClassification(context.Background(), ConfirmMaterialRequest{
		ID:           "pending-resume",
		MaterialType: "resume",
		Destination:  "我的简历",
	})
	if err != nil {
		t.Fatalf("ConfirmMaterialClassification() error = %v", err)
	}
	if result.Item.Type != WorkspaceTypeResume || result.RecordPath == "" || !result.RemovedInbox {
		t.Fatalf("unexpected confirm result: %+v", result)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(sourceRel))); !os.IsNotExist(err) {
		t.Fatalf("expected inbox source removed, err=%v", err)
	}
	roundTrip, err := ws.ReadInboxState()
	if err != nil {
		t.Fatalf("ReadInboxState() error = %v", err)
	}
	if len(roundTrip.Items) != 1 || roundTrip.Items[0].Status != InboxItemStatusConfirmed || roundTrip.Items[0].MaterialType != "resume" {
		t.Fatalf("expected confirmed inbox state, got %+v", roundTrip)
	}
	_, index, err := ws.Status()
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if len(index.Items) != 1 || index.Items[0].Type != WorkspaceTypeResume {
		t.Fatalf("expected resume in index, got %+v", index.Items)
	}
}

func TestCopilotServiceRunChatTurnWritesWorkspaceOutput(t *testing.T) {
	now := time.Date(2026, 5, 25, 10, 0, 0, 0, time.UTC)
	root := filepath.Join(t.TempDir(), "career")
	ws, err := OpenWorkspace(root, now)
	if err != nil {
		t.Fatalf("OpenWorkspace() error = %v", err)
	}
	if _, err := ws.AddMaterial(WorkspaceTypeJD, "# 示例 JD\n\n岗位职责：负责示例 Agent 系统。", now); err != nil {
		t.Fatalf("AddMaterial(jd) error = %v", err)
	}
	service := CopilotService{
		WorkspaceRoot: root,
		App: fakeCareerApplication{
			sessionID: "session-test",
			output:    "# 面试准备材料\n\n## 复习计划\n- 先看 JD 关键词。\n",
		},
		Now: func() time.Time { return now },
	}
	result, err := service.RunChatTurn(context.Background(), ChatTurnRequest{
		SessionID: "session-test",
		Input:     "帮我生成面试准备材料",
	})
	if err != nil {
		t.Fatalf("RunChatTurn() error = %v", err)
	}
	if result.Output == "" {
		t.Fatalf("expected chat output, got empty result: %+v", result)
	}
	if result.PrimaryPath != filepath.ToSlash(filepath.Join(WorkspaceDirOutputs, "latest-interview-brief.md")) {
		t.Fatalf("unexpected primary path: %+v", result)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(result.PrimaryPath))); err != nil {
		t.Fatalf("expected generated primary file, stat err=%v", err)
	}
}

type fakeCareerApplication struct {
	sessionID string
	output    string
}

func (f fakeCareerApplication) CreateSession(profileName string) (store.SessionRecord, error) {
	_ = profileName
	return store.SessionRecord{ID: firstNonEmpty(f.sessionID, "fake-session")}, nil
}

func (f fakeCareerApplication) AppendUserTurn(ctx context.Context, req app.AppendTurnRequest) (store.RunRecord, error) {
	_ = ctx
	return store.RunRecord{
		ID:        "run-test",
		SessionID: firstNonEmpty(req.SessionID, f.sessionID, "fake-session"),
		Profile:   req.ProfileName,
		Input:     req.Input,
		Output:    f.output,
	}, nil
}

func TestCopilotServiceConfirmMaterialClassificationRejectsUnsupportedType(t *testing.T) {
	root := filepath.Join(t.TempDir(), "career")
	ws, err := OpenWorkspace(root, time.Now())
	if err != nil {
		t.Fatalf("OpenWorkspace() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "inbox", "note.md"), []byte("# Note\n"), 0o644); err != nil {
		t.Fatalf("write note: %v", err)
	}
	if err := ws.WriteInboxState(InboxState{Items: []PendingInboxItem{{
		ID:           "pending-note",
		SourcePath:   "inbox/note.md",
		OriginalName: "note.md",
		MaterialType: "unknown",
		Status:       InboxItemStatusPending,
	}}}); err != nil {
		t.Fatalf("WriteInboxState() error = %v", err)
	}
	service := CopilotService{WorkspaceRoot: root}
	_, err = service.ConfirmMaterialClassification(context.Background(), ConfirmMaterialRequest{
		ID:           "pending-note",
		MaterialType: "unsupported",
	})
	if err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("expected unsupported type error, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "inbox", "note.md")); err != nil {
		t.Fatalf("source should stay in inbox after failed confirmation: %v", err)
	}
	_, index, err := ws.Status()
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if len(index.Items) != 0 {
		t.Fatalf("expected no indexed items after failed confirmation, got %+v", index.Items)
	}
}

func TestCopilotServiceSplitJobDescriptionsUsesLLM(t *testing.T) {
	now := time.Date(2026, 5, 25, 9, 0, 0, 0, time.UTC)
	root := filepath.Join(t.TempDir(), "career")
	ws, err := OpenWorkspace(root, now)
	if err != nil {
		t.Fatalf("OpenWorkspace() error = %v", err)
	}
	jd, err := ws.AddMaterial(WorkspaceTypeJD, "# JD 汇总\n\n岗位 A：负责平台。\n岗位 B：负责数据。", now)
	if err != nil {
		t.Fatalf("AddMaterial() error = %v", err)
	}
	runner := &fakeStructuredTaskRunner{output: `{"job_descriptions":[{"title":"平台工程师","company":"示例公司","team":"平台组","role":"工程师","responsibilities":["负责平台"],"requirements":["熟悉 Go"],"keywords":["Go"],"source_excerpt":"岗位 A：负责平台。","confidence":"high"},{"title":"数据工程师","company":"示例公司","team":"数据组","role":"工程师","responsibilities":["负责数据"],"requirements":["熟悉 SQL"],"keywords":["SQL"],"source_excerpt":"岗位 B：负责数据。","confidence":"medium"}]}`}
	service := CopilotService{WorkspaceRoot: root, TaskRunner: runner, Now: func() time.Time { return now }}
	result, err := service.SplitJobDescriptions(context.Background(), SplitJDRequest{SourcePath: jd.Path})
	if err != nil {
		t.Fatalf("SplitJobDescriptions() error = %v", err)
	}
	if len(result.Items) != 1 || result.Items[0].Type != WorkspaceTypeJD {
		t.Fatalf("expected one confirmed JD, got %+v", result.Items)
	}
	if len(result.PendingItems) != 1 || result.PendingItems[0].Confidence != ConfidenceMedium {
		t.Fatalf("expected one medium pending JD, got %+v", result.PendingItems)
	}
	state, err := ws.ReadInboxState()
	if err != nil {
		t.Fatalf("ReadInboxState() error = %v", err)
	}
	if len(state.Items) != 2 {
		t.Fatalf("expected split state records, got %+v", state)
	}
}

func TestCopilotServiceGenerateProjectPackWritesGeneratedState(t *testing.T) {
	now := time.Date(2026, 5, 25, 10, 0, 0, 0, time.UTC)
	root := filepath.Join(t.TempDir(), "career")
	ws, err := OpenWorkspace(root, now)
	if err != nil {
		t.Fatalf("OpenWorkspace() error = %v", err)
	}
	resume, err := ws.AddMaterial(WorkspaceTypeResume, "# 简历\n\n示例项目：负责链路治理。", now)
	if err != nil {
		t.Fatalf("AddMaterial() error = %v", err)
	}
	runner := &fakeStructuredTaskRunner{output: `{"title":"示例项目专项","primary_document":"项目专项/示例项目-interview-qa.md","documents":[{"path":"项目专项/示例项目-interview-qa.md","title":"示例项目专项","markdown":"# 示例项目专项\n\n## 一句话介绍\n基于来源资料整理。\n\n## 待补证据\n- 指标待补。"}],"source_refs":[{"path":"` + resume.Path + `","version":"sha256:test","excerpt":"示例项目","evidence_spans":["示例项目"]}],"risk_flags":[],"missing_info":["指标"]}`}
	service := CopilotService{WorkspaceRoot: root, TaskRunner: runner, Now: func() time.Time { return now }}
	result, err := service.GenerateProjectPack(context.Background(), GenerateProjectPackRequest{ProjectName: "示例项目", SourcePaths: []string{resume.Path}})
	if err != nil {
		t.Fatalf("GenerateProjectPack() error = %v", err)
	}
	if !strings.HasPrefix(result.Path, WorkspaceDirProjectPack+"/") {
		t.Fatalf("expected project pack under %s, got %s", WorkspaceDirProjectPack, result.Path)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(result.Path))); err != nil {
		t.Fatalf("expected generated markdown: %v", err)
	}
	state, err := ws.ReadGeneratedState()
	if err != nil {
		t.Fatalf("ReadGeneratedState() error = %v", err)
	}
	if len(state.Items) != 1 || state.Items[0].Kind != "project_pack" {
		t.Fatalf("expected generated project record, got %+v", state)
	}
}

func TestCopilotServiceGenerateProjectPackRepairsTruncatedBundleJSON(t *testing.T) {
	now := time.Date(2026, 5, 25, 10, 30, 0, 0, time.UTC)
	root := filepath.Join(t.TempDir(), "career")
	ws, err := OpenWorkspace(root, now)
	if err != nil {
		t.Fatalf("OpenWorkspace() error = %v", err)
	}
	resume, err := ws.AddMaterial(WorkspaceTypeResume, "# 简历\n\n示例项目：负责链路治理。", now)
	if err != nil {
		t.Fatalf("AddMaterial() error = %v", err)
	}
	valid := `{"title":"示例项目专项","primary_document":"项目专项/示例项目.md","documents":[{"path":"项目专项/示例项目.md","title":"示例项目专项","markdown":"# 示例项目专项\n\n基于来源资料整理。"}],"source_refs":[{"path":"` + resume.Path + `","version":"sha256:test","excerpt":"示例项目","evidence_spans":["示例项目"]}],"risk_flags":[],"missing_info":[]}`
	runner := &fakeStructuredTaskRunner{outputs: []string{
		`{"title":"示例项目专项","documents":[{"path":"项目专项/示例项目.md"`,
		valid,
	}}
	service := CopilotService{WorkspaceRoot: root, TaskRunner: runner, Now: func() time.Time { return now }}
	result, err := service.GenerateProjectPack(context.Background(), GenerateProjectPackRequest{ProjectName: "示例项目", SourcePaths: []string{resume.Path}})
	if err != nil {
		t.Fatalf("GenerateProjectPack() error = %v", err)
	}
	if result.Path != "项目专项/示例项目.md" {
		t.Fatalf("unexpected generated path: %s", result.Path)
	}
	if len(runner.calls) != 2 {
		t.Fatalf("expected repair call after truncated JSON, got %d", len(runner.calls))
	}
	if !strings.Contains(runner.calls[1].Input, "previous document bundle JSON was truncated") {
		t.Fatalf("expected repair prompt, got %q", runner.calls[1].Input)
	}
}

func TestCopilotServiceGenerateBattlePackRequiresResumeAndJD(t *testing.T) {
	now := time.Date(2026, 5, 25, 11, 0, 0, 0, time.UTC)
	root := filepath.Join(t.TempDir(), "career")
	if _, err := OpenWorkspace(root, now); err != nil {
		t.Fatalf("OpenWorkspace() error = %v", err)
	}
	service := CopilotService{WorkspaceRoot: root, TaskRunner: &fakeStructuredTaskRunner{output: `{}`}, Now: func() time.Time { return now }}
	_, err := service.GenerateBattlePack(context.Background(), GenerateBattlePackRequest{})
	if err == nil || !strings.Contains(err.Error(), "requires at least one source") {
		t.Fatalf("expected missing source error, got %v", err)
	}
	diagnosticsRoot := filepath.Join(root, WorkspaceInternalDir, WorkspaceDiagnosticsDir)
	entries, readErr := os.ReadDir(diagnosticsRoot)
	if readErr != nil {
		t.Fatalf("expected diagnostics dir: %v", readErr)
	}
	if len(entries) == 0 {
		t.Fatalf("expected diagnostic record for failed battle pack")
	}
	result, listErr := service.ListDiagnostics(context.Background(), ListDiagnosticsRequest{Limit: 5})
	if listErr != nil {
		t.Fatalf("ListDiagnostics() error = %v", listErr)
	}
	if len(result.Items) == 0 || result.Items[0].TaskName != "generate_battle_pack" {
		t.Fatalf("expected battle pack diagnostic, got %+v", result.Items)
	}
}

func TestCopilotServiceGenerateBattlePackEnablesAgentTaskToolScope(t *testing.T) {
	now := time.Date(2026, 5, 25, 11, 30, 0, 0, time.UTC)
	root := filepath.Join(t.TempDir(), "career")
	ws, err := OpenWorkspace(root, now)
	if err != nil {
		t.Fatalf("OpenWorkspace() error = %v", err)
	}
	resume, err := ws.AddMaterial(WorkspaceTypeResume, "# 简历\n\n项目经历。", now)
	if err != nil {
		t.Fatalf("AddMaterial() error = %v", err)
	}
	jd, err := ws.AddMaterial(WorkspaceTypeJD, "# JD\n\n岗位要求。", now)
	if err != nil {
		t.Fatalf("AddMaterial() error = %v", err)
	}
	runner := &fakeStructuredTaskRunner{output: `{"title":"示例作战包","primary_document":"我的面试/示例岗位/作战页.md","documents":[{"path":"我的面试/示例岗位/作战页.md","title":"示例作战包","markdown":"# 示例作战包"}],"source_refs":[{"path":"` + resume.Path + `","version":"sha256:resume","excerpt":"项目经历","evidence_spans":["项目经历"]},{"path":"` + jd.Path + `","version":"sha256:jd","excerpt":"岗位要求","evidence_spans":["岗位要求"]}],"risk_flags":[],"missing_info":[]}`}
	service := CopilotService{WorkspaceRoot: root, TaskRunner: runner, Now: func() time.Time { return now }}
	if _, err := service.GenerateBattlePack(context.Background(), GenerateBattlePackRequest{
		JDPath:      jd.Path,
		SourcePaths: []string{resume.Path},
	}); err != nil {
		t.Fatalf("GenerateBattlePack() error = %v", err)
	}
	if len(runner.calls) == 0 {
		t.Fatalf("expected structured task request")
	}
	first := runner.calls[0]
	if first.ProfileName != ProfileName {
		t.Fatalf("profile name = %q, want %q", first.ProfileName, ProfileName)
	}
	if !sameStrings(first.ToolScope, []string{"agent_task", "file_read", "final_answer"}) {
		t.Fatalf("tool scope = %v", first.ToolScope)
	}
	if !strings.Contains(first.Input, "MUST call agent_task three times") {
		t.Fatalf("expected battle pack prompt to require subagents, got %q", first.Input)
	}
}

func TestCopilotServiceConfirmNewResumeMarksBattlePackStale(t *testing.T) {
	oldTime := time.Date(2026, 5, 24, 9, 0, 0, 0, time.UTC)
	newTime := time.Date(2026, 5, 25, 9, 0, 0, 0, time.UTC)
	root := filepath.Join(t.TempDir(), "career")
	ws, err := OpenWorkspace(root, oldTime)
	if err != nil {
		t.Fatalf("OpenWorkspace() error = %v", err)
	}
	oldResume, err := ws.AddMaterial(WorkspaceTypeResume, "# 旧简历\n\n旧项目。", oldTime)
	if err != nil {
		t.Fatalf("AddMaterial() error = %v", err)
	}
	if err := ws.WriteGeneratedState(GeneratedState{Items: []GeneratedArtifactRecord{{
		Path: "我的面试/示例岗位/作战包.md",
		Kind: "battle_pack",
		SourceRefs: []SourceRef{{
			Path:    oldResume.Path,
			Version: "sha256:old-version",
		}},
		Meta: LLMTraceMeta{GeneratedAt: oldTime, Model: "fake", PromptVersion: PromptVersionBattlePack},
	}}}); err != nil {
		t.Fatalf("WriteGeneratedState() error = %v", err)
	}
	sourceRel := filepath.ToSlash(filepath.Join("inbox", "new-resume.md"))
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(sourceRel)), []byte("# 新简历\n\n新项目。"), 0o644); err != nil {
		t.Fatalf("write new resume: %v", err)
	}
	if err := ws.WriteInboxState(InboxState{Items: []PendingInboxItem{{
		ID:           "pending-new-resume",
		SourcePath:   sourceRel,
		SourceHash:   "sha256:new-version",
		OriginalName: "new-resume.md",
		MaterialType: "resume",
		Confidence:   ConfidenceHigh,
		Status:       InboxItemStatusPending,
	}}}); err != nil {
		t.Fatalf("WriteInboxState() error = %v", err)
	}

	service := CopilotService{WorkspaceRoot: root, Now: func() time.Time { return newTime }}
	if _, err := service.ConfirmMaterialClassification(context.Background(), ConfirmMaterialRequest{ID: "pending-new-resume"}); err != nil {
		t.Fatalf("ConfirmMaterialClassification() error = %v", err)
	}
	state, err := ws.ReadGeneratedState()
	if err != nil {
		t.Fatalf("ReadGeneratedState() error = %v", err)
	}
	if len(state.Items) != 1 || !state.Items[0].Stale || !strings.Contains(state.Items[0].StaleReason, "旧简历") {
		t.Fatalf("expected stale battle pack, got %+v", state)
	}
}

type fakeStructuredTaskRunner struct {
	output  string
	outputs []string
	calls   []StructuredTaskRequest
}

func (r *fakeStructuredTaskRunner) RunStructuredTask(ctx context.Context, req StructuredTaskRequest) (StructuredTaskResult, error) {
	_ = ctx
	r.calls = append(r.calls, req)
	output := r.output
	if len(r.outputs) > 0 {
		output = r.outputs[0]
		r.outputs = r.outputs[1:]
	}
	return StructuredTaskResult{
		Output:      output,
		Model:       "fake-model",
		RunID:       "fake-run",
		SessionID:   "fake-session",
		GeneratedAt: time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC),
	}, nil
}

type captureStructuredTaskApp struct {
	sessionID string
	reqs      []app.AppendTurnRequest
	output    string
}

func (c *captureStructuredTaskApp) CreateSession(profileName string) (store.SessionRecord, error) {
	_ = profileName
	return store.SessionRecord{ID: firstNonEmpty(c.sessionID, "structured-session")}, nil
}

func (c *captureStructuredTaskApp) AppendUserTurn(ctx context.Context, req app.AppendTurnRequest) (store.RunRecord, error) {
	_ = ctx
	c.reqs = append(c.reqs, req)
	return store.RunRecord{
		ID:        "run-structured",
		SessionID: firstNonEmpty(req.SessionID, c.sessionID, "structured-session"),
		Output:    firstNonEmpty(c.output, `{"files":[]}`),
	}, nil
}

func TestStructuredTaskRunnerUsesSourceBoundToolPrompt(t *testing.T) {
	app := &captureStructuredTaskApp{output: `{"files":[{"source_path":"inbox/test.md","source_hash":"sha256:test","material_type":"jd","confidence":"high","reason":"x","source_excerpt":"x","destination":"岗位明细","needs_user_confirmation":false,"questions_for_user":[]}]}`}
	runner := &appStructuredTaskRunner{
		App:    app,
		Config: config.Default(),
		Now:    func() time.Time { return time.Date(2026, 5, 26, 11, 0, 0, 0, time.UTC) },
	}
	_, err := runner.RunStructuredTask(context.Background(), StructuredTaskRequest{
		TaskName:      "classify_inbox",
		PromptVersion: PromptVersionFileClassification,
		Input:         `{"files":[]}`,
		SourcePaths:   []string{"inbox/test.md"},
	})
	if err != nil {
		t.Fatalf("RunStructuredTask() error = %v", err)
	}
	if len(app.reqs) != 1 {
		t.Fatalf("expected one append request, got %d", len(app.reqs))
	}
	if app.reqs[0].ProfileName != "" {
		t.Fatalf("expected blank profile for structured task, got %q", app.reqs[0].ProfileName)
	}
	if len(app.reqs[0].ApprovedTools) != 0 {
		t.Fatalf("expected no approved tools, got %v", app.reqs[0].ApprovedTools)
	}
	if !sameStrings(app.reqs[0].ToolScope, []string{"file_read", "final_answer"}) {
		t.Fatalf("tool scope = %v, want file_read/final_answer", app.reqs[0].ToolScope)
	}
	if !sameStrings(app.reqs[0].SourceReadPaths, []string{"inbox/test.md"}) {
		t.Fatalf("source read paths = %v", app.reqs[0].SourceReadPaths)
	}
	if !app.reqs[0].SuppressHistory || !app.reqs[0].SuppressMemory || !app.reqs[0].RequireSourceReads {
		t.Fatalf("expected source-bound one-shot request, got %+v", app.reqs[0])
	}
	if strings.Contains(app.reqs[0].SystemPrompt, "Never call tools.") || !strings.Contains(app.reqs[0].SystemPrompt, "file_read") {
		t.Fatalf("expected source-bound structured system prompt, got %q", app.reqs[0].SystemPrompt)
	}
}

func TestCareerBackgroundPromptsReferenceSourcesWithoutEmbeddingContent(t *testing.T) {
	secret := "示例资料正文-不应进入prompt"
	classificationPrompt := buildFileClassificationPrompt([]InboxFileForClassification{{
		SourcePath: "inbox/source.md",
		SourceHash: "sha256:test",
		Content:    secret,
	}})
	if strings.Contains(classificationPrompt, secret) || strings.Contains(classificationPrompt, "<content>") {
		t.Fatalf("classification prompt leaked content:\n%s", classificationPrompt)
	}
	if !strings.Contains(classificationPrompt, "inbox/source.md") || !strings.Contains(classificationPrompt, "sha256:test") {
		t.Fatalf("classification prompt missing source metadata:\n%s", classificationPrompt)
	}

	source := SourceRef{Path: "我的简历/source.md", Version: "sha256:resume"}
	bundlePrompt := buildGeneratedDocumentBundlePrompt("generate_project_pack", "生成项目专项", "项目专项", []SourceRef{source}, map[string]string{source.Path: secret})
	if strings.Contains(bundlePrompt, secret) || strings.Contains(bundlePrompt, "<content>") {
		t.Fatalf("bundle prompt leaked content:\n%s", bundlePrompt)
	}
	if !strings.Contains(bundlePrompt, source.Path) || !strings.Contains(bundlePrompt, source.Version) {
		t.Fatalf("bundle prompt missing source metadata:\n%s", bundlePrompt)
	}

	reviewPrompt := BuildReviewQuestionBankPrompt(ReviewQuestionBankRequest{
		SourceItem: WorkspaceItem{Path: "面经汇总/source.md"},
		Context: ReviewLibraryContext{
			ExperienceContent: secret,
			ResumePath:        "我的简历/source.md",
			ResumeContent:     secret,
			JDPath:            "岗位明细/source.md",
			JDContent:         secret,
		},
	})
	if strings.Contains(reviewPrompt, secret) || strings.Contains(reviewPrompt, "<materials>") {
		t.Fatalf("review question prompt leaked content:\n%s", reviewPrompt)
	}
	for _, want := range []string{"面经汇总/source.md", "我的简历/source.md", "岗位明细/source.md"} {
		if !strings.Contains(reviewPrompt, want) {
			t.Fatalf("review question prompt missing %q:\n%s", want, reviewPrompt)
		}
	}
}

func sameStrings(got []string, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
