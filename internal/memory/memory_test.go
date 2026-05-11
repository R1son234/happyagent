package memory

import (
	"testing"
	"unicode/utf8"
)

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

func TestBuildTruncatesChineseTextOnRuneBoundary(t *testing.T) {
	result := Build([]Turn{
		{Role: "user", Content: "简历项目增长分析复盘"},
	}, Strategy{Enabled: true, MaxTurns: 1, MaxChars: 14})

	if !result.Trimmed {
		t.Fatalf("expected truncated memory")
	}
	if !utf8.ValidString(result.Text) {
		t.Fatalf("expected valid UTF-8, got %q", result.Text)
	}
}
