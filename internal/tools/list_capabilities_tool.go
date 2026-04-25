package tools

import (
	"context"
	"fmt"
)

type CapabilityProvider interface {
	CapabilitiesJSON() (string, error)
}

type ListCapabilitiesTool struct {
	resolver func() CapabilityProvider
}

func NewListCapabilitiesTool(resolver func() CapabilityProvider) *ListCapabilitiesTool {
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

	provider := t.resolver()
	if provider == nil {
		return Result{}, fmt.Errorf("list_capabilities is unavailable outside an active runtime session")
	}

	output, err := provider.CapabilitiesJSON()
	if err != nil {
		return Result{}, err
	}
	return Result{Output: output}, nil
}
