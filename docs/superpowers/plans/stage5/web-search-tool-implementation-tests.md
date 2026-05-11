---
name: Web Search Tool Test Plan
description: Comprehensive testing strategy for DuckDuckGo web search tool
type: test-plan
parent: web-search-tool-implementation
created: 2026-05-11
---

# Web Search Tool Test Plan

## Test Strategy

This test plan covers three testing levels:
1. **Unit tests** - Individual function behavior (parser, URL cleaner, handler)
2. **Integration tests** - End-to-end tool invocation with mock HTTP
3. **Live validation tests** - Periodic checks against real DuckDuckGo (CI only)

## Prerequisite: HTML Structure Verification

Before writing any tests, complete Phase 0 from implementation plan:
```bash
curl "https://html.duckduckgo.com/html/?q=attention+is+all+you+need" > /tmp/ddg_sample.html
```
Document actual HTML structure in test file comments.

## Unit Tests

### Test 1: URL Cleaning - DDG Redirect Wrapper
**File**: `tools/research/search_web_test.go`
**Function**: `TestCleanSearchURL_DDGRedirect`

**Input**: `https://duckduckgo.com/l/?uddg=https%3A%2F%2Farxiv.org%2Fabs%2F1706.03762`
**Expected**: `https://arxiv.org/abs/1706.03762`
**Assertions**:
- Result is not empty
- Result starts with `https://`
- Result does not contain `duckduckgo.com`

**Edge Cases**:
- Malformed URL encoding → returns empty string
- Missing `uddg` parameter → returns empty string
- Non-HTTP(S) scheme (`ftp://`, `javascript:`) → returns empty string

### Test 2: URL Cleaning - Already Clean
**Function**: `TestCleanSearchURL_AlreadyClean`

**Input**: `https://arxiv.org/abs/1706.03762`
**Expected**: `https://arxiv.org/abs/1706.03762` (unchanged)

### Test 3: HTML Entity Unescaping
**Function**: `TestGetTextContent_HTMLEntities`

**Input**: HTML node containing `&quot;Attention&quot; &amp; Transformers`
**Expected**: `"Attention" & Transformers`
**Assertions**:
- `&quot;` converted to `"`
- `&amp;` converted to `&`
- No leading/trailing whitespace

### Test 4: Parser - Valid HTML Structure
**Function**: `TestParseDuckDuckGoHTML_ValidStructure`

**Setup**: Create minimal valid HTML based on verified structure from Phase 0
**Input**:
```html
<html><body>
<div class="[verified-result-class]">
  <a class="[verified-title-class]" href="https://example.com">Test Paper</a>
  <a class="[verified-snippet-class]">This is a snippet</a>
</div>
</body></html>
```
**Expected**: `[]WebSearchResult` with 1 entry
**Assertions**:
- `len(results) == 1`
- `results[0].Title == "Test Paper"`
- `results[0].URL == "https://example.com"`
- `results[0].Snippet == "This is a snippet"`

### Test 5: Parser - No Results
**Function**: `TestParseDuckDuckGoHTML_NoResults`

**Input**: Valid HTML with no result divs (DuckDuckGo "no results" page)
**Expected**: `[]WebSearchResult{}` (empty slice, no error)

### Test 6: Parser - Malformed HTML
**Function**: `TestParseDuckDuckGoHTML_MalformedHTML`

**Input**: `<html><body><div><a>broken`
**Expected**: Error returned (HTML parsing failed)

### Test 7: Parser - Missing Fields
**Function**: `TestParseDuckDuckGoHTML_MissingFields`

**Input**: HTML with result div but missing title link
**Expected**: Result skipped (not included in output)

## Integration Tests (Mock HTTP)

### Test 8: Handler - Valid Query
**Function**: `TestHandleSearchWeb_ValidQuery`

**Setup**:
- `httptest.NewServer` serving verified HTML structure
- `ResearchToolSet` with `searchWebBaseURL` set to mock server URL

**Input**: `{"query": "test query", "max_results": 5}`
**Expected**: JSON array of results
**Assertions**:
- No error returned
- Result is valid JSON array
- Array length ≤ 5
- Each result has non-empty `title` and `url`

### Test 9: Handler - Empty Query
**Function**: `TestHandleSearchWeb_EmptyQuery`

**Input**: `{"query": "", "max_results": 10}`
**Expected**: `{"error": "query is required", "recoverable": false}`
**Assertions**:
- Result contains `"error"`
- `recoverable` is `false`

### Test 10: Handler - Max Results Clamping
**Function**: `TestHandleSearchWeb_MaxResultsClamping`

**Setup**: Mock server returns 15 results
**Input**: `{"query": "test", "max_results": 20}`
**Expected**: JSON array with 10 results (clamped)
**Assertions**:
- `len(results) == 10` (not 15, not 20)

### Test 11: Handler - Max Results Default
**Function**: `TestHandleSearchWeb_MaxResultsDefault`

