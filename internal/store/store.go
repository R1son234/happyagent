package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Store struct {
	root string
}

func New(root string) (*Store, error) {
	if root == "" {
		return nil, fmt.Errorf("store root must not be empty")
	}
	if err := os.MkdirAll(filepath.Join(root, "sessions"), 0o755); err != nil {
		return nil, fmt.Errorf("create sessions dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "runs"), 0o755); err != nil {
		return nil, fmt.Errorf("create runs dir: %w", err)
	}
	return &Store{root: root}, nil
}

func (s *Store) SaveSession(record SessionRecord) error {
	return s.writeJSON(filepath.Join(s.root, "sessions", record.ID+".json"), record)
}

func (s *Store) SaveRun(record RunRecord) error {
	return s.writeJSON(filepath.Join(s.root, "runs", record.ID+".json"), record)
}

func (s *Store) GetSession(id string) (SessionRecord, error) {
	var record SessionRecord
	if err := s.readJSON(filepath.Join(s.root, "sessions", id+".json"), &record); err != nil {
		return SessionRecord{}, err
	}
	return record, nil
}

func (s *Store) GetRun(id string) (RunRecord, error) {
	var record RunRecord
	if err := s.readJSON(filepath.Join(s.root, "runs", id+".json"), &record); err != nil {
		return RunRecord{}, err
	}
	return record, nil
}

func (s *Store) ListRuns(sessionID string) ([]RunRecord, error) {
	session, err := s.GetSession(sessionID)
	if err != nil {
		return nil, err
	}

	runs := make([]RunRecord, 0, len(session.RunIDs))
	for _, runID := range session.RunIDs {
		run, err := s.GetRun(runID)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}

	sort.Slice(runs, func(i, j int) bool {
		return runs[i].StartedAt.Before(runs[j].StartedAt)
	})
	return runs, nil
}

func (s *Store) ListAllRuns() ([]RunRecord, error) {
	entries, err := os.ReadDir(filepath.Join(s.root, "runs"))
	if err != nil {
		return nil, fmt.Errorf("read runs dir: %w", err)
	}

	runs := make([]RunRecord, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		run, err := s.GetRun(strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	sort.Slice(runs, func(i, j int) bool {
		return runs[i].StartedAt.Before(runs[j].StartedAt)
	})
	return runs, nil
}

func (s *Store) writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %q: %w", path, err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %q: %w", path, err)
	}
	return nil
}

func (s *Store) readJSON(path string, dest any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %q: %w", path, err)
	}
	if err := json.Unmarshal(data, dest); err != nil {
		return fmt.Errorf("parse %q: %w", path, err)
	}
	return nil
}
