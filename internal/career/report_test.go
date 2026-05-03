package career

import (
	"strings"
	"testing"
)

func TestParseReportJSONValidatesCareerContract(t *testing.T) {
	report, err := ParseReportString(sampleReportJSON())
	if err != nil {
		t.Fatalf("ParseReportString() error = %v", err)
	}
	if report.Summary.MatchScore != 78 {
		t.Fatalf("unexpected match score: %d", report.Summary.MatchScore)
	}
	if len(report.ProjectEvidence) != 1 {
		t.Fatalf("unexpected project evidence count: %d", len(report.ProjectEvidence))
	}
}

func TestRenderMarkdownUsesTemplate(t *testing.T) {
	report, err := ParseReportString(sampleReportJSON())
	if err != nil {
		t.Fatalf("ParseReportString() error = %v", err)
	}
	markdown, err := RenderMarkdown(report, "../../docs/templates/career-report.md.tmpl")
	if err != nil {
		t.Fatalf("RenderMarkdown() error = %v", err)
	}
	for _, expected := range []string{"# Career Copilot Report", "JD Capability Breakdown", "Project Evidence", "Risk Flags"} {
		if !strings.Contains(markdown, expected) {
			t.Fatalf("rendered markdown missing %q:\n%s", expected, markdown)
		}
	}
}

func sampleReportJSON() string {
	return `{
  "summary": {
    "target_role": "Sample Target Role",
    "match_score": 78,
    "verdict": "Strong fit with evidence gaps"
  },
  "jd_analysis": {
    "required_capabilities": [
      {
        "name": "Cross-functional execution",
        "importance": "high",
        "evidence_needed": "Project brief, deliverables, metrics, and review notes"
      }
    ]
  },
  "project_evidence": [
    {
      "claim": "Coordinates cross-functional work",
      "evidence": [
        {
          "path": "resume.md",
          "reason": "Lists project planning, deliverables, and review notes"
        }
      ],
      "confidence": "high"
    }
  ],
  "resume_rewrite": {
    "bullets": [
      {
        "original": "Owned a project.",
        "recommended": "Led a cross-functional project from planning to delivery, aligning stakeholders, tracking execution, and summarizing reusable learnings.",
        "why": "Connects ownership to concrete responsibilities and evidence"
      }
    ]
  },
  "interview_brief": {
    "project_pitch": "The candidate uses structured material review to connect project evidence and role requirements.",
    "architecture_talk_track": "The workflow reviews JD, resume, project notes, artifacts, and risk flags before producing resume and interview guidance.",
    "tradeoffs": ["Process evidence is strong, but metrics must be confirmed before claiming impact."],
    "questions_to_expect": ["How did you measure impact?"]
  },
  "gap_plan": [
    {
      "priority": "P0",
      "item": "Add quantified outcomes",
      "why_it_matters": "Target roles require impact evidence",
      "acceptance": "Each major project bullet includes one confirmed metric or concrete result"
    }
  ],
  "risk_flags": [
    {
      "statement": "Do not claim measurable impact without evidence",
      "reason": "No confirmed metric exists"
    }
  ],
  "appendix": {
    "files_reviewed": ["resume.md"]
  }
}`
}
