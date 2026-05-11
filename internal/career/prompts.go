package career

import (
	"fmt"
	"path/filepath"
	"strings"
)

func BuildReportRepairPrompt(output string, parseErr error) string {
	return fmt.Sprintf(`The previous final_answer content was not valid career_report JSON.

JSON parse error:
%v

Repair task:
- Return only one valid JSON object for the career_report schema.
- Do not use Markdown fences.
- Do not include comments.
- Do not put raw line breaks inside JSON string values; use short single-line strings or arrays.
- Keep all fields required by the career_report schema: summary, jd_analysis, project_evidence, resume_rewrite, interview_brief, gap_plan, risk_flags, appendix.
- Preserve the same facts and evidence boundaries as much as possible.

Invalid previous content:
%s`, parseErr, output)
}

func BuildInteractivePrompt(input string, classification InputClassification) string {
	return BuildInteractivePromptWithAutoSaved(input, classification, nil, nil, WorkspaceMetadata{}, false, "")
}

func BuildInteractivePromptWithAutoSaved(input string, classification InputClassification, autoSaved []WorkspaceItem, ingestErrors []string, meta WorkspaceMetadata, analysisRequested bool, workspaceRoot string) string {
	return BuildInteractivePromptWithAutoSavedAndGuide(input, classification, autoSaved, ingestErrors, meta, analysisRequested, workspaceRoot, DefaultWorkspaceGuide())
}

func BuildInteractivePromptWithAutoSavedAndGuide(input string, classification InputClassification, autoSaved []WorkspaceItem, ingestErrors []string, meta WorkspaceMetadata, analysisRequested bool, workspaceRoot string, guide WorkspaceGuide) string {
	autoSavedSection := ""
	if len(autoSaved) > 0 {
		var lines []string
		for _, item := range autoSaved {
			lines = append(lines, fmt.Sprintf("- type: %s, title: %s, stored_path: %s", item.Type, item.Title, promptWorkspacePath(workspaceRoot, item.Path)))
		}
		autoSavedSection = "\n\nAuto-saved workspace assets:\n" + strings.Join(lines, "\n")
	}
	ingestErrorSection := ""
	if len(ingestErrors) > 0 {
		ingestErrorSection = "\n\nIngest warnings:\n- " + strings.Join(ingestErrors, "\n- ")
	}
	workspaceSection := ""
	if meta.CurrentResume != "" || meta.ActiveJD != "" || meta.ActiveProject != "" {
		workspaceSection = fmt.Sprintf("\n\nWorkspace pointers:\n- current_resume: %s\n- active_jd: %s\n- active_project: %s", emptyIfBlank(promptWorkspacePath(workspaceRoot, meta.CurrentResume)), emptyIfBlank(promptWorkspacePath(workspaceRoot, meta.ActiveJD)), emptyIfBlank(promptWorkspacePath(workspaceRoot, meta.ActiveProject)))
	}
	analysisSection := ""
	if analysisRequested {
		analysisSection = "\n\nAnalysis priority:\n- Use the newly saved resume and active JD for matching analysis when both exist.\n- Read all listed stored_path and current workspace pointer files directly, preferably in one multi-tool step when more than one file is needed.\n- Do not call file_list or file_search to rediscover files that are already listed in this prompt.\n- Do not inspect record directories just to verify saving; the application layer saves generated analysis artifacts after the model response.\n- If auto-saved workspace assets already exist, do not ask the user to choose storage paths, extraction tools, or workflow options.\n- Treat DOCX/PDF extraction as already handled by the application layer unless an explicit ingest warning says extraction failed.\n- Keep user-provided facts separate from suggestions."
	}
	guideSection := "\n\n" + guide.PromptSummary()
	return fmt.Sprintf(`You are running inside the Career Copilot continuous conversation workspace.

Input classification:
- type: %s
- confidence: %.2f
- signals: %s

User input:
%s%s%s%s%s%s`, classification.Type, classification.Confidence, strings.Join(classification.Signals, ", "), input, guideSection, autoSavedSection, ingestErrorSection, workspaceSection, analysisSection)
}

func promptWorkspacePath(workspaceRoot string, storedPath string) string {
	storedPath = strings.TrimSpace(storedPath)
	if storedPath == "" {
		return ""
	}
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	if workspaceRoot == "" {
		return filepath.ToSlash(storedPath)
	}
	return filepath.ToSlash(filepath.Join(workspaceRoot, storedPath))
}

func BuildAnalyzePrompt(options AnalyzeOptions) string {
	return fmt.Sprintf(`Run Career Copilot analysis for the target role described by the input files.

Use the career-copilot profile behavior and inspect evidence before concluding.

Inputs:
- JD: %s
- Resume draft: %s
- Target statement: %s
- Repository root: %s

Required tool use:
- Read the JD, resume, and target files with file_read.
- Use file_search or search_docs for supporting evidence when useful.
- Prefer scoped reads of the most relevant example, README, docs, or project files. Do not scan unrelated source files unless the target role or resume explicitly needs them.

When complete, call final_answer with valid JSON only, with this exact shape:
{
  "summary": {"target_role": "...", "match_score": 0, "verdict": "..."},
  "jd_analysis": {"required_capabilities": [{"name": "...", "importance": "high|medium|low", "evidence_needed": "..."}]},
  "project_evidence": [{"claim": "...", "evidence": [{"path": "...", "reason": "..."}], "confidence": "high|medium|low"}],
  "resume_rewrite": {"bullets": [{"original": "...", "recommended": "...", "why": "..."}]},
  "interview_brief": {"project_pitch": "...", "architecture_talk_track": "...", "tradeoffs": ["..."], "questions_to_expect": ["..."]},
  "gap_plan": [{"priority": "P0|P1|P2", "item": "...", "why_it_matters": "...", "acceptance": "..."}],
  "risk_flags": [{"statement": "...", "reason": "..."}],
  "appendix": {"files_reviewed": ["..."]}
}

Constraints:
- Adapt the analysis to the target role in the JD and target files; do not assume an engineering role.
- Do not invent employment history, production usage, metrics, financial impact, business impact, or other domain-specific outcomes.
- Do not strengthen vague claims into specific numbers, scale, tools, budgets, user counts, revenue, performance, conversion, accuracy, latency, adoption, efficiency, awards, or business outcomes unless reviewed evidence explicitly provides them.
- Resume rewrites may improve clarity, structure, seniority framing, and role alignment, but must not add new facts, responsibilities, metrics, technologies, domains, or outcomes.
- Resume rewrites must not add unstated delivery qualities, timing claims, adoption claims, collaboration scope, decision authority, iteration cadence, or impact language such as on schedule, real-time, team-wide, led, owned, improved, accelerated, or established unless reviewed evidence explicitly supports that exact claim.
- If a stronger bullet requires missing evidence, place it in gap_plan or mark it as needing user confirmation instead of writing it as fact.
- Treat resume-only claims as candidate-provided evidence, not independently verified evidence.
- Use high confidence only for claims directly supported by reviewed files; use medium or low confidence for inferred, self-reported, or partially supported claims.
- Every project_evidence item must cite at least one repository path.
- Include at least three resume bullets, three project evidence claims, three gap items, and one risk flag.
- Keep every JSON string value on one line. Do not place raw line breaks inside quoted strings.
- Return only the JSON object. Do not wrap it in Markdown fences.
- Keep the report actionable for editing a resume and preparing for interviews.`,
		options.JDPath,
		options.ResumePath,
		options.TargetPath,
		options.RepoPath,
	)
}
