# arXiv Text Extraction Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace PDF-based arXiv text extraction with HTML-based extraction for better quality and simpler codebase

**Architecture:** Clean replacement of `fetch_arxiv_pdf` with `fetch_arxiv_text`. Reuse go-readability library (same as fetch_webpage_content). Remove all PDF extraction code and dependencies. Simplified output schema (arxiv_id, text_content, truncated, error only).

**Tech Stack:** Go, go-shiori/go-readability library, arXiv HTML endpoint

---

## File Structure

**Files to modify:**
- `tools/research/definitions.go` — Add ArxivTextResult struct, BuildFetchArxivTextTool function
- `tools/research/handlers.go` — Add handleFetchArxivText function, delete PDF functions
- `tools/research/research.go` — Update tool registration
- `tools/research/estimators.go` — Add fetch_arxiv_text estimator
- `go.mod` — Remove PDF dependency

**Files to create:**
- `tools/research/fetch_arxiv_text_test.go` — All HTML extraction tests

**Files to delete:**
- `tools/research/fetch_arxiv_pdf_test.go` — PDF-specific tests (replaced)

---

## Task 1: Add ArxivTextResult struct and tool definition

**Files:**
- Modify: `tools/research/definitions.go:68-91`
- Test: Manual verification (no test needed for struct definition)

- [ ] **Step 1: Add ArxivTextResult struct**

Add this struct after the existing tool definitions (around line 66, after BuildSearchOpenAlexTool):

```go
type ArxivTextResult struct {
	ArxivID     string `json:"arxiv_id"`
	TextContent string `json:"text_content"`
	Truncated   bool   `json:"truncated"`
	Error       string `json:"error,omitempty"`
}
```

- [ ] **Step 2: Add BuildFetchArxivTextTool function**

Add this function after the ArxivTextResult struct:

```go
func BuildFetchArxivTextTool() anthropic.ToolUnionParam {
	t := anthropic.ToolUnionParamOfTool(
		anthropic.ToolInputSchemaParam{
			Type: "object",
			Properties: map[string]any{
				"arxiv_id": map[string]any{
					"type":        "string",
					"description": "arXiv identifier. Formats: 2301.00001, arXiv:2301.00001, 2301.00001v2 (new), or astro-ph/9901234 (old)",
				},
				"max_length": map[string]any{
					"type":        "integer",
					"description": "Maximum text content length in characters (default 25000, max 25000)",
					"default":     25000,
				},
			},
			Required: []string{"arxiv_id"},
		},
		"fetch_arxiv_text",
	)
	t.OfTool.Description = anthropic.String(
		"Fetches and extracts text content from an arXiv preprint HTML page. Returns cleaned article text extracted from arXiv's HTML rendering. Text is truncated to max_length (default/max 25000 characters). Not all papers have HTML versions - older papers or recent submissions may return an error. Rate limit: 1 request per 3 seconds per arXiv TOS.",
	)
	return t
}
```

- [ ] **Step 3: Verify code compiles**

```bash
cd tools/research
go build
```

Expected: No compilation errors

- [ ] **Step 4: Commit**

```bash
git add tools/research/definitions.go
git commit -m "feat: add ArxivTextResult struct and BuildFetchArxivTextTool definition"
```

---

## Task 2: Implement handleFetchArxivText handler

**Files:**
- Modify: `tools/research/handlers.go` (add new function after normalizeArxivID, around line 577)
- Test: Unit tests (will write in Task 4)

- [ ] **Step 1: Add handleFetchArxivText function**

Add this function after the `normalizeArxivID` function (around line 577):