**Input**: `{"query": "test"}` (no `max_results` field)
**Expected**: JSON array with ≤ 10 results
**Assertions**:
- Parser receives default value of 10

### Test 12: Handler - Network Timeout
**Function**: `TestHandleSearchWeb_NetworkTimeout`

**Setup**: Mock server with `time.Sleep(15 * time.Second)` (exceeds 10s timeout)
**Expected**: `{"error": "web search request failed: ...", "recoverable": true}`
**Assertions**:
- Request times out
- Error is marked recoverable

### Test 13: Handler - HTTP 503 (Retry Logic)
**Function**: `TestHandleSearchWeb_HTTP503Retry`

**Setup**: Mock server returns 503 on first call, 200 on second
**Expected**: Success (handler retried and succeeded)
**Assertions**:
- Mock server receives 2 requests
- Result is valid JSON array

### Test 14: Handler - Persistent HTTP 503
**Function**: `TestHandleSearchWeb_PersistentHTTP503`

**Setup**: Mock server always returns 503
**Expected**: `{"error": "DuckDuckGo returned status 503", "recoverable": true}`
**Assertions**:
- Mock server receives exactly 2 requests (initial + 1 retry)
- Error is marked recoverable

### Test 15: Handler - Parse Failure Increments Counter
**Function**: `TestHandleSearchWeb_ParseFailureCounter`

**Setup**: Mock server returns malformed HTML
**Expected**: Error returned, `r.searchWebFailures` incremented
**Assertions**:
- Before call: `r.searchWebFailures == 0`
- After call: `r.searchWebFailures == 1`
- Second call: `r.searchWebFailures == 2`

### Test 16: Handler - Circuit Breaker Engages
**Function**: `TestHandleSearchWeb_CircuitBreaker`

**Setup**:
1. Cause 3 consecutive parse failures (malformed HTML)
2. Make 4th call with valid input

**Expected**: 4th call returns error immediately without HTTP request
**Assertions**:
- Mock server receives exactly 3 HTTP requests (not 4)
- 4th call result: `{"error": "web search temporarily disabled due to repeated failures", "recoverable": false}`
- `r.searchWebFailures == 3` (does not increment further)

### Test 17: Handler - Circuit Breaker Reset
**Function**: `TestHandleSearchWeb_CircuitBreakerReset`

**Setup**:
1. Cause 2 parse failures
2. Succeed on 3rd call
3. Cause 1 more parse failure

**Expected**: Circuit breaker does not engage (failures reset after success)
**Assertions**:
- After 2 failures: `r.searchWebFailures == 2`
- After 1 success: `r.searchWebFailures == 0`
- After 1 more failure: `r.searchWebFailures == 1`
- All calls execute HTTP requests (circuit breaker never engaged)

### Test 18: Handler - Invalid JSON Input
**Function**: `TestHandleSearchWeb_InvalidJSON`

**Input**: `{invalid json`
**Expected**: `{"error": "invalid input: ...", "recoverable": false}`

### Test 19: Handler - User-Agent Header
**Function**: `TestHandleSearchWeb_UserAgent`

**Setup**: Mock server captures request headers
**Expected**: Request includes `User-Agent: arxiv-deep-research/1.0`
**Assertions**:
- Header is present
- Header value matches expected

## Live Integration Tests (Real DuckDuckGo)

**⚠️ These tests hit external APIs. Run in CI only, not on every local test run.**

### Test 20: Live Structure Validation
**Function**: `TestHandleSearchWeb_LiveStructureValidation`
**Build Tag**: `// +build integration`

**Setup**: Real HTTP client (not mock)
**Input**: `{"query": "test", "max_results": 5}`
**Expected**: Non-empty results (proves HTML structure still matches)
**Assertions**:
- `len(results) >= 1` (at least 1 result for generic query)
- Each result has non-empty `title` and `url`
- No parse errors

**CI Schedule**: Run weekly (not on every commit)
**Failure Action**: Alert tool owner, investigate HTML structure change

### Test 21: Live Academic Query
**Function**: `TestHandleSearchWeb_LiveAcademicQuery`
**Build Tag**: `// +build integration`

**Input**: `{"query": "attention is all you need transformer", "max_results": 10}`
**Expected**: Results contain academic-looking URLs (arxiv.org, scholar.google.com, etc.)
**Assertions**:
- At least 3 results returned
- At least 1 result URL contains "arxiv" or "scholar" (heuristic check)

### Test 22: Live No Results Query
**Function**: `TestHandleSearchWeb_LiveNoResults`
**Build Tag**: `// +build integration`

**Input**: `{"query": "asdfghjklqwertyuiopzxcvbnm", "max_results": 10}`
**Expected**: Empty results (DuckDuckGo returns no matches)
**Assertions**:
- `len(results) == 0`
- No errors (empty results are not errors)

## Tool Registration Test

