package tools

import (
	"context"
	"testing"
)

type stubCapabilityProvider struct {
	output string
}

func (p stubCapabilityProvider) CapabilitiesJSON() (string, error) {
	return p.output, nil
}

func TestListCapabilitiesTool(t *testing.T) {
	tool := NewListCapabilitiesTool(func() CapabilityProvider {
		return stubCapabilityProvider{output: `{"skills":[],"active_skills":[],"mcp_resources":[]}`}
	})

	result, err := tool.Execute(context.Background(), Call{})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Output != `{"skills":[],"active_skills":[],"mcp_resources":[]}` {
		t.Fatalf("unexpected output: %q", result.Output)
	}
}
