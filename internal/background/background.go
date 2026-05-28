package background

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Status string

const (
	StatusRunning  Status = "running"
	StatusComplete Status = "complete"
	StatusFailed   Status = "failed"
)

type Job struct {
	ID        string    `json:"id"`                 // ID identifies the background job.
	Kind      string    `json:"kind"`               // Kind describes the source such as shell or team.
	Status    Status    `json:"status"`             // Status is running, complete, or failed.
	Output    string    `json:"output,omitempty"`   // Output contains a bounded completion summary.
	Error     string    `json:"error,omitempty"`    // Error contains the failure reason when failed.
	LogPath   string    `json:"log_path,omitempty"` // LogPath points at the full output log for long jobs.
	CreatedAt time.Time `json:"created_at"`         // CreatedAt records job creation time.
	UpdatedAt time.Time `json:"updated_at"`         // UpdatedAt records the latest status change.
	Consumed  bool      `json:"consumed"`           // Consumed prevents duplicate lead notifications.
}

type Store struct {
	root string
	mu   sync.Mutex
	jobs map[string]Job
}

func NewStore(root string) *Store {
	if root == "" {
		root = "."
	}
	return &Store{root: filepath.Join(root, ".happyagent", "background"), jobs: map[string]Job{}}
}

func (s *Store) Start(kind string) Job {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	job := Job{
		ID:        fmt.Sprintf("job-%d", now.UnixNano()),
		Kind:      kind,
		Status:    StatusRunning,
		CreatedAt: now,
		UpdatedAt: now,
	}
	job.LogPath = filepath.ToSlash(filepath.Join(".happyagent", "background", job.ID, "output.log"))
	s.jobs[job.ID] = job
	return job
}

func (s *Store) Complete(id string, output string, err error) Job {
	s.mu.Lock()
	defer s.mu.Unlock()
	job := s.jobs[id]
	job.UpdatedAt = time.Now()
	if err != nil {
		job.Status = StatusFailed
		job.Error = err.Error()
	} else {
		job.Status = StatusComplete
		job.Output = output
	}
	if job.LogPath != "" {
		path := filepath.Join(s.root, id, "output.log")
		_ = os.MkdirAll(filepath.Dir(path), 0o755)
		_ = os.WriteFile(path, []byte(output), 0o644)
	}
	s.jobs[id] = job
	return job
}

func (s *Store) UnconsumedFinished() []Job {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Job
	for _, job := range s.jobs {
		if job.Consumed || job.Status == StatusRunning {
			continue
		}
		job.Consumed = true
		s.jobs[job.ID] = job
		out = append(out, job)
	}
	return out
}
