package runtime

import (
	"context"
	"encoding/json"
	"fmt"

	"happyagent/internal/tasks"
)

type runtimeTaskProvider struct {
	store *tasks.Store
}

func (p runtimeTaskProvider) CreateTask(ctx context.Context, args json.RawMessage) (string, error) {
	_ = ctx
	if p.store == nil {
		return "", fmt.Errorf("task store is unavailable")
	}
	var input tasks.Task
	if err := json.Unmarshal(args, &input); err != nil {
		return "", fmt.Errorf("decode task_create arguments: %w", err)
	}
	task, err := p.store.Create(input)
	return marshalToolJSON(task, err)
}

func (p runtimeTaskProvider) ListTasks(ctx context.Context, args json.RawMessage) (string, error) {
	_ = ctx
	_ = args
	if p.store == nil {
		return "", fmt.Errorf("task store is unavailable")
	}
	tasks, err := p.store.List()
	return marshalToolJSON(tasks, err)
}

func (p runtimeTaskProvider) GetTask(ctx context.Context, args json.RawMessage) (string, error) {
	_ = ctx
	if p.store == nil {
		return "", fmt.Errorf("task store is unavailable")
	}
	var input struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return "", fmt.Errorf("decode task_get arguments: %w", err)
	}
	task, err := p.store.Get(input.ID)
	return marshalToolJSON(task, err)
}

func (p runtimeTaskProvider) ClaimTask(ctx context.Context, args json.RawMessage) (string, error) {
	_ = ctx
	if p.store == nil {
		return "", fmt.Errorf("task store is unavailable")
	}
	var input struct {
		ID    string `json:"id"`
		Owner string `json:"owner"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return "", fmt.Errorf("decode task_claim arguments: %w", err)
	}
	task, err := p.store.Claim(input.ID, input.Owner)
	return marshalToolJSON(task, err)
}

func (p runtimeTaskProvider) UpdateTask(ctx context.Context, args json.RawMessage) (string, error) {
	_ = ctx
	if p.store == nil {
		return "", fmt.Errorf("task store is unavailable")
	}
	var input tasks.Task
	if err := json.Unmarshal(args, &input); err != nil {
		return "", fmt.Errorf("decode task_update arguments: %w", err)
	}
	task, err := p.store.Update(input)
	return marshalToolJSON(task, err)
}

func (p runtimeTaskProvider) CompleteTask(ctx context.Context, args json.RawMessage) (string, error) {
	_ = ctx
	if p.store == nil {
		return "", fmt.Errorf("task store is unavailable")
	}
	var input struct {
		ID          string   `json:"id"`
		ResultPaths []string `json:"result_paths"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return "", fmt.Errorf("decode task_complete arguments: %w", err)
	}
	task, err := p.store.Complete(input.ID, input.ResultPaths)
	return marshalToolJSON(task, err)
}

func (p runtimeTaskProvider) ReleaseTask(ctx context.Context, args json.RawMessage) (string, error) {
	_ = ctx
	if p.store == nil {
		return "", fmt.Errorf("task store is unavailable")
	}
	var input struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return "", fmt.Errorf("decode task_release arguments: %w", err)
	}
	task, err := p.store.Release(input.ID)
	return marshalToolJSON(task, err)
}

func marshalToolJSON(value any, err error) (string, error) {
	if err != nil {
		return "", err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}
