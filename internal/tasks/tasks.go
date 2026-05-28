package tasks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"happyagent/internal/jsonfile"
)

type Status string

const (
	StatusPending    Status = "pending"
	StatusInProgress Status = "in_progress"
	StatusBlocked    Status = "blocked"
	StatusCompleted  Status = "completed"
	StatusCancelled  Status = "cancelled"
)

type Task struct {
	ID            string    `json:"id"`
	Title         string    `json:"title"`
	Description   string    `json:"description,omitempty"`
	Status        Status    `json:"status"`
	BlockedBy     []string  `json:"blocked_by,omitempty"`
	Owner         string    `json:"owner,omitempty"`
	Workspace     string    `json:"workspace,omitempty"`
	EvidencePaths []string  `json:"evidence_paths,omitempty"`
	ResultPaths   []string  `json:"result_paths,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type Store struct {
	dir string
	mu  sync.Mutex
}

func NewStore(root string) (*Store, error) {
	if root == "" {
		root = "."
	}
	dir := filepath.Join(root, ".happyagent", "tasks")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create tasks dir: %w", err)
	}
	return &Store{dir: dir}, nil
}

func (s *Store) Create(task Task) (Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	task.ID = strings.TrimSpace(task.ID)
	if task.ID == "" {
		task.ID = fmt.Sprintf("task-%d", now.UnixNano())
	}
	task.Title = strings.TrimSpace(task.Title)
	if task.Title == "" {
		return Task{}, fmt.Errorf("task title cannot be empty")
	}
	if task.Status == "" {
		task.Status = StatusPending
	}
	task.CreatedAt = now
	task.UpdatedAt = now
	if err := s.validateLocked(task); err != nil {
		return Task{}, err
	}
	return task, s.writeLocked(task)
}

func (s *Store) Update(task Task) (Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, err := s.getLocked(task.ID)
	if err != nil {
		return Task{}, err
	}
	if strings.TrimSpace(task.Title) == "" {
		task.Title = existing.Title
	}
	if task.Status == "" {
		task.Status = existing.Status
	}
	task.CreatedAt = existing.CreatedAt
	task.UpdatedAt = time.Now()
	if err := s.validateLocked(task); err != nil {
		return Task{}, err
	}
	return task, s.writeLocked(task)
}

func (s *Store) List() ([]Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.listLocked()
}

func (s *Store) Get(id string) (Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.getLocked(id)
}

func (s *Store) Claim(id string, owner string) (Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	task, err := s.getLocked(id)
	if err != nil {
		return Task{}, err
	}
	if task.Status != StatusPending && task.Status != StatusBlocked {
		return Task{}, fmt.Errorf("task %s is %s, cannot claim", id, task.Status)
	}
	if task.Owner != "" {
		return Task{}, fmt.Errorf("task %s already owned by %s", id, task.Owner)
	}
	if blocked, deps := s.blockingDepsLocked(task); blocked {
		return Task{}, fmt.Errorf("task %s blocked by %s", id, strings.Join(deps, ", "))
	}
	task.Status = StatusInProgress
	task.Owner = strings.TrimSpace(owner)
	task.UpdatedAt = time.Now()
	return task, s.writeLocked(task)
}

func (s *Store) Complete(id string, resultPaths []string) (Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	task, err := s.getLocked(id)
	if err != nil {
		return Task{}, err
	}
	if task.Status != StatusInProgress {
		return Task{}, fmt.Errorf("task %s is %s, cannot complete", id, task.Status)
	}
	task.Status = StatusCompleted
	task.ResultPaths = appendUnique(task.ResultPaths, resultPaths...)
	task.UpdatedAt = time.Now()
	return task, s.writeLocked(task)
}

func (s *Store) Release(id string) (Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	task, err := s.getLocked(id)
	if err != nil {
		return Task{}, err
	}
	task.Owner = ""
	if task.Status == StatusInProgress {
		task.Status = StatusPending
	}
	task.UpdatedAt = time.Now()
	return task, s.writeLocked(task)
}

func (s *Store) getLocked(id string) (Task, error) {
	var task Task
	data, err := os.ReadFile(s.path(id))
	if err != nil {
		return Task{}, err
	}
	if err := json.Unmarshal(data, &task); err != nil {
		return Task{}, err
	}
	return task, nil
}

func (s *Store) listLocked() ([]Task, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	var out []Task
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		task, err := s.getLocked(strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil {
			return nil, err
		}
		out = append(out, task)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

func (s *Store) validateLocked(task Task) error {
	if !validStatus(task.Status) {
		return fmt.Errorf("invalid task status %q", task.Status)
	}
	for _, dep := range task.BlockedBy {
		if dep == task.ID {
			return fmt.Errorf("task %s cannot depend on itself", task.ID)
		}
		if _, err := os.Stat(s.path(dep)); err != nil {
			return fmt.Errorf("task %s depends on missing task %s", task.ID, dep)
		}
	}
	return s.detectCycleLocked(task)
}

func (s *Store) detectCycleLocked(task Task) error {
	seen := map[string]bool{}
	var visit func(string) bool
	visit = func(id string) bool {
		if id == task.ID {
			return true
		}
		if seen[id] {
			return false
		}
		seen[id] = true
		dep, err := s.getLocked(id)
		if err != nil {
			return false
		}
		for _, next := range dep.BlockedBy {
			if visit(next) {
				return true
			}
		}
		return false
	}
	for _, dep := range task.BlockedBy {
		if visit(dep) {
			return fmt.Errorf("task dependency cycle involving %s", task.ID)
		}
	}
	return nil
}

func (s *Store) blockingDepsLocked(task Task) (bool, []string) {
	var deps []string
	for _, depID := range task.BlockedBy {
		dep, err := s.getLocked(depID)
		if err != nil || dep.Status != StatusCompleted {
			deps = append(deps, depID)
		}
	}
	return len(deps) > 0, deps
}

func (s *Store) writeLocked(task Task) error {
	return jsonfile.Write(s.path(task.ID), task)
}

func (s *Store) path(id string) string {
	return filepath.Join(s.dir, sanitizeID(id)+".json")
}

func sanitizeID(id string) string {
	id = strings.TrimSpace(id)
	var b strings.Builder
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), ".-")
}

func validStatus(status Status) bool {
	switch status {
	case StatusPending, StatusInProgress, StatusBlocked, StatusCompleted, StatusCancelled:
		return true
	default:
		return false
	}
}

func appendUnique(values []string, additions ...string) []string {
	seen := map[string]struct{}{}
	for _, value := range values {
		seen[value] = struct{}{}
	}
	for _, value := range additions {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		values = append(values, value)
	}
	return values
}
