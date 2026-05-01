package store

import (
	"time"

	"happyagent/internal/engine"
	"happyagent/internal/observe"
)

type SessionRecord struct {
	ID        string    `json:"id"`
	Profile   string    `json:"profile"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	RunIDs    []string  `json:"run_ids"`
}

type RunRecord struct {
	ID                string              `json:"id"`
	SessionID         string              `json:"session_id"`
	Profile           string              `json:"profile"`
	Input             string              `json:"input"`
	Output            string              `json:"output,omitempty"`
	SystemPrompt      string              `json:"system_prompt,omitempty"`
	Status            string              `json:"status"`
	TerminationReason string              `json:"termination_reason,omitempty"`
	ErrorCategory     string              `json:"error_category,omitempty"`
	ErrorMessage      string              `json:"error_message,omitempty"`
	StartedAt         time.Time           `json:"started_at"`
	FinishedAt        time.Time           `json:"finished_at"`
	Trace             engine.RunTrace     `json:"trace"`
	Steps             []engine.StepRecord `json:"steps,omitempty"`
	Events            []observe.Event     `json:"events,omitempty"`
}