```go
func (r *ResearchToolSet) handleFetchArxivText(ctx context.Context, input json.RawMessage) (string, error) {
	// Parse and validate input
	var params struct {
		ArxivID   string `json:"arxiv_id"`
		MaxLength int    `json:"max_length"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return toolError("invalid input: "+err.Error(), false), nil
	}
	
	// Validate arxiv_id required
	if params.ArxivID == "" {
		return toolError("arxiv_id is required", false), nil
	}
	
	// Apply max_length constraints (default 25000, max 25000)
	if params.MaxLength <= 0 {
		params.MaxLength = 25000
	}
	if params.MaxLength > 25000 {
		params.MaxLength = 25000
	}
	
	// Normalize arXiv ID (reuse existing function)
	normalized, _, err := normalizeArxivID(params.ArxivID)
	if err != nil {
		return toolError("invalid arXiv ID format: "+err.Error(), false), nil
	}
	
	// Construct HTML URL
	htmlURL := fmt.Sprintf("https://arxiv.org/html/%s", normalized)
	
	// Rate limit (reuse existing rate limiter)
	r.arxivRateLimiter.Wait()
	
	// Fetch HTML
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, htmlURL, nil)
	if err != nil {
		return toolError("failed to create request: "+err.Error(), false), nil
	}
	
	resp, err := r.client.Do(req)
	if err != nil {
		return toolError("request failed: "+err.Error(), true), nil
	}
	defer resp.Body.Close()
	
	// Check HTML availability
	if resp.StatusCode == http.StatusNotFound {
		result := ArxivTextResult{
			ArxivID:     normalized,
			TextContent: "",
			Truncated:   false,
			Error:       "HTML version not available for this paper",
		}
		b, _ := json.Marshal(result)
		return string(b), nil
	}
	
	// Handle other error status codes
	if resp.StatusCode != http.StatusOK {
		return toolError(fmt.Sprintf("arXiv returned status %d", resp.StatusCode), true), nil
	}
	
	// Parse URL for readability
	parsedURL, err := url.Parse(htmlURL)
	if err != nil {
		return toolError("failed to parse URL: "+err.Error(), false), nil
	}
	
	// Extract text using go-readability
	article, err := readability.FromReader(resp.Body, parsedURL)
	if err != nil {
		return toolError("failed to extract content: "+err.Error(), true), nil
	}
	
	// Truncate to max_length
	textContent := article.TextContent
	truncated := false
	if len(textContent) > params.MaxLength {
		textContent = textContent[:params.MaxLength]
		truncated = true
	}
	
	// Return result
	result := ArxivTextResult{
		ArxivID:     normalized,
		TextContent: textContent,
		Truncated:   truncated,
	}
	b, _ := json.Marshal(result)
	return string(b), nil
}
```

- [ ] **Step 2: Verify code compiles**

```bash
cd tools/research
go build
```

Expected: No compilation errors

- [ ] **Step 3: Commit**

```bash
git add tools/research/handlers.go
git commit -m "feat: implement handleFetchArxivText with HTML extraction"
```

---

## Task 3: Update tool registry

**Files:**
- Modify: `tools/research/research.go:77-85`
- Test: Manual verification

- [ ] **Step 1: Update Register method**

In `tools/research/research.go`, replace line 80:

```go
reg.Register("fetch_arxiv_pdf", BuildFetchArxivPdfTool(), r.handleFetchArxivPdf)
```

with:

```go
reg.Register("fetch_arxiv_text", BuildFetchArxivTextTool(), r.handleFetchArxivText)
```

- [ ] **Step 2: Verify code compiles**

```bash
cd tools/research
go build
```

Expected: No compilation errors

- [ ] **Step 3: Commit**

```bash
git add tools/research/research.go
git commit -m "feat: register fetch_arxiv_text tool (replace fetch_arxiv_pdf)"
```

---

## Task 4: Write test for successful HTML extraction

**Files:**
- Create: `tools/research/fetch_arxiv_text_test.go`
- Test: `go test -v -run TestFetchArxivText_Success`

- [ ] **Step 1: Write failing test**

Create `tools/research/fetch_arxiv_text_test.go`:

```go
package research

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

