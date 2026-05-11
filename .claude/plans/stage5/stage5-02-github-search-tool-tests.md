---
name: GitHub Repository Search Tool - Test Plan
description: Verification strategy for search_github_repos tool
type: test-plan
parent: stage5-02-github-search-tool
---

## Test Strategy

This test plan covers unit tests, integration tests, and manual verification for the `search_github_repos` tool. All automated tests use `httptest.Server` with mock GitHub API responses to avoid rate limits and ensure deterministic results.

## Unit Tests

### Test File: `tools/research/search_github_test.go`

#### 1. Happy Path: Successful Search

**Test**: `TestHandleSearchGithub_Success`

**Input**:
```json
{"query": "transformer implementation", "max_results": 5}
```

**Mock Response**: 2 repos with complete metadata (stars, license, topics, updated_at)

**Assertions**:
- Verify HTTP headers: `Accept: application/vnd.github+json`, `X-GitHub-Api-Version: 2022-11-28`
- Verify User-Agent contains "arxiv-deep-research"
- Verify query params: `sort=stars`, `order=desc`, `per_page=5`
- Verify quality filters added to query: `stars:>100`, `pushed:>2024-01-01`
- Parse JSON result, expect 2 repos
- First repo has correct name, license, topics, non-archived

**Expected**: Returns valid JSON array of GitHubRepoResult

---

#### 2. Input Validation: Empty Query

**Test**: `TestHandleSearchGithub_EmptyQuery`

**Input**:
```json
{"query": ""}
```

**Assertions**:
- Result contains `"error"` and `"required"`
- No HTTP request sent

**Expected**: Returns toolError with `recoverable: false`

---

#### 3. Input Validation: Max Results Capping

**Test**: `TestHandleSearchGithub_MaxResultsCapped`

**Subtests**:
1. **Default**: `{"query": "test"}` → expects per_page=5
2. **Explicit 3**: `{"query": "test", "max_results": 3}` → expects per_page=3
3. **Over limit 100**: `{"query": "test", "max_results": 100}` → expects per_page=5 (capped)
4. **Negative**: `{"query": "test", "max_results": -1}` → expects per_page=5 (default)

**Assertions**: Verify `per_page` query param in mock server request

**Expected**: Max results always capped at 5

---

#### 4. Rate Limiting: Primary Rate Limit Exceeded

**Test**: `TestHandleSearchGithub_RateLimitExceeded`

**Mock Response**:
- Status: 403
- Headers: `X-RateLimit-Remaining: 0`, `X-RateLimit-Reset: 1715443200`

**Assertions**:
- Result contains `"rate limit exceeded"`
- Result contains formatted reset time (RFC3339): `"2024-05-11T12:00:00"`
- Result contains relative time: `"in ~42 minutes"`
- Result includes `"recoverable": true`

**Expected**: Returns recoverable error with human-readable reset time

---

#### 5. Rate Limiting: Secondary Rate Limit

**Test**: `TestHandleSearchGithub_SecondaryRateLimit`

**Mock Response**:
- Status: 429
- Headers: `Retry-After: 60`

**Assertions**:
- Result contains `"secondary rate limit"`
- Result contains `"Retry after 60 seconds"`
- `recoverable: true`

**Expected**: Returns recoverable error with retry guidance

---

#### 6. Filtering: Archived Repositories

**Test**: `TestHandleSearchGithub_FilterArchived`

**Mock Response**: 2 repos, one with `"archived": true` (5000 stars), one active (1000 stars)

**Assertions**:
- Parsed result contains only 1 repo (active)
- Archived repo not included despite higher stars

**Expected**: Client-side filtering removes archived repos

---

#### 7. Retry Logic: 500 Error with Successful Retry

**Test**: `TestHandleSearchGithub_RetryOn500`

**Mock Behavior**:
- Attempt 1: Return 500
- Attempt 2: Return 200 with valid response

**Assertions**:
- Mock server called exactly 2 times
- Final result is successful (no error)

**Expected**: Automatic retry on 5xx errors

---

#### 8. Retry Logic: Persistent 500 Error

**Test**: `TestHandleSearchGithub_RetryExhausted`

**Mock Behavior**: Always return 500

**Assertions**:
- Mock server called 3 times (initial + 2 retries)
- Result contains error with status 500
- `recoverable: true`

