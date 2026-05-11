package research

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSearchWeb_Success(t *testing.T) {
	mockHTML := `<!DOCTYPE html>
<html>
<body>
	<div class="result results_links results_links_deep web-result">
		<div class="links_main">
			<a class="result__a" href="/l/?uddg=https%3A%2F%2Fexample.com%2Fpaper1">First Paper</a>
			<div class="result__snippet">This is the first result snippet</div>
		</div>
	</div>
	<div class="result results_links results_links_deep web-result">
		<div class="links_main">
			<a class="result__a" href="/l/?uddg=https%3A%2F%2Fexample.com%2Fpaper2">Second Paper</a>
			<div class="result__snippet">This is the second result snippet</div>
		</div>
	</div>
</body>
</html>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(mockHTML))
	}))
	defer srv.Close()

	ts := &ResearchToolSet{
		client:           &http.Client{},
		searchWebBaseURL: srv.URL,
	}
	ts.searchWebFailures.Store(0)

	input, _ := json.Marshal(map[string]any{"query": "test query", "max_results": 5})
	result, err := ts.handleSearchWeb(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var results []WebSearchResult
	if err := json.Unmarshal([]byte(result), &results); err != nil {
		t.Fatalf("failed to parse results: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	if results[0].Title != "First Paper" {
		t.Errorf("expected first title 'First Paper', got: %s", results[0].Title)
	}
	if results[0].URL != "https://example.com/paper1" {
		t.Errorf("expected cleaned URL, got: %s", results[0].URL)
	}

	// Verify failure counter was reset on success
	if failures := ts.searchWebFailures.Load(); failures != 0 {
		t.Errorf("expected failure counter to be 0 after success, got: %d", failures)
	}
}

func TestSearchWeb_CAPTCHADetection(t *testing.T) {
	mockHTML := `<!DOCTYPE html>
<html>
<body>
	<p>Unfortunately, bots use DuckDuckGo too. Please complete the following challenge</p>
</body>
</html>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(mockHTML))
	}))
	defer srv.Close()

	ts := &ResearchToolSet{
		client:           &http.Client{},
		searchWebBaseURL: srv.URL,
	}
	ts.searchWebFailures.Store(0)

	input, _ := json.Marshal(map[string]any{"query": "test"})
	result, err := ts.handleSearchWeb(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}

	var errResp map[string]any
	if err := json.Unmarshal([]byte(result), &errResp); err != nil {
		t.Fatalf("failed to parse error response: %v", err)
	}

	if errResp["error"] == nil {
		t.Error("expected error field in response")
	}
	errorMsg := errResp["error"].(string)
	if !strings.Contains(errorMsg, "CAPTCHA") {
		t.Errorf("error should mention CAPTCHA, got: %s", errorMsg)
	}

	// Verify failure counter incremented
	if failures := ts.searchWebFailures.Load(); failures != 1 {
		t.Errorf("expected failure counter to be 1, got: %d", failures)
	}
}

func TestSearchWeb_CircuitBreaker(t *testing.T) {
	ts := &ResearchToolSet{
		client:           &http.Client{},
		searchWebBaseURL: "http://unused",
	}
	ts.searchWebFailures.Store(3) // At threshold

	input, _ := json.Marshal(map[string]any{"query": "test"})
	result, err := ts.handleSearchWeb(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}

	var errResp map[string]any
	if err := json.Unmarshal([]byte(result), &errResp); err != nil {
		t.Fatalf("failed to parse error response: %v", err)
	}

	errorMsg := errResp["error"].(string)
	if !strings.Contains(errorMsg, "temporarily disabled") {
		t.Errorf("error should mention circuit breaker, got: %s", errorMsg)
	}
}