### Test 23: Tool is Registered
**Function**: `TestToolRegistration`
**File**: `tools/research/research_test.go` (or new file)

**Setup**:
- Create `ToolRegistry`
- Create `ResearchToolSet`
- Call `Register()`

**Expected**: `search_web` tool is registered
**Assertions**:
- `registry.HasTool("search_web") == true`
- Tool definition matches `BuildSearchWebTool()` output
- Handler is callable via registry dispatch

## Token Estimator Test

### Test 24: Token Estimate
**Function**: `TestSearchWebTokenEstimate`
**File**: `tools/research/estimators_test.go`

**Input**: `search_web` tool name
**Expected**: Estimator returns 2050 tokens
**Assertions**:
- Estimate >= 2000 (conservative)
- Estimate is consistent across calls

## Performance Tests

### Test 25: Response Time - Valid Query
**Function**: `BenchmarkHandleSearchWeb_ValidQuery`

**Setup**: Mock server with realistic HTML (1000 result divs)
**Input**: `{"query": "test", "max_results": 10}`
**Expected**: Completes in <100ms (parsing overhead)
**Assertions**:
- Benchmark reports ns/op
- No memory leaks (check allocations)

### Test 26: Response Time - Large HTML
**Function**: `BenchmarkParseDuckDuckGoHTML_LargeHTML`

**Input**: 10KB HTML with 50 result divs
**Expected**: Completes in <50ms
**Assertions**:
- Parsing is O(n) in HTML size (linear scan)

## Error Case Tests

### Test 27: Context Cancellation
**Function**: `TestHandleSearchWeb_ContextCancelled`

**Setup**: Context cancelled before HTTP request completes
**Expected**: `{"error": "web search request failed: context canceled", "recoverable": true}`

### Test 28: Nil Client
**Function**: `TestNewResearchToolSet_NilClient`

**Setup**: Call `NewResearchToolSet(nil)`
**Expected**: Panic or error (depending on implementation)

### Test 29: Concurrent Calls (Race Detector)
**Function**: `TestHandleSearchWeb_Concurrent`
**Run With**: `go test -race`

**Setup**: Spawn 10 goroutines calling `handleSearchWeb` simultaneously
**Expected**: No data races on `r.searchWebFailures` counter
**Assertions**:
- All calls complete without panic
- Race detector reports no issues
- `searchWebFailures` counter is accurate (use mutex if needed)

## Test Coverage Goals

| Component | Target Coverage |
|-----------|----------------|
| `cleanSearchURL` | 100% (all branches: valid, invalid, redirect) |
| `parseDuckDuckGoHTML` | 95% (main path + error cases) |
| `extractResult` | 90% |
| `getTextContent` | 100% (simple recursive function) |
| `handleSearchWeb` | 85% (happy path + major error cases) |
| Overall package | 80% |

## Running Tests

**Local development** (unit + integration, no live):
```bash
go test ./tools/research -v -run TestSearchWeb
```

**With race detector**:
```bash
go test ./tools/research -race -run TestSearchWeb
```

**Live integration tests** (CI only):
```bash
go test ./tools/research -tags=integration -v -run TestHandleSearchWeb_Live
```

**All tests with coverage**:
```bash
go test ./tools/research -coverprofile=coverage.out
go tool cover -html=coverage.out
```

## CI Configuration

**On every commit**:
- Run unit tests (Tests 1-19, 23-29)
- Run race detector on Test 29
- Generate coverage report (enforce 80% minimum)

**Weekly scheduled job**:
- Run live integration tests (Tests 20-22)
- Alert tool owner if any fail
- Create GitHub issue with failure details

**Pre-merge requirement**:
- All unit tests pass
- Coverage ≥ 80%
- No race conditions detected

## Manual Testing Checklist

Before marking PR ready for review:

- [ ] Verify DuckDuckGo HTML structure (Phase 0)
- [ ] Run all unit tests locally: `make test-go`
- [ ] Run integration tests with mock: Tests 8-19
- [ ] Manually test with real DuckDuckGo (one query)
- [ ] Check logs for parse errors
- [ ] Test circuit breaker recovery (3 failures → success → reset)
- [ ] Verify tool is registered in `research.go:Register()`
- [ ] Verify token estimator returns correct value
- [ ] Run race detector: `make test-race`
- [ ] Check coverage: ≥80%

## Test Maintenance

**When DuckDuckGo HTML changes**:
1. Update verified HTML structure in Phase 0 comments
2. Update Test 4 (`TestParseDuckDuckGoHTML_ValidStructure`) with new HTML
3. Update Test 8 (`TestHandleSearchWeb_ValidQuery`) mock server response
4. Re-run all tests to ensure parser still works
5. Update test file comments with new structure documentation

**Quarterly review**:
- Review live integration test results (Tests 20-22)
- Check if HTML structure has changed
- Update test cases if needed
- Verify CI schedule is running weekly
