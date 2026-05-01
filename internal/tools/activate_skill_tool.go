package tools

import (
	"context"
	"encoding/json"
	"fmt"
)

type ActivateSkillProvider interface {
	Definition() Definition
	ActivateSkills(ctx context.Context, skillNames []string) (string, error)
}

const activateSkillInputSchema = `{"type":"object","properties":{"skill_names":{"type":"array","items":{"type":"string"},"description":"Skill names to activate."}},"required":["skill_names"]}`

type ActivateSkillTool struct {
	resolver func(ctx context.Context) ActivateSkillProvider
}

func NewActivateSkillTool(resolver func(ctx context.Context) ActivateSkillProvider) *ActivateSkillTool {
	return &ActivateSkillTool{
		resolver: resolver,
	}
}

func (t *ActivateSkillTool) Definition() Definition {
	provider := ActivateSkillProviderFromContext(context.Background())
	if provider == nil {
		return Definition{
			Name:        "activate_skill",
			Description: "No skills are available.",
			InputSchema: activateSkillInputSchema,
		}
	}
	return provider.Definition()
}

func (t *ActivateSkillTool) Execute(ctx context.Context, call Call) (Result, error) {
	provider := t.resolver(ctx)
	if provider == nil {
		return Result{}, fmt.Errorf("activate_skill is unavailable outside an active runtime session")
	}

	var input struct {
		SkillNames []string `json:"skill_names"`
	}
	if err := json.Unmarshal(call.Arguments, &input); err != nil {
		return Result{}, fmt.Errorf("decode activate_skill arguments: %w", err)
	}

	output, err := provider.ActivateSkills(ctx, input.SkillNames)
	if err != nil {
		return Result{}, err
	}
	return Result{Output: output}, nil
}

type activateSkillProviderContextKey struct{}

func WithActivateSkillProvider(ctx context.Context, provider ActivateSkillProvider) context.Context {
	return context.WithValue(ctx, activateSkillProviderContextKey{}, provider)
}

func ActivateSkillProviderFromContext(ctx context.Context) ActivateSkillProvider {
	if ctx == nil {
		return nil
	}
	provider, _ := ctx.Value(activateSkillProviderContextKey{}).(ActivateSkillProvider)
	return provider
}
