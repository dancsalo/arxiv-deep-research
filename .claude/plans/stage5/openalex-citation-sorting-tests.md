---
name: OpenAlex Citation Sorting Test Plan
description: Test strategy for citation sorting enhancement
type: test-plan
parent: openalex-citation-sorting
created: 2026-05-11
---

# OpenAlex Citation Sorting Test Plan

## Test Coverage Goals

| Component | Target Coverage |
|-----------|----------------|
| Validation logic | 100% (all branches: valid, invalid, empty) |
| URL construction | 100% (with sort, without sort) |
| Result population | 100% (null citation, zero citation, positive citation) |
| Overall enhancement | 90% |

## Unit Tests

### Test 1: Citation Sort - Valid Input
**Function**: `TestHandleSearchOpenAlex_CitationSort`
**File**: `tools/research/openalex_test.go`

**Setup**: Mock OpenAlex API response with 3 papers
**Mock Response**:
```json
{
  "results": [
    {"title": "Paper A", "cited_by_count": 1000},
    {"title": "Paper B", "cited_by_count": 500},
    {"title": "Paper C", "cited_by_count": 100}
  ]
}
```

**Input**: `{"query": "transformer", "sort": "cited_by_count", "max_results": 3}`
**Expected URL**: Contains `&sort=cited_by_count:desc`
**Expected Output**: 3 results in citation order (1000, 500, 100)
**Assertions**:
- `len(results) == 3`
- `results[0].Title == "Paper A"` and `*results[0].CitedByCount == 1000`
- `results[1].Title == "Paper B"` and `*results[1].CitedByCount == 500`
- `results[2].Title == "Paper C"` and `*results[2].CitedByCount == 100`
- URL contains `&sort=cited_by_count:desc`

### Test 2: No Sort Parameter (Backward Compatibility)
**Function**: `TestHandleSearchOpenAlex_NoSort`

**Input**: `{"query": "test", "max_results": 5}` (omit sort field)
**Expected URL**: Does NOT contain `&sort=`
**Expected Output**: Results returned successfully
**Assertions**:
- No error
- Results returned (any order, depends on OpenAlex relevance)
- URL does NOT contain `&sort=`
- `params.Sort == ""` inside handler

### Test 3: Invalid Sort Value
**Function**: `TestHandleSearchOpenAlex_InvalidSort`

**Input**: `{"query": "test", "sort": "invalid_value"}`
**Expected Output**: Error with `recoverable: true`
**Assertions**:
- Error message contains `"invalid sort value"`
- Error message contains `"must be 'cited_by_count'"`
- `recoverable: true`
- No HTTP request made (validation happens before API call)

### Test 4: Citation Count - Null Handling
**Function**: `TestHandleSearchOpenAlex_CitationCountNull`

**Mock Response**:
```json
{
  "results": [
    {"title": "Very New Paper", "cited_by_count": null}
  ]
}
```

**Input**: `{"query": "test"}`
**Expected Output**: Result with `CitedByCount == nil`
**Assertions**:
- `results[0].CitedByCount == nil`
- JSON output omits field: `{"title": "Very New Paper"}` (no `cited_by_count` key)

### Test 5: Citation Count - Zero Handling
**Function**: `TestHandleSearchOpenAlex_CitationCountZero`

**Mock Response**:
```json
{
  "results": [
    {"title": "Unpopular Paper", "cited_by_count": 0}
  ]
}
```

**Input**: `{"query": "test"}`
**Expected Output**: Result with `CitedByCount == &0`
**Assertions**:
- `results[0].CitedByCount != nil`
- `*results[0].CitedByCount == 0`
- JSON output includes: `{"title": "Unpopular Paper", "cited_by_count": 0}`

### Test 6: Existing Tests Pass (Regression)
**Function**: Run all existing `openalex_test.go` tests
**Expected**: No failures (backward compatibility maintained)
**Verify**: Existing test mocks don't need `cited_by_count` field (should default to nil)

## Integration Tests

### Test 7: End-to-End Agent Workflow (Manual or Automated)
**Scenario**: User asks "Show me the most cited papers on transformers"

**Steps**:
1. Agent parses intent: user wants highly-cited papers
2. Agent calls: `search_openalex(query="transformer deep learning", sort="cited_by_count", max_results=5)`
3. Tool returns 5 papers sorted by citation count
4. Agent presents results with citation counts visible

**Assertions**:
- Tool call includes `sort="cited_by_count"`
- Results are sorted (highest citation first)
- Agent output shows citation counts (e.g., "52,341 citations")

**Implementation**:
- If agent test harness exists: Automate this test
- If not: Manual testing checklist

