package career

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"happyagent/internal/app"
	"happyagent/internal/config"
)

const (
	PromptVersionFileClassification = "career-file-classification-v1"
	PromptVersionJDSplit            = "career-jd-split-v1"
	PromptVersionProjectPack        = "career-project-pack-v1"
	PromptVersionBattlePack         = "career-battle-pack-v1"
	PromptVersionInterviewReview    = "career-interview-review-v1"
	PromptVersionReviewLibrary      = "career-review-library-v1"
)

type StructuredTaskRunner interface {
	RunStructuredTask(ctx context.Context, req StructuredTaskRequest) (StructuredTaskResult, error)
}

type StructuredTaskRequest struct {
	TaskName      string   `json:"task_name"`
	PromptVersion string   `json:"prompt_version"`
	Input         string   `json:"input"`
	SourcePaths   []string `json:"source_paths"`
}

type StructuredTaskResult struct {
	Output      string    `json:"output"`
	Model       string    `json:"model"`
	RunID       string    `json:"run_id"`
	SessionID   string    `json:"session_id"`
	GeneratedAt time.Time `json:"generated_at"`
}

type appStructuredTaskRunner struct {
	App       Application
	Config    config.Config
	SessionID string
	Now       func() time.Time
}

func (r *appStructuredTaskRunner) RunStructuredTask(ctx context.Context, req StructuredTaskRequest) (StructuredTaskResult, error) {
	if r == nil || r.App == nil {
		return StructuredTaskResult{}, fmt.Errorf("structured task runner requires application")
	}
	sessionID := strings.TrimSpace(r.SessionID)
	if sessionID == "" {
		session, err := r.App.CreateSession(ProfileName)
		if err != nil {
			return StructuredTaskResult{}, fmt.Errorf("create structured task session: %w", err)
		}
		sessionID = session.ID
		r.SessionID = sessionID
	}
	record, err := r.App.AppendUserTurn(ctx, app.AppendTurnRequest{
		SessionID:     sessionID,
		ProfileName:   ProfileName,
		Input:         req.Input,
		SystemPrompt:  r.Config.Engine.SystemPrompt,
		ApprovedTools: r.Config.Tools.ApprovedTools,
	})
	if err != nil {
		return StructuredTaskResult{}, err
	}
	now := time.Now()
	if r.Now != nil {
		now = r.Now()
	}
	return StructuredTaskResult{
		Output:      record.Output,
		Model:       r.Config.LLM.Model,
		RunID:       record.ID,
		SessionID:   record.SessionID,
		GeneratedAt: now,
	}, nil
}

type fileClassificationOutput struct {
	Files []fileClassificationItem `json:"files"`
}

type fileClassificationItem struct {
	SourcePath            string          `json:"source_path"`
	SourceHash            string          `json:"source_hash"`
	MaterialType          string          `json:"material_type"`
	Confidence            ConfidenceLevel `json:"confidence"`
	Reason                string          `json:"reason"`
	SourceExcerpt         string          `json:"source_excerpt"`
	Destination           string          `json:"destination"`
	NeedsUserConfirmation bool            `json:"needs_user_confirmation"`
	QuestionsForUser      []string        `json:"questions_for_user"`
}

func parseFileClassificationOutput(output string) (fileClassificationOutput, error) {
	var parsed fileClassificationOutput
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &parsed); err != nil {
		return fileClassificationOutput{}, fmt.Errorf("parse file classification json: %w", err)
	}
	if len(parsed.Files) == 0 {
		return fileClassificationOutput{}, fmt.Errorf("file classification missing files")
	}
	for i, item := range parsed.Files {
		if strings.TrimSpace(item.SourcePath) == "" {
			return fileClassificationOutput{}, fmt.Errorf("files[%d].source_path must not be empty", i)
		}
		if _, ok := workspaceTypeForMaterialType(item.MaterialType); !ok && strings.TrimSpace(item.MaterialType) != "unknown" {
			return fileClassificationOutput{}, fmt.Errorf("files[%d].material_type %q is not supported", i, item.MaterialType)
		}
		switch item.Confidence {
		case ConfidenceHigh, ConfidenceMedium, ConfidenceLow:
		default:
			return fileClassificationOutput{}, fmt.Errorf("files[%d].confidence %q is not supported", i, item.Confidence)
		}
		if strings.TrimSpace(item.Destination) == "" {
			return fileClassificationOutput{}, fmt.Errorf("files[%d].destination must not be empty", i)
		}
	}
	return parsed, nil
}

func buildFileClassificationPrompt(files []InboxFileForClassification) string {
	var b strings.Builder
	b.WriteString("<career_file_classification>\n")
	b.WriteString("  <task>Classify inbox files for Career Copilot. Return JSON only.</task>\n")
	b.WriteString("  <allowed_material_types>resume,jd,public_interview_experience,project_material,real_interview_record,review_note,unknown</allowed_material_types>\n")
	b.WriteString("  <allowed_confidence>high,medium,low</allowed_confidence>\n")
	b.WriteString("  <rules>Use content understanding. Do not fabricate facts. medium and low require user confirmation.</rules>\n")
	b.WriteString("  <files>\n")
	for _, file := range files {
		b.WriteString("    <file>\n")
		b.WriteString("      <source_path>" + xmlEscape(file.SourcePath) + "</source_path>\n")
		b.WriteString("      <source_hash>" + xmlEscape(file.SourceHash) + "</source_hash>\n")
		b.WriteString("      <content>\n" + xmlEscape(limitPromptContent(file.Content)) + "\n      </content>\n")
		b.WriteString("    </file>\n")
	}
	b.WriteString("  </files>\n")
	b.WriteString(`  <output_contract>{"files":[{"source_path":"inbox/example.md","source_hash":"sha256:...","material_type":"jd","confidence":"high","reason":"...","source_excerpt":"...","destination":"岗位明细","needs_user_confirmation":false,"questions_for_user":[]}]}</output_contract>` + "\n")
	b.WriteString("</career_file_classification>")
	return b.String()
}

