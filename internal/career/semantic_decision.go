package career

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

const (
	SemanticFileActionInclude = "include"
	SemanticFileActionIgnore  = "ignore"
	SemanticFileActionAskUser = "ask_user"
)

const (
	OutputKindChat            = "chat"
	OutputKindReport          = "report"
	OutputKindResumeReview    = "resume-review"
	OutputKindInterviewBrief  = "interview-brief"
	OutputKindGapPlan         = "gap-plan"
	OutputKindInterviewReview = "interview-review"
	OutputKindReviewLibrary   = "review-library"
	OutputKindProjectPack     = "project-pack"
	OutputKindBattlePack      = "battle-pack"
)

type UserInputSemanticDecision struct {
	Intent                CareerIntent              `json:"intent"`
	Confidence            ConfidenceLevel           `json:"confidence"`
	Reason                string                    `json:"reason"`
	ShouldScanInbox       bool                      `json:"should_scan_inbox"`
	ShouldSaveUserInput   bool                      `json:"should_save_user_input"`
	UserInputMaterialType string                    `json:"user_input_material_type"`
	UserInputDestination  string                    `json:"user_input_destination"`
	NeedsUserConfirmation bool                      `json:"needs_user_confirmation"`
	QuestionsForUser      []string                  `json:"questions_for_user"`
	ReferencedFiles       []ReferencedFileDecision  `json:"referenced_files"`
	RequestedOutputs      []RequestedOutputDecision `json:"requested_outputs"`
	RequiredState         []string                  `json:"required_state"`
	RiskFlags             []string                  `json:"risk_flags"`
	Meta                  LLMTraceMeta              `json:"meta,omitempty"`
}

type ReferencedFileDecision struct {
	CandidateID           string          `json:"candidate_id"`
	SourcePath            string          `json:"source_path"`
	Action                string          `json:"action"`
	MaterialType          string          `json:"material_type"`
	Destination           string          `json:"destination"`
	Confidence            ConfidenceLevel `json:"confidence"`
	Reason                string          `json:"reason"`
	NeedsUserConfirmation bool            `json:"needs_user_confirmation"`
}

type RequestedOutputDecision struct {
	Kind            string   `json:"kind"`
	Title           string   `json:"title"`
	Reason          string   `json:"reason"`
	RequiredSources []string `json:"required_sources"`
}

type SemanticDecisionRequest struct {
	Input              string
	Guide              WorkspaceGuide
	Metadata           WorkspaceMetadata
	InboxCounts        map[string]int
	PendingItems       []PendingInboxItem
	Candidates         []SemanticFileCandidate
	WorkspaceRoot      string
	StructuredTaskName string
}

type SemanticFileCandidate struct {
	ID            string
	SourcePath    string
	ReadPath      string
	SourceHash    string
	Name          string
	Ext           string
	Size          int64
	ModifiedUnix  int64
	ExtractStatus string
	ExtractError  string
	Excerpt       string
}

func ClassifyUserInputWithLLM(ctx context.Context, runner StructuredTaskRunner, req SemanticDecisionRequest) (UserInputSemanticDecision, error) {
	if runner == nil {
		return UserInputSemanticDecision{}, fmt.Errorf("semantic decision requires structured task runner")
	}
	taskName := strings.TrimSpace(req.StructuredTaskName)
	if taskName == "" {
		taskName = "classify_career_user_input"
	}
	result, err := runner.RunStructuredTask(ctx, StructuredTaskRequest{
		TaskName:      taskName,
		PromptVersion: PromptVersionUserInputSemanticDecision,
		Input:         buildUserInputSemanticDecisionPrompt(req),
		SourcePaths:   semanticCandidateReadPaths(req.Candidates),
	})
	if err != nil {
		return UserInputSemanticDecision{}, err
	}
	decision, err := parseUserInputSemanticDecisionOutput(result.Output, req.Candidates)
	if err != nil {
		return UserInputSemanticDecision{}, err
	}
	decision.Meta = LLMTraceMeta{
		GeneratedAt:   result.GeneratedAt,
		Model:         result.Model,
		PromptVersion: PromptVersionUserInputSemanticDecision,
	}
	return decision, nil
}

