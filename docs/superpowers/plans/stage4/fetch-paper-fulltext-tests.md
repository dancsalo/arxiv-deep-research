# Test Plan: fetch_arxiv_pdf Tool

## Test Strategy

This test plan covers the `fetch_arxiv_pdf` tool implementation. Tests follow the existing research tool patterns (`arxiv_test.go`, `openalex_test.go`) using `httptest.NewServer()` with mocked HTTP responses.

## Unit Tests

### 1. arXiv ID Normalization Tests

**File**: `tools/research/fetch_arxiv_pdf_test.go`

#### Test 1.1: `TestNormalizeArxivID_NewFormat`
- **Input**: `"2301.00001"`
- **Expected**: `normalized="2301.00001"`, `version=""`, `err=nil`
- **Validates**: New format (YYMM.NNNNN) parsing

#### Test 1.2: `TestNormalizeArxivID_NewFormatWithVersion`
- **Input**: `"2301.00001v2"`
- **Expected**: `normalized="2301.00001"`, `version="v2"`, `err=nil`
- **Validates**: Version suffix extraction

#### Test 1.3: `TestNormalizeArxivID_OldFormat`
- **Input**: `"astro-ph/9901234"`
- **Expected**: `normalized="astro-ph/9901234"`, `version=""`, `err=nil`
- **Validates**: Old format (category/YYMMNNN) parsing

#### Test 1.4: `TestNormalizeArxivID_WithPrefix`
- **Input**: `"arXiv:2301.00001"`
- **Expected**: `normalized="2301.00001"`, `version=""`, `err=nil`
- **Validates**: Prefix stripping

#### Test 1.5: `TestNormalizeArxivID_WithURLPrefix`
- **Input**: `"https://arxiv.org/abs/2301.00001"`
- **Expected**: `normalized="2301.00001"`, `version=""`, `err=nil`
- **Validates**: Full URL prefix stripping

#### Test 1.6: `TestNormalizeArxivID_InvalidFormat`
- **Input**: `"invalid-id"`
- **Expected**: `err != nil` with message "invalid arXiv ID format"
- **Validates**: Rejects non-arXiv IDs

#### Test 1.7: `TestNormalizeArxivID_OldFormatWithVersion`
- **Input**: `"astro-ph/9901234v1"`
- **Expected**: `normalized="astro-ph/9901234"`, `version="v1"`, `err=nil`
- **Validates**: Old format with version suffix

#### Test 1.8: `TestNormalizeArxivID_CategoryWithV`
- **Input**: `"survey/1234567"`
- **Expected**: `normalized="survey/1234567"`, `version=""`, `err=nil`
- **Validates**: Version regex doesn't match "v" in category name

## Integration Tests (Mocked HTTP)

### 2. Success Cases

#### Test 2.1: `TestFetchArxivPdf_Success`
- **Setup**: Mock server returns 200 with Content-Type: application/pdf
- **Input**: `{"arxiv_id": "2301.00001"}`
- **Expected**: Result with `pdf_url="https://export.arxiv.org/pdf/2301.00001.pdf"`
- **Validates**: Happy path

#### Test 2.2: `TestFetchArxivPdf_WithRedirect`
- **Setup**: Mock server returns 301 → Location: https://arxiv.org/pdf/2301.00001.pdf
- **Input**: `{"arxiv_id": "2301.00001"}`
- **Expected**: Result with final redirected URL
- **Validates**: Redirect following

#### Test 2.3: `TestFetchArxivPdf_OldFormatId`
- **Setup**: Mock server returns 200
- **Input**: `{"arxiv_id": "astro-ph/9901234"}`
- **Expected**: Result with `pdf_url="https://export.arxiv.org/pdf/astro-ph/9901234.pdf"`
- **Validates**: Old format ID support

### 3. Error Cases

#### Test 3.1: `TestFetchArxivPdf_NotFound`
- **Setup**: Mock server returns 404
- **Input**: `{"arxiv_id": "9999.99999"}`
- **Expected**: Error response with `recoverable=true`, message="paper not found"
- **Validates**: 404 handling

#### Test 3.2: `TestFetchArxivPdf_InvalidJSON`
- **Setup**: N/A (no HTTP call)
- **Input**: Invalid JSON `{invalid}`
- **Expected**: Error response with `recoverable=false`, message="invalid input"
- **Validates**: JSON unmarshal error handling

#### Test 3.3: `TestFetchArxivPdf_MissingField`
- **Setup**: N/A (no HTTP call)
- **Input**: `{}`
- **Expected**: Error response with `recoverable=false`, message="arxiv_id is required"
- **Validates**: Required field validation

#### Test 3.4: `TestFetchArxivPdf_InvalidArxivID`
- **Setup**: N/A (no HTTP call)
- **Input**: `{"arxiv_id": "not-an-arxiv-id"}`
- **Expected**: Error response with `recoverable=false`, message="invalid arXiv ID format"
- **Validates**: ID format validation

#### Test 3.5: `TestFetchArxivPdf_ContextCancelled`
- **Setup**: Mock server with slow response, context cancelled before completion
- **Input**: `{"arxiv_id": "2301.00001"}`
- **Expected**: Error response with message containing "context"
- **Validates**: Context cancellation handling

