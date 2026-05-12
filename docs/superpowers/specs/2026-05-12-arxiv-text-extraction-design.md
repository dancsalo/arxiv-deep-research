# arXiv Text Extraction via HTML Design

**Date:** 2026-05-12  
**Status:** Approved  
**Goal:** Replace PDF-based text extraction with HTML-based extraction for better quality and simpler codebase

## Problem

The current `fetch_arxiv_pdf` tool downloads PDFs and extracts text from them. This approach has two issues:
1. Poor extraction quality on certain types of papers (especially those with complex math or figures)
2. arXiv provides better HTML versions that are cleaner and more reliable

## Solution

Replace `fetch_arxiv_pdf` entirely with `fetch_arxiv_text` that fetches and extracts text from arXiv's HTML rendering. This is a clean replacement with no fallback — simpler codebase, clearer intent.

## Architecture & Flow

### High-Level Flow

```
fetch_arxiv_text(arxiv_id, max_length?)
  → Normalize & validate arXiv ID
  → Rate limit (3 seconds between requests)
  → Construct HTML URL: https://arxiv.org/html/{arxiv_id}
  → HTTP GET request
  → Check if HTML available (200 vs 404)
  → Extract text using go-readability library
  → Truncate to max_length (default/max 25k)
  → Return {arxiv_id, text_content, truncated, error}
```

### Files Modified

1. **tools/research/handlers.go**
   - Rename: `handleFetchArxivPdf` → `handleFetchArxivText`
   - Replace PDF download/extraction with HTML fetch + readability
   - Delete: `extractPdfText()`, `fixPdfSpacing()`, `assessExtractionQuality()`
   - Keep: `normalizeArxivID()` (still needed for ID validation)

2. **tools/research/definitions.go**
   - Rename: `BuildFetchArxivPdfTool()` → `BuildFetchArxivTextTool()`
   - Update tool name: `fetch_arxiv_pdf` → `fetch_arxiv_text`
   - Simplify result struct: `ArxivPdfResult` → `ArxivTextResult`
   - Update parameter: `max_length` default and max both 25000

3. **tools/research/fetch_arxiv_pdf_test.go**
   - Rename file: → `fetch_arxiv_text_test.go`
   - Rewrite tests for HTML extraction
   - Remove PDF-specific tests (page count, density, spacing fixes)

4. **tools/research/research.go**
   - Update tool registry: register `fetch_arxiv_text` handler

5. **go.mod**
   - Remove: `github.com/ledongthuc/pdf` dependency
   - Keep: `github.com/go-shiori/go-readability` (already present)

### Component Dependencies

- **Reuse:** go-readability library (same as `fetch_webpage_content`)
- **Remove:** PDF extraction library
- **Keep:** HTTP client, rate limiter, arXiv ID normalization

## Data Structures & Schemas

### Input Schema

```json
{
  "arxiv_id": "1706.03762",     // Required: arXiv ID (supports all formats)
  "max_length": 25000            // Optional: max chars (default 25000, max 25000)
}
```

**Supported arXiv ID formats** (via existing `normalizeArxivID`):
- `2301.00001` (new format)
- `arXiv:2301.00001` (with prefix)
- `2301.00001v2` (with version — version stripped for URL)
- `astro-ph/9901234` (old format)
- `https://arxiv.org/abs/2301.00001` (full URL)

### Output Schema

```go
type ArxivTextResult struct {
    ArxivID     string `json:"arxiv_id"`        // Normalized ID
    TextContent string `json:"text_content"`    // Extracted HTML text
    Truncated   bool   `json:"truncated"`       // True if text exceeded max_length
    Error       string `json:"error,omitempty"` // Error message if extraction failed
}
```

**Simplifications from PDF version:**
- ❌ Removed: `page_count` (no pages in HTML)
- ❌ Removed: `char_count` (can infer from `len(text_content)`)
- ❌ Removed: `extraction_quality` (HTML is reliable — either success or failed)
- ❌ Removed: `version` suffix (not needed in output)

### Tool Definition