func parseUserInputSemanticDecisionOutput(output string, candidates []SemanticFileCandidate) (UserInputSemanticDecision, error) {
	var decision UserInputSemanticDecision
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &decision); err != nil {
		return UserInputSemanticDecision{}, fmt.Errorf("parse semantic decision json: %w", err)
	}
	if strings.TrimSpace(string(decision.Intent)) == "" {
		return UserInputSemanticDecision{}, fmt.Errorf("semantic decision intent must not be empty")
	}
	if !isSupportedCareerIntent(decision.Intent) {
		return UserInputSemanticDecision{}, fmt.Errorf("semantic decision intent %q is not supported", decision.Intent)
	}
	if err := validateConfidence(decision.Confidence, "confidence"); err != nil {
		return UserInputSemanticDecision{}, err
	}
	if err := validateMaterialTypeOrUnknown(decision.UserInputMaterialType, "user_input_material_type"); err != nil {
		return UserInputSemanticDecision{}, err
	}
	candidatesByID := map[string]SemanticFileCandidate{}
	candidatesByPath := map[string]SemanticFileCandidate{}
	for _, candidate := range candidates {
		candidatesByID[candidate.ID] = candidate
		candidatesByPath[filepath.ToSlash(candidate.SourcePath)] = candidate
	}
	for i, file := range decision.ReferencedFiles {
		if strings.TrimSpace(file.CandidateID) == "" {
			return UserInputSemanticDecision{}, fmt.Errorf("referenced_files[%d].candidate_id must not be empty", i)
		}
		candidate, ok := candidatesByID[file.CandidateID]
		if !ok {
			return UserInputSemanticDecision{}, fmt.Errorf("referenced_files[%d].candidate_id %q is unknown", i, file.CandidateID)
		}
		sourcePath := filepath.ToSlash(strings.TrimSpace(file.SourcePath))
		if sourcePath == "" {
			sourcePath = filepath.ToSlash(candidate.SourcePath)
			decision.ReferencedFiles[i].SourcePath = sourcePath
		}
		if _, ok := candidatesByPath[sourcePath]; !ok || sourcePath != filepath.ToSlash(candidate.SourcePath) {
			return UserInputSemanticDecision{}, fmt.Errorf("referenced_files[%d].source_path %q does not match candidate %q", i, file.SourcePath, file.CandidateID)
		}
		switch strings.TrimSpace(file.Action) {
		case SemanticFileActionInclude, SemanticFileActionIgnore, SemanticFileActionAskUser:
		default:
			return UserInputSemanticDecision{}, fmt.Errorf("referenced_files[%d].action %q is not supported", i, file.Action)
		}
		if err := validateMaterialTypeOrUnknown(file.MaterialType, fmt.Sprintf("referenced_files[%d].material_type", i)); err != nil {
			return UserInputSemanticDecision{}, err
		}
		if err := validateConfidence(file.Confidence, fmt.Sprintf("referenced_files[%d].confidence", i)); err != nil {
			return UserInputSemanticDecision{}, err
		}
	}
	for i, output := range decision.RequestedOutputs {
		if !isSupportedOutputKind(output.Kind) {
			return UserInputSemanticDecision{}, fmt.Errorf("requested_outputs[%d].kind %q is not supported", i, output.Kind)
		}
	}
	return decision, nil
}

