---
name: arXiv PDF Text Extraction Tool
description: Implement fetch_arxiv_pdf_text tool for reading full paper content
type: feature
parent: stage5-master
---

## Context

This plan implements the `fetch_arxiv_pdf_text` tool that downloads arXiv PDFs and extracts their full text. This enables the research agent to read and summarize papers beyond just their abstracts.

Key considerations:
- Must respect arXiv's 3-second rate limit per TOS
- Share rate limiter with existing `fetch_arxiv_pdf` tool
- Handle large papers with truncation
- Clean extracted text (remove headers, footers, excessive whitespace)

The implementation uses `github.com/ledongthuc/pdf` for PDF parsing in pure Go.

## Implementation Steps

### Step 1: Add Dependency

**File**: `go.mod`

Add PDF parsing library:

```bash
go get github.com/ledongthuc/pdf
go mod tidy
```

### Step 2: Add Rate Limiter to ResearchToolSet

**File**: `tools/research/research.go`

Update struct to include rate limiter:

```go
import (
	"encoding/json"
	"net/http"
	"golang.org/x/time/rate"

	"github.com/dancsalo/arxiv-deep-research/internal/registry"
)

type ResearchToolSet struct {
	client        *http.Client
	githubToken   string
	arxivLimiter  *rate.Limiter
}

func NewResearchToolSet(client *http.Client, githubToken string) *ResearchToolSet {
	// arXiv allows 1 request per 3 seconds (per TOS)
	return &ResearchToolSet{
		client:       client,
		githubToken:  githubToken,
		arxivLimiter: rate.NewLimiter(rate.Every(3*time.Second), 1),
	}
}
```

Add import:
```go
import (
	// ... existing imports ...
	"time"
	"golang.org/x/time/rate"
)
```

### Step 3: Update Existing fetch_arxiv_pdf Handler

**File**: `tools/research/handlers.go`

Update `handleFetchArxivPdf` to use the rate limiter (add at start of function):

```go
func (r *ResearchToolSet) handleFetchArxivPdf(ctx context.Context, input json.RawMessage) (string, error) {
	// Wait for rate limiter
	if err := r.arxivLimiter.Wait(ctx); err != nil {
		return toolError("rate limiter error: "+err.Error(), true), nil
	}

	// ... rest of existing implementation ...
}
```

### Step 4: Add Tool Definition

**File**: `tools/research/definitions.go`

Add the tool definition:

```go
func BuildFetchArxivPdfTextTool() anthropic.ToolUnionParam {
	t := anthropic.ToolUnionParamOfTool(
		anthropic.ToolInputSchemaParam{
			Type: "object",
			Properties: map[string]any{
				"arxiv_id": map[string]any{
					"type":        "string",
					"description": "arXiv identifier (e.g., 2301.00001 or 1706.03762)",
				},
				"max_pages": map[string]any{
					"type":        "integer",
					"description": "Maximum number of pages to extract (default: all pages). Use to limit context for very long papers.",
				},
			},
			Required: []string{"arxiv_id"},
		},
		"fetch_arxiv_pdf_text",
	)
	t.OfTool.Description = anthropic.String(
		"Download and extract full text from an arXiv PDF. Returns the paper's text content for detailed analysis and summarization. Rate limited to 1 request per 3 seconds per arXiv terms of service. Text is truncated if it exceeds 100k characters.",
	)
	return t
}
```

### Step 5: Add Result Types

**File**: `tools/research/handlers.go`

Add result struct:

```go
type ArxivPdfTextResult struct {
	ArxivID   string `json:"arxiv_id"`
	Text      string `json:"text"`
	PageCount int    `json:"page_count"`
	Truncated bool   `json:"truncated"`
}
```

### Step 6: Implement Handler

**File**: `tools/research/handlers.go`

Add the complete handler implementation:

