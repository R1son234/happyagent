package report

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"happyagent/internal/engine"
)

type RunReport struct {
	Model        string              `json:"model"`
	Input        string              `json:"input"`
	Output       string              `json:"output"`
	Trace        engine.RunTrace     `json:"trace"`
	Steps        []engine.StepRecord `json:"steps"`
	SystemPrompt string              `json:"system_prompt,omitempty"`
}

func WriteJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal json report: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create report directory for %q: %w", path, err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write json report %q: %w", path, err)
	}
	return nil
}
