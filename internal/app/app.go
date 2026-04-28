package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"happyagent/internal/memory"
	"happyagent/internal/observe"
	"happyagent/internal/runtime"
	"happyagent/internal/store"
)

type Runner interface {
	Run(ctx context.Context, req runtime.RunRequest) (runtime.RunResult, error)
}

type Application struct {
	runner  Runner
	store   *store.Store
	metrics *observe.Metrics
}

func New(runner Runner, store *store.Store, metrics *observe.Metrics) (*Application, error) {
	if runner == nil {
		return nil, fmt.Errorf("runner must not be nil")
	}
	if store == nil {
		return nil, fmt.Errorf("store must not be nil")
	}
	if metrics == nil {
		metrics = observe.NewMetrics()
	}
	return &Application{runner: runner, store: store, metrics: metrics}, nil
}

type AppendTurnRequest struct {
	SessionID     string
	ProfileName   string
	Input         string
	SystemPrompt  string
	ApprovedTools []string
}

func (a *Application) CreateSession(profileName string) (store.SessionRecord, error) {
	now := time.Now()
	session := store.SessionRecord{
		ID:        newID("session"),
		Profile:   profileName,
		CreatedAt: now,
		UpdatedAt: now,
		RunIDs:    []string{},
	}
	if err := a.store.SaveSession(session); err != nil {
		return store.SessionRecord{}, err
	}
	return session, nil
}

func (a *Application) AppendUserTurn(ctx context.Context, req AppendTurnRequest) (store.RunRecord, error) {
	session, err := a.store.GetSession(req.SessionID)
	if err != nil {
		return store.RunRecord{}, err
	}
	historyRuns, err := a.store.ListRuns(req.SessionID)
	if err != nil {
		return store.RunRecord{}, err
	}

	runID := newID("run")
	startedAt := time.Now()
	result, runErr := a.runner.Run(ctx, runtime.RunRequest{
		Input:         req.Input,
		SystemPrompt:  req.SystemPrompt,
		ProfileName:   req.ProfileName,
		SessionID:     req.SessionID,
		ApprovedTools: req.ApprovedTools,
		History:       buildHistory(historyRuns),
	})

	record := store.RunRecord{
		ID:           runID,
		SessionID:    req.SessionID,
		Profile:      req.ProfileName,
		Input:        req.Input,
		SystemPrompt: result.SystemPrompt,
		StartedAt:    startedAt,
		FinishedAt:   time.Now(),
		Trace:        result.Trace,
		Steps:        result.Steps,
		Events:       result.Events,
	}
	if runErr != nil {
		record.Status = "failed"
		record.TerminationReason = "runtime_error"
		record.ErrorCategory = observe.ClassifyError(runErr)
		record.ErrorMessage = runErr.Error()
		a.metrics.RecordRun(false, record.Trace.StepCount, record.Trace.ToolCallCount, record.Trace.TotalTokens, record.ErrorCategory)
		if err := a.store.SaveRun(record); err != nil {
			return store.RunRecord{}, err
		}
		session.RunIDs = append(session.RunIDs, record.ID)
		session.UpdatedAt = time.Now()
		if err := a.store.SaveSession(session); err != nil {
			return store.RunRecord{}, err
		}
		return record, runErr
	}

	record.Output = result.Output
	record.Status = "completed"
	record.TerminationReason = result.Trace.TerminationReason
	a.metrics.RecordRun(true, record.Trace.StepCount, record.Trace.ToolCallCount, record.Trace.TotalTokens, "")
	if err := a.store.SaveRun(record); err != nil {
		return store.RunRecord{}, err
	}

	session.RunIDs = append(session.RunIDs, record.ID)
	session.UpdatedAt = time.Now()
	if err := a.store.SaveSession(session); err != nil {
		return store.RunRecord{}, err
	}
	return record, nil
}

func (a *Application) GetSession(id string) (store.SessionRecord, error) {
	return a.store.GetSession(id)
}

func (a *Application) GetRun(id string) (store.RunRecord, error) {
	return a.store.GetRun(id)
}

func (a *Application) GetTrace(id string) (any, error) {
	run, err := a.store.GetRun(id)
	if err != nil {
		return nil, err
	}
	return run.Trace, nil
}

func (a *Application) ReplayRun(id string) (store.RunRecord, error) {
	return a.store.GetRun(id)
}

func (a *Application) Metrics() observe.Metrics {
	return a.metrics.Snapshot()
}

func (a *Application) HistoricalMetrics() (observe.Metrics, error) {
	runs, err := a.store.ListAllRuns()
	if err != nil {
		return observe.Metrics{}, err
	}
	metrics := observe.NewMetrics()
	for _, run := range runs {
		metrics.RecordRun(run.Status == "completed", run.Trace.StepCount, run.Trace.ToolCallCount, run.Trace.TotalTokens, run.ErrorCategory)
	}
	return metrics.Snapshot(), nil
}

func buildHistory(runs []store.RunRecord) []memory.Turn {
	history := make([]memory.Turn, 0, len(runs)*2)
	for _, run := range runs {
		history = append(history, memory.Turn{Role: "user", Content: run.Input})
		if run.Output != "" {
			history = append(history, memory.Turn{Role: "assistant", Content: run.Output})
		}
	}
	return history
}

func newID(prefix string) string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}
	return fmt.Sprintf("%s-%d-%s", prefix, time.Now().UnixNano(), hex.EncodeToString(buf))
}
