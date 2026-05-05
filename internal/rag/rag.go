package rag

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Indexer struct {
	root  string
	paths []string
}

type Retriever interface {
	SearchWithLimit(query string, maxResults int) (BuildResult, error)
}

type BuildResult struct {
	Text      string
	Citations []string
}

type SearchResult struct {
	Path      string
	StartLine int
	EndLine   int
	Snippet   string
	Score     int
}

const (
	defaultChunkLines   = 24
	defaultChunkOverlap = 4
)

func NewIndexer(root string) *Indexer {
	if strings.TrimSpace(root) == "" {
		return nil
	}
	return &Indexer{root: root}
}

func NewScopedIndexer(root string, paths []string) *Indexer {
	if strings.TrimSpace(root) == "" {
		return nil
	}
	cleaned := make([]string, 0, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		cleaned = append(cleaned, filepath.Clean(path))
	}
	return &Indexer{root: root, paths: cleaned}
}

func (i *Indexer) Search(query string) (BuildResult, error) {
	return i.SearchWithLimit(query, 3)
}

func (i *Indexer) SearchWithLimit(query string, maxResults int) (BuildResult, error) {
	if i == nil || strings.TrimSpace(query) == "" {
		return BuildResult{}, nil
	}
	if maxResults <= 0 {
		maxResults = 3
	}

	results, err := i.SearchResults(query, maxResults)
	if err != nil {
		return BuildResult{}, err
	}
	if len(results) == 0 {
		return BuildResult{}, nil
	}

	citations := make([]string, 0, len(results))
	var builder strings.Builder
	builder.WriteString("Relevant local references:\n")
	for _, result := range results {
		citation := formatCitation(result)
		citations = append(citations, citation)
		builder.WriteString("- [")
		builder.WriteString(citation)
		builder.WriteString("] ")
		builder.WriteString(result.Snippet)
		builder.WriteString("\n")
	}
	return BuildResult{
		Text:      strings.TrimSpace(builder.String()),
		Citations: citations,
	}, nil
}

func (i *Indexer) SearchResults(query string, maxResults int) ([]SearchResult, error) {
	if i == nil || strings.TrimSpace(query) == "" {
		return nil, nil
	}
	if maxResults <= 0 {
		maxResults = 3
	}

	terms := tokenize(query)
	if len(terms) == 0 {
		return nil, nil
	}

	var results []SearchResult
	searchPaths := i.paths
	if len(searchPaths) == 0 {
		searchPaths = []string{"."}
	}
	for _, searchPath := range searchPaths {
		resolved, err := resolveSearchPath(i.root, searchPath)
		if err != nil {
			return nil, err
		}
		info, err := os.Stat(resolved)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			if err := collectFileMatches(i.root, resolved, terms, &results); err != nil {
				return nil, err
			}
			continue
		}
		err = filepath.WalkDir(resolved, func(path string, d os.DirEntry, walkErr error) error {
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
			return collectFileMatches(i.root, path, terms, &results)
		})
		if err != nil {
			return nil, err
		}
	}
	if len(results) == 0 {
		return nil, nil
	}
	sort.Slice(results, func(a, b int) bool {
		if results[a].Score != results[b].Score {
			return results[a].Score > results[b].Score
		}
		if results[a].Path != results[b].Path {
			return results[a].Path < results[b].Path
		}
		return results[a].StartLine < results[b].StartLine
	})

	limit := maxResults
	if len(results) < limit {
		limit = len(results)
	}
	return results[:limit], nil
}

func resolveSearchPath(root string, path string) (string, error) {
	target := path
	if !filepath.IsAbs(target) {
		target = filepath.Join(root, target)
	}
	clean := filepath.Clean(target)
	rel, err := filepath.Rel(root, clean)
	if err != nil {
		return "", fmt.Errorf("calculate relative path for %q: %w", clean, err)
	}
	if rel == ".." || strings.HasPrefix(rel, "../") {
		return "", fmt.Errorf("path %q escapes root %q", path, root)
	}
	return clean, nil
}

func collectFileMatches(root string, path string, terms []string, results *[]SearchResult) error {
	if !isSupportedFile(path) {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	rel, _ := filepath.Rel(root, path)
	for _, chunk := range chunkText(string(data), defaultChunkLines, defaultChunkOverlap) {
		score := matchScore(chunk.Text, terms)
		if score == 0 {
			continue
		}
		*results = append(*results, SearchResult{
			Path:      rel,
			StartLine: chunk.StartLine,
			EndLine:   chunk.EndLine,
			Snippet:   extractSnippet(chunk.Text, terms),
			Score:     score,
		})
	}
	return nil
}

type textChunk struct {
	StartLine int
	EndLine   int
	Text      string
}

func chunkText(content string, chunkLines int, overlap int) []textChunk {
	if chunkLines <= 0 {
		chunkLines = defaultChunkLines
	}
	if overlap < 0 {
		overlap = 0
	}
	if overlap >= chunkLines {
		overlap = chunkLines - 1
	}

	content = strings.TrimSuffix(content, "\n")
	content = strings.TrimSuffix(content, "\r")
	lines := strings.Split(content, "\n")
	if len(lines) == 0 {
		return nil
	}
	chunks := make([]textChunk, 0, (len(lines)/chunkLines)+1)
	step := chunkLines - overlap
	for start := 0; start < len(lines); start += step {
		end := start + chunkLines
		if end > len(lines) {
			end = len(lines)
		}
		text := strings.TrimSpace(strings.Join(lines[start:end], "\n"))
		if text != "" {
			chunks = append(chunks, textChunk{
				StartLine: start + 1,
				EndLine:   end,
				Text:      text,
			})
		}
		if end == len(lines) {
			break
		}
	}
	return chunks
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

func formatCitation(result SearchResult) string {
	if result.StartLine <= 0 || result.EndLine <= 0 {
		return result.Path
	}
	if result.StartLine == result.EndLine {
		return fmt.Sprintf("%s:%d", result.Path, result.StartLine)
	}
	return fmt.Sprintf("%s:%d-%d", result.Path, result.StartLine, result.EndLine)
}

func isSupportedFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".md", ".txt", ".json":
		return true
	default:
		return false
	}
}
