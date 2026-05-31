package career

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"happyagent/internal/app"
	"happyagent/internal/config"
	"happyagent/internal/store"
)

type LLMReviewQuestionBankGenerator struct {
	App       Application
	Config    config.Config
	SessionID string
}

func (g *LLMReviewQuestionBankGenerator) GenerateQuestionBank(ctx context.Context, req ReviewQuestionBankRequest) (ReviewQuestionBank, error) {
	if g == nil || g.App == nil {
		return ReviewQuestionBank{}, fmt.Errorf("LLM question bank generator requires application")
	}
	sessionID := strings.TrimSpace(g.SessionID)
	if sessionID == "" {
		session, err := g.App.CreateSession(ProfileName)
		if err != nil {
			return ReviewQuestionBank{}, fmt.Errorf("create LLM question bank session: %w", err)
		}
		sessionID = session.ID
		g.SessionID = sessionID
	}
	record, err := g.App.AppendUserTurn(ctx, app.AppendTurnRequest{
		SessionID:          sessionID,
		ProfileName:        ProfileName,
		Input:              BuildReviewQuestionBankPrompt(req),
		SystemPrompt:       g.Config.Engine.SystemPrompt,
		ToolScope:          []string{"file_read", "final_answer"},
		SourceReadPaths:    reviewQuestionBankSourcePaths(req),
		RequireSourceReads: true,
		SuppressHistory:    true,
		SuppressMemory:     true,
	})
	if err != nil {
		return ReviewQuestionBank{}, fmt.Errorf("run LLM question bank generation: %w", err)
	}
	bank, parseErr := ParseReviewQuestionBankString(record.Output)
	if parseErr == nil {
		return bank, nil
	}
	repaired, repairErr := g.repairQuestionBank(ctx, sessionID, record, parseErr)
	if repairErr != nil {
		return ReviewQuestionBank{}, fmt.Errorf("parse LLM question bank json: %w; repair failed: %v", parseErr, repairErr)
	}
	return repaired, nil
}

func (g *LLMReviewQuestionBankGenerator) repairQuestionBank(ctx context.Context, sessionID string, record store.RunRecord, parseErr error) (ReviewQuestionBank, error) {
	repairedRecord, err := g.App.AppendUserTurn(ctx, app.AppendTurnRequest{
		SessionID:       sessionID,
		ProfileName:     ProfileName,
		Input:           BuildReviewQuestionBankRepairPrompt(record.Output, parseErr),
		SystemPrompt:    g.Config.Engine.SystemPrompt,
		ToolScope:       []string{"final_answer"},
		SuppressHistory: true,
		SuppressMemory:  true,
	})
	if err != nil {
		return ReviewQuestionBank{}, err
	}
	return ParseReviewQuestionBankString(repairedRecord.Output)
}

func BuildReviewQuestionBankPrompt(req ReviewQuestionBankRequest) string {
	ctx := req.Context
	return fmt.Sprintf(`<review_question_bank_generation>
  <task>Generate an accurate, evidence-grounded interview review question bank for one knowledge topic.</task>
  <product_requirement>
    HappyAgent must provide accurate, clear, traceable analysis. Do not fabricate answers.
    Use the declared source files only. Read them with file_read before final_answer. If evidence is missing, say what is missing in risk_or_missing_evidence.
  </product_requirement>
  <topic>
    <name>%s</name>
    <domain>%s</domain>
  </topic>
  <source_paths>
    <experience>%s</experience>
    <experience_read_path>%s</experience_read_path>
    <resume>%s</resume>
    <resume_read_path>%s</resume_read_path>
    <jd>%s</jd>
    <jd_read_path>%s</jd_read_path>
  </source_paths>
  <output_contract>
Return final_answer with valid JSON only, no Markdown fences, with this exact shape:
{
  "topic_name": "...",
  "questions": [
    {
      "question": "...",
      "exam_points": ["..."],
      "answer": "...",
      "resume_based_answer": "...",
      "followups": ["..."],
      "risk_or_missing_evidence": ["..."],
      "source_paths": ["..."]
    }
  ]
}
  </output_contract>
  <constraints>
    - Generate 3 to 8 questions from the public interview experience and the topic.
    - answer must directly answer the question and be suitable for interview preparation.
    - resume_based_answer must only use facts present in the supplied resume or say that matching resume evidence is missing.
    - Do not invent employment history, metrics, projects, companies, dates, tools, titles, scale, production usage, or outcomes.
    - source_paths must cite the supplied workspace paths that support the answer.
    - Keep every JSON string value on one line. Do not put raw line breaks inside quoted strings.
  </constraints>
</review_question_bank_generation>`,
		xmlEscape(firstNonEmpty(req.Topic.Name, req.Domain.Name, "通用高频问题")),
		xmlEscape(req.Domain.Name),
		xmlEscape(req.SourceItem.Path),
		xmlEscape(workspaceRootReadPath(req.WorkspaceRoot, req.SourceItem.Path)),
		xmlEscape(emptyIfBlank(ctx.ResumePath)),
		xmlEscape(emptyIfBlank(workspaceRootReadPath(req.WorkspaceRoot, ctx.ResumePath))),
		xmlEscape(emptyIfBlank(ctx.JDPath)),
		xmlEscape(emptyIfBlank(workspaceRootReadPath(req.WorkspaceRoot, ctx.JDPath))),
	)
}

func BuildReviewQuestionBankRepairPrompt(output string, parseErr error) string {
	return fmt.Sprintf(`<review_question_bank_repair>
  <problem>The previous final_answer content was not valid review question bank JSON.</problem>
  <json_parse_error>%v</json_parse_error>
  <repair_task>
    - Return only one valid JSON object for the review question bank schema.
    - Do not use Markdown fences.
    - Preserve evidence boundaries. Do not add new facts.
    - Required fields: topic_name, questions[].question, exam_points, answer, resume_based_answer, followups, risk_or_missing_evidence, source_paths.
  </repair_task>
  <invalid_previous_content>
%s
  </invalid_previous_content>
</review_question_bank_repair>`, parseErr, output)
}

func reviewQuestionBankSourcePaths(req ReviewQuestionBankRequest) []string {
	ctx := req.Context
	paths := []string{
		workspaceRootReadPath(req.WorkspaceRoot, req.SourceItem.Path),
		workspaceRootReadPath(req.WorkspaceRoot, ctx.ResumePath),
		workspaceRootReadPath(req.WorkspaceRoot, ctx.JDPath),
	}
	var result []string
	seen := map[string]bool{}
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		key := filepath.ToSlash(path)
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, key)
	}
	return result
}

func workspaceRootReadPath(root string, rel string) string {
	return filepath.ToSlash(strings.TrimSpace(rel))
}

func limitPromptContent(content string) string {
	content = strings.TrimSpace(content)
	const maxRunes = 12000
	runes := []rune(content)
	if len(runes) <= maxRunes {
		return content
	}
	return string(runes[:maxRunes]) + "\n...[truncated]"
}

func xmlEscape(value string) string {
	replacer := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return replacer.Replace(value)
}

func reviewLibraryTimeout(cfg config.Config) time.Duration {
	timeout := time.Duration(cfg.Engine.RunTimeoutSeconds) * time.Second
	if timeout <= 0 {
		return 180 * time.Second
	}
	return timeout
}
