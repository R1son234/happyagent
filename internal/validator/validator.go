package validator

import (
	"encoding/json"
	"fmt"
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
	required := []string{"summary", "match_report", "rewrite_plan", "project_gap_analysis"}
	for _, field := range required {
		if _, ok := payload[field]; !ok {
			return fmt.Errorf("output validation: career_report missing field %q", field)
		}
	}
	return nil
}
