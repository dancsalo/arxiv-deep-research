# Manual Integration Test Results: fetch_arxiv_text

**Date:** 2026-05-12  
**Tool:** fetch_arxiv_text  
**CLI:** tools-cli v1.0  
**Environment:** macOS Darwin 23.6.0  

## Overview

Manual integration tests were performed against the live arXiv API to verify the `fetch_arxiv_text` tool's behavior with real papers. This tool fetches HTML content from arXiv papers and extracts plain text.

## Test Cases

### Test 1: Recent Paper with HTML (Success Case)

**Command:**
```bash
./tools-cli fetch-pdf "2412.00001"
```

**Expected Behavior:**
- Returns arxiv_id: "2412.00001"
- Returns non-empty text_content
- Returns truncated: true (default max_length is 8000)
- No error field

**Actual Result:** PASS

**JSON Response:**
```json
{
  "arxiv_id": "2412.00001",
  "text_content": "Centralizer Algebras of Two Permutation Groups of Order 1344 \n\nM. Kosuda1, M. Oura2, Sarbaini3...",
  "truncated": true
}
```

**Notes:**
- Successfully extracted text from paper "Centralizer Algebras of Two Permutation Groups of Order 1344"
- Text content length: 8000 characters (as per default max_length)
- Truncated flag correctly set to true
- No error field present in response

---

### Test 2: Old Paper Without HTML (404 Error)

**Command:**
```bash
./tools-cli fetch-pdf "astro-ph/9901234"
```

**Expected Behavior:**
- Returns arxiv_id: "astro-ph/9901234"
- Returns empty text_content
- Returns truncated: false
- Returns error: "HTML version not available for this paper"

**Actual Result:** PASS

**JSON Response:**
```json
{
  "arxiv_id": "astro-ph/9901234",
  "text_content": "",
  "truncated": false,
  "error": "HTML version not available for this paper"
}
```

**Notes:**
- Correctly handled 404 response from arXiv
- Old-format arXiv ID preserved in response
- Error message is clear and actionable
- Partial result returned (arxiv_id present) even with error

---

### Test 3: Paper with Version Suffix (Version Stripping)

**Command:**
```bash
./tools-cli fetch-pdf "1706.03762v2"
```

**Expected Behavior:**
- Returns arxiv_id: "1706.03762" (version suffix stripped)
- Returns non-empty text_content
- Text extracted successfully

**Actual Result:** PASS

**JSON Response:**
```json
{
  "arxiv_id": "1706.03762",
  "text_content": "Provided proper attribution is provided, Google hereby grants permission to reproduce the tables and figures in this paper solely for use in journalistic or scholarly works...",
  "truncated": true
}
```

**Notes:**
- Version suffix "v2" correctly stripped from arxiv_id
- Successfully extracted text from "Attention Is All You Need" paper
- Demonstrates version normalization works as expected

---

### Test 4: Custom max_length Parameter

**Command:**
```bash
./tools-cli fetch-pdf "2412.00001" --max-length=1000
```

**Expected Behavior:**
- Returns text_content with maximum 1000 characters
- Returns truncated: true
- Content should be shorter than Test 1

**Actual Result:** PASS

**JSON Response:**
```json
{
  "arxiv_id": "2412.00001",
  "text_content": "Centralizer Algebras of Two Permutation Groups of Order 1344 \n\nM. Kosuda1, M. Oura2, Sarbaini3\n\n††footnotetext: 2020 Mathematics Subject Classification: 20C05 and 20B35...",
  "truncated": true
}
```

**Notes:**
- Text content length: 1000 characters (as specified)
- Truncation parameter respected
- Content properly truncated at word boundary

---

## Summary

All 4 test cases passed successfully. The `fetch_arxiv_text` tool demonstrates:

1. **HTML Extraction:** Successfully extracts text from papers with HTML versions available
2. **Error Handling:** Gracefully handles 404 errors for papers without HTML versions
3. **Version Normalization:** Correctly strips version suffixes from arXiv IDs
4. **Content Truncation:** Respects max_length parameter and sets truncated flag appropriately
5. **Partial Results:** Returns partial results (arxiv_id) even when errors occur

## Known Limitations

1. **HTML Availability:** Not all arXiv papers have HTML versions. Papers published before ~2018 typically lack HTML versions.
2. **Paper Tested (2301.00001):** This paper ID did not have HTML available, contrary to initial expectations. Adjusted to use 2412.00001 instead.
3. **CLI Command Name:** The CLI command is currently `fetch-pdf` but calls `fetch_arxiv_text` tool internally. This naming inconsistency may cause confusion.

## Recommendations

1. Consider renaming the CLI command from `fetch-pdf` to `fetch-text` or `fetch-arxiv-text` to match the actual tool name
2. Update CLI help text to clarify that this extracts HTML text, not PDF content
3. Consider adding a fallback mechanism to PDF extraction when HTML is not available

## Tool Metadata

- **Tool Name:** fetch_arxiv_text
- **Registry Name:** fetch_arxiv_text
- **CLI Command:** fetch-pdf (inconsistent naming)
- **Default max_length:** 8000 characters
- **Rate Limiting:** 3 requests/second (arXiv API limit)

## Related Files

- Tool implementation: `tools/research/handlers.go` (handleFetchArxivText)
- Tool definition: `tools/research/definitions.go` (BuildFetchArxivTextTool)
- CLI handler: `cmd/tools-cli/commands.go` (executeFetchPdf)
- Test file: `tools/research/fetch_arxiv_text_test.go`
