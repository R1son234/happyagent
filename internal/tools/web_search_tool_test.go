package tools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"happyagent/internal/config"
)

func TestWebSearchToolReturnsSearXNGResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("q") != "golang" || r.URL.Query().Get("format") != "json" {
			t.Fatalf("unexpected query: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"title":"Go","url":"https://go.dev","content":"Go home"},{"title":"Docs","url":"https://go.dev/doc","content":"Docs"}]}`))
	}))
	defer server.Close()

	tool := NewWebSearchTool(config.WebConfig{
		SearchBackend:         "searxng",
		SearXNGURL:            server.URL,
		RequestTimeoutSeconds: 5,
		MaxSearchResults:      1,
	})
	result, err := tool.Execute(context.Background(), Call{
		Name:      WebSearchToolName,
		Arguments: []byte(`{"query":"golang","max_results":5}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	var output map[string]any
	if err := json.Unmarshal([]byte(result.Output), &output); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, result.Output)
	}
	results := output["results"].([]any)
	if len(results) != 1 {
		t.Fatalf("results length = %d, want 1: %s", len(results), result.Output)
	}
	first := results[0].(map[string]any)
	if first["url"] != "https://go.dev" || first["position"].(float64) != 1 {
		t.Fatalf("unexpected first result: %#v", first)
	}
	if output["backend"] != "searxng" || output["best_effort"] != false {
		t.Fatalf("unexpected backend metadata: %s", result.Output)
	}
}

func TestWebSearchToolReturnsJSONErrorOnBackendFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer server.Close()

	tool := NewWebSearchTool(config.WebConfig{
		SearchBackend:         "searxng",
		SearXNGURL:            server.URL,
		RequestTimeoutSeconds: 5,
		MaxSearchResults:      5,
	})
	result, err := tool.Execute(context.Background(), Call{
		Name:      WebSearchToolName,
		Arguments: []byte(`{"query":"golang"}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(result.Output, `"success": false`) || !strings.Contains(result.Output, "HTTP 500") {
		t.Fatalf("unexpected output: %s", result.Output)
	}
}

func TestWebSearchToolFallsBackToDirectSearch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("wd") != "阿里云 面经" {
			t.Fatalf("unexpected query: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`
<html><body>
  <div class="result c-container">
    <h3><a href="https://example.com/a">阿里云 &amp; Agent 面经</a></h3>
    <div class="c-abstract">这是一条面经摘要。</div>
  </div></div>
  <div class="result c-container">
    <h3><a href="https://example.com/b">第二条</a></h3>
    <div class="c-abstract">第二条摘要。</div>
  </div></div>
</body></html>`))
	}))
	defer server.Close()

	tool := NewWebSearchTool(config.WebConfig{
		SearchBackend:         "auto",
		DirectSearchURL:       server.URL + "/s",
		RequestTimeoutSeconds: 5,
		MaxSearchResults:      10,
	})
	result, err := tool.Execute(context.Background(), Call{
		Name:      WebSearchToolName,
		Arguments: []byte(`{"query":"阿里云 面经","max_results":1}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	var output map[string]any
	if err := json.Unmarshal([]byte(result.Output), &output); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, result.Output)
	}
	results := output["results"].([]any)
	if len(results) != 1 {
		t.Fatalf("results length = %d, want 1: %s", len(results), result.Output)
	}
	first := results[0].(map[string]any)
	if first["title"] != "阿里云 & Agent 面经" || first["url"] != "https://example.com/a" || first["snippet"] != "这是一条面经摘要。" {
		t.Fatalf("unexpected first result: %#v", first)
	}
	if output["backend"] != "baidu_html" || output["best_effort"] != true {
		t.Fatalf("unexpected backend metadata: %s", result.Output)
	}
}

func TestWebSearchToolParsesDirectSearchAfterLargePrelude(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><body>` + strings.Repeat("x", 600*1024) + `
  <div class="result c-container xpath-log new-pmd">
    <h3 class="t title"><a class="sc-link" href="https://example.com/late">迟到的结果</a></h3>
    <div class="c-abstract">结果在大量样式和脚本之后。</div>
  </div>
</body></html>`))
	}))
	defer server.Close()

	tool := NewWebSearchTool(config.WebConfig{
		SearchBackend:         "direct",
		DirectSearchURL:       server.URL + "/s",
		RequestTimeoutSeconds: 5,
		MaxSearchResults:      10,
	})
	result, err := tool.Execute(context.Background(), Call{
		Name:      WebSearchToolName,
		Arguments: []byte(`{"query":"阿里云 面经","max_results":1}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(result.Output, `"url": "https://example.com/late"`) {
		t.Fatalf("expected late result after large prelude, got: %s", result.Output)
	}
}

func TestWebSearchToolFallsBackToBingWhenBaiduIsBlocked(t *testing.T) {
	baiduServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("wd") != "阿里云 面经" {
			t.Fatalf("unexpected baidu query: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><head><title>百度安全验证</title></head><body>网络不给力，请稍后重试</body></html>`))
	}))
	defer baiduServer.Close()

	targetURL := "https://example.com/bing-result"
	encodedTargetURL := "a1" + base64.RawURLEncoding.EncodeToString([]byte(targetURL))
	bingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("q") != "阿里云 面经" {
			t.Fatalf("unexpected bing query: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`
<html><body><ol id="b_results">
  <li class="b_algo">
    <h2><a href="https://www.bing.com/ck/a?!&amp;&amp;u=` + encodedTargetURL + `&amp;ntb=1">Bing &amp; Agent 面经</a></h2>
    <div class="b_caption"><p>这是一条 Bing 降级搜索结果。</p></div>
  </li>
</ol></body></html>`))
	}))
	defer bingServer.Close()

	tool := NewWebSearchTool(config.WebConfig{
		RequestTimeoutSeconds: 5,
		MaxSearchResults:      10,
	})
	output, err := tool.searchDirectProviders(context.Background(), "阿里云 面经", 5, []directSearchProvider{
		{
			Backend:    "baidu_html",
			Endpoint:   baiduServer.URL + "/s",
			QueryParam: "wd",
			Parse:      parseBaiduSearchResults,
			IsBlocked:  isBaiduChallengeResponse,
		},
		{
			Backend:    "bing_html",
			Endpoint:   bingServer.URL + "/search",
			QueryParam: "q",
			Parse:      parseBingSearchResults,
			IsBlocked:  nil,
		},
	})
	if err != nil {
		t.Fatalf("searchDirectProviders() error = %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, output)
	}
	if parsed["backend"] != "bing_html" {
		t.Fatalf("backend = %v, want bing_html: %s", parsed["backend"], output)
	}
	results := parsed["results"].([]any)
	if len(results) != 1 {
		t.Fatalf("results length = %d, want 1: %s", len(results), output)
	}
	first := results[0].(map[string]any)
	if first["url"] != targetURL || first["title"] != "Bing & Agent 面经" || first["snippet"] != "这是一条 Bing 降级搜索结果。" {
		t.Fatalf("unexpected first result: %#v", first)
	}
}

func TestWebSearchToolRejectsEmptyQuery(t *testing.T) {
	tool := NewWebSearchTool(config.WebConfig{})

	_, err := tool.Execute(context.Background(), Call{
		Name:      WebSearchToolName,
		Arguments: []byte(`{"query":" "}`),
	})
	if err == nil || err.Error() != "web_search query must not be empty" {
		t.Fatalf("unexpected error: %v", err)
	}
}
