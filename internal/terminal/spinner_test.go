package terminal

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestSpinnerRenderShowsChecklistAndThinkingTimer(t *testing.T) {
	var buf bytes.Buffer
	spinner := NewSpinner(&buf)
	spinner.msg = "Thinking... (step 2)"
	spinner.thinkingStartedAt = time.Now().Add(-3 * time.Second)
	spinner.showThinkingTimer = true
	spinner.UpdateChecklist([]ChecklistItem{
		{Content: "read the current spinner implementation", Status: "completed"},
		{Content: "wire TODO updates into the terminal UI", Status: "in_progress"},
		{Content: "verify rendering behavior", Status: "pending"},
	})

	spinner.render("⠋")

	output := buf.String()
	if !strings.Contains(output, "Thinking... (step 2, 3s)") {
		t.Fatalf("expected current-step thinking timer in output, got %q", output)
	}
	if !strings.Contains(output, "  ⎿  ◼ read the current spinner implementation") {
		t.Fatalf("expected completed checklist item in output, got %q", output)
	}
	if !strings.Contains(output, "     ◼ wire TODO updates into the terminal UI") {
		t.Fatalf("expected in-progress checklist item in output, got %q", output)
	}
	if !strings.Contains(output, "     ◻ verify rendering behavior") {
		t.Fatalf("expected pending checklist item in output, got %q", output)
	}
	if spinner.renderedLines != 4 {
		t.Fatalf("expected 4 rendered lines, got %d", spinner.renderedLines)
	}
}

func TestSpinnerUpdateMessageClearsThinkingTimer(t *testing.T) {
	var buf bytes.Buffer
	spinner := NewSpinner(&buf)
	spinner.UpdateThinkingMessage("Thinking... (step 1)")
	spinner.UpdateMessage("Executing file_read...")
	spinner.thinkingStartedAt = time.Now().Add(-2 * time.Second)

	spinner.render("⠋")

	output := buf.String()
	if strings.Contains(output, "2s") {
		t.Fatalf("expected non-thinking message to omit timer, got %q", output)
	}
	if !strings.Contains(output, "Executing file_read...") {
		t.Fatalf("expected executing message in output, got %q", output)
	}
}

func TestSpinnerChecklistTruncatesLongContent(t *testing.T) {
	var buf bytes.Buffer
	spinner := NewSpinner(&buf)
	spinner.msg = "Thinking..."
	spinner.UpdateChecklist([]ChecklistItem{
		{Content: "abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz", Status: "pending"},
	})

	spinner.render("⠋")

	output := buf.String()
	if !strings.Contains(output, "◻ abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvw…") {
		t.Fatalf("expected truncated checklist item in output, got %q", output)
	}
}
