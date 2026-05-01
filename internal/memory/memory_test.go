package memory

import "testing"

func TestBuildUsesRecentTurnsAndTruncates(t *testing.T) {
	result := Build([]Turn{
		{Role: "user", Content: "one"},
		{Role: "assistant", Content: "two"},
		{Role: "user", Content: "three"},
	}, Strategy{Enabled: true, MaxTurns: 2, MaxChars: 20})

	if result.SourceTurns != 2 {
		t.Fatalf("unexpected source turns: %d", result.SourceTurns)
	}
	if result.Text == "" {
		t.Fatalf("expected memory text")
	}
	if !result.Trimmed {
		t.Fatalf("expected truncated memory")
	}
}