```go
func BuildFetchArxivTextTool() anthropic.ToolUnionParam {
    t := anthropic.ToolUnionParamOfTool(
        anthropic.ToolInputSchemaParam{
            Type: "object",
            Properties: map[string]any{
                "arxiv_id": map[string]any{
                    "type": "string",
                    "description": "arXiv identifier. Formats: 2301.00001, arXiv:2301.00001, 2301.00001v2 (new), or astro-ph/9901234 (old)",
                },
                "max_length": map[string]any{
                    "type": "integer",
                    "description": "Maximum text content length in characters (default 25000, max 25000)",
                    "default": 25000,
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

## Implementation Details

### HTML Fetching & Extraction

```go
func (r *ResearchToolSet) handleFetchArxivText(ctx context.Context, input json.RawMessage) (string, error) {
    // 1. Parse and validate input
    var params struct {
        ArxivID   string `json:"arxiv_id"`
        MaxLength int    `json:"max_length"`
    }
    // Parse JSON, validate arxiv_id required
    // Default max_length to 25000, cap at 25000

    // 2. Normalize arXiv ID (reuse existing function)
    normalized, _, err := normalizeArxivID(params.ArxivID)
    // version discarded - not needed for HTML URL or output

    // 3. Construct HTML URL
    htmlURL := fmt.Sprintf("https://arxiv.org/html/%s", normalized)

    // 4. Rate limit
    r.arxivRateLimiter.Wait()  // Reuse existing rate limiter

    // 5. Fetch HTML
    req, err := http.NewRequestWithContext(ctx, http.MethodGet, htmlURL, nil)
    resp, err := r.client.Do(req)

    // 6. Check HTML availability
    if resp.StatusCode == 404 {
        return error response: "HTML version not available for this paper"
    }

    // 7. Parse URL for readability
    parsedURL, _ := url.Parse(htmlURL)

    // 8. Extract text using go-readability
    article, err := readability.FromReader(resp.Body, parsedURL)
    // article.TextContent contains cleaned text

    // 9. Truncate to max_length
    textContent := article.TextContent
    truncated := false
    if len(textContent) > params.MaxLength {
        textContent = textContent[:params.MaxLength]
        truncated = true
    }

    // 10. Return result
    result := ArxivTextResult{
        ArxivID:     normalized,
        TextContent: textContent,
        Truncated:   truncated,
    }
    return json.Marshal(result)
}
```

### Error Handling

| Scenario | Response | Recoverable? |
|----------|----------|--------------|
| Invalid arxiv_id format | `{error: "invalid arXiv ID format: ..."}` | No (non-recoverable) |
| HTML not available (404) | `{arxiv_id: "...", text_content: "", truncated: false, error: "HTML version not available for this paper"}` | No (paper doesn't have HTML) |
| Network error | `{error: "request failed: ..."}` | Yes (transient) |
| arXiv service error (5xx) | `{error: "arXiv returned status 503"}` | Yes (transient) |
| Readability extraction fails | `{error: "failed to extract content: ..."}` | Yes (retry might work) |

### Code to Delete

From `handlers.go`:
- `extractPdfText()` — entire function (~60 lines)
- `fixPdfSpacing()` — entire function (~20 lines)
- `assessExtractionQuality()` — entire function (~15 lines)
- PDF download logic in handler

## Testing Strategy

### Unit Tests (in fetch_arxiv_text_test.go)

**Test arXiv ID normalization (keep existing tests):**
- `TestNormalizeArxivID_*` — All existing normalization tests remain valid
- New format, old format, with/without prefix, version handling

**New HTML extraction tests:**

1. **Success cases:**
   - `TestFetchArxivText_Success` — Mock HTML response, verify text extraction
   - `TestFetchArxivText_Truncation` — Text > 25k chars, verify truncated=true and correct length
   - `TestFetchArxivText_UnderLimit` — Text < 25k chars, verify truncated=false
   - `TestFetchArxivText_CustomMaxLength` — User specifies max_length < 25000

2. **Error cases:**
   - `TestFetchArxivText_HTMLNotAvailable` — Mock 404, verify error message
   - `TestFetchArxivText_InvalidArxivID` — Invalid format, non-recoverable error
   - `TestFetchArxivText_NetworkError` — Network failure, recoverable error
   - `TestFetchArxivText_ServiceUnavailable` — 503 response, recoverable error
   - `TestFetchArxivText_MissingField` — No arxiv_id in input
   - `TestFetchArxivText_ContextCancelled` — Cancelled context

3. **Edge cases:**
   - `TestFetchArxivText_EmptyHTML` — Valid response but no text content
   - `TestFetchArxivText_OldFormatID` — Old format like astro-ph/9901234
   - `TestFetchArxivText_WithVersionSuffix` — ID like 2301.00001v2 (version stripped for URL)

**Tests to delete:**
- All PDF-specific tests: page count, density, spacing fixes, panic recovery
- `TestExtractPdfText_*` tests
- `TestAssessExtractionQuality_*` tests
- `TestArxivPdfEstimator_*` tests (update for new estimator)

### Estimator Update

In `estimators.go`, update the token estimator:

```go
// Old: Based on max_length with 200 base + text tokens
// New: Simpler - just text tokens (HTML extraction is cleaner)
"fetch_arxiv_text": func(params map[string]any) int {
    maxLength := 25000
    if ml, ok := params["max_length"].(float64); ok && ml > 0 {
        maxLength = int(ml)
        if maxLength > 25000 {
            maxLength = 25000
        }
    }
    // Estimate ~4 chars per token for text content
    // Add 100 tokens for JSON structure overhead
    return (maxLength / 4) + 100
}
```

### Manual Integration Testing

Test with `tools-cli` against real arXiv papers:
- Recent paper with HTML: `2301.00001` (should succeed)
- Old paper without HTML: `astro-ph/9901234` (should return 404 error)
- Paper with version: `1706.03762v2` (should strip version, fetch HTML)
- Very long paper (test truncation at 25k)

## Implementation Order

1. **Update dependencies**
   - Remove `github.com/ledongthuc/pdf` from go.mod
   - Run `go mod tidy`

2. **Update data structures (definitions.go)**
   - Create new `ArxivTextResult` struct
   - Rename `BuildFetchArxivPdfTool()` → `BuildFetchArxivTextTool()`
   - Update tool name, description, parameters

3. **Implement HTML fetching (handlers.go)**
   - Rename `handleFetchArxivPdf` → `handleFetchArxivText`
   - Replace PDF download/extraction with HTML fetch + readability
   - Update error handling for HTML-specific cases
   - Delete: `extractPdfText()`, `fixPdfSpacing()`, `assessExtractionQuality()`
   - Keep: `normalizeArxivID()` (still needed)

4. **Update tool registry (research.go)**
   - Replace `fetch_arxiv_pdf` → `fetch_arxiv_text` in handler map

5. **Update estimator (estimators.go)**
   - Update token estimator for `fetch_arxiv_text`

6. **Rewrite tests (fetch_arxiv_text_test.go)**
   - Rename test file
   - Delete PDF-specific tests
   - Write new HTML extraction tests
   - Keep normalization tests

7. **Manual testing**
   - Test with tools-cli against real papers
   - Verify HTML extraction quality
   - Test error cases (404, old papers)

8. **Update documentation**
   - Update any README or docs referencing `fetch_arxiv_pdf`

## Success Criteria

### ✅ Functional
- Tool fetches and extracts text from arXiv HTML pages
- Handles papers with/without HTML gracefully (error on 404)
- Text truncation works correctly at 25k limit
- Rate limiting maintained (3 seconds between requests)

### ✅ Code Quality
- All unit tests pass
- PDF extraction code removed (cleaner codebase)
- No unused dependencies remain
- Consistent with `fetch_webpage_content` pattern (readability usage)

### ✅ User Experience
- Agent gets cleaner text than PDF extraction
- Clear error messages when HTML unavailable
- Tool description accurately explains limitations

## Trade-offs

**Chosen: Clean Replacement (Approach A)**

We considered three approaches:
- **A) Clean Replacement:** Remove PDF entirely, HTML only (chosen)
- **B) Side-by-Side Tools:** Keep both PDF and HTML tools
- **C) Unified Tool:** Single tool with strategy parameter

We chose A because:
1. HTML solves the quality issues that motivated the change
2. Simpler codebase — no dead code or unused dependencies
3. Breaking change is minimal (internal tool in active development)
4. One clear way to fetch arXiv content

Trade-offs accepted:
- No fallback if HTML unavailable (acceptable — return clear error)
- Can't compare PDF vs HTML quality (no longer needed — HTML is better)
