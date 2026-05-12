# PDF Text Extraction Design

**Date:** 2026-05-11  
**Status:** Approved  
**Owner:** Claude Code

## Problem Statement

The `fetch_arxiv_pdf` tool currently only validates PDF URLs and returns metadata. The research agent has no way to read actual PDF content, which is a critical limitation for a research agent that needs to analyze academic papers.

The tool returns a URL that the agent cannot follow, making it effectively useless for deep research tasks.

## Solution

Modify `fetch_arxiv_pdf` to download PDFs and extract their text content with configurable length limits and quality indicators.

## Requirements

1. Download and extract text from arXiv PDFs
2. Apply configurable length limits (default 8000, max 50000 characters)
3. Include extraction quality indicators in response
4. Handle failures gracefully with partial results
5. Maintain existing rate limiting (1 request per 3 seconds)
6. Use pure Go implementation (no external tools)

## Architecture

### High-Level Flow

```
fetch_arxiv_pdf(arxiv_id, max_length?) 
  → Validate arXiv ID
  → Rate limit (334ms minimum interval)
  → Download PDF bytes via HTTP
  → Extract text using github.com/ledongthuc/pdf
  → Calculate quality metrics
  → Truncate to max_length
  → Return structured response
```

### Component Changes

**Dependencies:**
- Add: `github.com/ledongthuc/pdf` for text extraction

**Modified files:**
- `tools/research/handlers.go` - Update `handleFetchArxivPdf` implementation
- `tools/research/definitions.go` - Update tool definition and `ArxivPdfResult` struct
- `tools/research/fetch_arxiv_pdf_test.go` - Add extraction tests
- `go.mod` - Add PDF library dependency

## Data Structures

### Input Schema

```json
{
  "arxiv_id": "1706.03762",     // Required: arXiv paper ID
  "max_length": 8000             // Optional: max characters (default 8000, max 50000)
}
```

### Output Schema

```go
type ArxivPdfResult struct {
    ArxivID           string `json:"arxiv_id"`
    TextContent       string `json:"text_content"`           // Extracted text
    PageCount         int    `json:"page_count"`             // Number of pages
    CharCount         int    `json:"char_count"`             // Characters before truncation
    ExtractionQuality string `json:"extraction_quality"`     // "good", "poor", "failed"
    Truncated         bool   `json:"truncated"`              // True if text was cut
    Version           string `json:"version,omitempty"`      // Version suffix if present
    Error             string `json:"error,omitempty"`        // Error details if extraction failed
}
```

### Quality Assessment Logic

| Quality | Criteria |
|---------|----------|
| `"good"` | Text density > 100 chars/page AND total chars > 1000 |
| `"poor"` | Text density < 100 chars/page OR total chars < 1000 |
| `"failed"` | Extraction error occurred |

**Text density formula:** `totalChars / pageCount`

## Implementation Details

### PDF Extraction Flow

1. **Download PDF**
   - HTTP GET to `https://export.arxiv.org/pdf/{arxiv_id}.pdf`
   - Read full response body into memory (arXiv PDFs typically 1-10MB)
   - Use existing `httpClient` with context
   - Respect rate limiter: `r.arxivRateLimiter.Wait()`

2. **Parse PDF**
   - Create reader: `pdf.NewReader(bytes.NewReader(pdfBytes), int64(len(pdfBytes)))`
   - Get page count: `reader.NumPage()`
   - Iterate pages: `for i := 1; i <= numPages; i++`
   - Extract text: `page.GetPlainText()`

3. **Process Text**
   - Concatenate all page text with `\n\n` between pages
   - Count total characters before truncation
   - Truncate to `max_length` if exceeded: `text[:maxLength]`
   - Calculate density: `charCount / pageCount`

4. **Assess Quality**
   - Check density threshold (100 chars/page)
   - Check minimum text threshold (1000 chars)
   - Set `extraction_quality` field accordingly

### Error Handling

**Graceful degradation approach:**