func buildUserInputSemanticDecisionPrompt(req SemanticDecisionRequest) string {
	var b strings.Builder
	b.WriteString("<career_user_input_semantic_decision>\n")
	b.WriteString("  <task>Decide the semantic intent, material classification, referenced files, and requested outputs for this Career Copilot turn. Return JSON only.</task>\n")
	b.WriteString("  <rules>\n")
	b.WriteString("    - Use semantic understanding of the user input and candidate materials.\n")
	b.WriteString("    - Do not invent paths. referenced_files must use only listed candidate_id and source_path values.\n")
	b.WriteString("    - Do not save ordinary requests as material. Save user input only when the input itself contains resume, JD, interview experience, interview record, project material, or review note content.\n")
	b.WriteString("    - Distinguish a request to optimize a resume from pasted resume content.\n")
	b.WriteString("    - If uncertain, set needs_user_confirmation=true and provide questions_for_user.\n")
	b.WriteString("    - Do not fabricate facts, companies, metrics, user experience, or source content.\n")
	b.WriteString("  </rules>\n")
	b.WriteString("  <allowed_intents>chat,ingest,analyze,resume_review,interview_brief,gap_plan,interview_review,status,memory</allowed_intents>\n")
	b.WriteString("  <allowed_material_types>resume,jd,public_interview_experience,project_material,real_interview_record,review_note,unknown</allowed_material_types>\n")
	b.WriteString("  <allowed_output_kinds>chat,report,resume-review,interview-brief,gap-plan,interview-review,review-library,project-pack,battle-pack</allowed_output_kinds>\n")
	b.WriteString("  <workspace_guide>\n" + xmlEscape(req.Guide.PromptSummary()) + "\n  </workspace_guide>\n")
	b.WriteString("  <workspace_state>\n")
	b.WriteString("    <current_resume>" + xmlEscape(emptyIfBlank(req.Metadata.CurrentResume)) + "</current_resume>\n")
	b.WriteString("    <active_jd>" + xmlEscape(emptyIfBlank(req.Metadata.ActiveJD)) + "</active_jd>\n")
	b.WriteString("    <active_project>" + xmlEscape(emptyIfBlank(req.Metadata.ActiveProject)) + "</active_project>\n")
	b.WriteString("  </workspace_state>\n")
	if len(req.InboxCounts) > 0 || len(req.PendingItems) > 0 {
		b.WriteString("  <inbox_state>\n")
		for _, key := range sortedIntKeys(req.InboxCounts) {
			b.WriteString(fmt.Sprintf("    <count status=\"%s\">%d</count>\n", xmlEscape(key), req.InboxCounts[key]))
		}
		for _, item := range req.PendingItems {
			b.WriteString("    <pending>\n")
			b.WriteString("      <source_path>" + xmlEscape(item.SourcePath) + "</source_path>\n")
			b.WriteString("      <material_type>" + xmlEscape(item.MaterialType) + "</material_type>\n")
			b.WriteString("      <confidence>" + xmlEscape(string(item.Confidence)) + "</confidence>\n")
			b.WriteString("      <reason>" + xmlEscape(item.Reason) + "</reason>\n")
			b.WriteString("    </pending>\n")
		}
		b.WriteString("  </inbox_state>\n")
	}
	b.WriteString("  <file_candidates>\n")
	for _, candidate := range req.Candidates {
		b.WriteString("    <candidate>\n")
		b.WriteString("      <candidate_id>" + xmlEscape(candidate.ID) + "</candidate_id>\n")
		b.WriteString("      <source_path>" + xmlEscape(filepath.ToSlash(candidate.SourcePath)) + "</source_path>\n")
		b.WriteString("      <read_path>" + xmlEscape(filepath.ToSlash(firstNonEmpty(candidate.ReadPath, candidate.SourcePath))) + "</read_path>\n")
		b.WriteString("      <source_hash>" + xmlEscape(candidate.SourceHash) + "</source_hash>\n")
		b.WriteString("      <name>" + xmlEscape(candidate.Name) + "</name>\n")
		b.WriteString("      <ext>" + xmlEscape(candidate.Ext) + "</ext>\n")
		b.WriteString(fmt.Sprintf("      <size>%d</size>\n", candidate.Size))
		b.WriteString(fmt.Sprintf("      <modified_unix>%d</modified_unix>\n", candidate.ModifiedUnix))
		b.WriteString("      <extract_status>" + xmlEscape(candidate.ExtractStatus) + "</extract_status>\n")
		if strings.TrimSpace(candidate.ExtractError) != "" {
			b.WriteString("      <extract_error>" + xmlEscape(candidate.ExtractError) + "</extract_error>\n")
		}
		b.WriteString("      <excerpt>" + xmlEscape(candidate.Excerpt) + "</excerpt>\n")
		b.WriteString("    </candidate>\n")
	}
	b.WriteString("  </file_candidates>\n")
	b.WriteString("  <user_input>\n" + xmlEscape(req.Input) + "\n  </user_input>\n")
	b.WriteString(`  <output_contract>{"intent":"chat","confidence":"high","reason":"...","should_scan_inbox":false,"should_save_user_input":false,"user_input_material_type":"unknown","user_input_destination":"","needs_user_confirmation":false,"questions_for_user":[],"referenced_files":[{"candidate_id":"file-1","source_path":"inbox/example.md","action":"include","material_type":"resume","destination":"我的简历","confidence":"high","reason":"...","needs_user_confirmation":false}],"requested_outputs":[{"kind":"chat","title":"回答","reason":"...","required_sources":[]}],"required_state":[],"risk_flags":[]}</output_contract>` + "\n")
	b.WriteString("</career_user_input_semantic_decision>")
	return b.String()
}

func semanticCandidateReadPaths(candidates []SemanticFileCandidate) []string {
	paths := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		path := strings.TrimSpace(firstNonEmpty(candidate.ReadPath, candidate.SourcePath))
		if path != "" {
			paths = append(paths, path)
		}
	}
	return uniqueStrings(paths)
}

func isSupportedCareerIntent(intent CareerIntent) bool {
	switch intent {
	case CareerIntentChat, CareerIntentIngest, CareerIntentAnalyze, CareerIntentResumeReview, CareerIntentInterviewBrief, CareerIntentGapPlan, CareerIntentInterviewReview, CareerIntentStatus, CareerIntentMemory:
		return true
	default:
		return false
	}
}

func validateConfidence(confidence ConfidenceLevel, field string) error {
	switch confidence {
	case ConfidenceHigh, ConfidenceMedium, ConfidenceLow:
		return nil
	default:
		return fmt.Errorf("%s %q is not supported", field, confidence)
	}
}

func validateMaterialTypeOrUnknown(materialType string, field string) error {
	materialType = strings.TrimSpace(materialType)
	if materialType == "" || materialType == "unknown" {
		return nil
	}
	if _, ok := workspaceTypeForMaterialType(materialType); ok {
		return nil
	}
	if IsSupportedWorkspaceType(materialType) && materialType != WorkspaceTypeGeneral {
		return nil
	}
	return fmt.Errorf("%s %q is not supported", field, materialType)
}

func isSupportedOutputKind(kind string) bool {
	switch strings.TrimSpace(kind) {
	case OutputKindChat, OutputKindReport, OutputKindResumeReview, OutputKindInterviewBrief, OutputKindGapPlan, OutputKindInterviewReview, OutputKindReviewLibrary, OutputKindProjectPack, OutputKindBattlePack:
		return true
	default:
		return false
	}
}

func sortedIntKeys(values map[string]int) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
