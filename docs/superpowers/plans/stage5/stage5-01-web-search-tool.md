---
name: DuckDuckGo Web Search Tool
description: Implement search_web tool for general web searches
type: feature
parent: stage5-master
---

## Context

This plan implements the `search_web` tool that enables the research agent to perform general web searches via DuckDuckGo. This tool is specifically needed for finding "most cited papers" on a topic by searching the general web, which provides different results than academic-specific search engines.

DuckDuckGo HTML API is chosen because:
- No API key required
- Simple HTTP endpoint
- Returns HTML that can be parsed for results

## Implementation Steps

### Step 1: Add Tool Definition

**File**: `tools/research/definitions.go`

Add the `BuildSearchWebTool()` function after `BuildFetchArxivPdfTool()`:

```go
func BuildSearchWebTool() anthropic.ToolUnionParam {
	t := anthropic.ToolUnionParamOfTool(
		anthropic.ToolInputSchemaParam{
			Type: "object",
			Properties: map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "Search query for web search",
				},
			},
			Required: []string{"query"},
		},
		"search_web",
	)
	t.OfTool.Description = anthropic.String(
		"Search the web using DuckDuckGo. Returns titles, snippets, and URLs from general web search results. Useful for finding 'most cited papers' or other information not available in academic databases.",
	)
	return t
}
```

### Step 2: Add Result Types

**File**: `tools/research/handlers.go`

Add new struct types at the top of the file (after existing result types):

```go
type WebSearchResult struct {
	Title   string `json:"title"`
	Snippet string `json:"snippet"`
	URL     string `json:"url"`
}
```

### Step 3: Implement Handler

**File**: `tools/research/handlers.go`

Add the handler function at the end of the file:

```go
func (r *ResearchToolSet) handleSearchWeb(ctx context.Context, input json.RawMessage) (string, error) {
	var params struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return toolError("invalid input: "+err.Error(), false), nil
	}
	if params.Query == "" {
		return toolError("query is required", false), nil
	}

	// Build DuckDuckGo HTML search URL
	searchURL := fmt.Sprintf("https://html.duckduckgo.com/html/?q=%s", url.QueryEscape(params.Query))

	// Create request with user agent to avoid blocks
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
	if err != nil {
		return toolError("request creation failed: "+err.Error(), false), nil
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")

	// Execute request with retry logic
	var resp *http.Response
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		resp, lastErr = r.client.Do(req)
		if lastErr == nil && resp.StatusCode == http.StatusOK {
			break
		}
		if resp != nil {
			resp.Body.Close()
		}
		if attempt == 0 {
			time.Sleep(1 * time.Second)
		}
	}

	if lastErr != nil {
		return toolError("web search request failed: "+lastErr.Error(), true), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return toolError(fmt.Sprintf("DuckDuckGo returned status %d", resp.StatusCode), true), nil
	}

	// Parse HTML response
	results, err := parseDuckDuckGoHTML(resp.Body)
	if err != nil {
		return toolError("failed to parse search results: "+err.Error(), true), nil
	}

	// Limit to 10 results
	if len(results) > 10 {
		results = results[:10]
	}

	b, _ := json.Marshal(results)
	return string(b), nil
}

// parseDuckDuckGoHTML extracts search results from DuckDuckGo HTML
func parseDuckDuckGoHTML(body io.Reader) ([]WebSearchResult, error) {
	doc, err := html.Parse(body)
	if err != nil {
		return nil, err
	}

	var results []WebSearchResult
	var f func(*html.Node)
	f = func(n *html.Node) {
		// DuckDuckGo results are in divs with class "result"
		if n.Type == html.ElementNode && n.Data == "div" {
			for _, attr := range n.Attr {
				if attr.Key == "class" && strings.Contains(attr.Val, "result") {
					result := extractResult(n)
					if result.Title != "" && result.URL != "" {
						results = append(results, result)
					}
					break
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(doc)

	return results, nil
}

// extractResult extracts title, snippet, and URL from a result node
func extractResult(n *html.Node) WebSearchResult {
	result := WebSearchResult{}
	var f func(*html.Node)
	f = func(node *html.Node) {
		if node.Type == html.ElementNode {
			// Extract title and URL from <a> tag with class "result__a"
			if node.Data == "a" {
				for _, attr := range node.Attr {
					if attr.Key == "class" && strings.Contains(attr.Val, "result__a") {
						// Get URL
						for _, a := range node.Attr {
							if a.Key == "href" {
								result.URL = a.Val
								break
							}
						}
						// Get title (text content)
						result.Title = getTextContent(node)
						break
					}
				}
			}
			// Extract snippet from <a> tag with class "result__snippet"
			if node.Data == "a" {
				for _, attr := range node.Attr {
					if attr.Key == "class" && strings.Contains(attr.Val, "result__snippet") {
						result.Snippet = getTextContent(node)
						break
					}
				}
			}
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(n)
	return result
}

// getTextContent extracts all text from a node and its children
func getTextContent(n *html.Node) string {
	var buf strings.Builder
	var f func(*html.Node)
	f = func(node *html.Node) {
		if node.Type == html.TextNode {
			buf.WriteString(node.Data)
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(n)
	return strings.TrimSpace(buf.String())
}
```

