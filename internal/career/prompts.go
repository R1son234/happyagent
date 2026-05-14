package career

import (
	"fmt"
	"path/filepath"
	"strings"
)

func BuildReportRepairPrompt(output string, parseErr error) string {
	return fmt.Sprintf(`<career_report_repair>
  <problem>The previous final_answer content was not valid career_report JSON.</problem>
  <json_parse_error>
%v
  </json_parse_error>
  <repair_task>
    - Return only one valid JSON object for the career_report schema.
    - Do not use Markdown fences.
    - Do not include comments.
    - Do not put raw line breaks inside JSON string values; use short single-line strings or arrays.
    - Keep all fields required by the career_report schema: summary, jd_analysis, project_evidence, resume_rewrite, interview_brief, gap_plan, risk_flags, appendix.
    - Preserve the same facts and evidence boundaries as much as possible.
  </repair_task>
  <invalid_previous_content>
%s
  </invalid_previous_content>
</career_report_repair>`, parseErr, output)
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
		autoSavedSection = "\n  <auto_saved_workspace_assets>\n" + strings.Join(lines, "\n") + "\n  </auto_saved_workspace_assets>"
	}
	ingestErrorSection := ""
	if len(ingestErrors) > 0 {
		ingestErrorSection = "\n  <ingest_warnings>\n- " + strings.Join(ingestErrors, "\n- ") + "\n  </ingest_warnings>"
	}
	workspaceSection := ""
	if meta.CurrentResume != "" || meta.ActiveJD != "" || meta.ActiveProject != "" {
		workspaceSection = fmt.Sprintf("\n  <workspace_pointers>\n- current_resume: %s\n- active_jd: %s\n- active_project: %s\n  </workspace_pointers>", emptyIfBlank(promptWorkspacePath(workspaceRoot, meta.CurrentResume)), emptyIfBlank(promptWorkspacePath(workspaceRoot, meta.ActiveJD)), emptyIfBlank(promptWorkspacePath(workspaceRoot, meta.ActiveProject)))
	}
	analysisSection := ""
	if analysisRequested {
		analysisSection = "\n  <analysis_priority>\n- Use the newly saved resume and active JD for matching analysis when both exist.\n- Read all listed stored_path and current workspace pointer files directly, preferably in one multi-tool step when more than one file is needed.\n- Do not call file_list or file_search to rediscover files that are already listed in this prompt.\n- Do not inspect record directories just to verify saving; the application layer saves generated analysis artifacts after the model response.\n- If auto-saved workspace assets already exist, do not ask the user to choose storage paths, extraction tools, or workflow options.\n- Treat DOCX/PDF extraction as already handled by the application layer unless an explicit ingest warning says extraction failed.\n- Keep user-provided facts separate from suggestions.\n  </analysis_priority>"
	}
	deliverySection := "\n  <delivery_policy>\n- If the user asks to save, write, generate, or place a document in the workspace, only say it was saved after the relevant write tool succeeds.\n- If a write tool fails or is unavailable, say the file was not written and include the full recoverable content or exact next recovery step.\n- Do not describe a failed write as a permissions problem unless the tool error explicitly says permission was denied.\n  </delivery_policy>"
	groundingSection := "\n  <implementation_grounding>\n- Treat implementation details, repository paths, field names, storage engines, protocols, metrics, CI jobs, and evaluation claims as facts only after reading evidence that supports them.\n- If an implementation detail is useful but not evidenced, label it as a suggested design or a point needing user confirmation.\n  </implementation_grounding>"
	guideSection := "\n  <workspace_guide>\n" + guide.PromptSummary() + "\n  </workspace_guide>"
	return fmt.Sprintf(`<career_turn>
  <workspace>Career Copilot continuous conversation workspace</workspace>
  <input_classification>
    <type>%s</type>
    <confidence>%.2f</confidence>
    <signals>%s</signals>
  </input_classification>
  <user_input>
%s
  </user_input>%s%s%s%s%s%s%s
</career_turn>`, classification.Type, classification.Confidence, strings.Join(classification.Signals, ", "), input, guideSection, autoSavedSection, ingestErrorSection, workspaceSection, analysisSection, deliverySection, groundingSection)
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
	return fmt.Sprintf(`<career_analysis>
  <task>Run Career Copilot analysis for the target role described by the input files.</task>
  <profile_behavior>Use the career-copilot profile behavior and inspect evidence before concluding.</profile_behavior>
  <inputs>
    <jd>%s</jd>
    <resume_draft>%s</resume_draft>
    <target_statement>%s</target_statement>
    <repository_root>%s</repository_root>
  </inputs>
  <required_tool_use>
    - Read the JD, resume, and target files with file_read.
    - Use file_search or search_docs for supporting evidence when useful.
    - Prefer scoped reads of the most relevant example, README, docs, or project files. Do not scan unrelated source files unless the target role or resume explicitly needs them.
  </required_tool_use>
  <output_contract>
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
  </output_contract>
  <constraints>
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
    - Keep the report actionable for editing a resume and preparing for interviews.
  </constraints>
</career_analysis>`,
		options.JDPath,
		options.ResumePath,
		options.TargetPath,
		options.RepoPath,
	)
}