func TestSearchWeb_ParseFailure(t *testing.T) {
	mockHTML := `<!DOCTYPE html>
<html>
<body>
	<p>No search results here</p>
</body>
</html>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(mockHTML))
	}))
	defer srv.Close()

	ts := &ResearchToolSet{
		client:           &http.Client{},
		searchWebBaseURL: srv.URL,
	}
	ts.searchWebFailures.Store(0)

	input, _ := json.Marshal(map[string]any{"query": "test"})
	result, err := ts.handleSearchWeb(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}

	var errResp map[string]any
	if err := json.Unmarshal([]byte(result), &errResp); err != nil {
		t.Fatalf("failed to parse error response: %v", err)
	}

	errorMsg := errResp["error"].(string)
	if !strings.Contains(errorMsg, "parse failed") {
		t.Errorf("error should mention parse failure, got: %s", errorMsg)
	}

	// Verify failure counter incremented
	if failures := ts.searchWebFailures.Load(); failures != 1 {
		t.Errorf("expected failure counter to be 1, got: %d", failures)
	}
}

func TestSearchWeb_MaxResults(t *testing.T) {
	// Generate HTML with 15 results
	var htmlBuilder strings.Builder
	htmlBuilder.WriteString(`<!DOCTYPE html><html><body>`)
	for i := 1; i <= 15; i++ {
		htmlBuilder.WriteString(`<div class="result results_links">`)
		htmlBuilder.WriteString(`<a class="result__a" href="/l/?uddg=https://example.com/result`)
		htmlBuilder.WriteString(fmt.Sprintf("%d", i))
		htmlBuilder.WriteString(`">Result `)
		htmlBuilder.WriteString(fmt.Sprintf("%d", i))
		htmlBuilder.WriteString(`</a></div>`)
	}
	htmlBuilder.WriteString(`</body></html>`)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(htmlBuilder.String()))
	}))
	defer srv.Close()

	ts := &ResearchToolSet{
		client:           &http.Client{},
		searchWebBaseURL: srv.URL,
	}
	ts.searchWebFailures.Store(0)

	// Request only 10 results
	input, _ := json.Marshal(map[string]any{"query": "test", "max_results": 10})
	result, err := ts.handleSearchWeb(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var results []WebSearchResult
	if err := json.Unmarshal([]byte(result), &results); err != nil {
		t.Fatalf("failed to parse results: %v", err)
	}

	if len(results) != 10 {
		t.Errorf("expected 10 results (max_results limit), got %d", len(results))
	}
}

func TestSearchWeb_InvalidMaxResults(t *testing.T) {
	mockHTML := `<!DOCTYPE html>
<html>
<body>
	<div class="result results_links">
		<a class="result__a" href="http://example.com">Result</a>
	</div>
</body>
</html>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(mockHTML))
	}))
	defer srv.Close()

	ts := &ResearchToolSet{
		client:           &http.Client{},
		searchWebBaseURL: srv.URL,
	}
	ts.searchWebFailures.Store(0)

	// Test with max_results > 10
	input, _ := json.Marshal(map[string]any{"query": "test", "max_results": 50})
	result, err := ts.handleSearchWeb(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var results []WebSearchResult
	if err := json.Unmarshal([]byte(result), &results); err != nil {
		t.Fatalf("failed to parse results: %v", err)
	}

	// Should be clamped to 10 (though only 1 result in HTML)
	if len(results) > 10 {
		t.Errorf("max_results should be clamped to 10, got %d results", len(results))
	}
}

func TestCleanSearchURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "DuckDuckGo redirect with uddg parameter",
			input:    "/l/?uddg=https%3A%2F%2Fexample.com%2Fpath",
			expected: "https://example.com/path",
		},
		{
			name:     "Already clean URL",
			input:    "https://example.com/path",
			expected: "https://example.com/path",
		},
		{
			name:     "DuckDuckGo redirect with complex URL",
			input:    "/l/?uddg=https%3A%2F%2Farxiv.org%2Fabs%2F1706.03762&rut=abc123",
			expected: "https://arxiv.org/abs/1706.03762",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cleanSearchURL(tt.input)
			if result != tt.expected {
				t.Errorf("cleanSearchURL(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
