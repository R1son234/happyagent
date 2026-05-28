package tools

import (
	"context"
	"encoding/json"
	"fmt"
)

const (
	TaskCreateToolName   = "task_create"
	TaskListToolName     = "task_list"
	TaskGetToolName      = "task_get"
	TaskClaimToolName    = "task_claim"
	TaskUpdateToolName   = "task_update"
	TaskCompleteToolName = "task_complete"
	TaskReleaseToolName  = "task_release"
)

type TaskProvider interface {
	CreateTask(ctx context.Context, args json.RawMessage) (string, error)
	ListTasks(ctx context.Context, args json.RawMessage) (string, error)
	GetTask(ctx context.Context, args json.RawMessage) (string, error)
	ClaimTask(ctx context.Context, args json.RawMessage) (string, error)
	UpdateTask(ctx context.Context, args json.RawMessage) (string, error)
	CompleteTask(ctx context.Context, args json.RawMessage) (string, error)
	ReleaseTask(ctx context.Context, args json.RawMessage) (string, error)
}

type TaskTool struct {
	name        string
	description string
	schema      string
	resolver    func(context.Context) TaskProvider
}

func NewTaskTools(resolver func(context.Context) TaskProvider) []*TaskTool {
	return []*TaskTool{
		{name: TaskCreateToolName, description: "Create a durable task with optional dependencies.", schema: `{"type":"object","required":["title"],"properties":{"id":{"type":"string"},"title":{"type":"string"},"description":{"type":"string"},"blocked_by":{"type":"array","items":{"type":"string"}},"workspace":{"type":"string"},"evidence_paths":{"type":"array","items":{"type":"string"}}}}`, resolver: resolver},
		{name: TaskListToolName, description: "List durable tasks and their status.", schema: `{"type":"object","properties":{},"additionalProperties":false}`, resolver: resolver},
		{name: TaskGetToolName, description: "Get a durable task by id.", schema: `{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}}`, resolver: resolver},
		{name: TaskClaimToolName, description: "Claim an unblocked durable task for an owner.", schema: `{"type":"object","required":["id","owner"],"properties":{"id":{"type":"string"},"owner":{"type":"string"}}}`, resolver: resolver},
		{name: TaskUpdateToolName, description: "Update durable task fields.", schema: `{"type":"object","required":["id"],"properties":{"id":{"type":"string"},"title":{"type":"string"},"description":{"type":"string"},"status":{"type":"string"},"blocked_by":{"type":"array","items":{"type":"string"}},"owner":{"type":"string"},"workspace":{"type":"string"},"evidence_paths":{"type":"array","items":{"type":"string"}},"result_paths":{"type":"array","items":{"type":"string"}}}}`, resolver: resolver},
		{name: TaskCompleteToolName, description: "Mark an in-progress durable task completed.", schema: `{"type":"object","required":["id"],"properties":{"id":{"type":"string"},"result_paths":{"type":"array","items":{"type":"string"}}}}`, resolver: resolver},
		{name: TaskReleaseToolName, description: "Release ownership of a durable task.", schema: `{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}}`, resolver: resolver},
	}
}

func (t *TaskTool) Definition() Definition {
	return Definition{Name: t.name, Description: t.description, InputSchema: t.schema, Dangerous: false}
}

func (t *TaskTool) Execute(ctx context.Context, call Call) (Result, error) {
	provider := t.resolver(ctx)
	if provider == nil {
		return Result{}, fmt.Errorf("%s is unavailable outside an active runtime session", t.name)
	}
	var output string
	var err error
	switch t.name {
	case TaskCreateToolName:
		output, err = provider.CreateTask(ctx, call.Arguments)
	case TaskListToolName:
		output, err = provider.ListTasks(ctx, call.Arguments)
	case TaskGetToolName:
		output, err = provider.GetTask(ctx, call.Arguments)
	case TaskClaimToolName:
		output, err = provider.ClaimTask(ctx, call.Arguments)
	case TaskUpdateToolName:
		output, err = provider.UpdateTask(ctx, call.Arguments)
	case TaskCompleteToolName:
		output, err = provider.CompleteTask(ctx, call.Arguments)
	case TaskReleaseToolName:
		output, err = provider.ReleaseTask(ctx, call.Arguments)
	default:
		err = fmt.Errorf("unknown task tool %q", t.name)
	}
	if err != nil {
		return Result{}, err
	}
	return Result{Output: output}, nil
}

type taskProviderContextKey struct{}

func WithTaskProvider(ctx context.Context, provider TaskProvider) context.Context {
	return context.WithValue(ctx, taskProviderContextKey{}, provider)
}

func TaskProviderFromContext(ctx context.Context) TaskProvider {
	if ctx == nil {
		return nil
	}
	provider, _ := ctx.Value(taskProviderContextKey{}).(TaskProvider)
	return provider
}