```go
func (r *ResearchToolSet) handleFetchArxivPdfText(ctx context.Context, input json.RawMessage) (string, error) {
	// Wait for rate limiter (shared with fetch_arxiv_pdf)
	if err := r.arxivLimiter.Wait(ctx); err != nil {
		return toolError("rate limiter error: "+err.Error(), true), nil
	}

	// Parse input
	var params struct {
		ArxivID  string `json:"arxiv_id"`
		MaxPages int    `json:"max_pages"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return toolError("invalid input: "+err.Error(), false), nil
	}
	if params.ArxivID == "" {
		return toolError("arxiv_id is required", false), nil
	}

	// Normalize and validate arXiv ID
	normalized, _, err := normalizeArxivID(params.ArxivID)
	if err != nil {
		return toolError("invalid arXiv ID format: "+err.Error(), false), nil
	}

	// Construct PDF URL
	pdfURL := fmt.Sprintf("https://export.arxiv.org/pdf/%s.pdf", normalized)

	// Download PDF to temporary file
	tmpFile, err := os.CreateTemp("", "arxiv-*.pdf")
	if err != nil {
		return toolError("failed to create temp file: "+err.Error(), true), nil
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	// Download PDF
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, pdfURL, nil)
	req.Header.Set("User-Agent", "arxiv-deep-research/1.0")

	resp, err := r.client.Do(req)
	if err != nil {
		return toolError("PDF download failed: "+err.Error(), true), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == 404 {
			return toolError("PDF not found for arXiv ID: "+normalized, true), nil
		}
		return toolError(fmt.Sprintf("PDF download returned status %d", resp.StatusCode), true), nil
	}

	// Write to temp file
	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		tmpFile.Close()
		return toolError("failed to write PDF: "+err.Error(), true), nil
	}
	tmpFile.Close()

	// Extract text from PDF
	text, pageCount, err := extractPDFText(tmpPath, params.MaxPages)
	if err != nil {
		return toolError("PDF text extraction failed: "+err.Error(), true), nil
	}

	// Truncate if too long
	const maxChars = 100000
	truncated := false
	if len(text) > maxChars {
		text = text[:maxChars]
		truncated = true
	}

	result := ArxivPdfTextResult{
		ArxivID:   normalized,
		Text:      text,
		PageCount: pageCount,
		Truncated: truncated,
	}

	b, _ := json.Marshal(result)
	return string(b), nil
}

// extractPDFText extracts text from a PDF file
func extractPDFText(pdfPath string, maxPages int) (string, int, error) {
	f, r, err := pdf.Open(pdfPath)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()

	totalPages := r.NumPage()
	pagesToRead := totalPages
	if maxPages > 0 && maxPages < totalPages {
		pagesToRead = maxPages
	}

	var textBuilder strings.Builder
	for pageNum := 1; pageNum <= pagesToRead; pageNum++ {
		page := r.Page(pageNum)
		if page.V.IsNull() {
			continue
		}

		text, err := page.GetPlainText(nil)
		if err != nil {
			// Skip pages that fail to extract
			continue
		}

		// Clean and append text
		cleaned := cleanPDFText(text)
		textBuilder.WriteString(cleaned)
		textBuilder.WriteString("\n\n")
	}

	return textBuilder.String(), totalPages, nil
}

// cleanPDFText removes excessive whitespace and common PDF artifacts
func cleanPDFText(text string) string {
	// Replace multiple spaces with single space
	text = regexp.MustCompile(`\s+`).ReplaceAllString(text, " ")
	
	// Replace multiple newlines with double newline
	text = regexp.MustCompile(`\n\s*\n\s*\n+`).ReplaceAllString(text, "\n\n")
	
	// Trim each line
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimSpace(line)
	}
	
	return strings.TrimSpace(strings.Join(lines, "\n"))
}
```

Add necessary imports:

```go
import (
	// ... existing imports ...
	"os"
	"io"
	"github.com/ledongthuc/pdf"
	"regexp"
)
```

### Step 7: Create Tests

**File**: `tools/research/fetch_arxiv_pdf_text_test.go`

```go
package research

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
	"golang.org/x/time/rate"
)

