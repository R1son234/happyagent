package career

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

const WorkspaceDiagnosticsDir = "record/diagnostics"

type DiagnosticRecord struct {
	ID            string    `json:"id"`
	TaskName      string    `json:"task_name"`
	PromptVersion string    `json:"prompt_version,omitempty"`
	SourcePaths   []string  `json:"source_paths,omitempty"`
	Error         string    `json:"error"`
	RawOutput     string    `json:"raw_output,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

func (w *Workspace) WriteDiagnostic(record DiagnosticRecord) (string, error) {
	now := record.CreatedAt
	if now.IsZero() {
		now = time.Now()
		record.CreatedAt = now
	}
	record.TaskName = strings.TrimSpace(record.TaskName)
	if record.TaskName == "" {
		record.TaskName = "unknown"
	}
	if record.ID == "" {
		record.ID = fmt.Sprintf("%s-%s", now.Format("20060102-150405"), slugForPath(record.TaskName))
	}
	for i, path := range record.SourcePaths {
		rel := filepath.ToSlash(strings.TrimSpace(path))
		if err := validateWorkspaceRelPath(rel); err != nil {
			return "", fmt.Errorf("diagnostic source_paths[%d]: %w", i, err)
		}
		record.SourcePaths[i] = rel
	}
	rel := filepath.ToSlash(filepath.Join(WorkspaceInternalDir, WorkspaceDiagnosticsDir, safeFileNameWithExt(record.ID, ".json")))
	if err := w.writeJSON(filepath.Join(w.Root, rel), record); err != nil {
		return "", err
	}
	return rel, nil
}
