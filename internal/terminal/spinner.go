package terminal

import (
	"fmt"
	"io"
	"sync"
	"time"
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

type Spinner struct {
	w       io.Writer
	mu      sync.Mutex
	msg     string
	done    chan struct{}
	stopped bool
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
	s.mu.Unlock()

	go s.run()
}

func (s *Spinner) UpdateMessage(msg string) {
	s.mu.Lock()
	s.msg = msg
	s.mu.Unlock()
}

func (s *Spinner) Stop() {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	s.stopped = true
	s.mu.Unlock()

	close(s.done)
	fmt.Fprintf(s.w, "\r\x1b[2K")
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
			msg := s.msg
			s.mu.Unlock()
			frame := spinnerFrames[i%len(spinnerFrames)]
			fmt.Fprintf(s.w, "\r\x1b[2K\x1b[36m%s\x1b[0m %s", frame, msg)
			i++
		}
	}
}
