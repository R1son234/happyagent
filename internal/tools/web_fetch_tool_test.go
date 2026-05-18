package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"happyagent/internal/config"
)

func TestWebFetchToolExtractsHTMLText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><head><title>Example &amp; Test</title><style>.x{}</style><script>alert(1)</script></head><body><h1>Hello</h1><p>Readable text.</p></body></html>`))
	}))
	defer server.Close()

	tool := NewWebFetchTool(config.WebConfig{
		RequestTimeoutSeconds: 5,
		MaxFetchBytes:         4096,
		AllowPrivateNetworks:  true,
	})
	result, err := tool.Execute(context.Background(), Call{
		Name:      WebFetchToolName,
		Arguments: []byte(`{"url":"` + server.URL + `"}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	var output map[string]any
	if err := json.Unmarshal([]byte(result.Output), &output); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, result.Output)
	}
	if output["title"] != "Example & Test" {
		t.Fatalf("title = %q", output["title"])
	}
	content := output["content"].(string)
	if !strings.Contains(content, "Hello") || !strings.Contains(content, "Readable text.") {
		t.Fatalf("content missing body text: %q", content)
	}
	if strings.Contains(content, "alert") || strings.Contains(content, ".x") {
		t.Fatalf("content included dropped nodes: %q", content)
	}
}

func TestWebFetchToolReturnsTextContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("hello\n\nworld"))
	}))
	defer server.Close()

	tool := NewWebFetchTool(config.WebConfig{
		RequestTimeoutSeconds: 5,
		MaxFetchBytes:         4096,
		AllowPrivateNetworks:  true,
	})
	result, err := tool.Execute(context.Background(), Call{
		Name:      WebFetchToolName,
		Arguments: []byte(`{"url":"` + server.URL + `"}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(result.Output, "hello") || !strings.Contains(result.Output, "world") {
		t.Fatalf("unexpected output: %s", result.Output)
	}
}

func TestWebFetchToolRejectsPrivateURLByDefault(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("internal"))
	}))
	defer server.Close()

	tool := NewWebFetchTool(config.WebConfig{
		RequestTimeoutSeconds: 5,
		MaxFetchBytes:         4096,
	})
	result, err := tool.Execute(context.Background(), Call{
		Name:      WebFetchToolName,
		Arguments: []byte(`{"url":"` + server.URL + `"}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(result.Output, `"success": false`) || !strings.Contains(result.Output, "private") {
		t.Fatalf("unexpected output: %s", result.Output)
	}
}

func TestWebFetchToolRejectsBinaryContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte{0x89, 0x50, 0x4e, 0x47})
	}))
	defer server.Close()

	tool := NewWebFetchTool(config.WebConfig{
		RequestTimeoutSeconds: 5,
		MaxFetchBytes:         4096,
		AllowPrivateNetworks:  true,
	})
	result, err := tool.Execute(context.Background(), Call{
		Name:      WebFetchToolName,
		Arguments: []byte(`{"url":"` + server.URL + `"}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(result.Output, "unsupported content type") {
		t.Fatalf("unexpected output: %s", result.Output)
	}
}

func TestWebFetchToolTruncatesLongContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(strings.Repeat("a", 3000)))
	}))
	defer server.Close()

	tool := NewWebFetchTool(config.WebConfig{
		RequestTimeoutSeconds: 5,
		MaxFetchBytes:         2048,
		AllowPrivateNetworks:  true,
	})
	result, err := tool.Execute(context.Background(), Call{
		Name:      WebFetchToolName,
		Arguments: []byte(`{"url":"` + server.URL + `"}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(result.Output, `"truncated": true`) {
		t.Fatalf("expected truncated output: %s", result.Output)
	}
}

func TestWebFetchToolAllowsLocalProxyForPublicTarget(t *testing.T) {
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.String() != "http://93.184.216.34/article" {
			t.Fatalf("proxy saw unexpected URL: %s", r.URL.String())
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><head><title>Via Proxy</title></head><body><p>Fetched through local proxy.</p></body></html>`))
	}))
	defer proxy.Close()

	tool := NewWebFetchTool(config.WebConfig{
		RequestTimeoutSeconds: 5,
		MaxFetchBytes:         4096,
	})
	proxyURL, err := url.Parse(proxy.URL)
	if err != nil {
		t.Fatalf("parse proxy URL: %v", err)
	}
	transport, ok := tool.client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport type = %T", tool.client.Transport)
	}
	transport.Proxy = http.ProxyURL(proxyURL)

	result, err := tool.Execute(context.Background(), Call{
		Name:      WebFetchToolName,
		Arguments: []byte(`{"url":"http://93.184.216.34/article"}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(result.Output, `"success": true`) || !strings.Contains(result.Output, "Fetched through local proxy.") {
		t.Fatalf("unexpected output: %s", result.Output)
	}
}