func TestFetchArxivText_Success(t *testing.T) {
	// Mock HTML content
	htmlContent := `
		<!DOCTYPE html>
		<html>
		<head><title>Test Paper</title></head>
		<body>
			<article>
				<h1>Deep Learning for Computer Vision</h1>
				<p>This paper presents a novel approach to image classification using deep neural networks.</p>
				<p>Our experiments show significant improvements over existing methods.</p>
			</article>
		</body>
		</html>
	`
	
	mockClient := &mockHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			// Verify URL format
			expectedURL := "https://arxiv.org/html/2301.00001"
			if req.URL.String() != expectedURL {
				t.Errorf("unexpected URL: got %s, want %s", req.URL.String(), expectedURL)
			}
			
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString(htmlContent)),
				Header:     make(http.Header),
			}, nil
		},
	}
	
	ts := newResearchToolSetWithBase(mockClient, "")
	
	input := `{"arxiv_id": "2301.00001", "max_length": 25000}`
	result, err := ts.handleFetchArxivText(context.Background(), json.RawMessage(input))
	
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	
	var parsed ArxivTextResult
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	
	// Verify result fields
	if parsed.ArxivID != "2301.00001" {
		t.Errorf("arxiv_id: got %s, want 2301.00001", parsed.ArxivID)
	}
	
	if parsed.TextContent == "" {
		t.Error("text_content is empty")
	}
	
	if parsed.Truncated {
		t.Error("truncated should be false for short content")
	}
	
	if parsed.Error != "" {
		t.Errorf("unexpected error field: %s", parsed.Error)
	}
	
	// Verify content extraction worked
	if !contains(parsed.TextContent, "Deep Learning") || !contains(parsed.TextContent, "Computer Vision") {
		t.Errorf("text_content missing expected keywords: %s", parsed.TextContent)
	}
}

// Helper function to check substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || 
		(len(s) > 0 && len(substr) > 0 && findInString(s, substr)))
}

func findInString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test -v -run TestFetchArxivText_Success ./tools/research
```

Expected: FAIL (mockHTTPClient not defined yet)

- [ ] **Step 3: Add mockHTTPClient**

Add to the top of `fetch_arxiv_text_test.go` after imports:

```go
type mockHTTPClient struct {
	doFunc func(req *http.Request) (*http.Response, error)
}

func (m *mockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	if m.doFunc != nil {
		return m.doFunc(req)
	}
	return nil, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test -v -run TestFetchArxivText_Success ./tools/research
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add tools/research/fetch_arxiv_text_test.go
git commit -m "test: add TestFetchArxivText_Success"
```

---

## Task 5: Write test for truncation behavior

**Files:**
- Modify: `tools/research/fetch_arxiv_text_test.go`
- Test: `go test -v -run TestFetchArxivText_Truncation`

- [ ] **Step 1: Write failing test**

Add to `fetch_arxiv_text_test.go`:

```go
func TestFetchArxivText_Truncation(t *testing.T) {
	// Generate content > 25000 characters
	longText := ""
	for i := 0; i < 26000; i++ {
		longText += "x"
	}
	
	htmlContent := `
		<!DOCTYPE html>
		<html>
		<body><article><p>` + longText + `</p></article></body>
		</html>
	`
	
	mockClient := &mockHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString(htmlContent)),
				Header:     make(http.Header),
			}, nil
		},
	}
	
	ts := newResearchToolSetWithBase(mockClient, "")
	
	input := `{"arxiv_id": "2301.00001"}`
	result, err := ts.handleFetchArxivText(context.Background(), json.RawMessage(input))
	
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	
	var parsed ArxivTextResult
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	
	// Verify truncation
	if !parsed.Truncated {
		t.Error("truncated should be true for content > 25000 chars")
	}
	
	if len(parsed.TextContent) != 25000 {
		t.Errorf("text_content length: got %d, want 25000", len(parsed.TextContent))
	}
}
```

- [ ] **Step 2: Run test to verify it passes**

```bash
go test -v -run TestFetchArxivText_Truncation ./tools/research
```

Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add tools/research/fetch_arxiv_text_test.go
git commit -m "test: add TestFetchArxivText_Truncation"
```

---

## Task 6: Write test for custom max_length

**Files:**
- Modify: `tools/research/fetch_arxiv_text_test.go`
- Test: `go test -v -run TestFetchArxivText_CustomMaxLength`

- [ ] **Step 1: Write failing test**

Add to `fetch_arxiv_text_test.go`:

