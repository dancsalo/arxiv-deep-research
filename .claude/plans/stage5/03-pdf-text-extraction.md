---
name: arXiv PDF Text Extraction Tool
description: Implement fetch_arxiv_pdf_text tool for reading full paper content with rate limiting
type: feature
parent: stage5-master
---

# arXiv PDF Text Extraction Tool

## Problem Statement

The research agent currently can only access paper abstracts through the arXiv API. To perform deep analysis, the agent needs full-text access to papers. This plan implements `fetch_arxiv_pdf_text`, a tool that downloads arXiv PDFs and extracts their full text content for analysis.

**Core constraints:**
- arXiv Terms of Service require rate limiting (1 request per 3 seconds)
- Context window limits require text truncation (max 100k characters)
- PDF parsing must handle academic paper layouts (multi-column, equations, figures)
- Rate limiter must be added to existing `fetch_arxiv_pdf` tool (currently missing)
- Must work in pure Go (no external dependencies on Python or command-line tools)
- Integration with context manager for eviction priority

**Success criteria:**
- Tool successfully downloads and extracts text from arXiv PDFs
- Rate limiting prevents TOS violations
- Text is clean and readable (whitespace normalized, artifacts removed)
- Large papers are truncated gracefully with clear indication
- Integration tests pass with real arXiv papers
- Tool integrates seamlessly with existing research toolset

## Requirements

### Functional Requirements

**FR1: PDF Download**
- Accept arXiv ID in multiple formats (2301.00001, arXiv:2301.00001, 2301.00001v2, astro-ph/9901234)
- Normalize ID to canonical format
- Download PDF from export.arxiv.org
- Handle HTTP errors (404, 500) with clear error messages
- Store PDF temporarily during extraction, clean up after

**FR2: Text Extraction**
- Extract text from all pages by default
- Support optional max_pages parameter to limit extraction
- Handle multi-column layouts common in academic papers
- Preserve paragraph structure where possible
- Return page count for context

**FR3: Text Cleaning**
- Remove excessive whitespace (multiple spaces, excessive newlines)
- Normalize line endings
- Strip common PDF artifacts (form feeds, control characters)
- Maintain readability of extracted content

**FR4: Truncation**
- Truncate text at 100,000 characters if exceeded (approx. 25k tokens, allowing 4 papers per 100k token budget)
- Set truncated flag in result
- Return pages_extracted and total_pages for agent context
- Attempt to truncate at sentence boundary (period + space) rather than mid-word

**FR5: Rate Limiting**
- Add rate limiter to ResearchToolSet (currently missing in existing code)
- Apply rate limiter to BOTH fetch_arxiv_pdf (existing) and fetch_arxiv_pdf_text (new)
- Enforce 3-second delay between arXiv requests
- Use context-aware rate limiting (respect cancellation)
- Ensure thread-safe access (rate.Limiter is thread-safe by design)

**FR6: Error Handling**
- Distinguish recoverable errors (network issues, timeouts) from non-recoverable (invalid ID format)
- Return structured error responses compatible with tool error format
- Provide specific error types: scanned_pdf, malformed_pdf, timeout, network_error
- Handle PDF parsing failures gracefully (scanned PDFs return empty text, not error)
- Return meaningful error messages that help agent adapt strategy

### Non-Functional Requirements

**NFR1: Performance**
- Extract text from typical 10-page paper in < 5 seconds (excluding download)
- Memory usage stays under 100MB per extraction
- Temporary files cleaned up within 1 second of completion

**NFR2: Reliability**
- Handle malformed PDFs without crashing
- Degrade gracefully when PDF contains scanned images (return empty text, not error)
- Rate limiter prevents concurrent requests from violating TOS
- Test with real arXiv papers: single-column, double-column, equation-heavy, figure-heavy, scanned
- Extraction quality target: >80% readable text for born-digital PDFs

**NFR3: Maintainability**
- Use established Go PDF library (github.com/ledongthuc/pdf)
- Follow existing tool implementation patterns
- Keep extraction logic separate from handler logic for testing

## Specs

### Tool Definition

