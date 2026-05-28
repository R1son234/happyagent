package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	defaultMaxLoopMessages       = 60
	defaultRecentToolResultsKept = 4
	defaultToolResultCompactOver = 512
)

type CompactionEvent struct {
	Type        string `json:"type"`
	BeforeBytes int    `json:"before_bytes"`
	AfterBytes  int    `json:"after_bytes"`
	Detail      string `json:"detail,omitempty"`
}

func manageContext(state *LoopState, input RunInput) {
	before := messagesBytes(state.Messages)
	compactOldToolResults(state, defaultRecentToolResultsKept)
	snipMessages(state, defaultMaxLoopMessages)
	after := messagesBytes(state.Messages)
	if after < before {
		state.CompactionEvents = append(state.CompactionEvents, CompactionEvent{
			Type:        "pre_model",
			BeforeBytes: before,
			AfterBytes:  after,
			Detail:      "compacted old tool observations and snipped middle messages",
		})
	}
}

func compactOldToolResults(state *LoopState, keepRecent int) {
	if keepRecent <= 0 {
		keepRecent = defaultRecentToolResultsKept
	}
	var toolIndexes []int
	for i, message := range state.Messages {
		if message.Role == "tool" {
			toolIndexes = append(toolIndexes, i)
		}
	}
	if len(toolIndexes) <= keepRecent {
		return
	}
	for _, idx := range toolIndexes[:len(toolIndexes)-keepRecent] {
		content := state.Messages[idx].Content
		if len(content) <= defaultToolResultCompactOver {
			continue
		}
		state.Messages[idx].Content = fmt.Sprintf("[earlier %s result compacted: %d bytes; rerun or read the original source if needed]", state.Messages[idx].ToolName, len(content))
	}
}

func snipMessages(state *LoopState, maxMessages int) {
	if maxMessages <= 0 || len(state.Messages) <= maxMessages {
		return
	}
	head := 2
	tail := maxMessages - head - 1
	if tail < 1 {
		return
	}
	snipped := len(state.Messages) - head - tail
	summary := deterministicRunSummary(*state)
	placeholder := MessageEnvelope{
		Role:    "user",
		Content: fmt.Sprintf("[snipped %d middle runtime message(s)]\n\n%s", snipped, summary),
	}
	next := make([]MessageEnvelope, 0, maxMessages)
	next = append(next, state.Messages[:head]...)
	next = append(next, placeholder)
	next = append(next, state.Messages[len(state.Messages)-tail:]...)
	state.Messages = next
}

func deterministicRunSummary(state LoopState) string {
	var parts []string
	if len(state.Todos) > 0 {
		var todoParts []string
		for _, todo := range state.Todos {
			todoParts = append(todoParts, fmt.Sprintf("%s:%s", todo.Status, todo.Content))
		}
		parts = append(parts, "todos="+strings.Join(todoParts, "; "))
	}
	if len(state.DeliveryToolFailures) > 0 {
		var failures []string
		for toolName, failure := range state.DeliveryToolFailures {
			failures = append(failures, toolName+"="+failure)
		}
		parts = append(parts, "delivery_failures="+strings.Join(failures, "; "))
	}
	if len(parts) == 0 {
		return "No compactable run summary facts were available."
	}
	return strings.Join(parts, "\n")
}

func messagesBytes(messages []MessageEnvelope) int {
	total := 0
	for _, message := range messages {
		total += len(message.Content) + len(message.ReasoningContent)
	}
	return total
}

func reactiveCompact(state *LoopState, input RunInput) error {
	if input.Config.Offload.RootDir != "" {
		path, err := writeTranscript(state.Messages, input.Config.Offload.RootDir, input.Config.Offload.RunID)
		if err == nil {
			state.TranscriptPath = path
		}
	}
	before := messagesBytes(state.Messages)
	summary := deterministicRunSummary(*state)
	tail := 8
	if len(state.Messages) < tail {
		tail = len(state.Messages)
	}
	next := []MessageEnvelope{{
		Role:    "user",
		Content: "[reactive compact]\n\n" + summary,
	}}
	next = append(next, state.Messages[len(state.Messages)-tail:]...)
	state.Messages = next
	after := messagesBytes(state.Messages)
	state.CompactionEvents = append(state.CompactionEvents, CompactionEvent{
		Type:        "reactive",
		BeforeBytes: before,
		AfterBytes:  after,
		Detail:      "prompt-too-long recovery compact",
	})
	return nil
}

func writeTranscript(messages []MessageEnvelope, root string, runID string) (string, error) {
	if runID == "" {
		runID = "run"
	}
	dir := filepath.Join(root, ".happyagent", "transcripts", sanitizePathSegment(runID))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "messages.txt")
	var builder strings.Builder
	for i, message := range messages {
		builder.WriteString(fmt.Sprintf("## %d %s %s\n", i+1, message.Role, message.ToolName))
		builder.WriteString(message.Content)
		builder.WriteString("\n\n")
	}
	if err := os.WriteFile(path, []byte(builder.String()), 0o600); err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path, nil
	}
	return filepath.ToSlash(rel), nil
}