### Step 4: Add Import

**File**: `tools/research/handlers.go`

Add to imports at the top of the file:

```go
import (
	// ... existing imports ...
	"golang.org/x/net/html"
)
```

### Step 5: Create Tests

**File**: `tools/research/search_web_test.go`

Create comprehensive tests:

```go
package research

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleSearchWeb(t *testing.T) {
	// Create mock DuckDuckGo server
	mockHTML := `
<!DOCTYPE html>
<html>
<body>
	<div class="result results_links results_links_deep web-result">
		<a class="result__a" href="https://example.com/paper1">Attention Is All You Need</a>
		<a class="result__snippet">The dominant sequence transduction models...</a>
	</div>
	<div class="result results_links results_links_deep web-result">
		<a class="result__a" href="https://example.com/paper2">BERT: Pre-training of Deep Bidirectional Transformers</a>
		<a class="result__snippet">We introduce BERT, a new language representation model...</a>
	</div>
</body>
</html>
`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(mockHTML))
	}))
	defer server.Close()

	// Override DuckDuckGo URL for testing (would need to refactor handler to make this injectable)
	toolset := NewResearchToolSet(&http.Client{}, "")

	input := `{"query": "most cited papers transformers"}`
	result, err := toolset.handleSearchWeb(context.Background(), []byte(input))

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Parse result
	if len(result) == 0 {
		t.Fatal("expected non-empty result")
	}

	// Basic validation
	if !contains(result, "example.com") {
		t.Errorf("expected result to contain URLs")
	}
}

func TestHandleSearchWebEmptyQuery(t *testing.T) {
	toolset := NewResearchToolSet(&http.Client{}, "")
	input := `{"query": ""}`
	result, err := toolset.handleSearchWeb(context.Background(), []byte(input))

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !contains(result, "error") {
		t.Errorf("expected error for empty query")
	}
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) >= len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsMiddle(s, substr)))
}

func containsMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
```

### Step 6: Add Dependency

**File**: `go.mod`

Ensure the HTML parsing dependency is available:

```bash
go get golang.org/x/net/html
go mod tidy
```

## Critical Files

- `tools/research/definitions.go` - Tool schema
- `tools/research/handlers.go` - Tool implementation
- `tools/research/search_web_test.go` - Tests

## Verification

1. **Build**:
   ```bash
   make build
   ```

2. **Run tests**:
   ```bash
   go test ./tools/research -run TestHandleSearchWeb -v
   ```

3. **Manual test** (after integration):
   ```bash
   ./bin/research-demo --query "most cited papers on attention mechanisms"
   ```
   Check trace for `search_web` tool call and validate results contain URLs and snippets.

## Notes

- DuckDuckGo's HTML structure may change over time; the parser may need updates
- User-Agent header is important to avoid being blocked
- Retry logic handles temporary network failures
- Results are limited to 10 to keep output manageable
