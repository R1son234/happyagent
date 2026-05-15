package terminal

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

type ChecklistItem struct {
	Content string
	Status  string
}

type Spinner struct {
	w                 io.Writer
	mu                sync.Mutex
	msg               string
	checklist         []string
	renderedLines     int
	thinkingStartedAt time.Time
	showThinkingTimer bool
	done              chan struct{}
	stopped           bool
}

func NewSpinner(w io.Writer) *Spinner {
	return &Spinner{w: w, done: make(chan struct{})}
}

func (s *Spinner) Start(initialMsg string) {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	s.msg = initialMsg
	if isThinkingMessage(initialMsg) {
		s.thinkingStartedAt = time.Now()
		s.showThinkingTimer = true
	}
	s.mu.Unlock()

	go s.run()
}

func (s *Spinner) UpdateMessage(msg string) {
	s.mu.Lock()
	s.msg = msg
	s.showThinkingTimer = false
	s.thinkingStartedAt = time.Time{}
	s.mu.Unlock()
}

func (s *Spinner) UpdateThinkingMessage(msg string) {
	s.mu.Lock()
	s.msg = msg
	s.thinkingStartedAt = time.Now()
	s.showThinkingTimer = true
	s.mu.Unlock()
}

func (s *Spinner) UpdateChecklist(items []ChecklistItem) {
	lines := make([]string, 0, len(items))
	for i, item := range items {
		prefix := "     "
		if i == 0 {
			prefix = "  ⎿  "
		}
		lines = append(lines, prefix+formatChecklistItem(item))
	}

	s.mu.Lock()
	s.checklist = lines
	s.mu.Unlock()
}

func (s *Spinner) Stop() {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	s.stopped = true
	close(s.done)
	clearLines(s.w, s.renderedLines)
	s.renderedLines = 0
	s.mu.Unlock()
}

func (s *Spinner) run() {
	i := 0
	ticker := time.NewTicker(80 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-s.done:
			return
		case <-ticker.C:
			s.mu.Lock()
			if s.stopped {
				s.mu.Unlock()
				return
			}
			frame := spinnerFrames[i%len(spinnerFrames)]
			s.render(frame)
			s.mu.Unlock()
			i++
		}
	}
}

func (s *Spinner) render(frame string) {
	msg := s.msg
	if s.showThinkingTimer && !s.thinkingStartedAt.IsZero() {
		msg = appendElapsedSeconds(msg, time.Since(s.thinkingStartedAt))
	}

	clearLines(s.w, s.renderedLines)
	fmt.Fprintf(s.w, "\r\x1b[2K\x1b[36m%s\x1b[0m %s", frame, msg)
	for _, line := range s.checklist {
		fmt.Fprintf(s.w, "\n\r\x1b[2K%s", line)
	}
	s.renderedLines = 1 + len(s.checklist)
}

func clearLines(w io.Writer, count int) {
	if count <= 0 {
		fmt.Fprintf(w, "\r\x1b[2K")
		return
	}
	for i := 0; i < count; i++ {
		fmt.Fprintf(w, "\r\x1b[2K")
		if i < count-1 {
			fmt.Fprintf(w, "\x1b[1A")
		}
	}
}

func formatChecklistItem(item ChecklistItem) string {
	symbol := "◻"
	if item.Status == "completed" || item.Status == "in_progress" {
		symbol = "◼"
	}
	return symbol + " " + truncateRunes(strings.TrimSpace(item.Content), 50)
}

func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	if limit <= 1 {
		return "…"
	}
	return string(runes[:limit-1]) + "…"
}

func appendElapsedSeconds(msg string, elapsed time.Duration) string {
	seconds := int(elapsed / time.Second)
	suffix := fmt.Sprintf("%ds", seconds)
	if strings.HasSuffix(msg, ")") {
		return strings.TrimSuffix(msg, ")") + ", " + suffix + ")"
	}
	return msg + " (" + suffix + ")"
}

func isThinkingMessage(msg string) bool {
	return strings.HasPrefix(msg, "Thinking...")
}