| Error Scenario | Response |
|----------------|----------|
| PDF download fails | Return result with `error` field, `extraction_quality: "failed"`, no text |
| PDF parsing fails | Return result with `error` field, `extraction_quality: "failed"`, no text |
| Extraction panic | Recover, return `error` field, `extraction_quality: "failed"` |
| Low quality extraction | Return text anyway with `extraction_quality: "poor"` |

**Example error response:**
```json
{
  "arxiv_id": "1706.03762",
  "text_content": "",
  "page_count": 0,
  "char_count": 0,
  "extraction_quality": "failed",
  "truncated": false,
  "error": "PDF download failed: 404 Not Found"
}
```

### Tool Definition Update

Update `BuildFetchArxivPdfTool()` description:

```
"Fetches and extracts text content from an arXiv preprint PDF. Returns extracted text 
with quality indicators (page count, character count, extraction quality). Text is 
truncated to max_length (default 8000, max 50000 characters). Extraction quality may 
be 'good', 'poor' (image-based PDFs), or 'failed' (errors). Rate limit: 1 request per 
3 seconds per arXiv TOS."
```

Add `max_length` parameter to input schema.

## Testing Strategy

### Unit Tests (in `fetch_arxiv_pdf_test.go`)

**Success cases:**
- Test with valid multi-page PDF bytes
- Verify page count, char count, extraction quality match expected
- Test truncation at max_length boundary (e.g., 100 char limit on 500 char PDF)
- Verify version suffix preserved (e.g., "2301.00001v2")

**Error cases:**
- Invalid PDF bytes → `extraction_quality: "failed"`, error message set
- HTTP 404 → error in response, failed quality
- Corrupted PDF header → graceful error handling
- Empty/zero-page PDF → poor quality indicator

**Quality assessment:**
- High-density text (200 chars/page) → `"good"`
- Low-density text (50 chars/page) → `"poor"`
- Short document (500 total chars) → `"poor"`

**Rate limiting:**
- Verify rate limiter still enforces 334ms minimum interval
- Test doesn't regress existing rate limiting behavior

### Integration Testing (manual with tools-cli)

Test with real arXiv papers:
- Text-heavy paper (e.g., "1706.03762" - Attention is All You Need)
- Figure-heavy paper (check for poor quality indicator)
- Recent paper (test with version suffix)
- Non-existent ID (test error handling)

Verify:
- JSON output matches schema
- Text content is readable
- Quality indicators are accurate
- tools-cli displays results correctly

## Implementation Order

1. Add `github.com/ledongthuc/pdf` dependency to `go.mod`
2. Update `ArxivPdfResult` struct in `handlers.go`
3. Implement PDF download in `handleFetchArxivPdf`
4. Implement text extraction with quality assessment
5. Update `BuildFetchArxivPdfTool` definition and description
6. Write unit tests in `fetch_arxiv_pdf_test.go`
7. Test with tools-cli against real arXiv papers
8. Update README documentation

## Non-Goals

- **No text cleaning or formatting** - Return raw extracted text
- **No caching** - Extract on every request
- **No OCR** - Pure text extraction only (image-based PDFs will be "poor" quality)
- **No semantic parsing** - No section detection, citation extraction, etc.
- **No external tools** - Pure Go implementation only

## Success Criteria

1. Agent can read actual paper content from arXiv PDFs
2. Extraction works for typical arXiv papers (text-based LaTeX PDFs)
3. Quality indicators help agent understand extraction reliability
4. Graceful degradation on failures (agent still gets useful error info)
5. Existing rate limiting maintained
6. All tests pass

## Risks & Mitigations

| Risk | Mitigation |
|------|------------|
| Low extraction quality on complex PDFs | Quality indicator warns agent, can try webpage/abstract instead |
| Memory usage on large PDFs | arXiv PDFs typically < 10MB, acceptable to load in memory |
| PDF library bugs/panics | Wrap extraction in recover(), return graceful error |
| Breaking existing tool behavior | Only tools-cli uses this currently, easy to update |
