package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"happyagent/internal/rag"
)

const SearchDocsToolName = "search_docs"

var defaultSearchDocsPaths = []string{
	"docs",
	"README.md",
	"AGENT.md",
	"AGENTS.md",
}

type SearchDocsTool struct {
	indexer *rag.Indexer
}

func NewSearchDocsTool(root string) (*SearchDocsTool, error) {
	resolver, err := NewRootedPathResolver(root)
	if err != nil {
		return nil, err
	}
	return &SearchDocsTool{
		indexer: rag.NewScopedIndexer(resolver.Root(), defaultSearchDocsPaths),
	}, nil
}

func (t *SearchDocsTool) Definition() Definition {
	return Definition{
		Name:        SearchDocsToolName,
		Description: "Search local project documentation on demand. Searches docs/ and root README/AGENT files only; use file_search for arbitrary repository code search.",
		InputSchema: `{"type":"object","properties":{"query":{"type":"string"},"max_results":{"type":"integer","minimum":1,"description":"Optional maximum number of documentation matches to return. Defaults to 5."}},"required":["query"]}`,
	}
}

func (t *SearchDocsTool) Execute(ctx context.Context, call Call) (Result, error) {
	_ = ctx

	var input struct {
		Query      string `json:"query"`
		MaxResults int    `json:"max_results"`
	}
	if err := json.Unmarshal(call.Arguments, &input); err != nil {
		return Result{}, fmt.Errorf("decode search_docs arguments: %w", err)
	}
	if strings.TrimSpace(input.Query) == "" {
		return Result{}, fmt.Errorf("search_docs query must not be empty")
	}
	maxResults := input.MaxResults
	if maxResults <= 0 {
		maxResults = 5
	}
	if maxResults > 20 {
		maxResults = 20
	}

	result, err := t.indexer.SearchWithLimit(input.Query, maxResults)
	if err != nil {
		return Result{}, fmt.Errorf("search docs: %w", err)
	}
	if strings.TrimSpace(result.Text) == "" {
		return Result{Output: "(no matching docs)"}, nil
	}
	return Result{Output: result.Text}, nil
}
