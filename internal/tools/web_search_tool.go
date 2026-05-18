package tools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	htmlstd "html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"happyagent/internal/config"
)

const WebSearchToolName = "web_search"

const directSearchMaxBytes = 2 * 1024 * 1024
const defaultBaiduSearchURL = "https://www.baidu.com/s"
const defaultBingSearchURL = "https://www.bing.com/search"

var (
	baiduTitleLinkPattern = regexp.MustCompile(`(?is)<h3[^>]*>.*?<a[^>]+href="([^"]+)"[^>]*>(.*?)</a>.*?</h3>`)
	baiduSnippetPattern   = regexp.MustCompile(`(?is)<span[^>]+class="[^"]*\bcontent-right_8Zs40[^"]*"[^>]*>(.*?)</span>|<div[^>]+class="[^"]*\bc-abstract[^"]*"[^>]*>(.*?)</div>`)
	bingResultPattern     = regexp.MustCompile(`(?is)<li[^>]+class="[^"]*\bb_algo\b[^"]*"[^>]*>(.*?)</li>`)
	bingTitleLinkPattern  = regexp.MustCompile(`(?is)<h2[^>]*>.*?<a[^>]+href="([^"]+)"[^>]*>(.*?)</a>.*?</h2>`)
	bingSnippetPattern    = regexp.MustCompile(`(?is)<div[^>]+class="[^"]*\bb_caption\b[^"]*"[^>]*>.*?<p[^>]*>(.*?)</p>`)
)

type WebSearchTool struct {
	cfg    config.WebConfig // cfg stores the selected search backend and result limits.
	client *http.Client     // client performs bounded HTTP requests to the configured search service.
}

type directSearchProvider struct {
	Backend    string                              // Backend is the output backend label used when this provider returns results.
	Endpoint   string                              // Endpoint is the HTTP search endpoint used by this provider.
	QueryParam string                              // QueryParam is the URL query parameter name that carries the search phrase.
	Parse      func(string, int) []webSearchResult // Parse extracts normalized results from this provider's HTML response.
	IsBlocked  func(string) bool                   // IsBlocked detects challenge, captcha, or anti-bot responses from this provider.
}

type searxngResponse struct {
	Results []searxngResult `json:"results"` // Results contains raw search results returned by SearXNG.
}

type searxngResult struct {
	Title   string  `json:"title"`   // Title is the result title shown by the upstream search engine.
	URL     string  `json:"url"`     // URL is the target page URL returned by the search engine.
	Content string  `json:"content"` // Content is SearXNG's result snippet or summary text.
	Score   float64 `json:"score"`   // Score is SearXNG's optional relevance score.
}