type InboxFileForClassification struct {
	SourcePath string
	SourceHash string
	Content    string
}

type jdSplitOutput struct {
	JobDescriptions []jdSplitItem `json:"job_descriptions"`
}

type jdSplitItem struct {
	Title            string          `json:"title"`
	Company          string          `json:"company"`
	Team             string          `json:"team"`
	Role             string          `json:"role"`
	Responsibilities []string        `json:"responsibilities"`
	Requirements     []string        `json:"requirements"`
	Keywords         []string        `json:"keywords"`
	SourceExcerpt    string          `json:"source_excerpt"`
	Confidence       ConfidenceLevel `json:"confidence"`
}

func parseJDSplitOutput(output string) (jdSplitOutput, error) {
	var parsed jdSplitOutput
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &parsed); err != nil {
		return jdSplitOutput{}, fmt.Errorf("parse jd split json: %w", err)
	}
	if len(parsed.JobDescriptions) == 0 {
		return jdSplitOutput{}, fmt.Errorf("jd split missing job_descriptions")
	}
	for i, item := range parsed.JobDescriptions {
		if strings.TrimSpace(item.Title) == "" {
			return jdSplitOutput{}, fmt.Errorf("job_descriptions[%d].title must not be empty", i)
		}
		if strings.TrimSpace(item.SourceExcerpt) == "" {
			return jdSplitOutput{}, fmt.Errorf("job_descriptions[%d].source_excerpt must not be empty", i)
		}
		switch item.Confidence {
		case ConfidenceHigh, ConfidenceMedium, ConfidenceLow:
		default:
			return jdSplitOutput{}, fmt.Errorf("job_descriptions[%d].confidence %q is not supported", i, item.Confidence)
		}
	}
	return parsed, nil
}

func buildJDSplitPrompt(sourcePath string, content string) string {
	var b strings.Builder
	b.WriteString("<career_jd_split>\n")
	b.WriteString("  <task>Split one source that may contain multiple job descriptions. Return JSON only.</task>\n")
	b.WriteString("  <rules>Use only the provided source. Do not fabricate company, team, requirements, or responsibilities. If missing, use empty strings or empty arrays.</rules>\n")
	b.WriteString("  <source_path>" + xmlEscape(sourcePath) + "</source_path>\n")
	b.WriteString("  <content>\n" + xmlEscape(limitPromptContent(content)) + "\n  </content>\n")
	b.WriteString(`  <output_contract>{"job_descriptions":[{"title":"...","company":"...","team":"...","role":"...","responsibilities":["..."],"requirements":["..."],"keywords":["..."],"source_excerpt":"...","confidence":"high"}]}</output_contract>` + "\n")
	b.WriteString("</career_jd_split>")
	return b.String()
}

type generatedMarkdownOutput struct {
	Title       string      `json:"title"`
	Markdown    string      `json:"markdown"`
	SourceRefs  []SourceRef `json:"source_refs"`
	RiskFlags   []string    `json:"risk_flags,omitempty"`
	MissingInfo []string    `json:"missing_info,omitempty"`
}

func parseGeneratedMarkdownOutput(output string) (generatedMarkdownOutput, error) {
	var parsed generatedMarkdownOutput
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &parsed); err != nil {
		return generatedMarkdownOutput{}, fmt.Errorf("parse generated markdown json: %w", err)
	}
	if strings.TrimSpace(parsed.Title) == "" {
		return generatedMarkdownOutput{}, fmt.Errorf("generated markdown title must not be empty")
	}
	if strings.TrimSpace(parsed.Markdown) == "" {
		return generatedMarkdownOutput{}, fmt.Errorf("generated markdown must not be empty")
	}
	for i, ref := range parsed.SourceRefs {
		if err := validateWorkspaceRelPath(ref.Path); err != nil {
			return generatedMarkdownOutput{}, fmt.Errorf("source_refs[%d].path: %w", i, err)
		}
	}
	return parsed, nil
}

func buildGeneratedMarkdownPrompt(taskName string, instructions string, sources []SourceRef, contents map[string]string) string {
	var b strings.Builder
	b.WriteString("<career_generation_task>\n")
	b.WriteString("  <task_name>" + xmlEscape(taskName) + "</task_name>\n")
	b.WriteString("  <instructions>" + xmlEscape(instructions) + "</instructions>\n")
	b.WriteString("  <rules>Use only the provided sources. Do not invent facts, metrics, dates, companies, projects, or user experience. Mark missing evidence as 待补证据.</rules>\n")
	b.WriteString("  <sources>\n")
	for _, ref := range sources {
		content := contents[filepath.ToSlash(ref.Path)]
		b.WriteString("    <source>\n")
		b.WriteString("      <path>" + xmlEscape(ref.Path) + "</path>\n")
		b.WriteString("      <version>" + xmlEscape(ref.Version) + "</version>\n")
		b.WriteString("      <content>\n" + xmlEscape(limitPromptContent(content)) + "\n      </content>\n")
		b.WriteString("    </source>\n")
	}
	b.WriteString("  </sources>\n")
	b.WriteString(`  <output_contract>{"title":"...","markdown":"...","source_refs":[{"path":"...","version":"...","excerpt":"...","evidence_spans":["..."]}],"risk_flags":[],"missing_info":[]}</output_contract>` + "\n")
	b.WriteString("</career_generation_task>")
	return b.String()
}
