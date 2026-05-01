package validator

import (
	"encoding/json"
	"fmt"
	"strings"
)

func ValidateOutput(schema string, output string) error {
	switch schema {
	case "", "none":
		return nil
	case "career_report":
		return validateCareerReport(output)
	default:
		return fmt.Errorf("output validation: unknown schema %q", schema)
	}
}

func validateCareerReport(output string) error {
	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		return fmt.Errorf("output validation: career_report must be valid JSON: %w", err)
	}
	required := []string{"summary", "jd_analysis", "project_evidence", "resume_rewrite", "interview_brief", "gap_plan", "risk_flags"}
	for _, field := range required {
		if _, ok := payload[field]; !ok {
			return fmt.Errorf("output validation: career_report missing field %q", field)
		}
	}
	summary, ok := payload["summary"].(map[string]any)
	if !ok {
		return fmt.Errorf("output validation: career_report summary must be an object")
	}
	if strings.TrimSpace(asString(summary["target_role"])) == "" {
		return fmt.Errorf("output validation: career_report missing summary.target_role")
	}
	score, ok := summary["match_score"].(float64)
	if !ok || score < 0 || score > 100 {
		return fmt.Errorf("output validation: career_report summary.match_score must be between 0 and 100")
	}
	if strings.TrimSpace(asString(summary["verdict"])) == "" {
		return fmt.Errorf("output validation: career_report missing summary.verdict")
	}
	if !hasObjectArray(payload["jd_analysis"], "required_capabilities") {
		return fmt.Errorf("output validation: career_report missing jd_analysis.required_capabilities")
	}
	if !hasNonEmptyArray(payload["project_evidence"]) {
		return fmt.Errorf("output validation: career_report missing project_evidence")
	}
	if !hasObjectArray(payload["resume_rewrite"], "bullets") {
		return fmt.Errorf("output validation: career_report missing resume_rewrite.bullets")
	}
	if !hasNonEmptyArray(payload["gap_plan"]) {
		return fmt.Errorf("output validation: career_report missing gap_plan")
	}
	if !hasNonEmptyArray(payload["risk_flags"]) {
		return fmt.Errorf("output validation: career_report missing risk_flags")
	}
	return nil
}

func asString(value any) string {
	text, _ := value.(string)
	return text
}

func hasObjectArray(value any, field string) bool {
	object, ok := value.(map[string]any)
	if !ok {
		return false
	}
	return hasNonEmptyArray(object[field])
}

func hasNonEmptyArray(value any) bool {
	items, ok := value.([]any)
	return ok && len(items) > 0
}
