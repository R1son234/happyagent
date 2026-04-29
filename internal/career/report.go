package career

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Report struct {
	Summary         Summary           `json:"summary"`
	JDAnalysis      JDAnalysis        `json:"jd_analysis"`
	ProjectEvidence []ProjectEvidence `json:"project_evidence"`
	ResumeRewrite   ResumeRewrite     `json:"resume_rewrite"`
	InterviewBrief  InterviewBrief    `json:"interview_brief"`
	GapPlan         []GapItem         `json:"gap_plan"`
	RiskFlags       []RiskFlag        `json:"risk_flags"`
	Appendix        Appendix          `json:"appendix"`
}

type Summary struct {
	TargetRole string `json:"target_role"`
	MatchScore int    `json:"match_score"`
	Verdict    string `json:"verdict"`
}

type JDAnalysis struct {
	RequiredCapabilities []Capability `json:"required_capabilities"`
}

type Capability struct {
	Name           string `json:"name"`
	Importance     string `json:"importance"`
	EvidenceNeeded string `json:"evidence_needed"`
}

type ProjectEvidence struct {
	Claim      string         `json:"claim"`
	Evidence   []EvidenceItem `json:"evidence"`
	Confidence string         `json:"confidence"`
}

type EvidenceItem struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

type ResumeRewrite struct {
	Bullets []ResumeBullet `json:"bullets"`
}

type ResumeBullet struct {
	Original    string `json:"original"`
	Recommended string `json:"recommended"`
	Why         string `json:"why"`
}

type InterviewBrief struct {
	ProjectPitch          string   `json:"project_pitch"`
	ArchitectureTalkTrack string   `json:"architecture_talk_track"`
	Tradeoffs             []string `json:"tradeoffs"`
	QuestionsToExpect     []string `json:"questions_to_expect"`
}

type GapItem struct {
	Priority     string `json:"priority"`
	Item         string `json:"item"`
	WhyItMatters string `json:"why_it_matters"`
	Acceptance   string `json:"acceptance"`
}

type RiskFlag struct {
	Statement string `json:"statement"`
	Reason    string `json:"reason"`
}

type Appendix struct {
	FilesReviewed []string `json:"files_reviewed"`
	TraceID       string   `json:"trace_id,omitempty"`
	RunID         string   `json:"run_id,omitempty"`
	SessionID     string   `json:"session_id,omitempty"`
}

func ParseReportJSON(data []byte) (Report, error) {
	var report Report
	if err := json.Unmarshal(data, &report); err != nil {
		return Report{}, fmt.Errorf("parse career report json: %w", err)
	}
	if err := ValidateReport(report); err != nil {
		return Report{}, err
	}
	return report, nil
}

func ParseReportString(output string) (Report, error) {
	return ParseReportJSON([]byte(strings.TrimSpace(output)))
}

func ReadReport(path string) (Report, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Report{}, fmt.Errorf("read career report %q: %w", path, err)
	}
	return ParseReportJSON(data)
}

func WriteReportJSON(path string, report Report) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal career report json: %w", err)
	}
	return writeFile(path, append(data, '\n'))
}

func WriteText(path string, content string) error {
	return writeFile(path, []byte(content))
}

