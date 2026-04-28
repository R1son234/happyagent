package app

import (
	"context"
	"testing"

	"happyagent/internal/engine"
	"happyagent/internal/observe"
	"happyagent/internal/runtime"
	"happyagent/internal/store"
)

type stubRunner struct {
	result runtime.RunResult
	err    error
}

func (r stubRunner) Run(ctx context.Context, req runtime.RunRequest) (runtime.RunResult, error) {
	return r.result, r.err
}

func TestApplicationCreateSessionAndAppendTurn(t *testing.T) {
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}
	app, err := New(stubRunner{
		result: runtime.RunResult{
			Output:       "done",
			SystemPrompt: "prompt",
			Trace: engine.RunTrace{
				StepCount:         1,
				ToolCallCount:     1,
				TotalTokens:       3,
				TerminationReason: "completed",
			},
			Events: []observe.Event{{Type: "run"}},
		},
	}, st, observe.NewMetrics())
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
	if run.Status != "completed" || run.Output != "done" {
		t.Fatalf("unexpected run: %+v", run)
	}
	if app.Metrics().RunsTotal != 1 {
		t.Fatalf("unexpected metrics: %+v", app.Metrics())
	}
}