func NewWebSearchTool(cfg config.WebConfig) *WebSearchTool {
	timeout := time.Duration(cfg.RequestTimeoutSeconds) * time.Second
	return &WebSearchTool{
		cfg: cfg,
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

func (t *WebSearchTool) Definition() Definition {
	return Definition{
		Name:        WebSearchToolName,
		Description: "Search the public web. Uses configured SearXNG when available, otherwise a zero-config direct best-effort HTML search backend. Returns result metadata only; use web_fetch to read a selected page.",
		InputSchema: `{"type":"object","properties":{"query":{"type":"string","description":"Search query."},"max_results":{"type":"integer","minimum":1,"description":"Maximum number of results to return. Defaults to the configured limit."}},"required":["query"]}`,
	}
}

func (t *WebSearchTool) Execute(ctx context.Context, call Call) (Result, error) {
	var input struct {
		Query      string `json:"query"`       // Query is the search phrase sent to the configured SearXNG service.
		MaxResults int    `json:"max_results"` // MaxResults optionally caps the number of returned search results.
	}
	if err := json.Unmarshal(call.Arguments, &input); err != nil {
		return Result{}, fmt.Errorf("decode web_search arguments: %w", err)
	}
	if strings.TrimSpace(input.Query) == "" {
		return Result{}, fmt.Errorf("web_search query must not be empty")
	}

	maxResults := normalizePositiveLimit(input.MaxResults, t.cfg.MaxSearchResults)
	response, err := t.search(ctx, input.Query, maxResults)
	if err != nil {
		return Result{Output: renderJSONError(err.Error())}, nil
	}
	return Result{Output: response}, nil
}

func (t *WebSearchTool) search(ctx context.Context, query string, maxResults int) (string, error) {
	switch t.searchBackend() {
	case "searxng":
		return t.searchSearXNG(ctx, query, maxResults)
	case "direct":
		return t.searchDirect(ctx, query, maxResults)
	default:
		return "", fmt.Errorf("unsupported web search backend %q", t.cfg.SearchBackend)
	}
}

func (t *WebSearchTool) searchBackend() string {
	backend := strings.TrimSpace(t.cfg.SearchBackend)
	if backend == "" || backend == "auto" {
		if strings.TrimSpace(t.cfg.SearXNGURL) != "" {
			return "searxng"
		}
		return "direct"
	}
	return backend
}

func (t *WebSearchTool) searchSearXNG(ctx context.Context, query string, maxResults int) (string, error) {
	base, err := url.Parse(strings.TrimSpace(t.cfg.SearXNGURL))
	if err != nil {
		return "", fmt.Errorf("parse web.searxng_url: %w", err)
	}
	if base.Scheme != "http" && base.Scheme != "https" {
		return "", fmt.Errorf("web.searxng_url scheme must be http or https")
	}
	base.Path = strings.TrimRight(base.Path, "/") + "/search"
	params := base.Query()
	params.Set("q", query)
	params.Set("format", "json")
	params.Set("pageno", "1")
	base.RawQuery = params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base.String(), nil)
	if err != nil {
		return "", fmt.Errorf("build web_search request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "happyagent-web-search/1.0")

	resp, err := t.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request SearXNG: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("SearXNG returned HTTP %d", resp.StatusCode)
	}

	var parsed searxngResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", fmt.Errorf("parse SearXNG JSON response: %w", err)
	}

	results := make([]webSearchResult, 0, minInt(maxResults, len(parsed.Results)))
	for _, item := range parsed.Results {
		if len(results) >= maxResults {
			break
		}
		if strings.TrimSpace(item.URL) == "" {
			continue
		}
		results = append(results, webSearchResult{
			Title:    strings.TrimSpace(item.Title),
			URL:      strings.TrimSpace(item.URL),
			Snippet:  strings.TrimSpace(item.Content),
			Position: len(results) + 1,
		})
	}
	return renderSearchOutput("searxng", false, results)
}

func (t *WebSearchTool) searchDirect(ctx context.Context, query string, maxResults int) (string, error) {
	endpoint := strings.TrimSpace(t.cfg.DirectSearchURL)
	if endpoint != "" {
		body, err := t.requestDirectHTML(ctx, endpoint, "wd", query)
		if err != nil {
			return "", err
		}
		results := parseBaiduSearchResults(body, maxResults)
		return renderSearchOutput("baidu_html", true, results)
	}

	return t.searchDirectProviders(ctx, query, maxResults, defaultDirectSearchProviders())
}

func defaultDirectSearchProviders() []directSearchProvider {
	return []directSearchProvider{
		{
			Backend:    "baidu_html",
			Endpoint:   defaultBaiduSearchURL,
			QueryParam: "wd",
			Parse:      parseBaiduSearchResults,
			IsBlocked:  isBaiduChallengeResponse,
		},
		{
			Backend:    "bing_html",
			Endpoint:   defaultBingSearchURL,
			QueryParam: "q",
			Parse:      parseBingSearchResults,
			IsBlocked:  nil,
		},
	}
}

func (t *WebSearchTool) searchDirectProviders(ctx context.Context, query string, maxResults int, providers []directSearchProvider) (string, error) {
	var failures []string
	for _, provider := range providers {
		body, err := t.requestDirectHTML(ctx, provider.Endpoint, provider.QueryParam, query)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", provider.Backend, err))
			continue
		}
		if provider.IsBlocked != nil && provider.IsBlocked(body) {
			failures = append(failures, fmt.Sprintf("%s: blocked by challenge response", provider.Backend))
			continue
		}
		results := provider.Parse(body, maxResults)
		if len(results) == 0 {
			failures = append(failures, fmt.Sprintf("%s: no parseable results", provider.Backend))
			continue
		}
		return renderSearchOutput(provider.Backend, true, results)
	}
	return "", fmt.Errorf("direct search returned no results (%s)", strings.Join(failures, "; "))
}

func (t *WebSearchTool) requestDirectHTML(ctx context.Context, endpoint string, queryParam string, query string) (string, error) {
	base, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("parse direct search url: %w", err)
	}
	if base.Scheme != "http" && base.Scheme != "https" {
		return "", fmt.Errorf("direct search url scheme must be http or https")
	}
	params := base.Query()
	params.Set(queryParam, query)
	base.RawQuery = params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base.String(), nil)
	if err != nil {
		return "", fmt.Errorf("build direct web_search request: %w", err)
	}
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; happyagent-web-search/1.0)")

	resp, err := t.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request direct search: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("direct search returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, directSearchMaxBytes))
	if err != nil {
		return "", fmt.Errorf("read direct search response: %w", err)
	}
	return string(body), nil
}

