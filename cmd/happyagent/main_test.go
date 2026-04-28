package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"happyagent/internal/app"
	"happyagent/internal/config"
	"happyagent/internal/observe"
	"happyagent/internal/store"
)

type stubSessionApplication struct {
	session       store.SessionRecord
	runs          []store.RunRecord
	appendRequests []app.AppendTurnRequest
}

func (s *stubSessionApplication) CreateSession(profileName string) (store.SessionRecord, error) {
	return s.session, nil
}

func (s *stubSessionApplication) AppendUserTurn(ctx context.Context, req app.AppendTurnRequest) (store.RunRecord, error) {
	s.appendRequests = append(s.appendRequests, req)
	index := len(s.appendRequests) - 1
	if index >= len(s.runs) {
		index = len(s.runs) - 1
	}
	return s.runs[index], nil
}

func (s *stubSessionApplication) GetSession(id string) (store.SessionRecord, error) {
	return s.session, nil
}

func (s *stubSessionApplication) GetRun(id string) (store.RunRecord, error) {
	return store.RunRecord{}, nil
}

func (s *stubSessionApplication) ReplayRun(id string) (store.RunRecord, error) {
	return store.RunRecord{}, nil
}

func (s *stubSessionApplication) HistoricalMetrics() (observe.Metrics, error) {
	return observe.Metrics{}, nil
}

func TestRunInteractiveSessionUsesSingleSessionAcrossTurns(t *testing.T) {
	app := &stubSessionApplication{
		session: store.SessionRecord{ID: "session-1"},
		runs: []store.RunRecord{
			{ID: "run-1", SessionID: "session-1", Output: "first"},
			{ID: "run-2", SessionID: "session-1", Output: "second"},
		},
	}
	cfg := config.Default()
	cfg.LLM.Model = "test-model"
	cfg.Engine.RunTimeoutSeconds = 1

	input := strings.NewReader("second question\n/exit\n")
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := runInteractiveSession(app, cfg, input, &stdout, &stderr, "session-1", "general-assistant", []string{"shell"}, "first question", true, nil)
	if err != nil {
		t.Fatalf("runInteractiveSession() error = %v", err)
	}
	if len(app.appendRequests) != 2 {
		t.Fatalf("expected 2 turns, got %d", len(app.appendRequests))
	}
	for i, req := range app.appendRequests {
		if req.SessionID != "session-1" {
			t.Fatalf("request %d used wrong session: %+v", i, req)
		}
		if req.ProfileName != "general-assistant" {
			t.Fatalf("request %d used wrong profile: %+v", i, req)
		}
		if len(req.ApprovedTools) != 1 || req.ApprovedTools[0] != "shell" {
			t.Fatalf("request %d used wrong approved tools: %+v", i, req)
		}
	}

	output := stdout.String()
	if !strings.Contains(output, "interactive session started. profile=general-assistant session_id=session-1") {
		t.Fatalf("missing session header: %s", output)
	}
	if !strings.Contains(output, "assistant> first") || !strings.Contains(output, "assistant> second") {
		t.Fatalf("missing assistant output: %s", output)
	}

	logs := stderr.String()
	if !strings.Contains(logs, "created_session=session-1") {
		t.Fatalf("missing created session log: %s", logs)
	}
	if !strings.Contains(logs, "run_id=run-1 session_id=session-1") || !strings.Contains(logs, "run_id=run-2 session_id=session-1") {
		t.Fatalf("missing run ids: %s", logs)
	}
}

func TestResolveSessionCreatesSessionWhenInteractive(t *testing.T) {
	now := time.Now()
	app := &stubSessionApplication{
		session: store.SessionRecord{
			ID:        "session-new",
			Profile:   "general-assistant",
			CreatedAt: now,
			UpdatedAt: now,
		},
	}

	sessionID, created, err := resolveSession(app, "", "general-assistant", true)
	if err != nil {
		t.Fatalf("resolveSession() error = %v", err)
	}
	if !created || sessionID != "session-new" {
		t.Fatalf("unexpected session resolution: created=%v id=%s", created, sessionID)
	}
}
