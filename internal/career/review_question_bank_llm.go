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
	// ExistingTopicNames lists topic names already present in the workspace.
	// The LLM should avoid creating topics with the same or very similar names.
	ExistingTopicNames []string
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
	existingTopicsBlock := ""
	if len(req.ExistingTopicNames) > 0 {
		existingTopicsBlock = "\n  <existing_topics>\n    以下 topic 已在复习资料库中存在，请避免生成相同或高度相似的 topic_name。如果内容有重叠，请合并到已有 topic 的题库中，不要新建重复目录。\n"
		for _, name := range req.ExistingTopicNames {
			existingTopicsBlock += "    - " + xmlEscape(name) + "\n"
		}
		existingTopicsBlock += "  </existing_topics>\n"
	}
	return fmt.Sprintf(`<review_question_bank_generation>
  <task>基于面经和 JD 生成一套完整的面试复习资料库。</task>
  <product_requirement>
    必须基于真实材料生成，不编造。读取所有声明的源文件后再输出。
  </product_requirement>
  <instructions>
    1. 读取所有源文件（面经、JD、简历），全面理解材料内容后再输出。
    2. 确定岗位方向名（如"AI Agent 开发""后端开发"），使用标准技术术语，不要带"实习生""工程师""岗位"等后缀。
    3. 把面经和 JD 中的问题归类到 3-5 个主题分类中。分类维度建议：基础概念与架构、工程实践与框架、上下文与记忆管理、评测与可观测性、异常处理与边界 case。每个问题必须出现在某个分类里。
    4. 每个分类生成一个题库，包含 8-15 个问题。面经中已有的问题必须包含，并基于 JD 和面经扩展更多高频问题。
    5. 每个问题必须有：
       - 考点：面试官考察的核心能力点
       - 标准答案：详细、有技术深度、包含实际案例和边界讨论，可直接用于面试准备
       - 结合简历的回答：结合简历中的真实项目来回答，如无简历则说明缺少证据
       - 追问：2-3 个追问，每个追问必须附带完整的参考答案
       - 风险/待补证据：需要补充的材料或可能被 challenge 的点
    6. answer 必须有足够深度：先给核心结论，再展开技术细节，最后给出实际案例或边界讨论。不能只有一两句话。
    7. 如果有简历，提取 1-2 个关键项目用于回答项目类问题。
  </instructions>
  <source_paths>
    <experience>%s</experience>
    <experience_read_path>%s</experience_read_path>
    <resume>%s</resume>
    <resume_read_path>%s</resume_read_path>
    <jd>%s</jd>
    <jd_read_path>%s</jd_read_path>
  </source_paths>%s
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
          "answer": "详细标准答案，可直接用于面试准备，需要有技术深度和实际案例",
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
    - domain_name 是岗位方向抽象名（如"AI Agent 开发"），使用标准技术术语，不要包含"面经_"前缀或"实习生/工程师"等后缀。
    - topic_name 是宽泛的技术主题分类（如"Agent 架构设计""Tool Calling 与 MCP""上下文与记忆管理"），使用标准技术术语，不要照搬面试问题原文。
    - 必须生成 3-5 个 topics，全面覆盖面经和 JD 中的主要技术方向。
    - 每个 topic 包含 8-15 个 questions，确保覆盖面足够广。
    - answer 必须是可直接用于面试准备的详细答案：先给核心结论，再展开技术细节，最后给出实际案例。每个 answer 至少 3 段。
    - followups 中每个追问必须附带完整参考答案，格式为"追问问题？→ 参考答案"。
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
		existingTopicsBlock,
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
