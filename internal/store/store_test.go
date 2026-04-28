package store

import (
	"testing"
	"time"
)

func TestStoreSaveAndLoadSessionRun(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	session := SessionRecord{
		ID:        "session-1",
		Profile:   "general-assistant",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		RunIDs:    []string{"run-1"},
	}
	run := RunRecord{
		ID:         "run-1",
		SessionID:  "session-1",
		Profile:    "general-assistant",
		Input:      "hello",
		Status:     "completed",
		StartedAt:  time.Now(),
		FinishedAt: time.Now(),
	}

	if err := s.SaveSession(session); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
	}
	if err := s.SaveRun(run); err != nil {
		t.Fatalf("SaveRun() error = %v", err)
	}

	gotSession, err := s.GetSession("session-1")
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if gotSession.ID != "session-1" || len(gotSession.RunIDs) != 1 {
		t.Fatalf("unexpected session: %+v", gotSession)
	}

	gotRuns, err := s.ListRuns("session-1")
	if err != nil {
		t.Fatalf("ListRuns() error = %v", err)
	}
	if len(gotRuns) != 1 || gotRuns[0].ID != "run-1" {
		t.Fatalf("unexpected runs: %+v", gotRuns)
	}
}
