package memory

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

type Turn struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Strategy struct {
	Enabled            bool `json:"enabled"`
	MaxTurns           int  `json:"max_turns"`
	MaxChars           int  `json:"max_chars"`
	SummaryEnabled     bool `json:"summary_enabled"`
	SummaryMaxChars    int  `json:"summary_max_chars"`
	SummarySourceTurns int  `json:"summary_source_turns"`
}

type BuildResult struct {
	Text               string
	Trimmed            bool
	SourceTurns        int
	SummarySourceTurns int
}

func Build(turns []Turn, strategy Strategy) BuildResult {
	if !strategy.Enabled || len(turns) == 0 {
		return BuildResult{}
	}

	maxTurns := strategy.MaxTurns
	if maxTurns <= 0 || maxTurns > len(turns) {
		maxTurns = len(turns)
	}
	selected := turns[len(turns)-maxTurns:]

	var builder strings.Builder
	summarySourceTurns := 0
	if strategy.SummaryEnabled {
		summaryTurns := summarySource(turns, len(selected), strategy.SummarySourceTurns)
		summarySourceTurns = len(summaryTurns)
		summary := buildSummary(summaryTurns, strategy.SummaryMaxChars)
		if summary != "" {
			builder.WriteString(summary)
			builder.WriteString("\n\n")
		}
	}
	writeRecentTurns(&builder, selected)

	text := strings.TrimSpace(builder.String())
	trimmed := false
	if strategy.MaxChars > 0 {
		runes := []rune(text)
		if len(runes) > strategy.MaxChars {
			text = string(runes[:strategy.MaxChars])
			text = strings.TrimSpace(text) + "\n...[memory truncated]"
			trimmed = true
		}
	}

	return BuildResult{
		Text:               text,
		Trimmed:            trimmed,
		SourceTurns:        len(selected),
		SummarySourceTurns: summarySourceTurns,
	}
}

func writeRecentTurns(builder *strings.Builder, turns []Turn) {
	builder.WriteString("Recent session turns:\n")
	for _, turn := range turns {
		role := strings.TrimSpace(turn.Role)
		if role == "" {
			role = "unknown"
		}
		builder.WriteString("- ")
		builder.WriteString(role)
		builder.WriteString(": ")
		builder.WriteString(strings.TrimSpace(turn.Content))
		builder.WriteString("\n")
	}
}

func summarySource(turns []Turn, recentCount int, maxSourceTurns int) []Turn {
	olderEnd := len(turns) - recentCount
	if olderEnd <= 0 {
		return nil
	}
	older := turns[:olderEnd]
	if maxSourceTurns <= 0 || maxSourceTurns >= len(older) {
		return older
	}
	return older[len(older)-maxSourceTurns:]
}

type summaryBuckets struct {
	goals     []string
	decisions []string
	files     []string
	openItems []string
}

var pathPattern = regexp.MustCompile(`(^|\s)(\.{1,2}/|/)?[A-Za-z0-9_.-]+(/[A-Za-z0-9_.-]+)+(:[0-9]+)?`)

func buildSummary(turns []Turn, maxChars int) string {
	if len(turns) == 0 {
		return ""
	}
	buckets := summaryBuckets{}
	for _, turn := range turns {
		role := strings.TrimSpace(turn.Role)
		for _, sentence := range splitSummarySentences(turn.Content) {
			classifySummarySentence(&buckets, role, sentence)
		}
	}

	var builder strings.Builder
	builder.WriteString("Session memory summary:\n")
	writeSummaryBucket(&builder, "Goals", buckets.goals)
	writeSummaryBucket(&builder, "Decisions", buckets.decisions)
	writeSummaryBucket(&builder, "Files and artifacts", buckets.files)
	writeSummaryBucket(&builder, "Open items", buckets.openItems)

	summary := strings.TrimSpace(builder.String())
	if summary == "Session memory summary:" {
		return ""
	}
	return trimRunes(summary, maxChars, "\n...[session memory summary truncated]")
}

func splitSummarySentences(content string) []string {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil
	}
	parts := strings.FieldsFunc(content, func(r rune) bool {
		return r == '\n' || r == '。' || r == '！' || r == '？'
	})
	sentences := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(strings.Trim(part, "-* \t\r"))
		if part == "" {
			continue
		}
		sentences = append(sentences, part)
	}
	return sentences
}

func classifySummarySentence(buckets *summaryBuckets, role string, sentence string) {
	lower := strings.ToLower(sentence)
	tagged := fmt.Sprintf("%s: %s", roleOrUnknown(role), compactSentence(sentence, 240))
	switch {
	case containsAny(lower, "todo", "待办", "下一步", "还差", "阻塞", "风险", "open item"):
		buckets.openItems = appendUniqueLimit(buckets.openItems, tagged, 8)
	case containsAny(lower, "决定", "确认", "采用", "选择", "结论", "decision", "decided", "implemented"):
		buckets.decisions = appendUniqueLimit(buckets.decisions, tagged, 8)
	case pathPattern.MatchString(sentence) || containsAny(lower, "file", "path", "目录", "文件", ".go", ".md", ".json", "go test", "make build"):
		buckets.files = appendUniqueLimit(buckets.files, tagged, 10)
	case containsAny(lower, "目标", "需要", "希望", "实现", "修复", "增加", "goal", "need", "want"):
		buckets.goals = appendUniqueLimit(buckets.goals, tagged, 8)
	}
}

func writeSummaryBucket(builder *strings.Builder, title string, values []string) {
	if len(values) == 0 {
		return
	}
	builder.WriteString("- ")
	builder.WriteString(title)
	builder.WriteString(":\n")
	for _, value := range values {
		builder.WriteString("  - ")
		builder.WriteString(value)
		builder.WriteString("\n")
	}
}

func appendUniqueLimit(values []string, value string, limit int) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	if limit > 0 && len(values) >= limit {
		return values
	}
	return append(values, value)
}

func containsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func roleOrUnknown(role string) string {
	role = strings.TrimSpace(role)
	if role == "" {
		return "unknown"
	}
	return role
}

func compactSentence(sentence string, maxRunes int) string {
	sentence = strings.Join(strings.Fields(sentence), " ")
	return trimRunes(sentence, maxRunes, "...")
}

func trimRunes(text string, maxRunes int, suffix string) string {
	if maxRunes <= 0 || utf8.RuneCountInString(text) <= maxRunes {
		return text
	}
	runes := []rune(text)
	available := maxRunes - utf8.RuneCountInString(suffix)
	if available <= 0 {
		return string(runes[:maxRunes])
	}
	return strings.TrimSpace(string(runes[:available])) + suffix
}