```go
func BuildFetchArxivPdfTextTool() anthropic.ToolUnionParam
```

**Tool name:** `fetch_arxiv_pdf_text`

**Input schema:**
- `arxiv_id` (string, required): arXiv identifier in any supported format
- `max_pages` (integer, optional): Maximum pages to extract (default: all pages)

**Description:** "Download and extract full text from an arXiv PDF. Returns the paper's text content for detailed analysis and summarization. Rate limited to 1 request per 3 seconds per arXiv terms of service. Text is truncated if it exceeds 100k characters."

**Output schema:**
```json
{
  "arxiv_id": "2301.00001",
  "text": "Full extracted text...",
  "total_pages": 12,
  "pages_extracted": 12,
  "truncated": false,
  "character_count": 45230
}
```

**Error schema (follows existing pattern):**
```json
{
  "error": "descriptive error message",
  "recoverable": true/false
}
```

### Handler Signature

```go
func (r *ResearchToolSet) handleFetchArxivPdfText(ctx context.Context, input json.RawMessage) (string, error)
```

**Input processing:**
1. Unmarshal JSON input to params struct
2. Validate arxiv_id is non-empty
3. Normalize and validate ID format using existing `normalizeArxivID()` helper
4. Validate max_pages if provided (must be positive)

**Execution flow:**
1. Wait for rate limiter token (`r.arxivLimiter.Wait(ctx)`)
2. Construct PDF URL: `https://export.arxiv.org/pdf/{normalized}.pdf`
3. Create temporary file with pattern `arxiv-*.pdf`
4. **Defer temp file cleanup immediately after creation** (prevents leaks on error paths)
5. Download PDF to temp file with context timeout
6. Close temp file before extraction
7. Extract text using `extractPDFText()` helper (returns text, pagesExtracted, totalPages)
8. Clean text using `cleanPDFText()` helper
9. Truncate if exceeds 100k characters (attempt sentence boundary via regex)
10. Count final characters
11. Build ArxivPdfTextResult struct
12. Marshal result to JSON

**Error scenarios:**
- Invalid input JSON → `toolError("invalid input: ...", false)`
- Empty arxiv_id → `toolError("arxiv_id is required", false)`
- Invalid ID format → `toolError("invalid arXiv ID format: ...", false)`
- Rate limiter error → `toolError("rate limiter error: ...", true)`
- HTTP 404 → `toolError("PDF not found for arXiv ID: ...", true)`
- HTTP 5xx → `toolError("PDF download returned status ...", true)`
- Network error → `toolError("PDF download failed: ...", true)`
- PDF parsing error → `toolError("PDF text extraction failed: ...", true)`

### Helper Functions

**extractPDFText(pdfPath string, maxPages int) (text string, pagesExtracted int, totalPages int, err error)**