```go
func TestFetchArxivText_CustomMaxLength(t *testing.T) {
	// Generate content > 1000 characters
	longText := ""
	for i := 0; i < 1500; i++ {
		longText += "x"
	}
	
	htmlContent := `
		<!DOCTYPE html>
		<html>
		<body><article><p>` + longText + `</p></article></body>
		</html>
	`
	
	mockClient := &mockHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString(htmlContent)),
				Header:     make(http.Header),
			}, nil
		},
	}
	
	ts := newResearchToolSetWithBase(mockClient, "")
	
	input := `{"arxiv_id": "2301.00001", "max_length": 1000}`
	result, err := ts.handleFetchArxivText(context.Background(), json.RawMessage(input))
	
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	
	var parsed ArxivTextResult
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	
	// Verify custom max_length applied
	if !parsed.Truncated {
		t.Error("truncated should be true for content > 1000 chars")
	}
	
	if len(parsed.TextContent) != 1000 {
		t.Errorf("text_content length: got %d, want 1000", len(parsed.TextContent))
	}
}
```

- [ ] **Step 2: Run test to verify it passes**

```bash
go test -v -run TestFetchArxivText_CustomMaxLength ./tools/research
```

Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add tools/research/fetch_arxiv_text_test.go
git commit -m "test: add TestFetchArxivText_CustomMaxLength"
```

---

## Task 7: Write test for HTML not available (404)

**Files:**
- Modify: `tools/research/fetch_arxiv_text_test.go`
- Test: `go test -v -run TestFetchArxivText_HTMLNotAvailable`

- [ ] **Step 1: Write failing test**

Add to `fetch_arxiv_text_test.go`:

```go
func TestFetchArxivText_HTMLNotAvailable(t *testing.T) {
	mockClient := &mockHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Body:       io.NopCloser(bytes.NewBufferString("Not Found")),
				Header:     make(http.Header),
			}, nil
		},
	}
	
	ts := newResearchToolSetWithBase(mockClient, "")
	
	input := `{"arxiv_id": "astro-ph/9901234"}`
	result, err := ts.handleFetchArxivText(context.Background(), json.RawMessage(input))
	
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	
	var parsed ArxivTextResult
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	
	// Verify error message
	expectedError := "HTML version not available for this paper"
	if parsed.Error != expectedError {
		t.Errorf("error: got %s, want %s", parsed.Error, expectedError)
	}
	
	// Verify arxiv_id still present
	if parsed.ArxivID != "astro-ph/9901234" {
		t.Errorf("arxiv_id: got %s, want astro-ph/9901234", parsed.ArxivID)
	}
	
	// Verify content is empty
	if parsed.TextContent != "" {
		t.Error("text_content should be empty for 404")
	}
	
	if parsed.Truncated {
		t.Error("truncated should be false for 404")
	}
}
```

- [ ] **Step 2: Run test to verify it passes**

```bash
go test -v -run TestFetchArxivText_HTMLNotAvailable ./tools/research
```

Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add tools/research/fetch_arxiv_text_test.go
git commit -m "test: add TestFetchArxivText_HTMLNotAvailable"
```

---

## Task 8: Write test for invalid arXiv ID

**Files:**
- Modify: `tools/research/fetch_arxiv_text_test.go`
- Test: `go test -v -run TestFetchArxivText_InvalidArxivID`

- [ ] **Step 1: Write failing test**

Add to `fetch_arxiv_text_test.go`:

```go
func TestFetchArxivText_InvalidArxivID(t *testing.T) {
	mockClient := &mockHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			t.Fatal("should not make HTTP request for invalid ID")
			return nil, nil
		},
	}
	
	ts := newResearchToolSetWithBase(mockClient, "")
	
	input := `{"arxiv_id": "invalid-id-format"}`
	result, err := ts.handleFetchArxivText(context.Background(), json.RawMessage(input))
	
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	
	// Parse as error response
	var errResp map[string]any
	if err := json.Unmarshal([]byte(result), &errResp); err != nil {
		t.Fatalf("failed to parse error response: %v", err)
	}
	
	// Verify error message
	errMsg, ok := errResp["error"].(string)
	if !ok {
		t.Fatal("error field missing or not a string")
	}
	
	if !contains(errMsg, "invalid arXiv ID format") {
		t.Errorf("error message should mention invalid format: %s", errMsg)
	}
	
	// Verify non-recoverable
	recoverable, ok := errResp["recoverable"].(bool)
	if !ok {
		t.Fatal("recoverable field missing or not a bool")
	}
	
	if recoverable {
		t.Error("invalid ID error should be non-recoverable")
	}
}
```

- [ ] **Step 2: Run test to verify it passes**