#### Test 3.6: `TestFetchArxivPdf_ArxivMaintenance`
- **Setup**: Mock server returns 503
- **Input**: `{"arxiv_id": "2301.00001"}`
- **Expected**: Error response with `recoverable=true`, message="HTTP 503"
- **Validates**: Service unavailable handling

### 4. Security Tests

#### Test 4.1: `TestFetchArxivPdf_SuspiciousRedirect`
- **Setup**: Mock server returns 301 → Location: https://malicious.com/fake.pdf
- **Input**: `{"arxiv_id": "2301.00001"}`
- **Expected**: Error response with `recoverable=false`, message="suspicious redirect"
- **Validates**: Redirect target validation

#### Test 4.2: `TestFetchArxivPdf_TooManyRedirects`
- **Setup**: Mock server returns 301 → 301 → 301 → ... (4 redirects)
- **Input**: `{"arxiv_id": "2301.00001"}`
- **Expected**: Error response with message="too many redirects"
- **Validates**: Redirect loop protection

#### Test 4.3: `TestFetchArxivPdf_HTTPRedirectToHTTPS`
- **Setup**: Mock server returns 301 → Location: http://arxiv.org/... (downgrade to HTTP)
- **Input**: `{"arxiv_id": "2301.00001"}`
- **Expected**: Result accepts this (arXiv legitimately redirects)
- **Validates**: Protocol change handling

### 5. Edge Cases

#### Test 5.1: `TestFetchArxivPdf_VersionSuffixPreserved`
- **Setup**: Mock server returns 200
- **Input**: `{"arxiv_id": "2301.00001v2"}`
- **Expected**: Result with `version="v2"` field set
- **Validates**: Version info returned in result

#### Test 5.2: `TestFetchArxivPdf_LeadingTrailingWhitespace`
- **Setup**: Mock server returns 200
- **Input**: `{"arxiv_id": "  2301.00001  "}`
- **Expected**: Result with normalized ID (whitespace stripped)
- **Validates**: Input sanitization

#### Test 5.3: `TestFetchArxivPdf_EmptyStringID`
- **Setup**: N/A
- **Input**: `{"arxiv_id": ""}`
- **Expected**: Error response with message="arxiv_id is required"
- **Validates**: Empty string rejection

## Estimator Tests

### 6. Token Estimation

#### Test 6.1: `TestArxivPdfEstimator`
- **Setup**: Get estimator from `ResearchToolEstimators()`
- **Input**: `{"arxiv_id": "2301.00001"}` (args passed to estimator)
- **Expected**: Returns `100` (fixed cost)
- **Validates**: Estimator function correctness

## Test Infrastructure

### Mocking Pattern

Reuse `rewriteTransport` pattern from existing tests:

```go
type rewriteTransport struct {
    base string
    rt   http.RoundTripper
}

func (t *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
    req = req.Clone(req.Context())
    req.URL.Scheme = "http"
    req.URL.Host = t.base[len("http://"):]
    return t.rt.RoundTrip(req)
}
```

### Test Helper Functions

```go
// Helper to create test toolset with mocked HTTP client
func testToolSet(t *testing.T, handler http.HandlerFunc) *ResearchToolSet {
    srv := httptest.NewServer(handler)
    t.Cleanup(srv.Close)
    return &ResearchToolSet{
        client: &http.Client{
            Transport: &rewriteTransport{
                base: srv.URL,
                rt:   http.DefaultTransport,
            },
        },
    }
}
```

## Coverage Goals

- **Line coverage**: >90% for new code
- **Branch coverage**: >85% for conditionals
- **Critical paths**: 100% (happy path, error paths, security checks)

## Test Execution

```bash
# Run all research tool tests
go test ./tools/research/... -v

# Run only fetch_arxiv_pdf tests
go test ./tools/research/... -run TestFetchArxivPdf -v
go test ./tools/research/... -run TestNormalizeArxivID -v

# Run with coverage
go test ./tools/research/... -cover -coverprofile=coverage.out
go tool cover -html=coverage.out
```

## Test Organization

```
tools/research/
├── fetch_arxiv_pdf_test.go (new file, ~400 lines)
│   ├── TestNormalizeArxivID_* (8 tests)
│   ├── TestFetchArxivPdf_* (15 tests)
│   └── TestArxivPdfEstimator (1 test)
├── arxiv_test.go (existing)
├── openalex_test.go (existing)
└── duckduckgo_test.go (existing)
```

## Regression Testing

After implementation, verify existing tests still pass:

```bash
# Ensure no regressions in other research tools
go test ./tools/research/arxiv_test.go -v
go test ./tools/research/openalex_test.go -v
go test ./tools/research/duckduckgo_test.go -v
```

## Manual Testing

After automated tests pass, manually verify:

1. Start server: `make run-server`
2. Call tool via HTTP with real arXiv ID
3. Verify returned URL is valid
4. Test with invalid ID, observe error message
5. Check server logs for proper User-Agent header
6. Monitor for rate limit compliance (manual timing check)

## Success Criteria

- All 24 automated tests pass
- Code coverage >90%
- No regressions in existing tests
- Manual verification succeeds
- Tool integrates correctly with registry
