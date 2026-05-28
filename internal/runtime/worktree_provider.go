package runtime

import (
	"context"
	"encoding/json"
	"fmt"

	"happyagent/internal/worktree"
)

type runtimeWorktreeProvider struct {
	manager *worktree.Manager
}

func (p runtimeWorktreeProvider) CreateWorktree(ctx context.Context, args json.RawMessage) (string, error) {
	if p.manager == nil {
		return "", fmt.Errorf("worktree manager is unavailable")
	}
	var input struct {
		Slug   string `json:"slug"`
		Branch string `json:"branch"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return "", fmt.Errorf("decode worktree_create arguments: %w", err)
	}
	path, err := p.manager.Create(ctx, input.Slug, input.Branch)
	return marshalToolJSON(map[string]string{"path": path}, err)
}

func (p runtimeWorktreeProvider) EnterWorktree(ctx context.Context, args json.RawMessage) (string, error) {
	_ = ctx
	if p.manager == nil {
		return "", fmt.Errorf("worktree manager is unavailable")
	}
	var input struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return "", fmt.Errorf("decode worktree_enter arguments: %w", err)
	}
	path, err := p.manager.Enter(input.Path)
	return marshalToolJSON(map[string]string{"path": path}, err)
}

func (p runtimeWorktreeProvider) KeepWorktree(ctx context.Context, args json.RawMessage) (string, error) {
	_ = ctx
	if p.manager == nil {
		return "", fmt.Errorf("worktree manager is unavailable")
	}
	var input struct {
		Path   string `json:"path"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return "", fmt.Errorf("decode worktree_keep arguments: %w", err)
	}
	path, err := p.manager.Keep(input.Path)
	return marshalToolJSON(map[string]string{"path": path, "status": "kept", "reason": input.Reason}, err)
}

func (p runtimeWorktreeProvider) RemoveWorktree(ctx context.Context, args json.RawMessage) (string, error) {
	if p.manager == nil {
		return "", fmt.Errorf("worktree manager is unavailable")
	}
	var input struct {
		Path         string `json:"path"`
		DiscardDirty bool   `json:"discard_dirty"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return "", fmt.Errorf("decode worktree_remove arguments: %w", err)
	}
	if err := p.manager.Remove(ctx, input.Path, input.DiscardDirty); err != nil {
		return "", err
	}
	return marshalToolJSON(map[string]string{"removed": input.Path}, nil)
}
