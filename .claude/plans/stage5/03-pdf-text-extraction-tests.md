---
name: arXiv PDF Text Extraction Tool - Test Plan
description: Test strategy and verification for fetch_arxiv_pdf_text tool implementation
type: test-plan
parent: 03-pdf-text-extraction
---

# Test Plan: arXiv PDF Text Extraction Tool

## Test Strategy

This test plan validates the `fetch_arxiv_pdf_text` tool implementation across three levels:
1. **Unit tests**: Individual functions in isolation (cleanPDFText, extractPDFText signature)
2. **Integration tests**: Handler with mocked HTTP, rate limiting, file cleanup
3. **Quality validation**: Real arXiv PDFs to measure extraction quality

## Unit Tests

### Test Suite: cleanPDFText

**File**: `tools/research/fetch_arxiv_pdf_text_test.go`

**Test cases**:

1. **TestCleanPDFText_MultipleSpaces**
   - Input: `"hello    world    foo"`
   - Expected: `"hello world foo"`
   - Verifies: `multiSpace` regex replaces multiple spaces with single space

2. **TestCleanPDFText_MultipleNewlines**
   - Input: `"line1\n\n\n\nline2"`
   - Expected: `"line1\n\nline2"`
   - Verifies: `multiNewline` regex replaces 3+ newlines with double newline

3. **TestCleanPDFText_MixedWhitespace**
   - Input: `"  hello  \n\n  world  "`
   - Expected: `"hello\n\nworld"`
   - Verifies: Trim whitespace from lines and overall result

4. **TestCleanPDFText_EmptyString**
   - Input: `""`
   - Expected: `""`
   - Verifies: No panic on empty input

5. **TestCleanPDFText_OnlWhitespace**
   - Input: `"     \n\n\n   "`
   - Expected: `""`
   - Verifies: Reduces pure whitespace to empty string

### Test Suite: ArxivPdfTextResult marshaling

**File**: `tools/research/fetch_arxiv_pdf_text_test.go`

**Test cases**:

1. **TestArxivPdfTextResult_JSONMarshaling**
   - Create result struct with all fields populated
   - Marshal to JSON
   - Verify JSON contains all expected fields
   - Verify field types are correct (truncated is bool, pages are int, etc.)

## Integration Tests

### Test Suite: Input Validation

**File**: `tools/research/fetch_arxiv_pdf_text_test.go`

**Test cases**:

1. **TestHandleFetchArxivPdfText_EmptyArxivID**
   - Input: `{"arxiv_id": ""}`
   - Expected: Error response with `"recoverable": false`
   - Expected error message: `"arxiv_id is required"`

2. **TestHandleFetchArxivPdfText_InvalidArxivIDFormat**
   - Input: `{"arxiv_id": "invalid-format"}`
   - Expected: Error response with `"recoverable": false`
   - Expected error message contains `"invalid arXiv ID format"`

3. **TestHandleFetchArxivPdfText_NegativeMaxPages**
   - Input: `{"arxiv_id": "1706.03762", "max_pages": -5}`
   - Expected: Error response with `"recoverable": false`
   - Expected error message about invalid max_pages

4. **TestHandleFetchArxivPdfText_ValidInputs**
   - Input: `{"arxiv_id": "1706.03762"}`
   - Expected: Proceeds to PDF download (verified by subsequent steps)
   - Input: `{"arxiv_id": "arXiv:1706.03762v5"}`
   - Expected: Normalizes and proceeds

### Test Suite: Rate Limiting

**File**: `tools/research/fetch_arxiv_pdf_text_test.go`

**Test cases**:

1. **TestArxivRateLimiting_ConsecutiveCalls**
   - Setup: Create toolset with fast limiter (100ms interval)
   - Call: `handleFetchArxivPdfText` with same arxiv_id twice
   - Measure: Time between start of call 1 and start of call 2
   - Expected: Second call delayed by ~100ms (check elapsed > 90ms)
   - Note: Mock PDF download to avoid network dependency

2. **TestArxivRateLimiting_SharedBetweenTools**
   - Setup: Create toolset with fast limiter (100ms interval)
   - Call: `handleFetchArxivPdf` (existing tool)
   - Call: `handleFetchArxivPdfText` (new tool)
   - Measure: Time between calls
   - Expected: Second call delayed by ~100ms
   - Verifies: Both tools share same rate limiter instance

3. **TestArxivRateLimiting_ContextCancellation**
   - Setup: Create toolset with slow limiter (3s interval)
   - Call: `handleFetchArxivPdfText` once (consumes token)
   - Create context with 100ms timeout
   - Call: `handleFetchArxivPdfText` with timeout context
   - Expected: Returns rate limiter error quickly (< 200ms), doesn't wait 3s
   - Verifies: Rate limiter respects context cancellation

