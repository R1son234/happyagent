package policy

import (
	"testing"

	"happyagent/internal/tools"
)

func TestPolicyRequiresApprovalForDangerousTool(t *testing.T) {
	engine := New(nil, nil)
	decision, reason := engine.Decide(tools.Definition{Name: "shell", Dangerous: true})
	if decision != DecisionRequireApproval {
		t.Fatalf("unexpected decision: %s", decision)
	}
	if reason == "" {
		t.Fatalf("expected denial reason")
	}
}

func TestPolicyAllowsApprovedDangerousTool(t *testing.T) {
	engine := New([]string{"shell"}, nil)
	decision, _ := engine.Decide(tools.Definition{Name: "shell", Dangerous: true})
	if decision != DecisionAllow {
		t.Fatalf("unexpected decision: %s", decision)
	}
}