```bash
go test -v -run TestFetchArxivText_InvalidArxivID ./tools/research
```

Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add tools/research/fetch_arxiv_text_test.go
git commit -m "test: add TestFetchArxivText_InvalidArxivID"
```

---

## Task 9: Write test for missing arxiv_id field

**Files:**
- Modify: `tools/research/fetch_arxiv_text_test.go`
- Test: `go test -v -run TestFetchArxivText_MissingField`

- [ ] **Step 1: Write failing test**

Add to `fetch_arxiv_text_test.go`:

```go
func TestFetchArxivText_MissingField(t *testing.T) {
	mockClient := &mockHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			t.Fatal("should not make HTTP request when arxiv_id missing")
			return nil, nil
		},
	}
	
	ts := newResearchToolSetWithBase(mockClient, "")
	
	input := `{"max_length": 1000}`
	result, err := ts.handleFetchArxivText(context.Background(), json.RawMessage(input))
	
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	
	// Parse as error response
	var errResp map[string]any
	if err := json.Unmarshal([]byte(result), &errResp); err != nil {
		t.Fatalf("failed to parse error response: %v", err)
	}
	
	// Verify error message
	errMsg, ok := errResp["error"].(string)
	if !ok {
		t.Fatal("error field missing or not a string")
	}
	
	if !contains(errMsg, "arxiv_id is required") {
		t.Errorf("error message should mention required field: %s", errMsg)
	}
	
	// Verify non-recoverable
	recoverable, ok := errResp["recoverable"].(bool)
	if !ok {
		t.Fatal("recoverable field missing or not a bool")
	}
	
	if recoverable {
		t.Error("missing field error should be non-recoverable")
	}
}
```

- [ ] **Step 2: Run test to verify it passes**

```bash
go test -v -run TestFetchArxivText_MissingField ./tools/research
```

Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add tools/research/fetch_arxiv_text_test.go
git commit -m "test: add TestFetchArxivText_MissingField"
```

---

## Task 10: Write test for network error

**Files:**
- Modify: `tools/research/fetch_arxiv_text_test.go`
- Test: `go test -v -run TestFetchArxivText_NetworkError`

- [ ] **Step 1: Write failing test**

Add to `fetch_arxiv_text_test.go`:

```go
func TestFetchArxivText_NetworkError(t *testing.T) {
	mockClient := &mockHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			return nil, fmt.Errorf("network timeout")
		},
	}
	
	ts := newResearchToolSetWithBase(mockClient, "")
	
	input := `{"arxiv_id": "2301.00001"}`
	result, err := ts.handleFetchArxivText(context.Background(), json.RawMessage(input))
	
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	
	// Parse as error response
	var errResp map[string]any
	if err := json.Unmarshal([]byte(result), &errResp); err != nil {
		t.Fatalf("failed to parse error response: %v", err)
	}
	
	// Verify error message
	errMsg, ok := errResp["error"].(string)
	if !ok {
		t.Fatal("error field missing or not a string")
	}
	
	if !contains(errMsg, "request failed") {
		t.Errorf("error message should mention request failure: %s", errMsg)
	}
	
	// Verify recoverable
	recoverable, ok := errResp["recoverable"].(bool)
	if !ok {
		t.Fatal("recoverable field missing or not a bool")
	}
	
	if !recoverable {
		t.Error("network error should be recoverable")
	}
}
```

- [ ] **Step 2: Run test to verify it passes**