func ValidateReport(report Report) error {
	if strings.TrimSpace(report.Summary.TargetRole) == "" {
		return fmt.Errorf("career report missing summary.target_role")
	}
	if report.Summary.MatchScore < 0 || report.Summary.MatchScore > 100 {
		return fmt.Errorf("career report summary.match_score must be between 0 and 100")
	}
	if strings.TrimSpace(report.Summary.Verdict) == "" {
		return fmt.Errorf("career report missing summary.verdict")
	}
	if len(report.JDAnalysis.RequiredCapabilities) == 0 {
		return fmt.Errorf("career report missing jd_analysis.required_capabilities")
	}
	if len(report.ProjectEvidence) == 0 {
		return fmt.Errorf("career report missing project_evidence")
	}
	if len(report.ResumeRewrite.Bullets) == 0 {
		return fmt.Errorf("career report missing resume_rewrite.bullets")
	}
	if strings.TrimSpace(report.InterviewBrief.ProjectPitch) == "" {
		return fmt.Errorf("career report missing interview_brief.project_pitch")
	}
	if strings.TrimSpace(report.InterviewBrief.ArchitectureTalkTrack) == "" {
		return fmt.Errorf("career report missing interview_brief.architecture_talk_track")
	}
	if len(report.GapPlan) == 0 {
		return fmt.Errorf("career report missing gap_plan")
	}
	if len(report.RiskFlags) == 0 {
		return fmt.Errorf("career report missing risk_flags")
	}
	for i, capability := range report.JDAnalysis.RequiredCapabilities {
		if strings.TrimSpace(capability.Name) == "" {
			return fmt.Errorf("career report jd_analysis.required_capabilities[%d].name must not be empty", i)
		}
		if strings.TrimSpace(capability.Importance) == "" {
			return fmt.Errorf("career report jd_analysis.required_capabilities[%d].importance must not be empty", i)
		}
		if strings.TrimSpace(capability.EvidenceNeeded) == "" {
			return fmt.Errorf("career report jd_analysis.required_capabilities[%d].evidence_needed must not be empty", i)
		}
	}
	for i, evidence := range report.ProjectEvidence {
		if strings.TrimSpace(evidence.Claim) == "" {
			return fmt.Errorf("career report project_evidence[%d].claim must not be empty", i)
		}
		if len(evidence.Evidence) == 0 {
			return fmt.Errorf("career report project_evidence[%d].evidence must not be empty", i)
		}
		if strings.TrimSpace(evidence.Confidence) == "" {
			return fmt.Errorf("career report project_evidence[%d].confidence must not be empty", i)
		}
		for j, item := range evidence.Evidence {
			if strings.TrimSpace(item.Path) == "" {
				return fmt.Errorf("career report project_evidence[%d].evidence[%d].path must not be empty", i, j)
			}
			if strings.TrimSpace(item.Reason) == "" {
				return fmt.Errorf("career report project_evidence[%d].evidence[%d].reason must not be empty", i, j)
			}
		}
	}
	for i, bullet := range report.ResumeRewrite.Bullets {
		if strings.TrimSpace(bullet.Recommended) == "" {
			return fmt.Errorf("career report resume_rewrite.bullets[%d].recommended must not be empty", i)
		}
		if strings.TrimSpace(bullet.Why) == "" {
			return fmt.Errorf("career report resume_rewrite.bullets[%d].why must not be empty", i)
		}
	}
	for i, gap := range report.GapPlan {
		if strings.TrimSpace(gap.Priority) == "" || strings.TrimSpace(gap.Item) == "" || strings.TrimSpace(gap.Acceptance) == "" {
			return fmt.Errorf("career report gap_plan[%d] must include priority, item, and acceptance", i)
		}
	}
	for i, risk := range report.RiskFlags {
		if strings.TrimSpace(risk.Statement) == "" || strings.TrimSpace(risk.Reason) == "" {
			return fmt.Errorf("career report risk_flags[%d] must include statement and reason", i)
		}
	}
	return nil
}

func TopEvidenceClaims(report Report, limit int) []string {
	var claims []string
	for _, item := range report.ProjectEvidence {
		if strings.TrimSpace(item.Claim) != "" {
			claims = append(claims, item.Claim)
		}
		if len(claims) == limit {
			return claims
		}
	}
	return claims
}

func TopGapItems(report Report, limit int) []string {
	var gaps []string
	for _, item := range report.GapPlan {
		if strings.TrimSpace(item.Item) != "" {
			gaps = append(gaps, item.Item)
		}
		if len(gaps) == limit {
			return gaps
		}
	}
	return gaps
}

func writeFile(path string, data []byte) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("output path must not be empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create directory for %q: %w", path, err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %q: %w", path, err)
	}
	return nil
}
