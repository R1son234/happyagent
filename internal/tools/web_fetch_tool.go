package tools

import (
	"context"
	"encoding/json"
	"fmt"
	htmlstd "html"
	"io"
	"mime"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode"

	"happyagent/internal/config"
)

const WebFetchToolName = "web_fetch"

var (
	htmlTitlePattern        = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	htmlTagPattern          = regexp.MustCompile(`(?s)<[^>]+>`)
	htmlBlockPattern        = regexp.MustCompile(`(?i)</?(p|div|section|article|main|header|footer|aside|nav|br|li|ul|ol|h[1-6]|tr|table|blockquote)[^>]*>`)
	htmlDroppedNodePatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`),
		regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`),
		regexp.MustCompile(`(?is)<noscript[^>]*>.*?</noscript>`),
		regexp.MustCompile(`(?is)<svg[^>]*>.*?</svg>`),
		regexp.MustCompile(`(?is)<canvas[^>]*>.*?</canvas>`),
		regexp.MustCompile(`(?is)<iframe[^>]*>.*?</iframe>`),
	}
)

type WebFetchTool struct {
	cfg    config.WebConfig // cfg stores fetch limits and URL safety policy.
	policy webURLPolicy     // policy validates URLs before requests and after redirects.
	client *http.Client     // client performs bounded HTTP requests through a safety-aware transport.
}

func NewWebFetchTool(cfg config.WebConfig) *WebFetchTool {
	policy := newWebURLPolicy(cfg)
	timeout := time.Duration(cfg.RequestTimeoutSeconds) * time.Second
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
	}
	client := &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return fmt.Errorf("stopped after 5 redirects")
		}
		if _, err := policy.validatePublicHTTPURL(req.Context(), req.URL.String()); err != nil {
			return err
		}
		return nil
	}
	return &WebFetchTool{cfg: cfg, policy: policy, client: client}
}

func (t *WebFetchTool) Definition() Definition {
	return Definition{
		Name:        WebFetchToolName,
		Description: "Fetch a public HTTP or HTTPS URL and return a bounded readable text preview. Private/internal network URLs and secret-like URLs are blocked by default.",
		InputSchema: `{"type":"object","properties":{"url":{"type":"string","description":"Public http or https URL to fetch."},"max_bytes":{"type":"integer","minimum":1024,"description":"Optional maximum bytes of extracted text to return. Defaults to the configured limit."}},"required":["url"]}`,
	}
}

func (t *WebFetchTool) Execute(ctx context.Context, call Call) (Result, error) {
	var input struct {
		URL      string `json:"url"`       // URL is the public page address to fetch.
		MaxBytes int    `json:"max_bytes"` // MaxBytes optionally caps the returned extracted content.
	}
	if err := json.Unmarshal(call.Arguments, &input); err != nil {
		return Result{}, fmt.Errorf("decode web_fetch arguments: %w", err)
	}

	output, err := t.fetch(ctx, input.URL, normalizeFetchLimit(input.MaxBytes, t.cfg.MaxFetchBytes))
	if err != nil {
		return Result{Output: renderJSONError(err.Error())}, nil
	}
	return Result{Output: output}, nil
}