### Test 8: Feature Flag Disable (Manual)
**Scenario**: Feature flag is set to disable citation sorting

**Steps**:
1. Set `ENABLE_OPENALEX_CITATION_SORTING=false`
2. Attempt: `search_openalex(query="test", sort="cited_by_count")`
3. Expected: Error with `recoverable: true` and message "citation sorting temporarily disabled"

**Assertions**:
- Error returned (not success)
- Error is recoverable
- Message is clear

## Performance Tests

### Test 9: Latency Comparison
**Benchmark**: `BenchmarkHandleSearchOpenAlex_CitationSort`

**Measure**:
- Latency for citation-sorted query
- Latency for relevance query (no sort)
- Compare p50, p95, p99

**Expected**: Citation sorting adds <100ms overhead (server-side sorting)

### Test 10: Token Estimator Accuracy
**Test**: Verify token estimate matches reality

**Steps**:
1. Call `search_openalex` with `max_results=10` and `sort="cited_by_count"`
2. Count actual tokens in response
3. Compare to estimator value

**Expected**: Estimator is within 10% of actual tokens

## API Verification Tests (Run Before Implementation)

### Test 11: OpenAlex API - Citation Sort Works
**Command**:
```bash
curl "https://api.openalex.org/works?search=transformer&sort=cited_by_count:desc&per_page=5&mailto=arxiv-deep-research@users.noreply.github.com" | jq '.results[].cited_by_count'
```

**Expected**: Numbers in descending order (e.g., 50000, 30000, 20000, ...)
**Verify**: Sort parameter is accepted and results are sorted

### Test 12: OpenAlex API - Citation Count Field Always Present
**Command**:
```bash
curl "https://api.openalex.org/works?search=test&per_page=10&mailto=arxiv-deep-research@users.noreply.github.com" | jq '.results[] | has("cited_by_count")'
```

**Expected**: All results return `true` (field exists)
**Check**: Look for any `null` values (indicates very new papers)

### Test 13: OpenAlex API - Omitting Sort Uses Relevance
**Command**:
```bash
curl "https://api.openalex.org/works?search=deep+learning&per_page=5&mailto=arxiv-deep-research@users.noreply.github.com" | jq '.results[].cited_by_count'
```

**Expected**: Numbers NOT in citation order (proves relevance sorting is different)
**Verify**: Compare to Test 11 - results should be in different order

## Running Tests

**Local development**:
```bash
go test ./tools/research -v -run TestHandleSearchOpenAlex
```

**With coverage**:
```bash
go test ./tools/research -coverprofile=coverage.out -run TestHandleSearchOpenAlex
go tool cover -html=coverage.out
```

**API verification** (run once before implementation):
```bash
# Test 11-13 (curl commands above)
```

**All tests**:
```bash
make test-go
```

## CI Configuration

**On every commit**:
- Run unit tests (Tests 1-6)
- Generate coverage report (enforce 90% for changed files)

**Before merge**:
- All tests pass
- Coverage meets threshold
- Manual integration test checklist completed

## Test Data

**Mock OpenAlex Response Template**:
```json
{
  "results": [
    {
      "title": "Attention Is All You Need",
      "doi": "10.1234/example",
      "publication_year": 2017,
      "cited_by_count": 52341,
      "authorships": [{"author": {"display_name": "Vaswani, A."}}],
      "abstract_inverted_index": {"The": [0], "dominant": [1], ...}
    }
  ]
}
```

**Null Citation Count**:
```json
{
  "cited_by_count": null
}
```

**Zero Citation Count**:
```json
{
  "cited_by_count": 0
}
```

## Manual Testing Checklist

Before marking PR ready for review:

- [ ] Run API verification tests (Tests 11-13)
- [ ] Confirm OpenAlex API behavior matches plan
- [ ] Run unit tests locally: `make test-go`
- [ ] Verify backward compatibility (Test 2 passes)
- [ ] Test invalid sort value (Test 3 passes)
- [ ] Test null citation count (Test 4 passes)
- [ ] Test zero citation count (Test 5 passes)
- [ ] Check coverage: ≥90% for changed files
- [ ] Manual integration test (if agent harness not available)
- [ ] Verify token estimator formula in `estimators.go`

## Test Maintenance

**When OpenAlex API changes**:
1. Re-run API verification tests (Tests 11-13)
2. Update mock responses if API schema changes
3. Update test assertions if sort behavior changes
4. Document findings in PR

**Quarterly review**:
- Verify API verification tests still pass
- Check if new sort options are available (publication_date, etc.)
- Review test coverage and add missing edge cases
