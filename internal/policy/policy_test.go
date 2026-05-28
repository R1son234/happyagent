package policy

import (
	"testing"

	"happyagent/internal/tools"
)

func TestPolicyRequiresApprovalForDangerousTool(t *testing.T) {
	engine := New(nil, nil)
	decision, reason := engine.Decide(tools.Definition{Name: "shell", Dangerous: true})
	if decision != DecisionAsk {
		t.Fatalf("unexpected decision: %s", decision)
	}
	if reason == "" {
		t.Fatalf("expected denial reason")
	}
}

func TestPolicyDenyBeatsApprovedDangerousTool(t *testing.T) {
	engine := New([]string{"shell"}, []string{"shell"})
	decision, _ := engine.Decide(tools.Definition{Name: "shell", Dangerous: true})
	if decision != DecisionDeny {
		t.Fatalf("unexpected decision: %s", decision)
	}
}

func TestPolicyDeniesDangerousShellArguments(t *testing.T) {
	engine := New([]string{"shell"}, nil)
	decision, reason := engine.DecideRequest(Request{
		Tool: tools.Definition{Name: "shell", Dangerous: true},
		Args: []byte(`{"argv":["rm","-rf","/"]}`),
	})
	if decision != DecisionDeny {
		t.Fatalf("unexpected decision: %s (%s)", decision, reason)
	}
}

func TestPolicyAllowsApprovedDangerousTool(t *testing.T) {
	engine := New([]string{"shell"}, nil)
	decision, _ := engine.Decide(tools.Definition{Name: "shell", Dangerous: true})
	if decision != DecisionAllow {
		t.Fatalf("unexpected decision: %s", decision)
	}
}
