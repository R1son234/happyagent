package tools

import (
	"context"
	"encoding/json"
	"fmt"
)

const (
	WorktreeCreateToolName = "worktree_create"
	WorktreeEnterToolName  = "worktree_enter"
	WorktreeKeepToolName   = "worktree_keep"
	WorktreeRemoveToolName = "worktree_remove"
)

type WorktreeProvider interface {
	CreateWorktree(ctx context.Context, args json.RawMessage) (string, error)
	EnterWorktree(ctx context.Context, args json.RawMessage) (string, error)
	KeepWorktree(ctx context.Context, args json.RawMessage) (string, error)
	RemoveWorktree(ctx context.Context, args json.RawMessage) (string, error)
}

type WorktreeTool struct {
	name        string
	description string
	schema      string
	resolver    func(context.Context) WorktreeProvider
}

func NewWorktreeTools(resolver func(context.Context) WorktreeProvider) []*WorktreeTool {
	return []*WorktreeTool{
		{name: WorktreeCreateToolName, description: "Create an isolated git worktree under .happyagent/worktrees.", schema: `{"type":"object","required":["slug"],"properties":{"slug":{"type":"string"},"branch":{"type":"string"}},"additionalProperties":false}`, resolver: resolver},
		{name: WorktreeEnterToolName, description: "Resolve a worktree path for a child agent cwd override.", schema: `{"type":"object","required":["path"],"properties":{"path":{"type":"string"}},"additionalProperties":false}`, resolver: resolver},
		{name: WorktreeKeepToolName, description: "Keep an isolated git worktree for manual review and return its validated path.", schema: `{"type":"object","required":["path"],"properties":{"path":{"type":"string"},"reason":{"type":"string"}},"additionalProperties":false}`, resolver: resolver},
		{name: WorktreeRemoveToolName, description: "Remove an isolated git worktree, refusing dirty worktrees unless discard_dirty is true.", schema: `{"type":"object","required":["path"],"properties":{"path":{"type":"string"},"discard_dirty":{"type":"boolean"}},"additionalProperties":false}`, resolver: resolver},
	}
}

func (t *WorktreeTool) Definition() Definition {
	return Definition{Name: t.name, Description: t.description, InputSchema: t.schema, Dangerous: true}
}

func (t *WorktreeTool) Execute(ctx context.Context, call Call) (Result, error) {
	provider := t.resolver(ctx)
	if provider == nil {
		return Result{}, fmt.Errorf("%s is unavailable outside an active runtime session", t.name)
	}
	var output string
	var err error
	switch t.name {
	case WorktreeCreateToolName:
		output, err = provider.CreateWorktree(ctx, call.Arguments)
	case WorktreeEnterToolName:
		output, err = provider.EnterWorktree(ctx, call.Arguments)
	case WorktreeKeepToolName:
		output, err = provider.KeepWorktree(ctx, call.Arguments)
	case WorktreeRemoveToolName:
		output, err = provider.RemoveWorktree(ctx, call.Arguments)
	default:
		err = fmt.Errorf("unknown worktree tool %q", t.name)
	}
	if err != nil {
		return Result{}, err
	}
	return Result{Output: output}, nil
}

type worktreeProviderContextKey struct{}

func WithWorktreeProvider(ctx context.Context, provider WorktreeProvider) context.Context {
	return context.WithValue(ctx, worktreeProviderContextKey{}, provider)
}

func WorktreeProviderFromContext(ctx context.Context) WorktreeProvider {
	if ctx == nil {
		return nil
	}
	provider, _ := ctx.Value(worktreeProviderContextKey{}).(WorktreeProvider)
	return provider
}
