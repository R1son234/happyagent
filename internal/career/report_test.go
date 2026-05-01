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
    "target_role": "AI Agent Backend Engineer",
    "match_score": 78,
    "verdict": "Strong project fit with productization gaps"
  },
  "jd_analysis": {
    "required_capabilities": [
      {
        "name": "Agent runtime engineering",
        "importance": "high",
        "evidence_needed": "Tool loop, state management, traceability"
      }
    ]
  },
  "project_evidence": [
    {
      "claim": "Implements tool orchestration",
      "evidence": [
        {
          "path": "internal/engine/loop.go",
          "reason": "Executes model actions and tool calls"
        }
      ],
      "confidence": "high"
    }
  ],
  "resume_rewrite": {
    "bullets": [
      {
        "original": "Built a Go CLI agent runtime.",
        "recommended": "Built a Go agent runtime with structured tool calling, trace output, and profile-scoped behavior.",
        "why": "Connects implementation to agent platform value"
      }
    ]
  },
  "interview_brief": {
    "project_pitch": "happyagent is a local AI agent runtime for evidence-based workflows.",
    "architecture_talk_track": "The CLI creates a session, loads a profile, runs the engine loop, executes tools, validates output, and stores traces.",
    "tradeoffs": ["Local files are inspectable but require careful path policy."],
    "questions_to_expect": ["How do you prevent unsafe tool use?"]
  },
  "gap_plan": [
    {
      "priority": "P0",
      "item": "Make product demo flow reliable",
      "why_it_matters": "Converts runtime into usable product",
      "acceptance": "career analyze produces report, JSON, and trace from fixed inputs"
    }
  ],
  "risk_flags": [
    {
      "statement": "Do not claim production-scale deployment",
      "reason": "No server deployment or real traffic evidence exists"
    }
  ],
  "appendix": {
    "files_reviewed": ["internal/engine/loop.go"]
  }
}`
}
