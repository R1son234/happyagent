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

// GenerateQuestionBank implements the old ReviewQuestionBankGenerator interface.
// It is kept for backward compatibility but the new GenerateQuestionBankSet should be preferred.
func (g *LLMReviewQuestionBankGenerator) GenerateQuestionBank(ctx context.Context, req ReviewQuestionBankRequest) (ReviewQuestionBank, error) {
	// Delegate to set-based generation with a single topic
	set, err := g.GenerateQuestionBankSet(ctx, ReviewQuestionBankSetRequest{
		WorkspaceRoot: req.WorkspaceRoot,
		SourceItem:    req.SourceItem,
		Context:       req.Context,
	})
	if err != nil {
		return ReviewQuestionBank{}, err
	}
	if len(set.Topics) == 0 {
		return ReviewQuestionBank{}, fmt.Errorf("LLM returned no topics")
	}
	return set.Topics[0], nil
}

// ReviewQuestionBankSetRequest is the input for the LLM-driven review library generation.
type ReviewQuestionBankSetRequest struct {
	WorkspaceRoot string
	SourceItem    WorkspaceItem
	Context       ReviewLibraryContext
}

func (g *LLMReviewQuestionBankGenerator) GenerateQuestionBankSet(ctx context.Context, req ReviewQuestionBankSetRequest) (ReviewQuestionBankSet, error) {
	if g == nil || g.App == nil {
		return ReviewQuestionBankSet{}, fmt.Errorf("LLM question bank generator requires application")
	}
	sessionID := strings.TrimSpace(g.SessionID)
	if sessionID == "" {
		session, err := g.App.CreateSession(ProfileName)
		if err != nil {
			return ReviewQuestionBankSet{}, fmt.Errorf("create LLM question bank session: %w", err)
		}
		sessionID = session.ID
		g.SessionID = sessionID
	}
	record, err := g.App.AppendUserTurn(ctx, app.AppendTurnRequest{
		SessionID:          sessionID,
		ProfileName:        ProfileName,
		Input:              BuildReviewQuestionBankSetPrompt(req),
		SystemPrompt:       g.Config.Engine.SystemPrompt,
		ToolScope:          []string{"file_read", "final_answer"},
		SourceReadPaths:    reviewQuestionBankSetSourcePaths(req),
		RequireSourceReads: true,
		SuppressHistory:    true,
		SuppressMemory:     true,
	})
	if err != nil {
		return ReviewQuestionBankSet{}, fmt.Errorf("run LLM question bank set generation: %w", err)
	}
	set, parseErr := ParseReviewQuestionBankSetString(record.Output)
	if parseErr == nil {
		return set, nil
	}
	repaired, repairErr := g.repairQuestionBankSet(ctx, sessionID, record, parseErr)
	if repairErr != nil {
		return ReviewQuestionBankSet{}, fmt.Errorf("parse LLM question bank set json: %w; repair failed: %v", parseErr, repairErr)
	}
	return repaired, nil
}

func (g *LLMReviewQuestionBankGenerator) repairQuestionBankSet(ctx context.Context, sessionID string, record store.RunRecord, parseErr error) (ReviewQuestionBankSet, error) {
	repairedRecord, err := g.App.AppendUserTurn(ctx, app.AppendTurnRequest{
		SessionID:       sessionID,
		ProfileName:     ProfileName,
		Input:           BuildReviewQuestionBankSetRepairPrompt(record.Output, parseErr),
		SystemPrompt:    g.Config.Engine.SystemPrompt,
		ToolScope:       []string{"final_answer"},
		SuppressHistory: true,
		SuppressMemory:  true,
	})
	if err != nil {
		return ReviewQuestionBankSet{}, err
	}
	return ParseReviewQuestionBankSetString(repairedRecord.Output)
}