### Test Suite: PDF Download and Extraction

**File**: `tools/research/fetch_arxiv_pdf_text_test.go`

**Test cases**:

1. **TestHandleFetchArxivPdfText_HTTPMock_Success**
   - Setup: httptest.Server serving small valid PDF (embedded or generated)
   - Input: Valid arxiv_id pointing to mock server
   - Expected: Result with text extracted, total_pages > 0, truncated = false
   - Verifies: Full download and extraction flow

2. **TestHandleFetchArxivPdfText_HTTPMock_404NotFound**
   - Setup: httptest.Server returning 404
   - Input: Valid arxiv_id
   - Expected: Error response with `"recoverable": true`
   - Expected error message: `"PDF not found for arXiv ID"`

3. **TestHandleFetchArxivPdfText_HTTPMock_500ServerError**
   - Setup: httptest.Server returning 500
   - Input: Valid arxiv_id
   - Expected: Error response with `"recoverable": true`
   - Expected error message contains status code

4. **TestHandleFetchArxivPdfText_HTTPMock_NetworkTimeout**
   - Setup: httptest.Server with slow handler (>5s delay)
   - Input: Valid arxiv_id with short context timeout
   - Expected: Error response with `"recoverable": true`
   - Expected error message: `"PDF download failed"`

### Test Suite: Text Truncation

**File**: `tools/research/fetch_arxiv_pdf_text_test.go`

**Test cases**:

1. **TestTruncation_UnderLimit**
   - Setup: Mock extraction returns 50k characters
   - Expected: Result has full text, truncated = false, character_count = 50000

2. **TestTruncation_OverLimit**
   - Setup: Mock extraction returns 150k characters
   - Expected: Result text length = 100000, truncated = true, character_count = 100000

3. **TestTruncation_SentenceBoundary**
   - Setup: Mock extraction returns 101k characters with ". " at position 99500
   - Expected: Text truncated at 99501 (after period and space), truncated = true
   - Verifies: Sentence boundary search in last 1000 chars

4. **TestTruncation_NoSentenceBoundary**
   - Setup: Mock extraction returns 101k characters, no ". " in last 1000 chars
   - Expected: Text truncated at exactly 100000, truncated = true
   - Verifies: Falls back to hard truncation

### Test Suite: Temporary File Cleanup

**File**: `tools/research/fetch_arxiv_pdf_text_test.go`

**Test cases**:

1. **TestTempFileCleanup_Success**
   - Setup: httptest.Server with valid PDF
   - Call: `handleFetchArxivPdfText`
   - After call: Check /tmp for arxiv-*.pdf files created during test
   - Expected: No temp files remain (os.Stat returns IsNotExist)

2. **TestTempFileCleanup_ErrorPath**
   - Setup: httptest.Server returning 404
   - Call: `handleFetchArxivPdfText`
   - After call: Check /tmp for arxiv-*.pdf files
   - Expected: No temp files remain even on error path
   - Verifies: defer cleanup works on error

3. **TestTempFileCleanup_ExtractionError**
   - Setup: Mock PDF that fails parsing
   - Call: `handleFetchArxivPdfText`
   - Expected: No temp files remain
   - Verifies: cleanup happens even if PDF extraction fails

### Test Suite: Result Structure

**File**: `tools/research/fetch_arxiv_pdf_text_test.go`

**Test cases**:

1. **TestResultStructure_AllFields**
   - Setup: Mock successful extraction
   - Call: `handleFetchArxivPdfText`
   - Parse result JSON
   - Expected: Contains arxiv_id, text, total_pages, pages_extracted, character_count, truncated
   - Verify: pages_extracted <= total_pages
   - Verify: character_count = len(text)

2. **TestResultStructure_EmptyText**
   - Setup: Mock extraction returns empty text (scanned PDF case)
   - Expected: Result with empty string for text, total_pages > 0, pages_extracted = total_pages
   - Verifies: Scanned PDFs return valid result, not error

## Quality Validation Tests

### Manual Test Suite: Real arXiv Papers

**Execution**: Manual testing before PR, documented in PR description

**Test cases**:

1. **QV1: Single-column modern paper**
   - arXiv ID: 2301.00001
   - Download and extract with real tool
   - Manual review: Read extracted text, compare to PDF
   - Measure: >80% of text readable and coherent
   - Record: Issues with equations, figures, special characters

2. **QV2: Double-column IEEE format**
   - arXiv ID: 1706.03762 (Attention Is All You Need)
   - Download and extract
   - Manual review: Check if columns are interleaved or sequential
   - Measure: >80% readable
   - Record: Column handling quality

