package career

import "time"

type ConfidenceLevel string

const (
	ConfidenceHigh   ConfidenceLevel = "high"
	ConfidenceMedium ConfidenceLevel = "medium"
	ConfidenceLow    ConfidenceLevel = "low"
)

const (
	InboxItemStatusUnclassified = "unclassified"
	InboxItemStatusPending      = "pending"
	InboxItemStatusConfirmed    = "confirmed"
	InboxItemStatusArchived     = "archived"
	InboxItemStatusFailed       = "failed"
)

type LLMTraceMeta struct {
	GeneratedAt   time.Time `json:"generated_at"`
	Model         string    `json:"model"`
	PromptVersion string    `json:"prompt_version"`
}

type SourceRef struct {
	Path          string   `json:"path"`
	ReadPath      string   `json:"read_path,omitempty"`
	Version       string   `json:"version"`
	Excerpt       string   `json:"excerpt"`
	EvidenceSpans []string `json:"evidence_spans,omitempty"`
}

type PendingInboxItem struct {
	ID                    string          `json:"id"`
	SourcePath            string          `json:"source_path"`
	SourceHash            string          `json:"source_hash,omitempty"`
	ExtractedPath         string          `json:"extracted_path,omitempty"`
	OriginalName          string          `json:"original_name"`
	MaterialType          string          `json:"material_type"`
	Destination           string          `json:"destination"`
	Confidence            ConfidenceLevel `json:"confidence"`
	Reason                string          `json:"reason"`
	SourceExcerpt         string          `json:"source_excerpt"`
	NeedsUserConfirmation bool            `json:"needs_user_confirmation"`
	QuestionsForUser      []string        `json:"questions_for_user,omitempty"`
	Status                string          `json:"status"`
	Meta                  LLMTraceMeta    `json:"meta"`
}

type GeneratedArtifactRecord struct {
	Path        string       `json:"path"`
	Kind        string       `json:"kind"`
	SourceRefs  []SourceRef  `json:"source_refs"`
	Meta        LLMTraceMeta `json:"meta"`
	Stale       bool         `json:"stale"`
	StaleReason string       `json:"stale_reason"`
	LastError   string       `json:"last_error,omitempty"`
}

type RunSummaryStatus string

const (
	RunSummaryStatusSuccess        RunSummaryStatus = "success"
	RunSummaryStatusPartialSuccess RunSummaryStatus = "partial_success"
	RunSummaryStatusFailed         RunSummaryStatus = "failed"
)

type RunSummaryRecord struct {
	ID          string           `json:"id"`
	TaskName    string           `json:"task_name"`
	Status      RunSummaryStatus `json:"status"`
	CreatedAt   time.Time        `json:"created_at"`
	InputPaths  []string         `json:"input_paths,omitempty"`
	Generated   []string         `json:"generated_paths,omitempty"`
	PrimaryPath string           `json:"primary_path,omitempty"`
	LogPath     string           `json:"log_path,omitempty"`
	Warnings    []string         `json:"warnings,omitempty"`
	Errors      []string         `json:"errors,omitempty"`
	NextActions []string         `json:"next_actions,omitempty"`
}