```bash
go test -v -run TestFetchArxivText_NetworkError ./tools/research
```

Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add tools/research/fetch_arxiv_text_test.go
git commit -m "test: add TestFetchArxivText_NetworkError"
```

---

## Task 11: Write test for service unavailable (503)

**Files:**
- Modify: `tools/research/fetch_arxiv_text_test.go`
- Test: `go test -v -run TestFetchArxivText_ServiceUnavailable`

- [ ] **Step 1: Write failing test**

Add to `fetch_arxiv_text_test.go`:

```go
func TestFetchArxivText_ServiceUnavailable(t *testing.T) {
	mockClient := &mockHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusServiceUnavailable,
				Body:       io.NopCloser(bytes.NewBufferString("Service Unavailable")),
				Header:     make(http.Header),
			}, nil
		},
	}
	
	ts := newResearchToolSetWithBase(mockClient, "")
	
	input := `{"arxiv_id": "2301.00001"}`
	result, err := ts.handleFetchArxivText(context.Background(), json.RawMessage(input))
	
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	
	// Parse as error response
	var errResp map[string]any
	if err := json.Unmarshal([]byte(result), &errResp); err != nil {
		t.Fatalf("failed to parse error response: %v", err)
	}
	
	// Verify error message
	errMsg, ok := errResp["error"].(string)
	if !ok {
		t.Fatal("error field missing or not a string")
	}
	
	if !contains(errMsg, "arXiv returned status 503") {
		t.Errorf("error message should mention status 503: %s", errMsg)
	}
	
	// Verify recoverable
	recoverable, ok := errResp["recoverable"].(bool)
	if !ok {
		t.Fatal("recoverable field missing or not a bool")
	}
	
	if !recoverable {
		t.Error("service unavailable error should be recoverable")
	}
}
```

- [ ] **Step 2: Run test to verify it passes**

```bash
go test -v -run TestFetchArxivText_ServiceUnavailable ./tools/research
```

Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add tools/research/fetch_arxiv_text_test.go
git commit -m "test: add TestFetchArxivText_ServiceUnavailable"
```

---

## Task 12: Write test for version suffix handling

**Files:**
- Modify: `tools/research/fetch_arxiv_text_test.go`
- Test: `go test -v -run TestFetchArxivText_WithVersionSuffix`

- [ ] **Step 1: Write failing test**

Add to `fetch_arxiv_text_test.go`:

```go
func TestFetchArxivText_WithVersionSuffix(t *testing.T) {
	htmlContent := `
		<!DOCTYPE html>
		<html>
		<body><article><p>Test content</p></article></body>
		</html>
	`
	
	mockClient := &mockHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			// Verify version suffix stripped from URL
			expectedURL := "https://arxiv.org/html/1706.03762"
			if req.URL.String() != expectedURL {
				t.Errorf("URL: got %s, want %s (version should be stripped)", req.URL.String(), expectedURL)
			}
			
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString(htmlContent)),
				Header:     make(http.Header),
			}, nil
		},
	}
	
	ts := newResearchToolSetWithBase(mockClient, "")
	
	input := `{"arxiv_id": "1706.03762v2"}`
	result, err := ts.handleFetchArxivText(context.Background(), json.RawMessage(input))
	
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	
	var parsed ArxivTextResult
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	
	// Verify normalized ID without version
	if parsed.ArxivID != "1706.03762" {
		t.Errorf("arxiv_id: got %s, want 1706.03762 (no version)", parsed.ArxivID)
	}
}
```

- [ ] **Step 2: Run test to verify it passes**