**Expected**: Returns error after max retries exhausted

---

#### 9. Empty Fields Handling: Missing Description and License

**Test**: `TestHandleSearchGithub_EmptyFields`

**Mock Response**: Repo with `description: ""`, `license: null`, `language: ""`

**Assertions**:
- Parsed result has `description: "(No description provided)"`
- `license: "Unknown"`
- `language: "Unknown"`

**Expected**: Gracefully handles null/empty fields with defaults

---

#### 10. JSON Marshal Error Handling

**Test**: `TestHandleSearchGithub_MarshalError` (difficult to trigger with normal structs)

**Alternative**: Code review to verify marshal error is logged and handled

**Expected**: If marshal fails, returns toolError with `"internal error"`

---

## Integration Tests

These tests verify end-to-end flow without mocking the GitHub API. **Run manually** to avoid rate limits in CI.

### Test File: `tools/research/search_github_integration_test.go` (with build tag `// +build integration`)

#### 11. Real GitHub API: Valid Query

**Test**: `TestIntegration_GitHubAPI_RealRequest`

**Build Tag**: `// +build integration`

**Input**:
```json
{"query": "pytorch transformer", "max_results": 3}
```

**Assertions**:
- No errors
- Returns 1-3 results
- All results have stars > 100
- All results updated after 2024-01-01
- No archived repos

**Run**: `go test -tags=integration ./tools/research -run TestIntegration_GitHubAPI_RealRequest`

---

## Estimator Tests

### Test File: `tools/research/estimators_test.go`

#### 12. Token Estimation: Default Max Results

**Test**: `TestSearchGithubEstimator_Default`

**Input**: `{"query": "test"}`

**Expected**: ~1050 tokens (50 base + 1 query token + 5 * 200 result tokens)

---

#### 13. Token Estimation: Custom Max Results

**Test**: `TestSearchGithubEstimator_Custom`

**Input**: `{"query": "long query string here", "max_results": 3}`

**Expected**: ~656 tokens (50 base + 6 query tokens + 3 * 200)

---

## Manual Verification

Run these tests by hand after implementation.

### 14. End-to-End: Research Demo

**Command**:
```bash
./bin/research-demo --query "find PyTorch implementations of diffusion models"
```

**Verify**:
- Agent calls `search_github_repos` tool
- Returns 1-5 results
- All repos have >100 stars, updated after 2024-01-01
- Agent presents results in readable format with names, stars, URLs

---

### 15. Rate Limit Monitoring

**Command**: Run research-demo 3 times, check logs

**Verify**:
- Logs contain `[GitHub API] rate_limit_remaining=59` (or lower)
- Each call decrements remaining count
- No unexpected errors

---

### 16. Quality Filter Validation

**Command**: Inspect network traffic or logs

**Verify**:
- GitHub API request URL includes `stars:>100` and `pushed:>2024-01-01` in query param

**Alternative**: Add temporary debug logging in handler

---

## Test Coverage Goals

- **Line coverage**: >85% for `handleSearchGithub` and `executeGithubSearch`
- **Branch coverage**: All error paths tested (empty query, rate limit, 500 errors, archived filtering)
- **Edge cases**: Null license, empty description, negative max_results

## Test Execution

### Local Development
```bash
# Unit tests only (fast, no API calls)
go test ./tools/research -run TestHandleSearchGithub -v

# With integration tests (slow, real API)
go test -tags=integration ./tools/research -v

# Coverage report
go test ./tools/research -coverprofile=coverage.out
go tool cover -html=coverage.out
```

### CI Pipeline
```bash
# Run unit tests only (integration tests skip in CI)
make test-go
```

## Known Limitations

1. **Unix timestamp parsing**: If GitHub changes header format, test will fail. Document dependency on `X-RateLimit-Reset` being Unix epoch.
2. **Mock server limitations**: Cannot test pagination (out of scope for Phase 1).
3. **Constructor testing**: Cannot test nil client panic without causing test failure. Rely on code review.

## Future Test Enhancements (Phase 2+)

- Test authentication with valid/invalid tokens
- Test pagination (fetch next page)
- Test query syntax edge cases (special characters, GitHub operators)
- Load testing (concurrent requests, rate limit contention)
- Fuzz testing for query input
