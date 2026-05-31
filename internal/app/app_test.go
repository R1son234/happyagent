package app

import (
	"context"
	"testing"

	"happyagent/internal/engine"
	"happyagent/internal/memory"
	"happyagent/internal/observe"
	"happyagent/internal/protocol"
	"happyagent/internal/runtime"
	"happyagent/internal/store"
)

type stubRunner struct {
	result  runtime.RunResult
	err     error
	lastReq *runtime.RunRequest
}

func (r *stubRunner) Run(ctx context.Context, req runtime.RunRequest) (runtime.RunResult, error) {
	r.lastReq = &req
	return r.result, r.err
}

func (r *stubRunner) MemoryStore() *memory.LongTermStore {
	return nil
}

func TestApplicationCreateSessionAndAppendTurn(t *testing.T) {
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}
	runner := &stubRunner{
		result: runtime.RunResult{
			Output:       "done",
			SystemPrompt: "prompt",
			Trace: engine.RunTrace{
				StepCount:         1,
				ToolCallCount:     1,
				TotalTokens:       3,
				TerminationReason: protocol.RunStatusCompleted,
			},
			Events: []observe.Event{{Type: "run"}},
		},
	}
	app, err := New(runner, st, observe.NewMetrics())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	session, err := app.CreateSession("general-assistant")
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	run, err := app.AppendUserTurn(context.Background(), AppendTurnRequest{
		SessionID:    session.ID,
		ProfileName:  "general-assistant",
		Input:        "hello",
		SystemPrompt: "base prompt",
	})
	if err != nil {
		t.Fatalf("AppendUserTurn() error = %v", err)
	}
	if run.Status != protocol.RunStatusCompleted || run.Output != "done" {
		t.Fatalf("unexpected run: %+v", run)
	}
	if app.Metrics().RunsTotal != 1 {
		t.Fatalf("unexpected metrics: %+v", app.Metrics())
	}
}

func TestApplicationAppendTurnCanSuppressHistoryAndMemory(t *testing.T) {
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}
	runner := &stubRunner{
		result: runtime.RunResult{
			Output:       "done",
			SystemPrompt: "prompt",
			Trace:        engine.RunTrace{TerminationReason: protocol.RunStatusCompleted},
		},
	}
	app, err := New(runner, st, observe.NewMetrics())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	session, err := app.CreateSession("career-copilot")
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if _, err := app.AppendUserTurn(context.Background(), AppendTurnRequest{
		SessionID:    session.ID,
		ProfileName:  "career-copilot",
		Input:        "previous turn with material",
		SystemPrompt: "base prompt",
	}); err != nil {
		t.Fatalf("AppendUserTurn() seed error = %v", err)
	}
	if _, err := app.AppendUserTurn(context.Background(), AppendTurnRequest{
		SessionID:          session.ID,
		ProfileName:        "career-copilot",
		Input:              "structured task",
		SystemPrompt:       "base prompt",
		ToolScope:          []string{"file_read", "final_answer"},
		SourceReadPaths:    []string{"inbox/source.md"},
		RequireSourceReads: true,
		SuppressHistory:    true,
		SuppressMemory:     true,
	}); err != nil {
		t.Fatalf("AppendUserTurn() suppressed error = %v", err)
	}
	if runner.lastReq == nil {
		t.Fatal("expected captured runtime request")
	}
	if len(runner.lastReq.History) != 0 || runner.lastReq.MemorySnapshot != "" {
		t.Fatalf("expected no history or memory, got history=%+v memory=%q", runner.lastReq.History, runner.lastReq.MemorySnapshot)
	}
	if !sameStringSlice(runner.lastReq.ToolScope, []string{"file_read", "final_answer"}) ||
		!sameStringSlice(runner.lastReq.SourceReadPaths, []string{"inbox/source.md"}) ||
		!runner.lastReq.RequireSourceReads {
		t.Fatalf("unexpected source-bound request: %+v", runner.lastReq)
	}
}

func sameStringSlice(got []string, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
