package rag

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Indexer struct {
	root string
}

type BuildResult struct {
	Text      string
	Citations []string
}

func NewIndexer(root string) *Indexer {
	if strings.TrimSpace(root) == "" {
		return nil
	}
	return &Indexer{root: root}
}

func (i *Indexer) Search(query string) (BuildResult, error) {
	if i == nil || strings.TrimSpace(query) == "" {
		return BuildResult{}, nil
	}

	terms := tokenize(query)
	if len(terms) == 0 {
		return BuildResult{}, nil
	}

	var matches []string
	err := filepath.WalkDir(i.root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "bin" || name == "logs" {
				return filepath.SkipDir
			}
			return nil
		}
		if !isSupportedFile(path) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		content := string(data)
		score := matchScore(content, terms)
		if score == 0 {
			return nil
		}
		rel, _ := filepath.Rel(i.root, path)
		matches = append(matches, fmt.Sprintf("%03d|%s|%s", score, rel, extractSnippet(content, terms)))
		return nil
	})
	if err != nil {
		return BuildResult{}, err
	}
	if len(matches) == 0 {
		return BuildResult{}, nil
	}
	sort.Sort(sort.Reverse(sort.StringSlice(matches)))

	limit := 3
	if len(matches) < limit {
		limit = len(matches)
	}
	citations := make([]string, 0, limit)
	var builder strings.Builder
	builder.WriteString("Relevant local references:\n")
	for _, entry := range matches[:limit] {
		parts := strings.SplitN(entry, "|", 3)
		if len(parts) != 3 {
			continue
		}
		citations = append(citations, parts[1])
		builder.WriteString("- [")
		builder.WriteString(parts[1])
		builder.WriteString("] ")
		builder.WriteString(parts[2])
		builder.WriteString("\n")
	}
	return BuildResult{
		Text:      strings.TrimSpace(builder.String()),
		Citations: citations,
	}, nil
}

func tokenize(query string) []string {
	fields := strings.Fields(strings.ToLower(query))
	terms := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.Trim(field, ".,:;!?()[]{}\"'")
		if len(field) < 3 {
			continue
		}
		terms = append(terms, field)
	}
	return terms
}

func matchScore(content string, terms []string) int {
	lowered := strings.ToLower(content)
	score := 0
	for _, term := range terms {
		score += strings.Count(lowered, term)
	}
	return score
}

func extractSnippet(content string, terms []string) string {
	lowered := strings.ToLower(content)
	for _, term := range terms {
		idx := strings.Index(lowered, term)
		if idx < 0 {
			continue
		}
		start := idx - 60
		if start < 0 {
			start = 0
		}
		end := idx + 160
		if end > len(content) {
			end = len(content)
		}
		return strings.ReplaceAll(strings.TrimSpace(content[start:end]), "\n", " ")
	}
	if len(content) > 160 {
		return strings.ReplaceAll(strings.TrimSpace(content[:160]), "\n", " ")
	}
	return strings.ReplaceAll(strings.TrimSpace(content), "\n", " ")
}

func isSupportedFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".md", ".txt", ".json":
		return true
	default:
		return false
	}
}