```bash
go test -v -run TestFetchArxivText_WithVersionSuffix ./tools/research
```

Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add tools/research/fetch_arxiv_text_test.go
git commit -m "test: add TestFetchArxivText_WithVersionSuffix"
```

---

## Task 13: Run all new tests together

**Files:**
- Test: `tools/research/fetch_arxiv_text_test.go`

- [ ] **Step 1: Run all fetch_arxiv_text tests**

```bash
go test -v ./tools/research -run TestFetchArxivText
```

Expected: All tests PASS

- [ ] **Step 2: Verify test coverage**

Check that all these tests pass:
- TestFetchArxivText_Success
- TestFetchArxivText_Truncation
- TestFetchArxivText_CustomMaxLength
- TestFetchArxivText_HTMLNotAvailable
- TestFetchArxivText_InvalidArxivID
- TestFetchArxivText_MissingField
- TestFetchArxivText_NetworkError
- TestFetchArxivText_ServiceUnavailable
- TestFetchArxivText_WithVersionSuffix

- [ ] **Step 3: No commit** (verification step only)

---

## Task 14: Add token estimator for fetch_arxiv_text

**Files:**
- Modify: `tools/research/estimators.go:20-37`
- Test: Manual verification (estimators are functions)

- [ ] **Step 1: Add fetch_arxiv_text estimator**

In `tools/research/estimators.go`, add this entry to the map (after the "fetch_arxiv_pdf" entry, around line 20):

```go
"fetch_arxiv_text": func(args map[string]any) int {
	maxLength := 25000
	if ml, ok := args["max_length"].(float64); ok && ml > 0 {
		maxLength = int(ml)
		if maxLength > 25000 {
			maxLength = 25000
		}
	}
	// Estimate ~4 chars per token for text content
	// Add 100 tokens for JSON structure overhead
	return (maxLength / 4) + 100
},
```

- [ ] **Step 2: Verify code compiles**

```bash
cd tools/research
go build
```

Expected: No compilation errors

- [ ] **Step 3: Commit**

```bash
git add tools/research/estimators.go
git commit -m "feat: add token estimator for fetch_arxiv_text"
```

---

## Task 15: Delete PDF-related functions

**Files:**
- Modify: `tools/research/handlers.go:308-549`
- Test: Compilation check

- [ ] **Step 1: Delete handleFetchArxivPdf function**

In `tools/research/handlers.go`, delete the entire `handleFetchArxivPdf` function (lines ~308-441).

- [ ] **Step 2: Delete extractPdfText function**

Delete the entire `extractPdfText` function (lines ~441-508).

- [ ] **Step 3: Delete fixPdfSpacing function**

Delete the entire `fixPdfSpacing` function (lines ~508-530).

- [ ] **Step 4: Delete assessExtractionQuality function**

Delete the entire `assessExtractionQuality` function (lines ~530-549).

- [ ] **Step 5: Delete ArxivPdfResult type**

Search for `type ArxivPdfResult struct` in handlers.go and delete it if present (it may be in definitions.go instead - check there).

- [ ] **Step 6: Verify code compiles**

```bash
cd tools/research
go build
```

Expected: No compilation errors (though fetch_arxiv_pdf_test.go will fail)

- [ ] **Step 7: Commit**

```bash
git add tools/research/handlers.go
git commit -m "refactor: remove PDF extraction functions"
```

---

## Task 16: Delete BuildFetchArxivPdfTool from definitions.go

**Files:**
- Modify: `tools/research/definitions.go:68-91`
- Test: Compilation check

- [ ] **Step 1: Delete BuildFetchArxivPdfTool function**

In `tools/research/definitions.go`, delete the entire `BuildFetchArxivPdfTool` function (lines 68-91).

- [ ] **Step 2: Verify code compiles**

```bash
cd tools/research
go build
```

Expected: No compilation errors

- [ ] **Step 3: Commit**

```bash
git add tools/research/definitions.go
git commit -m "refactor: remove BuildFetchArxivPdfTool definition"
```

---

## Task 17: Remove fetch_arxiv_pdf estimator

**Files:**
- Modify: `tools/research/estimators.go:20-37`
- Test: Compilation check

- [ ] **Step 1: Delete fetch_arxiv_pdf estimator entry**

In `tools/research/estimators.go`, delete the entire `"fetch_arxiv_pdf"` map entry (lines 20-37).

- [ ] **Step 2: Verify code compiles**

```bash
cd tools/research
go build
```

Expected: No compilation errors

- [ ] **Step 3: Commit**

```bash
git add tools/research/estimators.go
git commit -m "refactor: remove fetch_arxiv_pdf estimator"
```

---

## Task 18: Delete fetch_arxiv_pdf_test.go

**Files:**
- Delete: `tools/research/fetch_arxiv_pdf_test.go`
- Test: Compilation check

- [ ] **Step 1: Delete test file**

```bash
rm tools/research/fetch_arxiv_pdf_test.go
```

- [ ] **Step 2: Verify all tests still pass**

```bash
go test -v ./tools/research
```

Expected: All remaining tests PASS (fetch_arxiv_pdf tests gone)

- [ ] **Step 3: Commit**

```bash
git add tools/research/fetch_arxiv_pdf_test.go
git commit -m "refactor: remove fetch_arxiv_pdf tests"
```

---

## Task 19: Remove PDF dependency from go.mod

**Files:**
- Modify: `go.mod`
- Test: Compilation check

- [ ] **Step 1: Remove PDF import from handlers.go**

In `tools/research/handlers.go`, delete the PDF import line (around line 20):

```go
pdf "github.com/ledongthuc/pdf"
```

- [ ] **Step 2: Run go mod tidy**

```bash
go mod tidy
```

Expected: PDF dependency removed from go.mod

- [ ] **Step 3: Verify dependency removed**

```bash
grep "ledongthuc/pdf" go.mod
```

Expected: No output (dependency not found)

- [ ] **Step 4: Verify all tests pass**

```bash
go test ./tools/research
```

Expected: All tests PASS

- [ ] **Step 5: Commit**

```bash
git add go.mod go.sum tools/research/handlers.go
git commit -m "refactor: remove PDF extraction dependency"
```

---

## Task 20: Verify all tests pass

**Files:**
- Test: All research tool tests

- [ ] **Step 1: Run all research tests**

```bash
go test -v ./tools/research
```

Expected: All tests PASS, including:
- All fetch_arxiv_text tests
- All existing tests (arxiv, openalex, github, web search, etc.)

- [ ] **Step 2: Run tests with race detector**

```bash
go test -race ./tools/research
```

Expected: All tests PASS with no race conditions

- [ ] **Step 3: No commit** (verification step only)

---

## Task 21: Manual integration test with real arXiv papers

**Files:**
- Test: Manual testing with `tools-cli`

- [ ] **Step 1: Build tools-cli**

```bash
make build-tools
```

Expected: Build succeeds

- [ ] **Step 2: Test recent paper with HTML (should succeed)**

```bash
./bin/tools-cli fetch_arxiv_text '{"arxiv_id": "2301.00001"}'
```

Expected: Returns arxiv_id, text_content (non-empty), truncated, no error

- [ ] **Step 3: Test old paper without HTML (should return 404 error)**

```bash
./bin/tools-cli fetch_arxiv_text '{"arxiv_id": "astro-ph/9901234"}'
```

Expected: Returns arxiv_id, empty text_content, error: "HTML version not available for this paper"

- [ ] **Step 4: Test paper with version suffix (should strip version)**

```bash
./bin/tools-cli fetch_arxiv_text '{"arxiv_id": "1706.03762v2"}'
```

Expected: Returns arxiv_id: "1706.03762" (no version), text_content extracted

- [ ] **Step 5: Test custom max_length**

```bash
./bin/tools-cli fetch_arxiv_text '{"arxiv_id": "2301.00001", "max_length": 1000}'
```

Expected: Returns text_content with max 1000 characters, truncated: true

- [ ] **Step 6: Document results**

Create a file `docs/superpowers/tests/2026-05-12-fetch-arxiv-text-manual.md` with the results:

```markdown
# fetch_arxiv_text Manual Integration Test Results