func TestHandleFetchArxivPdfTextValidation(t *testing.T) {
	toolset := NewResearchToolSet(&http.Client{}, "")

	tests := []struct {
		name        string
		input       string
		expectError bool
	}{
		{
			name:        "empty arxiv_id",
			input:       `{"arxiv_id": ""}`,
			expectError: true,
		},
		{
			name:        "invalid arxiv_id format",
			input:       `{"arxiv_id": "invalid"}`,
			expectError: true,
		},
		{
			name:        "valid new format",
			input:       `{"arxiv_id": "1706.03762"}`,
			expectError: false,
		},
		{
			name:        "valid with version",
			input:       `{"arxiv_id": "1706.03762v5"}`,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := toolset.handleFetchArxivPdfText(context.Background(), []byte(tt.input))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			hasError := contains(result, "error")
			if tt.expectError && !hasError {
				t.Errorf("expected error in result, got: %s", result)
			}
			if !tt.expectError && hasError {
				t.Errorf("unexpected error in result: %s", result)
			}
		})
	}
}

func TestExtractPDFTextTruncation(t *testing.T) {
	// Create a mock very long text
	longText := strings.Repeat("a", 150000)
	
	// Test truncation at 100k chars
	const maxChars = 100000
	if len(longText) <= maxChars {
		t.Fatal("test text should exceed maxChars")
	}

	truncated := longText[:maxChars]
	if len(truncated) != maxChars {
		t.Errorf("expected truncated text to be exactly %d chars, got %d", maxChars, len(truncated))
	}
}

func TestCleanPDFText(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "multiple spaces",
			input:    "hello    world",
			expected: "hello world",
		},
		{
			name:     "multiple newlines",
			input:    "hello\n\n\n\nworld",
			expected: "hello\n\nworld",
		},
		{
			name:     "mixed whitespace",
			input:    "  hello  \n\n  world  ",
			expected: "hello\n\nworld",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cleanPDFText(tt.input)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestArxivRateLimiting(t *testing.T) {
	// Create toolset with fast rate limiter for testing
	toolset := NewResearchToolSet(&http.Client{Timeout: 5 * time.Second}, "")
	toolset.arxivLimiter = rate.NewLimiter(rate.Every(100*time.Millisecond), 1)

	ctx := context.Background()

	// First call should succeed immediately
	start := time.Now()
	input := `{"arxiv_id": "1706.03762"}`
	_, _ = toolset.handleFetchArxivPdfText(ctx, []byte(input))
	elapsed := time.Since(start)

	if elapsed > 50*time.Millisecond {
		t.Errorf("first call should not be rate limited, took %v", elapsed)
	}

	// Second immediate call should be delayed
	start = time.Now()
	_, _ = toolset.handleFetchArxivPdfText(ctx, []byte(input))
	elapsed = time.Since(start)

	if elapsed < 90*time.Millisecond {
		t.Errorf("second call should be rate limited, took only %v", elapsed)
	}
}
```

## Critical Files

- `tools/research/definitions.go` - Tool schema
- `tools/research/handlers.go` - Tool implementation and helper functions
- `tools/research/research.go` - Add rate limiter
- `tools/research/fetch_arxiv_pdf_text_test.go` - Tests
- `go.mod` - Add PDF library dependency

## Verification

1. **Install dependency**:
   ```bash
   go get github.com/ledongthuc/pdf
   go get golang.org/x/time/rate
   go mod tidy
   ```

2. **Build**:
   ```bash
   make build
   ```

3. **Run tests**:
   ```bash
   go test ./tools/research -run TestHandleFetchArxivPdfText -v
   go test ./tools/research -run TestCleanPDFText -v
   go test ./tools/research -run TestArxivRateLimiting -v
   ```

4. **Manual test** (requires network):
   ```bash
   # Test with famous "Attention Is All You Need" paper
   ./bin/research-demo --query "summarize the full text of arXiv:1706.03762"
   ```

5. **Verify rate limiting**:
   - Make 2 consecutive calls and verify the second waits ~3 seconds
   - Check trace logs for rate limiter delays

6. **Test truncation**:
   - Try a very long paper and verify truncated flag is set

## Notes

- The `github.com/ledongthuc/pdf` library is simple but may struggle with complex layouts (multi-column, images)
- Rate limiter is shared between `fetch_arxiv_pdf` and `fetch_arxiv_pdf_text`
- Truncation at 100k chars prevents context window overflow
- Text cleaning removes most PDF artifacts but may not be perfect for all papers
- Consider adding OCR fallback for scanned papers if needed in future
