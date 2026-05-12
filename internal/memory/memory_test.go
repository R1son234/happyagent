package memory

import (
	"strings"
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
	if !strings.Contains(result.Text, "Recent session turns") {
		t.Fatalf("expected recent turns heading, got %q", result.Text)
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

func TestBuildAddsStructuredSummaryFromOlderTurns(t *testing.T) {
	result := Build([]Turn{
		{Role: "user", Content: "目标：实现 context compression。需要修改 internal/memory/memory.go。"},
		{Role: "assistant", Content: "决定采用确定性摘要，不调用 LLM。下一步补测试。"},
		{Role: "user", Content: "继续做。"},
		{Role: "assistant", Content: "收到。"},
	}, Strategy{
		Enabled:            true,
		MaxTurns:           2,
		MaxChars:           2000,
		SummaryEnabled:     true,
		SummaryMaxChars:    1000,
		SummarySourceTurns: 10,
	})

	if result.SummarySourceTurns != 2 {
		t.Fatalf("unexpected summary source turns: %d", result.SummarySourceTurns)
	}
	for _, want := range []string{"Session memory summary", "Goals", "Decisions", "Files and artifacts", "Open items", "Recent session turns"} {
		if !strings.Contains(result.Text, want) {
			t.Fatalf("expected %q in memory text:\n%s", want, result.Text)
		}
	}
	if strings.Contains(result.Text, "user: 继续做") && strings.Contains(result.Text, "assistant: 收到") {
		return
	}
	t.Fatalf("expected recent turns to be preserved:\n%s", result.Text)
}

func TestBuildKeepsOldBehaviorWhenSummaryDisabled(t *testing.T) {
	result := Build([]Turn{
		{Role: "user", Content: "目标：旧内容"},
		{Role: "assistant", Content: "最近内容"},
	}, Strategy{Enabled: true, MaxTurns: 1, MaxChars: 2000})

	if result.SummarySourceTurns != 0 {
		t.Fatalf("unexpected summary source turns: %d", result.SummarySourceTurns)
	}
	if strings.Contains(result.Text, "Session memory summary") {
		t.Fatalf("did not expect summary: %q", result.Text)
	}
	if strings.Contains(result.Text, "旧内容") || !strings.Contains(result.Text, "最近内容") {
		t.Fatalf("unexpected memory text: %q", result.Text)
	}
}