3. **QV3: Equation-heavy math paper**
   - arXiv ID: Find a math paper with heavy LaTeX equations
   - Download and extract
   - Manual review: How are equations represented in plain text?
   - Measure: Text around equations is readable
   - Record: Equation extraction quality (may be poor, acceptable)

4. **QV4: Figure-heavy CV paper**
   - arXiv ID: Find cs.CV paper with many figures
   - Download and extract
   - Manual review: Are figure captions extracted? Is text around figures coherent?
   - Measure: Main text is readable
   - Record: Figure caption handling

5. **QV5: Old format astro-ph paper**
   - arXiv ID: astro-ph/9901234 or similar old-style ID
   - Download and extract
   - Manual review: Does old format extract correctly?
   - Measure: >80% readable
   - Record: Any format-specific issues

**Acceptance criteria**: At least 4 out of 5 papers achieve >80% readability. If <4 pass, escalate to use pdfcpu or unidoc library.

## Regression Tests

### Existing Tool Compatibility

**File**: Run existing test suite

**Test cases**:

1. **TestFetchArxivPdf_StillWorks**
   - Run: `go test ./tools/research -run TestFetch.*Pdf -v`
   - Expected: All existing tests pass
   - Verifies: Rate limiter addition didn't break existing tool

2. **TestSearchArxiv_Unchanged**
   - Run: `go test ./tools/research -run TestSearch -v`
   - Expected: All search tests pass
   - Verifies: No unintended side effects

3. **TestResearchToolSetConstruction**
   - Create: `research.NewResearchToolSet(httpClient)` in cmd/research-demo
   - Expected: Compiles without error
   - Verifies: Constructor signature unchanged, call sites work

## Performance Tests

### Optional (not required for MVP)

**Test cases**:

1. **PerfTest_10PagePaper**
   - Measure: Time to extract 10-page paper
   - Expected: < 5 seconds (excluding download time)
   - Verifies: NFR1 performance requirement

2. **PerfTest_MemoryUsage**
   - Measure: Peak memory during 50-page extraction
   - Expected: < 100MB
   - Verifies: NFR1 memory requirement

## Test Coverage Goals

- **Unit tests**: 100% coverage for cleanPDFText, ArxivPdfTextResult marshaling
- **Integration tests**: >80% coverage for handleFetchArxivPdfText
- **Overall package**: Maintain or improve existing coverage (check baseline first)

**Verification**:
```bash
go test ./tools/research -cover
```

## Test Execution Order

1. **Phase 1: Unit tests** (run during Phase 2 of implementation)
   - cleanPDFText tests
   - Result marshaling tests

2. **Phase 2: Integration tests** (run during Phase 6 of implementation)
   - Input validation
   - Rate limiting
   - HTTP mocking
   - Truncation
   - File cleanup
   - Result structure

3. **Phase 3: Regression tests** (run during Phase 7 of implementation)
   - Existing tool tests
   - Build verification

4. **Phase 4: Quality validation** (run during Phase 8 of implementation)
   - Manual testing with 5 real papers
   - Document findings

## Test Infrastructure Needs

**Mock artifacts**:
- Small valid PDF (10KB) for httptest.Server
  - Option 1: Embed base64-encoded minimal PDF
  - Option 2: Generate programmatically using minimal PDF spec
  - Option 3: Include sample PDF in testdata/ directory

**Helper functions to extract/create**:
- `contains(s, substr string) bool` - copied from fetch_arxiv_pdf_test.go
- `setupMockArxivServer(pdfContent []byte) *httptest.Server` - returns server serving PDF at /pdf/{id}.pdf

**Test data**:
- Sample arXiv IDs for different formats: 2301.00001, 1706.03762, astro-ph/9901234, 1706.03762v5

## Success Criteria

All tests must pass before PR:
- [ ] All unit tests pass
- [ ] All integration tests pass
- [ ] All regression tests pass
- [ ] Test coverage > 80% for new code
- [ ] At least 4/5 quality validation papers achieve >80% readability
- [ ] No memory leaks (temp files cleaned up)
- [ ] Rate limiting works across both tools
- [ ] Existing tools unaffected

## Known Limitations to Accept

These are expected failures that don't block PR:
- Scanned PDFs return empty text (not an error)
- Multi-column papers may have interleaved text (acceptable if >80% readable overall)
- Equations may be garbled (acceptable, focus is on prose)
- Figure captions may be missing (acceptable)
- Table contents may be mangled (acceptable)

These limitations are documented in the main plan's Open Questions and tradeoff decisions.