func (t *WebFetchTool) fetch(ctx context.Context, rawURL string, maxBytes int) (string, error) {
	parsed, err := t.policy.validatePublicHTTPURL(ctx, rawURL)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return "", fmt.Errorf("build web_fetch request: %w", err)
	}
	req.Header.Set("Accept", "text/html, text/plain, application/json, application/xml, text/xml;q=0.9, */*;q=0.1")
	req.Header.Set("User-Agent", "happyagent-web-fetch/1.0")

	resp, err := t.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request url: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("url returned HTTP %d", resp.StatusCode)
	}
	if _, err := t.policy.validatePublicHTTPURL(ctx, resp.Request.URL.String()); err != nil {
		return "", fmt.Errorf("redirect target rejected: %w", err)
	}

	body, responseTruncated, err := readLimited(resp.Body, maxBytes+1)
	if err != nil {
		return "", fmt.Errorf("read response body: %w", err)
	}
	if responseTruncated {
		body = body[:maxBytes]
	}

	contentType := resp.Header.Get("Content-Type")
	mediaType, _, _ := mime.ParseMediaType(contentType)
	title, content, err := extractWebContent(mediaType, string(body))
	if err != nil {
		return "", err
	}
	content, outputTruncated := truncateStringBytes(content, maxBytes)

	output := struct {
		Success     bool   `json:"success"`      // Success indicates whether the fetch completed.
		URL         string `json:"url"`          // URL is the original requested URL.
		FinalURL    string `json:"final_url"`    // FinalURL is the URL reached after redirects.
		Title       string `json:"title"`        // Title is the extracted page title when available.
		ContentType string `json:"content_type"` // ContentType is the response Content-Type header value.
		Content     string `json:"content"`      // Content is the bounded readable text preview.
		Truncated   bool   `json:"truncated"`    // Truncated reports whether response or output content was cut.
	}{
		Success:     true,
		URL:         parsed.String(),
		FinalURL:    resp.Request.URL.String(),
		Title:       title,
		ContentType: contentType,
		Content:     content,
		Truncated:   responseTruncated || outputTruncated,
	}
	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode web_fetch output: %w", err)
	}
	return string(data), nil
}

func normalizeFetchLimit(requested int, configured int) int {
	if configured <= 0 {
		configured = 64 * 1024
	}
	if requested <= 0 || requested > configured {
		return configured
	}
	if requested < 1024 {
		return 1024
	}
	return requested
}

func readLimited(reader io.Reader, limit int) ([]byte, bool, error) {
	data, err := io.ReadAll(io.LimitReader(reader, int64(limit)))
	if err != nil {
		return nil, false, err
	}
	if len(data) >= limit {
		return data, true, nil
	}
	return data, false, nil
}

func extractWebContent(mediaType string, body string) (string, string, error) {
	switch {
	case mediaType == "text/html" || mediaType == "":
		return extractHTMLText(body)
	case strings.HasPrefix(mediaType, "text/"),
		mediaType == "application/json",
		mediaType == "application/xml",
		mediaType == "text/xml":
		return "", normalizeExtractedWhitespace(body), nil
	default:
		return "", "", fmt.Errorf("unsupported content type %q", mediaType)
	}
}

func extractHTMLText(raw string) (string, string, error) {
	title := ""
	if match := htmlTitlePattern.FindStringSubmatch(raw); len(match) > 1 {
		title = normalizeExtractedWhitespace(htmlstd.UnescapeString(stripHTMLTags(match[1])))
	}
	cleaned := dropHTMLNodes(raw)
	cleaned = htmlBlockPattern.ReplaceAllString(cleaned, "\n")
	cleaned = stripHTMLTags(cleaned)
	cleaned = htmlstd.UnescapeString(cleaned)
	return title, normalizeExtractedWhitespace(cleaned), nil
}

func dropHTMLNodes(raw string) string {
	cleaned := raw
	for _, pattern := range htmlDroppedNodePatterns {
		cleaned = pattern.ReplaceAllString(cleaned, " ")
	}
	return cleaned
}

func stripHTMLTags(raw string) string {
	return htmlTagPattern.ReplaceAllString(raw, " ")
}

func normalizeExtractedWhitespace(raw string) string {
	var builder strings.Builder
	previousSpace := false
	previousNewline := false
	for _, r := range raw {
		if r == '\r' {
			continue
		}
		if r == '\n' {
			if !previousNewline {
				builder.WriteByte('\n')
			}
			previousSpace = false
			previousNewline = true
			continue
		}
		if unicode.IsSpace(r) {
			if !previousSpace && !previousNewline {
				builder.WriteByte(' ')
			}
			previousSpace = true
			continue
		}
		builder.WriteRune(r)
		previousSpace = false
		previousNewline = false
	}
	lines := strings.Split(builder.String(), "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

func truncateStringBytes(value string, maxBytes int) (string, bool) {
	if len(value) <= maxBytes {
		return value, false
	}
	if maxBytes <= 0 {
		return "", true
	}
	cut := maxBytes
	for cut > 0 && !isUTF8StartByte(value[cut]) {
		cut--
	}
	if cut <= 0 {
		return "", true
	}
	return value[:cut], true
}

func isUTF8StartByte(b byte) bool {
	return (b & 0xC0) != 0x80
}