func BuildReviewQuestionBankSetPrompt(req ReviewQuestionBankSetRequest) string {
	ctx := req.Context
	return fmt.Sprintf(`<review_question_bank_generation>
  <task>基于面经和 JD 生成一套完整的面试复习资料库。</task>
  <product_requirement>
    必须基于真实材料生成，不编造。读取所有声明的源文件后再输出。
  </product_requirement>
  <instructions>
    1. 读取所有源文件（面经、JD、简历）。
    2. 确定岗位方向名（如"市场营销新媒体运营"、"后端开发"），不要带"实习生""工程师""岗位"等后缀。
    3. 把面经和 JD 中的问题归类到 2-3 个主题分类中。每个问题必须出现在某个分类里。
    4. 每个分类生成一个题库，包含 3-5 个问题。面经中已有的问题必须包含。
    5. 每个问题必须有：考点、标准答案（可直接背诵）、结合简历的回答、1-2 个追问及追问答案。
    6. 追问必须给出参考答案，不能只列问题不给答案。
    7. 如果有简历，提取 1-2 个关键项目用于回答项目类问题。
  </instructions>
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
  "domain_name": "岗位方向名（不含实习生/工程师等后缀）",
  "topics": [
    {
      "topic_name": "主题分类名",
      "questions": [
        {
          "question": "面试问题",
          "exam_points": ["考点1", "考点2"],
          "answer": "详细标准答案，可直接用于面试准备",
          "resume_based_answer": "结合简历的回答，如无简历则说明缺少证据",
          "followups": ["追问1？→ 参考答案", "追问2？→ 参考答案"],
          "risk_or_missing_evidence": ["待补证据说明"],
          "source_paths": ["来源路径"]
        }
      ]
    }
  ],
  "projects": [
    {
      "project_name": "项目名",
      "evidence_lines": ["简历中的相关行"],
      "star_hint": "STAR回答提示"
    }
  ]
}
  </output_contract>
  <constraints>
    - domain_name 是岗位方向抽象名（如"市场营销新媒体运营"），不要包含"面经_"前缀或"实习生/工程师"等后缀。
    - topic_name 是宽泛的主题分类（如"内容策划""账号运营""活动运营"），不要照搬面试问题原文。
    - 必须生成 2-3 个 topics，覆盖面经和 JD 中的主要问题。
    - 每个 topic 包含 3-5 个 questions。
    - answer 必须是可直接背诵的面试答案，简洁但完整。
    - followups 中每个追问必须附带参考答案，格式为"追问问题？→ 参考答案"。
    - resume_based_answer 只能使用简历中已有的事实，不能编造。
    - projects 仅在有简历时提取，无简历返回空数组。
    - 每个 JSON 字符串值放在一行内，不要在引号内换行。
  </constraints>
</review_question_bank_generation>`,
		xmlEscape(req.SourceItem.Path),
		xmlEscape(workspaceRootReadPath(req.WorkspaceRoot, req.SourceItem.Path)),
		xmlEscape(emptyIfBlank(ctx.ResumePath)),
		xmlEscape(emptyIfBlank(workspaceRootReadPath(req.WorkspaceRoot, ctx.ResumePath))),
		xmlEscape(emptyIfBlank(ctx.JDPath)),
		xmlEscape(emptyIfBlank(workspaceRootReadPath(req.WorkspaceRoot, ctx.JDPath))),
	)
}

// BuildReviewQuestionBankPrompt is kept for backward compatibility with tests.
// It wraps BuildReviewQuestionBankSetPrompt.
func BuildReviewQuestionBankPrompt(req ReviewQuestionBankRequest) string {
	return BuildReviewQuestionBankSetPrompt(ReviewQuestionBankSetRequest{
		WorkspaceRoot: req.WorkspaceRoot,
		SourceItem:    req.SourceItem,
		Context:       req.Context,
	})
}

func BuildReviewQuestionBankSetRepairPrompt(output string, parseErr error) string {
	return fmt.Sprintf(`<review_question_bank_repair>
  <problem>The previous final_answer content was not valid review question bank set JSON.</problem>
  <json_parse_error>%v</json_parse_error>
  <repair_task>
    - Return only one valid JSON object for the review question bank set schema.
    - Do not use Markdown fences.
    - Preserve evidence boundaries. Do not add new facts.
    - Required fields: domain_name, topics (array with topic_name and questions).
    - Optional fields: projects (array with project_name, evidence_lines, star_hint).
    - Each question requires: question, exam_points, answer, resume_based_answer, followups, risk_or_missing_evidence, source_paths.
  </repair_task>
  <invalid_previous_content>
%s
  </invalid_previous_content>
</review_question_bank_repair>`, parseErr, output)
}

func reviewQuestionBankSetSourcePaths(req ReviewQuestionBankSetRequest) []string {
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