**Date:** 2026-05-12

## Test 1: Recent paper with HTML
- Input: `{"arxiv_id": "2301.00001"}`
- Result: ✅ Success
- Notes: Extracted clean text content

## Test 2: Old paper without HTML
- Input: `{"arxiv_id": "astro-ph/9901234"}`
- Result: ✅ Error as expected
- Error: "HTML version not available for this paper"

## Test 3: Paper with version suffix
- Input: `{"arxiv_id": "1706.03762v2"}`
- Result: ✅ Success
- Notes: Version stripped correctly

## Test 4: Custom max_length
- Input: `{"arxiv_id": "2301.00001", "max_length": 1000}`
- Result: ✅ Success
- Notes: Truncation applied correctly
```

- [ ] **Step 7: Commit manual test results**

```bash
git add docs/superpowers/tests/2026-05-12-fetch-arxiv-text-manual.md
git commit -m "test: add manual integration test results for fetch_arxiv_text"
```

---

## Success Criteria Checklist

After completing all tasks, verify these criteria:

### ✅ Functional
- [ ] Tool fetches and extracts text from arXiv HTML pages
- [ ] Handles papers with/without HTML gracefully (error on 404)
- [ ] Text truncation works correctly at 25k limit
- [ ] Rate limiting maintained (3 seconds between requests)
- [ ] All supported arXiv ID formats work (new, old, with version, with prefix)

### ✅ Code Quality
- [ ] All unit tests pass
- [ ] PDF extraction code removed (cleaner codebase)
- [ ] No unused dependencies remain (PDF library removed)
- [ ] Consistent with fetch_webpage_content pattern (readability usage)
- [ ] No compilation errors

### ✅ User Experience
- [ ] Agent gets cleaner text than PDF extraction
- [ ] Clear error messages when HTML unavailable
- [ ] Tool description accurately explains limitations
- [ ] Manual tests confirm real-world behavior
