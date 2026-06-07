package career

import (
	"strings"
	"testing"
)

func TestParseUserInputSemanticDecisionOutput(t *testing.T) {
	output := `{
		"intent":"resume_review",
		"confidence":"high",
		"reason":"用户要求优化简历",
		"should_scan_inbox":false,
		"should_save_user_input":false,
		"user_input_material_type":"unknown",
		"user_input_destination":"",
		"needs_user_confirmation":false,
		"questions_for_user":[],
		"referenced_files":[{"candidate_id":"file-1","source_path":"resume.md","action":"include","material_type":"resume","destination":"我的简历","confidence":"high","reason":"候选文件是简历","needs_user_confirmation":false}],
		"requested_outputs":[{"kind":"resume-review","title":"简历优化建议","reason":"用户要求优化简历","required_sources":["file-1"]}],
		"required_state":["current_resume"],
		"risk_flags":[]
	}`
	decision, err := parseUserInputSemanticDecisionOutput(output, []SemanticFileCandidate{{ID: "file-1", SourcePath: "resume.md"}})
	if err != nil {
		t.Fatalf("parseUserInputSemanticDecisionOutput() error = %v", err)
	}
	if decision.Intent != CareerIntentResumeReview {
		t.Fatalf("intent = %q, want %q", decision.Intent, CareerIntentResumeReview)
	}
	if len(decision.ReferencedFiles) != 1 || decision.ReferencedFiles[0].CandidateID != "file-1" {
		t.Fatalf("referenced files not parsed: %+v", decision.ReferencedFiles)
	}
}

func TestParseUserInputSemanticDecisionRejectsUnknownCandidate(t *testing.T) {
	output := `{"intent":"ingest","confidence":"high","reason":"x","should_scan_inbox":false,"should_save_user_input":false,"user_input_material_type":"unknown","user_input_destination":"","needs_user_confirmation":false,"questions_for_user":[],"referenced_files":[{"candidate_id":"file-2","source_path":"resume.md","action":"include","material_type":"resume","destination":"我的简历","confidence":"high","reason":"x","needs_user_confirmation":false}],"requested_outputs":[],"required_state":[],"risk_flags":[]}`
	_, err := parseUserInputSemanticDecisionOutput(output, []SemanticFileCandidate{{ID: "file-1", SourcePath: "resume.md"}})
	if err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("expected unknown candidate error, got %v", err)
	}
}

func TestParseUserInputSemanticDecisionRejectsUnsupportedEnums(t *testing.T) {
	output := `{"intent":"keyword_magic","confidence":"high","reason":"x","should_scan_inbox":false,"should_save_user_input":false,"user_input_material_type":"unknown","user_input_destination":"","needs_user_confirmation":false,"questions_for_user":[],"referenced_files":[],"requested_outputs":[],"required_state":[],"risk_flags":[]}`
	_, err := parseUserInputSemanticDecisionOutput(output, nil)
	if err == nil || !strings.Contains(err.Error(), "intent") {
		t.Fatalf("expected intent error, got %v", err)
	}
}

func TestBuildUserInputSemanticDecisionPromptIncludesGuideAndCandidates(t *testing.T) {
	prompt := buildUserInputSemanticDecisionPrompt(SemanticDecisionRequest{
		Input: "帮我整理这个简历",
		Guide: DefaultWorkspaceGuide(),
		Candidates: []SemanticFileCandidate{{
			ID:         "file-1",
			SourcePath: "resume.md",
			ReadPath:   "/tmp/resume.md",
			SourceHash: "sha256:abc",
			Name:       "resume.md",
			Ext:        ".md",
			Excerpt:    "工作经历：示例经历",
		}},
	})
	for _, want := range []string{"career_user_input_semantic_decision", "Workspace directory guide", "file-1", "resume.md", "帮我整理这个简历"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}