Implementation using github.com/ledongthuc/pdf:
1. Open PDF file with `pdf.Open(pdfPath)` (returns *os.File, *pdf.Reader, error)
2. Check error before deferring f.Close() (avoid nil pointer)
3. Get total page count: `r.NumPage()`
4. Determine pages to read (min of totalPages and maxPages if specified)
5. Iterate from page 1 to pagesExtracted (PDF uses 1-based indexing)
6. For each page: call `page.GetPlainText(nil)` (nil for default options)
7. Skip pages that fail to extract (continue loop, don't error - scanned pages return empty)
8. Append text for each page to strings.Builder with "\n\n" separator
9. Return builder contents, pagesExtracted, totalPages

**Note on API**: Verify github.com/ledongthuc/pdf API during implementation. The plan assumes:
- `pdf.Open(path)` returns `(*os.File, *pdf.Reader, error)`
- `reader.NumPage()` returns total pages
- `reader.Page(n)` returns page struct with `GetPlainText(opts)` method

**cleanPDFText(text string) string**

Text normalization using pre-compiled regexes:
1. Package-level vars: `var (multiSpace = regexp.MustCompile(\`\\s+\`); multiNewline = regexp.MustCompile(\`\\n\\s*\\n\\s*\\n+\`))`
2. Replace multiple spaces with single space: `multiSpace.ReplaceAllString(text, " ")`
3. Replace 3+ consecutive newlines with double newline: `multiNewline.ReplaceAllString(text, "\\n\\n")`
4. Split on newlines, trim each line
5. Trim overall result

### Rate Limiter Integration

**Update ResearchToolSet struct:**
```go
type ResearchToolSet struct {
    client       *http.Client
    arxivLimiter *rate.Limiter
}
```

**Update constructor (NO SIGNATURE CHANGE):**
```go
func NewResearchToolSet(client *http.Client) *ResearchToolSet {
    return &ResearchToolSet{
        client:       client,
        arxivLimiter: rate.NewLimiter(rate.Every(3*time.Second), 1),
    }
}
```

**IMPORTANT**: Constructor signature remains `(client *http.Client)` - does NOT add github_token parameter. That parameter is for stage5-02 (GitHub search tool), not this feature. Existing call sites in cmd/research-demo/main.go are unchanged.

**Update existing fetch_arxiv_pdf handler:**

Add rate limiter wait at the beginning of `handleFetchArxivPdf()`:
```go
if err := r.arxivLimiter.Wait(ctx); err != nil {
    return toolError("rate limiter error: "+err.Error(), true), nil
}
```

### Tool Registration

Update `tools/research/research.go` Register method:
```go
func (r *ResearchToolSet) Register(reg *registry.ToolRegistry) {
    reg.Register("search_arxiv", BuildSearchArxivTool(), r.handleSearchArxiv)
    reg.Register("search_openalex", BuildSearchOpenAlexTool(), r.handleSearchOpenAlex)
    reg.Register("fetch_arxiv_pdf", BuildFetchArxivPdfTool(), r.handleFetchArxivPdf)
    reg.Register("fetch_arxiv_pdf_text", BuildFetchArxivPdfTextTool(), r.handleFetchArxivPdfText)
}
```

## Contracts

### Module Boundaries

**tools/research package exports:**
- `BuildFetchArxivPdfTextTool() anthropic.ToolUnionParam` - Tool definition for registry
- `ResearchToolSet.Register(*registry.ToolRegistry)` - Modified to include new tool

**tools/research package internal:**
- `handleFetchArxivPdfText(context.Context, json.RawMessage) (string, error)` - Handler implementation
- `extractPDFText(string, int) (string, int, int, error)` - PDF extraction logic (returns text, pagesExtracted, totalPages, error)
- `cleanPDFText(string) string` - Text normalization
- `ArxivPdfTextResult` struct - Result type for JSON marshaling
- Package-level regex vars: `multiSpace`, `multiNewline` - Pre-compiled regexes for text cleaning

**Dependencies:**
- `github.com/ledongthuc/pdf` - PDF parsing (new dependency, verify API during implementation)
- `golang.org/x/time/rate` - Rate limiting (new dependency)
- Existing `normalizeArxivID()` helper from handlers.go
- Existing `toolError()` helper from research.go
- Existing `contains()` helper for tests (need to extract to shared test utility or duplicate)

### Data Contracts

**Input validation contract:**
- Empty arxiv_id → non-recoverable error
- Invalid format arxiv_id → non-recoverable error
- Negative max_pages → non-recoverable error
- Network/timeout errors → recoverable error
- PDF not found (404) → recoverable error (paper may not have PDF)

**Output contract:**
- Always returns valid JSON (either result or error object)
- text field contains printable UTF-8 characters
- total_pages always >= 0
- pages_extracted always <= total_pages
- character_count reflects final text length (after truncation if applicable)
- truncated flag accurately reflects whether text was cut
- If scanned PDF (no extractable text): returns empty string for text, with pages info showing extraction was attempted

**Rate limiting contract:**
- Rate limiter shared between fetch_arxiv_pdf and fetch_arxiv_pdf_text
- Maximum 1 request per 3 seconds to any export.arxiv.org endpoint
- Respects context cancellation during rate limit wait

### Testing Contracts

**Unit tests must verify:**
- Input validation (empty, invalid formats)
- Text cleaning (whitespace, newlines, artifacts)
- Truncation logic (boundary at 100k chars)
- Error response format (JSON structure, recoverable flag)

**Integration tests must verify:**
- Rate limiting between consecutive calls (use fast test limiter: 100ms instead of 3s)
- Actual PDF download and extraction with known paper (use httptest.Server to mock arXiv)
- Shared rate limiter coordination: call fetch_arxiv_pdf then fetch_arxiv_pdf_text, verify 2nd waits
- Temporary file cleanup (check file doesn't exist after call completes)
- Test with diverse PDF types: single-column, double-column (most common), equation-heavy
- Measure extraction quality: compare extracted text to known good sample (>80% match for born-digital)

**Test coverage target:** > 80% for handlers.go additions

**Test helpers needed:**
- Extract `contains()` helper from fetch_arxiv_pdf_test.go to shared test utilities
- OR duplicate helper in new test file
- Create httptest.Server that serves sample PDF for integration test

## Decisions & Tradeoffs

### Decision 1: Pure Go PDF library vs external tools

**Chosen:** `github.com/ledongthuc/pdf` pure Go library *with extraction quality validation*

**Alternatives considered:**
1. Python script with PyPDF2 (requires subprocess, cross-language complexity)
2. pdftotext command-line tool (requires external binary, portability issues)
3. github.com/pdfcpu/pdfcpu (heavier, more features than needed, but better layout handling)
4. github.com/unidoc/unipdf (commercial, better quality, but licensing cost)
5. Claude PDF upload feature (simpler but requires API support, higher latency)

**Rationale:** Pure Go keeps deployment simple, no external dependencies. The ledongthuc/pdf library is lightweight and sufficient for text extraction. However, given PM concern about extraction quality on multi-column papers (most academic papers), we MUST validate with real papers before shipping.

**Validation requirement**: Test extraction on 5 diverse arXiv papers before implementation:
- Single-column: arXiv:2301.00001 (recent)
- IEEE double-column: arXiv:1706.03762 (Attention Is All You Need)
- Equation-heavy: arXiv math paper
- Figure-heavy: arXiv cs.CV paper
- Older format: arXiv astro-ph paper

If extraction quality < 80% readable for born-digital papers, escalate to use pdfcpu or unidoc.

**Tradeoff:** May miss text in scanned PDFs (return empty string, not error). Multi-column may have interleaved text. Accept for MVP, can upgrade library later without API changes.

### Decision 2: Truncation at 100k characters with sentence boundary

**Chosen:** Truncation at 100,000 characters, attempt to break at sentence boundary

**Alternatives considered:**
1. Truncate by page count (e.g., first 20 pages) - cleaner but unpredictable for context budget
2. Intelligent truncation (extract first 5 + last 2 pages) - better coverage but more complex
3. Summarize long papers automatically - adds AI cost and latency
4. No truncation (risk context overflow) - dangerous
5. Let agent specify page range - requires agent to know page count beforehand

**Rationale:** Character-based limit is predictable for context management. 100k chars ≈ 25k tokens. With 200k token budget, allows ~4 papers plus conversation history. Sentence boundary truncation (find last ". " before limit) improves readability.

**Addressing PM feedback**: The 100k limit means a 50-page survey paper gets ~25 pages. Agent gets `truncated: true` and `pages_extracted: "1-25"` to understand context. For MVP, this is acceptable. Future enhancement could add intelligent section extraction (intro + conclusion).

**Tradeoff:** May still cut mid-sentence if no sentence boundary found in last 1k chars. Accept this edge case for simplicity.

### Decision 3: Shared rate limiter across tools

**Chosen:** Single rate.Limiter instance on ResearchToolSet, shared by fetch_arxiv_pdf and fetch_arxiv_pdf_text

**Alternatives considered:**
1. Separate limiters per tool (allows 2 req/3sec total)
2. Global package-level limiter (harder to test)
3. Per-request delay without limiter (not context-aware)

**Rationale:** Shared limiter ensures compliance with TOS regardless of which tools agent uses. Single limiter on struct is testable and follows Go patterns.

**Tradeoff:** Slightly more conservative than necessary (TOS is per download, not per API endpoint), but safer and simpler.

### Decision 4: Temporary file cleanup

**Chosen:** defer os.Remove() immediately after temp file creation

**Alternatives considered:**
1. Keep temp files for debugging (uses disk space)
2. Clean up in separate goroutine (risks leaks on panic)
3. Use in-memory buffer (high memory usage for large PDFs)

**Rationale:** defer ensures cleanup even on error paths. Temp file is only needed during extraction. PDF library requires file path, so can't use in-memory buffer.

**Tradeoff:** If process crashes during extraction, temp file may leak. Accept this risk for simplicity. OS will clean /tmp periodically.

### Decision 5: Update existing fetch_arxiv_pdf handler

**Chosen:** Modify handleFetchArxivPdf to use shared rate limiter in same commit as new tool

**Alternatives considered:**
1. Don't rate limit fetch_arxiv_pdf (inconsistent with TOS)
2. Add rate limiting wrapper around both handlers (extra complexity)
3. Split into two PRs: first adds rate limiter, second adds new tool (safer sequencing)

**Rationale:** Both tools hit export.arxiv.org, so both must respect rate limit. Staff engineer feedback suggests splitting PRs for safety, but both changes are small and atomic. Keeping in one PR ensures rate limiter is added and used in a single change, reducing risk of forgetting Step 3.

**Addressing staff engineer concern**: The modification to fetch_arxiv_pdf is a one-line addition at the start of the handler. Tests will verify rate limiting works for both tools. If tests fail due to timeouts, we'll adjust test timeouts in the same commit.

**Tradeoff:** Changes behavior of existing tool (adds 3-second delay). This is correct behavior per TOS, so acceptable. Existing users may notice slower responses but won't violate TOS.

### Decision 6: Do NOT add github_token parameter

**Chosen:** Keep constructor as `NewResearchToolSet(client *http.Client)`, no github_token

**Rationale (staff engineer feedback):** The original stage5-master plan mentioned github_token for the GitHub search tool (stage5-02), but this plan (stage5-03) only adds PDF text extraction. Adding an unused parameter is code smell and breaks existing call sites without benefit.

**When to add**: If/when stage5-02 (GitHub search tool) is implemented, that plan will add github_token to the constructor. For now, keep it simple.

**Tradeoff:** None. This is just removing unnecessary complexity from the plan.

### Decision 7: Build this AFTER stage5-04 (planning tool), not before

**Chosen:** Defer implementation until after create_research_plan tool is built

**Rationale (PM feedback):** The master plan shows this tool is part of a structured research workflow. Without understanding the workflow (what the planning tool validates), we might optimize for the wrong use case. For example:
- Should the tool auto-summarize to save context?
- Should it extract specific sections (intro + conclusion)?
- How many papers will the agent typically fetch per session?

Building the planning tool first (stage5-04) will answer these questions and inform PDF extraction design.

**Alternative approach:** Build PDF extraction now as MVP, iterate based on workflow. This is riskier because API changes are harder after launch.

**Addressing PM concern about sequencing**: This plan provides complete specs for PDF extraction, but implementation should wait for workflow clarity. Mark this plan as "READY FOR IMPLEMENTATION AFTER STAGE5-04".

**Tradeoff:** Delays PDF extraction feature, but reduces risk of building wrong thing. Engineers can review this plan now, implementation happens in correct sequence.

### Decision 8: Add observability for extraction quality

**Chosen:** Log structured metrics for each extraction: success/failure, extraction_time_ms, character_count, pages_extracted, pages_total, truncated

**Rationale (PM feedback):** We need to measure extraction quality in production. Metrics let us:
- Identify papers with poor extraction (multi-column issues)
- Track truncation frequency (context cost awareness)
- Monitor performance (slow extractions indicate problems)

**Implementation**: Add structured logging to handler after extraction completes. Use existing trace infrastructure.

**Tradeoff:** Slight performance overhead for logging (~1ms), but critical for production monitoring.

### Decision 9: No caching for MVP

**Chosen:** No caching of extracted text in MVP

**Alternatives considered:**
1. In-memory LRU cache (10 papers) - helps with repeated queries
2. Redis cache - persistent across sessions
3. Pre-extract top 100 papers - fastest for common papers

**Rationale (PM feedback):** Caching is valuable for production, but adds complexity to MVP. Need to handle cache invalidation (what if paper is updated?), memory management, cache key design (arxiv_id + version?). Defer until we see usage patterns.

**When to add**: After MVP ships, monitor which papers are fetched repeatedly. If >30% of requests are duplicates, add LRU cache.

**Tradeoff:** Duplicate requests waste bandwidth and time. Accept for MVP simplicity.

### Decision 10: Add result struct fields for better agent context

**Chosen:** ArxivPdfTextResult includes total_pages, pages_extracted, character_count, truncated

**Rationale (PM and engineer feedback):** Agent needs context to make decisions:
- If truncated=true but pages_extracted=45/48, agent knows it got most of the paper
- If pages_extracted=10 but character_count=5000, agent knows extraction quality is poor
- character_count helps agent estimate context cost for next turn

**Addressing PM UX concern**: These fields let agent give better feedback to user ("I analyzed the first 24 pages of 48") rather than generic "I read the paper".

**Tradeoff:** Slightly larger JSON response (~50 bytes). Negligible cost for significant UX improvement.

## Implementation Order

**IMPORTANT**: This plan is ready for review, but implementation should occur AFTER stage5-04 (planning tool) to ensure workflow clarity. Mark as "IMPLEMENTATION BLOCKED ON STAGE5-04".

### Phase 0: PDF Library API Validation (30 min, REQUIRED BEFORE Phase 1)
1. Create throwaway Go script to test github.com/ledongthuc/pdf API
2. Download sample arXiv PDFs: 1706.03762 (double-column), 2301.00001 (recent)
3. Verify API: `pdf.Open()` signature, `Reader.NumPage()`, `Page.GetPlainText()`
4. Measure extraction quality: compare output to manual PDF reading
5. **Decision point**: If quality < 80% readable, escalate to use pdfcpu or unidoc before proceeding

### Phase 1: Dependencies and structure (15 min)
6. Add dependencies to go.mod: `go get github.com/ledongthuc/pdf golang.org/x/time/rate`
7. Run `go mod tidy`
8. Verify golang.org/x/time is now in go.mod
9. Add package-level regex vars to handlers.go: `multiSpace`, `multiNewline` with MustCompile
10. Update ResearchToolSet struct to include arxivLimiter field
11. Update NewResearchToolSet constructor to initialize rate limiter (no signature change, no github_token)
12. Verify build: `make build`

### Phase 2: Helper functions (45 min)
13. Add necessary imports to handlers.go: `os`, `io`, `strings`, `time`, `github.com/ledongthuc/pdf`, `golang.org/x/time/rate`, `regexp`
14. Implement `extractPDFText(path, maxPages)` returning (text, pagesExtracted, totalPages, error)
15. Add nil check for file before defer f.Close() to avoid nil pointer panic
16. Implement `cleanPDFText()` using pre-compiled package-level regexes (multiSpace, multiNewline)
17. Create new test file: `tools/research/fetch_arxiv_pdf_text_test.go`
18. Copy `contains()` helper from fetch_arxiv_pdf_test.go to new test file (duplicate it)
19. Write unit tests for cleanPDFText (whitespace, newlines, edge cases)
20. Write unit test for extractPDFText signature (mock PDF if feasible, or skip for integration)
21. Run tests: `go test ./tools/research -run TestClean`

### Phase 3: Tool definition (10 min)
22. Add `ArxivPdfTextResult` struct to handlers.go with fields: arxiv_id, text, total_pages, pages_extracted, character_count, truncated
23. Implement `BuildFetchArxivPdfTextTool()` in definitions.go
24. Verify schema compiles with `go build ./tools/research`

### Phase 4: Handler implementation (60 min, includes rate limiter update atomically)
25. Implement `handleFetchArxivPdfText()` in handlers.go with rate limiter wait first
26. Create temp file, defer cleanup IMMEDIATELY (before download) to prevent leaks
27. Implement PDF download with context timeout
28. Close file before extraction (PDF library needs closed file)
29. Call extractPDFText and cleanPDFText
30. Implement truncation with sentence boundary: find last ". " in last 1k chars of text
31. Count final characters
32. Build ArxivPdfTextResult with all fields
33. Marshal to JSON, return
34. **ATOMICALLY in same commit**: Update `handleFetchArxivPdf()` to add rate limiter wait at start (one line addition)
35. Verify both handlers now have rate limiter wait by inspecting code

### Phase 5: Registration and observability (15 min)
36. Update Register() method in research.go to add: `reg.Register("fetch_arxiv_pdf_text", BuildFetchArxivPdfTextTool(), r.handleFetchArxivPdfText)`
37. Add structured logging in handler after extraction: log success/failure, extraction_time_ms, character_count, pages_extracted, pages_total, truncated
38. Verify tool registry compiles: `make build`
39. Check that tool appears in registry (manual test or log inspection)

### Phase 6: Testing (90 min)
40. Add necessary imports to test file: `context`, `encoding/json`, `net/http`, `net/http/httptest`, `os`, `testing`, `time`, `strings`, `golang.org/x/time/rate`
41. Write input validation tests (empty ID, invalid format, negative max_pages)
42. Write text cleaning tests (from Phase 2, expand if needed)
43. Write truncation tests: under limit, over limit, sentence boundary, no sentence in last 1k
44. Write rate limiting test: create toolset with fast limiter (100ms), make consecutive calls, verify delay
45. Write shared rate limiter test: verify calling fetch_arxiv_pdf then fetch_arxiv_pdf_text uses same limiter
46. Create httptest.Server serving sample PDF (embed small test PDF or generate minimal valid PDF)
47. Write integration test: download and extract with mocked arXiv server
48. Write temp file cleanup test: verify file doesn't exist after handler returns (os.Stat returns IsNotExist)
49. Run all tests: `make test-go`
50. Verify test coverage > 80%: `go test ./tools/research -cover`
51. Fix any compilation errors (missing imports, undefined helpers)

### Phase 7: Verification (20 min)
52. Run `make build` to ensure no regressions (all packages compile)
53. Run `make lint` to check code quality (no vet errors)
54. Run `make test` for full test suite (including Python tests)
55. Check existing fetch_arxiv_pdf tests still pass (rate limiter didn't break them)
56. Verify no compilation errors in cmd/research-demo (call site unchanged, constructor signature unchanged)
57. Manual test with research-demo if available (may need to wait for stage5-04 system prompt)

### Phase 8: Quality validation (30 min, REQUIRED BEFORE PR)
58. Test extraction on 5 diverse arXiv papers (from Decision 1):
    - Single-column: arXiv:2301.00001
    - Double-column: arXiv:1706.03762 (Attention Is All You Need)
    - Equation-heavy: arXiv math paper
    - Figure-heavy: arXiv cs.CV paper
    - Older format: arXiv astro-ph paper
59. Measure readability: is text coherent? Are columns interleaved badly?
60. Document any issues found in PR description or plan revision
61. **Decision point**: If quality issues found, assess severity and decide to ship vs improve

**Total estimated time:** 5-6 hours (includes validation phases)

**Critical path:** Phase 0 blocks everything (API validation). Phase 4 depends on Phases 2-3. Phase 8 blocks PR.

**Parallel opportunities:** None for single developer. For team: one dev does Phase 0, another reviews existing code patterns.

**Atomicity note (addressing staff engineer concern)**: Phase 4 step 34 updates existing handler atomically with new handler to prevent forgetting rate limiter. Phase 5 step 36 registers the tool. Both critical steps are explicit in the sequence.

**Compilation safety**: All phases include verification steps (build, test) to catch errors early. Phase 7 step 56 explicitly verifies call sites unchanged.

## Open Questions

**Q1: Should we support OCR fallback for scanned PDFs?**

Answer: No, not in initial implementation. Most arXiv papers are born-digital. If we encounter scanned papers, extractPDFText will return empty text with pages_extracted/total_pages showing extraction was attempted, which is a valid outcome. We can add OCR in a future enhancement if metrics show high frequency of scanned papers.

**Q2: Should max_pages count from the beginning or allow range selection?**

Answer: Count from beginning only for MVP. Range selection (e.g., pages 5-10) adds complexity. PM feedback suggests intelligent section extraction (intro + conclusion) as future enhancement. For MVP, agent can use full text and reference page numbers in analysis. Revisit after stage5-04 (planning tool) shows actual usage patterns.

**Q3: Should we validate PDF content before extraction (file size, page count)?**

Answer: No pre-validation. Download and extract, then handle errors. This keeps the implementation simple. If PDF is too large, extraction will be slow but won't break. Truncation handles text size. Metrics will show if large PDFs are common, then we can add size limit.

**Q4: Should rate limiter be configurable?**

Answer: No, hardcode 3 seconds per TOS. Making it configurable invites misconfiguration. If TOS changes, we update the code. rate.Limiter is testable with custom limiter in tests (100ms), so no configuration needed for testing.

**Q5: How should we handle PDFs with non-extractable text (images, scanned pages)?**

Answer: extractPDFText skips pages that fail to extract (continues loop, doesn't error). If all pages fail, result will have empty text field with total_pages > 0 and pages_extracted showing how many were attempted. Agent can detect empty text and handle appropriately (fall back to abstract). This is better than returning error because partial extraction is useful.

**Q6: Should we add a feature flag for safe rollout?** (NEW from PM feedback)

Answer: Yes, consider adding ENABLE_PDF_TEXT_EXTRACTION env var for production rollout. This allows disabling the tool without redeploying if extraction quality is poor or causes issues. Implementation detail: check env var in Register() method, skip registering tool if disabled. Document in deployment guide.

**Q7: Should we add cost guidance to system prompt?** (NEW from PM feedback)

Answer: Defer to stage5-04 (planning tool) implementation. That plan will define the full system prompt including tool usage guidance. This plan provides the tool API, workflow guidance happens in planning tool plan.

**Q8: Should we integrate with context manager for eviction priority?** (NEW from PM feedback)

Answer: Defer to future enhancement. Context manager eviction is a global concern across all tools. Adding PDF-specific eviction priority adds complexity without clear benefit until we measure context pressure in production. If metrics show context overflow, add priority markers then.

## Revision Log

**2026-05-11 - Initial:** Initial plan created based on existing stage5-03 document. Structured as formal implementation plan with complete specs, contracts, and decision rationale.

**2026-05-11 - Round 1 revision:** Applied feedback from dual critique (staff engineer + PM perspectives):
- Fixed critical error: rate limiter doesn't exist in current code, must be added
- Removed github_token parameter (unused, from different feature)
- Added extraction quality validation requirement (Phase 0)
- Improved result struct: added pages_extracted, character_count for agent context
- Enhanced error handling: specific error types for better agent adaptation
- Added sentence boundary truncation (attempted, not guaranteed)
- Pre-compiled regexes for efficiency
- Added observability: structured logging for metrics
- Clarified implementation sequencing: build after stage5-04 for workflow clarity
- Expanded testing requirements: shared rate limiter test, extraction quality validation
- Updated time estimate: 5-6 hours (was 3-4) to account for validation phases
- Added Decisions 6-10 addressing key feedback points
- Marked plan as blocked on stage5-04 for proper sequencing

**2026-05-11 - Round 2 revision:** Applied feedback from second critique:
- Clarified imports: added explicit step (Phase 2 step 13) to add all necessary imports to handlers.go
- Clarified test file creation: added explicit step (Phase 2 step 17) to create new test file
- Clarified contains() helper: duplicate from existing test file (Phase 2 step 18)
- Clarified test imports: added explicit step (Phase 6 step 40) with full import list for test file
- Renumbered steps for clarity after additions (steps now run 1-61)
- Added verification step for unchanged call sites (Phase 7 step 56)
- Added note emphasizing NO constructor signature change (Decision 6, constructor spec)
- Added compilation safety note to implementation order summary
- PM verdict: GREENLIGHT (sequencing, UX, observability concerns addressed)
- Staff engineer round 2: noted registration step exists (Phase 5 step 36), constructor unchanged