type webSearchResult struct {
	Title    string `json:"title"`    // Title is the normalized search result title.
	URL      string `json:"url"`      // URL is the normalized search result target URL.
	Snippet  string `json:"snippet"`  // Snippet is the normalized search result summary.
	Position int    `json:"position"` // Position is the 1-based result rank after truncation.
}

func parseBaiduSearchResults(html string, maxResults int) []webSearchResult {
	matches := baiduTitleLinkPattern.FindAllStringSubmatchIndex(html, -1)
	results := make([]webSearchResult, 0, minInt(maxResults, len(matches)))
	for i, match := range matches {
		if len(results) >= maxResults {
			break
		}
		blockEnd := len(html)
		if i+1 < len(matches) {
			blockEnd = matches[i+1][0]
		}
		block := html[match[0]:blockEnd]
		rawURL := strings.TrimSpace(htmlstd.UnescapeString(html[match[2]:match[3]]))
		title := cleanHTMLText(html[match[4]:match[5]])
		if rawURL == "" || title == "" {
			continue
		}
		results = append(results, webSearchResult{
			Title:    title,
			URL:      rawURL,
			Snippet:  extractBaiduSnippet(block),
			Position: len(results) + 1,
		})
	}
	return results
}

func extractBaiduSnippet(block string) string {
	match := baiduSnippetPattern.FindStringSubmatch(block)
	if len(match) == 0 {
		return ""
	}
	for _, value := range match[1:] {
		if strings.TrimSpace(value) != "" {
			return cleanHTMLText(value)
		}
	}
	return ""
}

func isBaiduChallengeResponse(html string) bool {
	return strings.Contains(html, "百度安全验证") || strings.Contains(html, "网络不给力，请稍后重试")
}

func parseBingSearchResults(html string, maxResults int) []webSearchResult {
	matches := bingResultPattern.FindAllStringSubmatch(html, -1)
	results := make([]webSearchResult, 0, minInt(maxResults, len(matches)))
	for _, match := range matches {
		if len(results) >= maxResults {
			break
		}
		block := match[1]
		titleMatch := bingTitleLinkPattern.FindStringSubmatch(block)
		if len(titleMatch) < 3 {
			continue
		}
		rawURL := strings.TrimSpace(htmlstd.UnescapeString(titleMatch[1]))
		title := cleanHTMLText(titleMatch[2])
		targetURL := normalizeBingResultURL(rawURL)
		if targetURL == "" || title == "" {
			continue
		}
		results = append(results, webSearchResult{
			Title:    title,
			URL:      targetURL,
			Snippet:  extractBingSnippet(block),
			Position: len(results) + 1,
		})
	}
	return results
}

func extractBingSnippet(block string) string {
	match := bingSnippetPattern.FindStringSubmatch(block)
	if len(match) < 2 {
		return ""
	}
	return cleanHTMLText(match[1])
}

func normalizeBingResultURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	encodedTarget := parsed.Query().Get("u")
	if encodedTarget == "" || !strings.HasPrefix(encodedTarget, "a1") {
		return rawURL
	}
	decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(encodedTarget, "a1"))
	if err != nil {
		return rawURL
	}
	targetURL := strings.TrimSpace(string(decoded))
	if targetURL == "" {
		return rawURL
	}
	return targetURL
}

func cleanHTMLText(raw string) string {
	text := stripHTMLTags(raw)
	text = htmlstd.UnescapeString(text)
	return normalizeExtractedWhitespace(text)
}

func renderSearchOutput(backend string, bestEffort bool, results []webSearchResult) (string, error) {
	output := struct {
		Success    bool              `json:"success"`     // Success indicates whether the search request completed.
		Backend    string            `json:"backend"`     // Backend names the search backend that produced the result.
		BestEffort bool              `json:"best_effort"` // BestEffort reports whether the backend is HTML scraping without a stable API contract.
		Results    []webSearchResult `json:"results"`     // Results contains normalized search result metadata.
	}{
		Success:    true,
		Backend:    backend,
		BestEffort: bestEffort,
		Results:    results,
	}
	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode web_search output: %w", err)
	}
	return string(data), nil
}

func renderJSONError(message string) string {
	output := struct {
		Success bool   `json:"success"` // Success is false when a tool operation failed.
		Error   string `json:"error"`   // Error contains a recoverable user-facing failure message.
	}{
		Success: false,
		Error:   message,
	}
	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return `{"success":false,"error":"failed to render error"}`
	}
	return string(data)
}

func normalizePositiveLimit(requested int, configured int) int {
	if configured <= 0 {
		configured = 10
	}
	if requested <= 0 || requested > configured {
		return configured
	}
	return requested
}

func minInt(a int, b int) int {
	if a < b {
		return a
	}
	return b
}
