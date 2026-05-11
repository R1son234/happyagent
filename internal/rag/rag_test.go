package rag

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIndexerSearchReturnsChunkLineCitations(t *testing.T) {
	root := t.TempDir()
	docsDir := filepath.Join(root, "docs")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}

	var builder strings.Builder
	for i := 1; i <= 35; i++ {
		if i == 28 {
			builder.WriteString("semantic retrieval grounding evidence needle\n")
			continue
		}
		builder.WriteString(fmt.Sprintf("background line %02d\n", i))
	}
	if err := os.WriteFile(filepath.Join(docsDir, "rag.md"), []byte(builder.String()), 0o644); err != nil {
		t.Fatalf("write docs: %v", err)
	}

	indexer := NewScopedIndexer(root, []string{"docs"})
	result, err := indexer.SearchWithLimit("needle grounding", 3)
	if err != nil {
		t.Fatalf("SearchWithLimit() error = %v", err)
	}
	if len(result.Citations) != 1 {
		t.Fatalf("expected one citation, got %+v", result.Citations)
	}
	if result.Citations[0] != "docs/rag.md:21-35" {
		t.Fatalf("unexpected citation: %q", result.Citations[0])
	}
	if !strings.Contains(result.Text, "semantic retrieval grounding evidence needle") {
		t.Fatalf("expected matching snippet, got %q", result.Text)
	}
}

func TestIndexerRanksChunksByScore(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	content := strings.Join([]string{
		"alpha beta",
		strings.Repeat("filler\n", 24),
		"alpha alpha beta beta beta",
	}, "\n")
	if err := os.WriteFile(filepath.Join(root, "docs", "rank.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write docs: %v", err)
	}

	indexer := NewScopedIndexer(root, []string{"docs"})
	results, err := indexer.SearchResults("alpha beta", 2)
	if err != nil {
		t.Fatalf("SearchResults() error = %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected two results, got %+v", results)
	}
	if results[0].Score <= results[1].Score {
		t.Fatalf("expected first result to have higher score, got %+v", results)
	}
	if results[0].StartLine != 21 {
		t.Fatalf("expected later chunk to rank first, got %+v", results[0])
	}
}

func TestScopedIndexerRejectsEscapingPath(t *testing.T) {
	root := t.TempDir()
	indexer := NewScopedIndexer(root, []string{".."})

	_, err := indexer.SearchWithLimit("needle", 1)
	if err == nil {
		t.Fatal("expected escaping path error")
	}
	if !strings.Contains(err.Error(), "escapes root") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestIndexerSearchMatchesChineseQuery(t *testing.T) {
	root := t.TempDir()
	docsDir := filepath.Join(root, "docs")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(docsDir, "career.md"), []byte("岗位职责：负责增长分析和内容策略。\n"), 0o644); err != nil {
		t.Fatalf("write docs: %v", err)
	}

	indexer := NewScopedIndexer(root, []string{"docs"})
	result, err := indexer.SearchWithLimit("增长分析", 1)
	if err != nil {
		t.Fatalf("SearchWithLimit() error = %v", err)
	}
	if len(result.Citations) != 1 {
		t.Fatalf("expected one citation, got %+v", result.Citations)
	}
	if !strings.Contains(result.Text, "增长分析") {
		t.Fatalf("expected Chinese query match in result, got %q", result.Text)
	}
}
