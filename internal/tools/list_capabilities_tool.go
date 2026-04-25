package tools

import (
	"context"
	"fmt"
)

type CapabilityProvider interface {
	CapabilitiesJSON() (string, error)
}

type ListCapabilitiesTool struct {
	resolver func(ctx context.Context) CapabilityProvider
}

func NewListCapabilitiesTool(resolver func(ctx context.Context) CapabilityProvider) *ListCapabilitiesTool {
	return &ListCapabilitiesTool{resolver: resolver}
}

func (t *ListCapabilitiesTool) Definition() Definition {
	return Definition{
		Name:        "list_capabilities",
		Description: "Return currently available skills, active skills, and MCP resources.",
		InputSchema: `{"type":"object","properties":{},"additionalProperties":false}`,
	}
}

func (t *ListCapabilitiesTool) Execute(ctx context.Context, call Call) (Result, error) {
	_ = ctx
	_ = call

	provider := t.resolver(ctx)
	if provider == nil {
		return Result{}, fmt.Errorf("list_capabilities is unavailable outside an active runtime session")
	}

	output, err := provider.CapabilitiesJSON()
	if err != nil {
		return Result{}, err
	}
	return Result{Output: output}, nil
}

type capabilityProviderContextKey struct{}

func WithCapabilityProvider(ctx context.Context, provider CapabilityProvider) context.Context {
	return context.WithValue(ctx, capabilityProviderContextKey{}, provider)
}

func CapabilityProviderFromContext(ctx context.Context) CapabilityProvider {
	if ctx == nil {
		return nil
	}
	provider, _ := ctx.Value(capabilityProviderContextKey{}).(CapabilityProvider)
	return provider
}
