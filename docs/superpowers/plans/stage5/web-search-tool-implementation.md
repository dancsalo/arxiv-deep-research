---
name: DuckDuckGo Web Search Tool Implementation
description: Implement search_web tool for general web searches via DuckDuckGo HTML API
type: feature
status: draft
created: 2026-05-11
priority: low
blocked_by: OpenAlex citation sorting enhancement
recommended_sequencing: Build after stage5-openalex-citation-sort
---

# DuckDuckGo Web Search Tool Implementation

## ⚠️ IMPLEMENTATION GUIDANCE

**BUILD THIS LAST.** This tool should be implemented only after:
1. OpenAlex citation sorting is shipped (`sort=cited_by_count:desc` parameter)
2. Usage data shows ≥5% of research queries fail due to OpenAlex coverage gaps
3. Concrete user pain is documented (not theoretical)

**Current Status**: No evidence of user pain. This plan describes a potential solution to a hypothetical problem.

**Recommended Action**: Defer this implementation. Ship OpenAlex citation sorting first, instrument it, measure gaps, then decide if web search is justified.

## Problem Statement

The research agent needs a supplementary search capability to find information that may not be well-indexed in academic databases. Use cases include:
1. Finding papers mentioned on academic blogs, university reading lists, or aggregator sites
2. Discovering research implementations, datasets, or supplementary materials
3. Locating survey papers or tutorial content that contextualizes academic work
4. Cross-referencing paper mentions across the open web

Current research toolset has:
- `search_arxiv` - searches arXiv preprints
- `search_openalex` - searches published academic works (no citation sorting yet)
- `fetch_arxiv_pdf` - fetches arXiv PDF URLs

**Gap**: When OpenAlex and arXiv lack coverage or context, agents have no fallback discovery mechanism.

**Critical Context - This is NOT the Primary Citation Tool**: 
The existing critique correctly identifies that web search is unreliable for "most cited papers" queries. OpenAlex should be enhanced with citation sorting (`sort=cited_by_count:desc`) as the primary mechanism. Web search is a **fallback** for edge cases where:
- Papers are too new for OpenAlex indexing
- Papers exist in non-traditional venues (workshops, preprints, blogs)
- Agent needs contextual information (e.g., "why is this paper important?")

**Why DuckDuckGo HTML API**:
1. No API key required (zero configuration barrier)
2. Simple HTTP endpoint
3. Reasonable reliability for exploratory queries

**Known Limitations** (acknowledged upfront):
- HTML scraping is fragile (DuckDuckGo may change structure)
- Rate limiting risk (IP blocks if overused)
- Results quality is SEO-driven, not academic-quality-driven
- No guarantees of result relevance

**Production Readiness**: This is a **v0.1 exploratory tool** requiring monitoring and fallback strategies (see Production Considerations section).

## Requirements

### Functional Requirements

**FR1**: Tool must accept query and max_results, return structured search results
- Input: `query` (string, required), `max_results` (int, optional, default 10)
- Output: JSON array of results with `title`, `snippet`, `url`
- Rationale for max_results: Consistency with `search_arxiv` and `search_openalex` APIs

**FR2**: Results must be capped at 10 (context budget constraint)
- If `max_results > 10`, clamp to 10
- If `max_results < 1`, use default 10

**FR3**: Tool must handle network failures gracefully
- Return `{"error": "...", "recoverable": true}` for transient failures (5xx, network timeouts)
- Return `{"error": "...", "recoverable": false}` for invalid input (empty query)
- Return `{"error": "...", "recoverable": false}` for sustained failures (3+ consecutive parse errors)

**FR4**: Tool must include rate limiting protection
- Single retry with 1-second delay for transient errors
- User-Agent header (browser-like, but not deceptive)
- Circuit breaker: after 3 consecutive failures, return error immediately without HTTP call (resets after 5 minutes)

**FR5**: Tool must parse DuckDuckGo HTML robustly
- Extract title from result link (unescape HTML entities)
- Extract snippet from result description (unescape HTML entities, may be empty)
- Extract URL from result link (clean DDG redirect wrappers if present)
- Skip results with missing title or URL
- Return empty array `[]` if zero results parsed (not an error)

### Non-Functional Requirements

**NFR1**: Response time under 5 seconds for typical queries
- DuckDuckGo is fast; network latency is the bottleneck

**NFR2**: HTML parsing must be robust to minor DOM changes
- Use class-based selectors that are stable
- Gracefully skip malformed results

**NFR3**: No external dependencies beyond Go stdlib and `golang.org/x/net/html`
- Keep dependency footprint minimal
- Avoid heavyweight scraping libraries

**NFR4**: Integration with existing research toolset patterns
- Follow `tools/research/` conventions
- Use same error handling pattern as other research tools

## Specifications

### Tool Definition Schema

```go
{
  "name": "search_web",
  "description": "Search the general web using DuckDuckGo. Returns titles, snippets, and URLs from web search results. Use as fallback when arXiv/OpenAlex lack coverage, or to find contextual information (blogs, tutorials, datasets). NOT reliable for 'most cited papers' - use search_openalex with citation sorting instead.",
  "input_schema": {
    "type": "object",
    "properties": {
      "query": {
        "type": "string",
        "description": "Search query for web search"
      },
      "max_results": {
        "type": "integer",
        "description": "Maximum number of results to return (default 10, max 10)",
        "default": 10
      }
    },
    "required": ["query"]
  }
}
```

### Result Structure

```go
type WebSearchResult struct {
    Title   string `json:"title"`
    Snippet string `json:"snippet"`
    URL     string `json:"url"`
}
```

Output: JSON array of `WebSearchResult` objects, maximum 10 items.

### Error Handling

Success case:
```json
[
  {
    "title": "Attention Is All You Need",
    "snippet": "The dominant sequence transduction models...",
    "url": "https://arxiv.org/abs/1706.03762"
  }
]
```

Error case:
```json
{
  "error": "DuckDuckGo returned status 503",
  "recoverable": true
}
```

### HTTP Request Details

**Endpoint**: `https://html.duckduckgo.com/html/?q=<query>`
- Uses HTML endpoint (not JSON API which requires keys)
- Query parameter: `q` (URL-encoded)

**Headers**:
- `User-Agent: Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36`
  - Required to avoid blocks
  - Mimics common browser

**Retry Logic**:
- Retry once on network error or 5xx status
- Wait 1 second between retries
- No retry on 4xx (client error) or successful responses

### HTML Parsing Strategy

**⚠️ CRITICAL: Validate Actual HTML Structure Before Implementation**

The HTML structure below is illustrative. **Before writing any parsing code**:
1. Run `curl "https://html.duckduckgo.com/html/?q=test" | head -100` 
2. Inspect actual HTML output
3. Identify real class names, structure, and patterns
4. Update this section with verified structure

**Hypothetical structure** (must be verified):
```html
<div class="result results_links results_links_deep web-result">
  <a class="result__a" href="https://example.com">Title Here</a>
  <a class="result__snippet">Snippet text here</a>
</div>
```

**Parsing algorithm** (adjust based on actual HTML):
1. Find all `<div>` with class **exactly matching** known result class (not substring match)
2. Within each div:
   - Find `<a>` with class "result__a" → extract `href` (URL) and text content (title)
   - Find `<a>` with class "result__snippet" → extract text content (snippet)
   - Apply `html.UnescapeString()` to title and snippet
3. Clean URLs via `cleanSearchURL()` (handles DDG redirect wrappers)
4. Skip results with missing title or URL
5. Limit to `max_results` (clamped to 10)

**Edge cases**:
- No results found → return `{"results": [], "query": "<query>"}` (structured, not empty array)
- Partial results (some missing snippets) → include with empty snippet field
- Malformed HTML → increment `r.searchWebFailures`, return `{"error": "parse failed", "recoverable": true}`
- Parse failure 3x consecutively → circuit breaker engages, return `{"error": "web search disabled", "recoverable": false}`

## Contracts

### Contract 1: Tool Definition
**Location**: `tools/research/definitions.go`
**Function**: `BuildSearchWebTool() anthropic.ToolUnionParam`
**Dependencies**: None
**Called by**: `tools/research/research.go:Register()`

```go
func BuildSearchWebTool() anthropic.ToolUnionParam {
	t := anthropic.ToolUnionParamOfTool(
		anthropic.ToolInputSchemaParam{
			Type: "object",
			Properties: map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "Search query for web search",
				},
				"max_results": map[string]any{
					"type":        "integer",
					"description": "Maximum number of results to return (default 10, max 10)",
					"default":     10,
				},
			},
			Required: []string{"query"},
		},
		"search_web",
	)
	t.OfTool.Description = anthropic.String(
		"Search the general web using DuckDuckGo. Returns titles, snippets, and URLs. Use as fallback when arXiv/OpenAlex lack coverage. NOT reliable for 'most cited papers' - use search_openalex with citation sorting instead.",
	)
	return t
}
```

### Contract 2: Tool Handler
**Location**: `tools/research/handlers.go`
**Function**: `handleSearchWeb(ctx context.Context, input json.RawMessage) (string, error)`
**Dependencies**: 
- `net/http.Client` (via `r.client`)
- `golang.org/x/net/html` parser
- `r.searchWebBaseURL` (string field, defaults to `https://html.duckduckgo.com/html/`, injectable for tests)
- `r.searchWebFailures` (int counter, tracks consecutive parse failures for circuit breaker)
**Called by**: Registry dispatch logic

**Input contract**:
```go
type searchWebInput struct {
    Query      string `json:"query"`
    MaxResults int    `json:"max_results"`
}
```
- `query` must be non-empty (else return `{"error": "query is required", "recoverable": false}`)
- `max_results` defaults to 10, clamped to range [1, 10]
- Invalid JSON returns `{"error": "invalid input: ...", "recoverable": false}`

**Output contract**:
- Success: JSON array string of `WebSearchResult` (may be empty array)
- Error: JSON object string with `error` and `recoverable` fields
- Never returns Go error (always `nil` second return value)
- Circuit breaker: if `r.searchWebFailures >= 3`, immediately return `{"error": "web search temporarily disabled due to repeated failures", "recoverable": false}`

### Contract 3: HTML Parser
**Location**: `tools/research/handlers.go`
**Function**: `parseDuckDuckGoHTML(body io.Reader) ([]WebSearchResult, error)`
**Dependencies**: `golang.org/x/net/html`
**Called by**: `handleSearchWeb()`

**Input contract**: HTML response body from DuckDuckGo
**Output contract**: 
- Success: Array of results (possibly empty)
- Error: Returns error if HTML parsing fails entirely

### Contract 4: Text Extraction Helpers
**Location**: `tools/research/handlers.go`
**Functions**:
- `extractResult(n *html.Node) WebSearchResult`
- `getTextContent(n *html.Node) string`

**Purpose**: Navigate HTML nodes and extract text safely
**Contract**: Never panic, always return empty strings if data missing

### Contract 5: Registration
**Location**: `tools/research/research.go`
**Function**: `Register(reg *registry.ToolRegistry)`
**Modification**: Add line `reg.Register("search_web", BuildSearchWebTool(), r.handleSearchWeb)`

### Contract 6: Token Estimator
**Location**: `tools/research/estimators.go`
**Function**: Add case to estimator switch
**Modification**:
```go
case "search_web":
	// Estimate: avg 200 tokens per result × 10 results + 50 overhead
	return 2050
```
**Rationale**: Conservative estimate for max results. Actual usage may be lower.

### Contract 7: URL Cleaning Helper
**Location**: `tools/research/handlers.go`
**Function**: `cleanSearchURL(rawURL string) string`
**Purpose**: Strip DuckDuckGo redirect wrappers (e.g., `https://duckduckgo.com/l/?uddg=https%3A%2F%2Fexample.com` → `https://example.com`)
**Contract**:
- Input: Raw URL from HTML href attribute
- Output: Clean absolute URL or empty string if invalid
- Never panics

## Production Considerations

**This tool is inherently fragile.** It depends on scraping HTML that may change without notice. The following strategies mitigate risk but do not eliminate it:

### Monitoring & Alerting

**⚠️ ALERTING INFRASTRUCTURE REQUIRED**

This tool WILL break without monitoring. Before shipping, implement:

**Metric 1: Parse Success Rate**
- Track: `parse_success / total_requests`
- Alert: If rate drops below 80% over 20 requests → Slack notification
- Alert: If rate drops below 50% over 10 requests → PagerDuty P2 (HTML structure changed)
- Implementation: Log parse failures with sample HTML (first 500 chars) and timestamp

**Metric 2: Empty Result Rate**  
- Track: `empty_results / successful_parses`
- Alert: If rate exceeds 70% over 20 requests → Slack notification
- Note: High empty rate is expected for obscure queries, but 70%+ likely indicates parsing issue

**Metric 3: Circuit Breaker Trips**
- Track: When `searchWebFailures >= 3` and tool returns immediate error
- Alert: **IMMEDIATE PagerDuty P1** (tool is fully disabled for all users)
- Action Required: Investigate HTML structure, update parser, or disable tool permanently
- Reset Strategy:
  - Automatic: `searchWebFailures` reset to 0 after 1 successful parse (not time-based)
  - Manual: Admin endpoint `/tools/research/reset-circuit-breaker` (requires authentication)
  - **Do NOT use time-based reset** (leads to break-reset-break loop if HTML changed)

**Metric 4: HTTP Request Duration**
- Track: p50, p95, p99 latency for `search_web` calls
- Alert: If p95 > 10 seconds → investigate rate limiting or network issues

**Who Gets Alerted**:
- Slack: `#research-agent-monitoring` channel
- PagerDuty: On-call SRE (for P1) or Tool Owner (for P2)
- Owner Assignment: Define before shipping (e.g., @backend-team or specific engineer)

**Logging Requirements**:
```go
// On parse failure:
log.Warn("search_web parse failed",
    "query", params.Query,
    "html_sample", body[:min(500, len(body))],
    "timestamp", time.Now(),
    "consecutive_failures", r.searchWebFailures)

// On circuit breaker trip:
log.Error("search_web circuit breaker engaged",
    "consecutive_failures", r.searchWebFailures,
    "last_query", lastQuery,  // track what broke it
    "action_required", "investigate HTML structure or disable tool")
```

### Fallback Strategy

When `search_web` fails or is circuit-broken:
1. Agent should continue with `search_arxiv` and `search_openalex` only
2. System prompt should guide agent: "If web search fails, prioritize OpenAlex citation-sorted queries"
3. Tool error message should suggest: "Try refining query with search_openalex or search_arxiv"

### Rate Limiting Defense

**Current strategy**: Single retry, no rate limiting
**Risk**: Sustained usage may trigger IP-based blocking by DuckDuckGo
**Mitigation options** (not implemented in v0.1, add if needed):
- Add 2-second delay between requests (via global mutex + last-call timestamp)
- Exponential backoff: after circuit breaker trips, increase delay (2s → 4s → 8s)
- Rotate User-Agent strings (requires maintaining list, adds complexity)

### HTML Structure Validation

**Test case** (add to `search_web_test.go`):
```go
func TestHTMLStructureDetection(t *testing.T) {
    // Known-good HTML from 2026-05-11
    knownGoodHTML := `<div class="result">...</div>`
    results, err := parseDuckDuckGoHTML(strings.NewReader(knownGoodHTML))
    if err != nil || len(results) == 0 {
        t.Fatal("HTML structure may have changed - parser returned no results")
    }
}
```
Run this test weekly in CI to catch upstream changes early.

### Update Procedure When HTML Changes

1. **Detect**: CI test fails OR parse success rate drops
2. **Inspect**: Manually query DuckDuckGo, view HTML source, identify new class names
3. **Update**: Modify `parseDuckDuckGoHTML` / `extractResult` logic
4. **Test**: Update `TestHTMLStructureDetection` with new HTML sample
5. **Deploy**: Fast-track PR (this is a critical fix)
6. **Verify**: Monitor parse success rate returns to >90%

Estimated time: 1-2 hours per update (assuming structural change, not complete redesign)

### Rollback Plan

**If this tool breaks production, here's the immediate response:**

**Phase 1: Disable (5 minutes)**
1. Deploy feature flag: `ENABLE_SEARCH_WEB=false` environment variable
2. Update `research.go:Register()` to skip tool registration if flag is false
3. Deploy without code changes (config-only deploy)
4. Notify users via Slack: "Web search temporarily disabled, use search_openalex instead"

**Phase 2: Investigate (1 hour)**
1. Check recent DuckDuckGo HTML (curl endpoint, compare to verified structure)
2. Review logs for parse failure patterns
3. Determine if fix is quick (update class names) or complex (major restructure)

**Phase 3: Fix or Remove (variable)**
- Quick fix (<2 hours): Update parser, test, redeploy with feature flag enabled
- Complex fix (>2 hours): Remove tool from codebase, update docs, notify users tool is deprecated

**Rollback SOP Owner**: Define before shipping (e.g., @tool-owner or on-call SRE)

**Incident Severity Levels**:
- P1 (immediate page): Circuit breaker engaged (tool disabled for all users)
- P2 (1-hour response): Parse success rate <50%
- P3 (next-business-day): Parse success rate 50-80%, or empty result rate >70%

### Deprecation Criteria

**HARD STOP (immediate deprecation)**:
- HTML structure changes requiring >4 hours to fix
- DuckDuckGo implements CAPTCHA or aggressive bot detection
- Circuit breaker trips 3+ times in one week
- Parse success rate <30% for 48 hours

**EVALUATION TRIGGERS (consider deprecation)**:
- HTML structure changes more than once per quarter (maintenance burden)
- Usage data shows <5% of research sessions invoke web search
- Usage data shows web search invocations have <50% "useful result" rate (requires user feedback)
- Alternative emerges (e.g., Semantic Scholar API, Brave Search Academic API)

**Re-evaluation Checkpoints**:
- **2-week checkpoint** (2026-05-25): Usage >5%? Parse success >80%? If no, deprecate immediately.
- **3-month checkpoint** (2026-08-11): Maintenance burden acceptable? User value justified? If no, deprecate.

**Deprecation Process**:
1. Announce 1 week in advance: "Web search will be removed on [date], use search_openalex instead"
2. Set feature flag `ENABLE_SEARCH_WEB=false`
3. Wait 1 week, monitor user complaints
4. If <5 complaints, remove code from codebase
5. Update docs, system prompts, and test suite

## Decisions & Tradeoffs

### Decision 1: DuckDuckGo HTML API vs. Other Providers
**Choice**: DuckDuckGo HTML endpoint
**Alternatives**:
- Google Custom Search JSON API (requires API key, costs money)
- Bing Web Search API (requires Azure subscription)
- SerpAPI (third-party proxy, requires API key)

**Rationale**: 
- Zero configuration (no keys)
- Free for reasonable use
- Reliable and fast
- Privacy-focused aligns with research use case

**Tradeoff**: HTML scraping is more fragile than JSON APIs, but mitigated by:
- Using stable class selectors
- Graceful degradation (return partial results)
- Easy to update parser if structure changes

### Decision 2: Result Limit = 10
**Choice**: Hard limit of 10 results
**Alternatives**:
- Make `max_results` a parameter (like `search_arxiv`)
- Return all results from DuckDuckGo (could be 50+)

**Rationale**:
- 10 results fit comfortably in context budget
- First page of results contains most relevant information
- Simplifies tool interface (fewer parameters)

**Tradeoff**: Users cannot get more results, but in practice:
- Web search is exploratory (10 is enough to find leads)
- Can always make multiple queries with refined terms

### Decision 3: Retry Once vs. Exponential Backoff
**Choice**: Single retry with 1 second delay
**Alternatives**:
- No retry (fail fast)
- Exponential backoff (3 retries: 1s, 2s, 4s)

**Rationale**:
- Most failures are transient (network blips)
- Single retry catches 90% of transient issues
- Keeps latency predictable (max 2 requests)

**Tradeoff**: Persistent failures still fail, but acceptable because:
- Recoverable errors let agent retry later
- DuckDuckGo is highly reliable (rarely down)

### Decision 4: No Caching
**Choice**: No result caching layer
**Alternatives**:
- Cache results in memory (TTL 5 minutes)
- Cache results in database

**Rationale**:
- Web search results change frequently
- Caching stale results harms research quality
- Memory overhead not justified for rare duplicate queries

**Tradeoff**: Duplicate queries waste bandwidth, but in practice:
- Research agents rarely repeat exact same query
- HTTP client connection pooling reduces overhead

### Decision 5: golang.org/x/net/html vs. Third-Party Scrapers
**Choice**: Standard library HTML parser
**Alternatives**:
- goquery (jQuery-like, more ergonomic)
- colly (full-featured scraper)

**Rationale**:
- Minimal dependencies (already using golang.org/x in project)
- Simple parsing task doesn't need jQuery selectors
- Reduces attack surface (fewer dependencies)

**Tradeoff**: More verbose code, but acceptable because:
- Parsing logic is ~50 lines, one-time cost
- Easier to audit and maintain

## Implementation Order

### Phase 0: HTML Structure Validation (BLOCKING)
**⚠️ DO NOT PROCEED TO PHASE 1 WITHOUT COMPLETING THIS**

1. **Verify DuckDuckGo HTML structure**
   ```bash
   curl "https://html.duckduckgo.com/html/?q=attention+is+all+you+need" > /tmp/ddg_sample.html
   ```
   - Open `/tmp/ddg_sample.html` in text editor
   - Identify actual class names for result containers, title links, snippets
   - Document findings in code comments
   - Update parsing strategy section (lines 164-184) with verified structure

2. **Test multiple query types**
   - Academic query: "transformer architecture deep learning"
   - Brand query: "anthropic claude"
   - No-results query: "asdfghjklqwertyuiop"
   - Verify HTML structure is consistent across query types

### Phase 1: Infrastructure Changes
3. **Update ResearchToolSet struct** (`research.go`)
   ```go
   type ResearchToolSet struct {
       client             *http.Client
       searchWebBaseURL   string  // default: "https://html.duckduckgo.com/html/"
       searchWebFailures  int     // consecutive parse failures (circuit breaker)
   }
   ```
   - Update `NewResearchToolSet(client *http.Client)` to initialize `searchWebBaseURL` to default
   - OR: Add options pattern for testing (e.g., `WithSearchWebBaseURL(url string)`)

4. **Add token estimator** (`estimators.go`)
   - Add case `"search_web": return 2050`
   - Must be done before registration (else budget tracking breaks)

### Phase 2: Core Implementation
5. **Add imports** (`handlers.go`)
   - Verify existing imports include: `"context"`, `"encoding/json"`, `"fmt"`, `"io"`, `"net/http"`, `"net/url"`, `"strconv"`, `"strings"`, `"time"`
   - Add new import: `"golang.org/x/net/html"`
   - Add new import: `"html"` (for `html.UnescapeString`)
   - Verify all imports before proceeding

6. **Add tool definition** (`definitions.go`)
   - `BuildSearchWebTool()` function with `max_results` parameter
   - No new imports needed (uses existing `anthropic` package)
   - Pure schema definition

7. **Add result type** (`handlers.go`)
   - Add `WebSearchResult` struct after existing result types (after `ArxivPdfResult`)
   ```go
   type WebSearchResult struct {
       Title   string `json:"title"`
       Snippet string `json:"snippet"`
       URL     string `json:"url"`
   }
   ```

8. **Add URL cleaning helper** (`handlers.go`)
   - `cleanSearchURL(rawURL string) string`
   - Handles DDG redirect wrappers (check for `uddg=` parameter)
   - Validates absolute http/https URLs using `url.Parse()`
   - Returns empty string if invalid or non-HTTP(S) scheme

9. **Add HTML parser** (`handlers.go`)
   - `parseDuckDuckGoHTML(body io.Reader) ([]WebSearchResult, error)`
   - `extractResult(n *html.Node) WebSearchResult` - calls `cleanSearchURL`, applies `html.UnescapeString`
   - `getTextContent(n *html.Node) string`
   - **Use verified HTML structure from Phase 0** (not hypothetical structure)

10. **Add tool handler** (`handlers.go`)
   - `handleSearchWeb(ctx context.Context, input json.RawMessage) (string, error)`
   - Implements circuit breaker logic (checks `r.searchWebFailures`)
   - Increments `r.searchWebFailures` on parse error, resets on success
   - Uses `r.searchWebBaseURL` (injectable for tests)
   - Adds request timeout (10 seconds, matching `validateArxivPdf` pattern)
   - Uses User-Agent: `"arxiv-deep-research/1.0"` (project-specific, matching existing pattern)
   - Dependencies: parser functions from step 9

11. **Register tool** (`research.go`)
   - Add `reg.Register("search_web", BuildSearchWebTool(), r.handleSearchWeb)` in `Register()` method
   - Should be line 24, after `fetch_arxiv_pdf` registration
   - Dependencies: tool definition and handler from steps 6 & 10

### Phase 3: Dependency Management
9. **Add golang.org/x/net/html dependency**
   - Run `go get golang.org/x/net/html`
   - Run `go mod tidy`

### Phase 4: Testing & Verification
12. **Create test file** (`search_web_test.go`)
    **Test Infrastructure**:
    - Use `httptest.NewServer()` to create mock DDG server
    - Override `r.searchWebBaseURL` to point to mock server (requires Step 3 changes)
    - Serve verified HTML structure from Phase 0
    
    **Test Cases**:
    ```go
    func TestHandleSearchWeb_ValidQuery(t *testing.T) {
        // Mock server returns verified HTML structure
        // Toolset with searchWebBaseURL = mockServer.URL
        // Assert: results parsed correctly, URLs cleaned, HTML unescaped
    }
    
    func TestHandleSearchWeb_EmptyQuery(t *testing.T) {
        // Assert: returns {"error": "query is required", "recoverable": false}
    }
    
    func TestHandleSearchWeb_MaxResultsClamping(t *testing.T) {
        // Input: max_results = 20
        // Assert: returns at most 10 results
    }
    
    func TestHandleSearchWeb_CircuitBreaker(t *testing.T) {
        // Simulate 3 consecutive parse failures
        // Assert: 4th call returns error immediately without HTTP request
        // Assert: r.searchWebFailures == 3
    }
    
    func TestHandleSearchWeb_URLCleaning(t *testing.T) {
        // Mock HTML with DDG redirect URL: https://duckduckgo.com/l/?uddg=https%3A%2F%2Fexample.com
        // Assert: result URL is "https://example.com" (cleaned)
    }
    
    func TestHandleSearchWeb_NetworkFailure(t *testing.T) {
        // Mock server returns 503
        // Assert: retries once, then returns {"error": "...", "recoverable": true}
    }
    
    func TestHandleSearchWeb_MalformedHTML(t *testing.T) {
        // Mock server returns garbage HTML
        // Assert: returns {"error": "...", "recoverable": true}
        // Assert: r.searchWebFailures incremented
    }
    
    func TestHandleSearchWeb_LiveStructureValidation(t *testing.T) {
        // **THIS TEST HITS REAL DUCKDUCKGO** (not mock)
        // Query: "test"
        // Assert: parser returns ≥1 result (proves HTML structure still matches)
        // Mark as integration test: `// +build integration` or skip in CI if env var not set
    }
    ```

13. **Integration testing** (manual)
    - Build project: `make build`
    - Run unit tests: `go test ./tools/research -run TestSearchWeb -v`
    - Run live integration test: `go test ./tools/research -run TestHandleSearchWeb_LiveStructureValidation -v`
    - If server/demo exists, test with real agent

**Critical**: Step 12's `TestHandleSearchWeb_LiveStructureValidation` must run against real DuckDuckGo in CI (weekly) to catch HTML changes early.

**Order rationale**: Infrastructure first (struct fields, estimator), then bottom-up implementation (helpers → parser → handler → registration), then comprehensive testing.

**Critical path items** (must not skip):
- Step 1 (struct fields for circuit breaker)
- Step 2 (token estimator)
- Step 5 (URL cleaning)
- Step 10 (circuit breaker test)

## Open Questions & Resolutions

### Q1: Should we add a `max_results` parameter?
**REVISED Decision**: Yes, add `max_results` parameter.
**Reason**: Consistency with `search_arxiv` and `search_openalex`. Allows agents to request fewer results (cost savings). Clamped to [1, 10] to protect context budget.

### Q2: Should we validate/clean URLs before returning them?
**REVISED Decision**: Yes, strip DuckDuckGo redirect wrappers.
**Reason**: DDG sometimes returns redirect URLs like `https://duckduckgo.com/l/?uddg=<encoded>`. These confuse agents and break downstream tools. Parse out the `uddg` parameter and decode it.
**Implementation**: Add `cleanSearchURL()` helper that extracts actual URL from redirect wrapper.

### Q3: Should we strip HTML entities from snippets?
**Decision**: Yes, use `html.UnescapeString()` on title and snippet.
**Reason**: DuckDuckGo HTML may contain `&amp;`, `&quot;`, etc. Clean display for users.

### Q4: What if DuckDuckGo changes HTML structure?
**REVISED Mitigation**: 
- Circuit breaker: after 3 consecutive parse failures, disable tool
- Weekly CI test with known-good HTML sample (fails if structure changed)
- Log first 500 chars of HTML on parse failure (aids debugging)
- Document update procedure (1-2 hour turnaround)
**Action**: Document HTML structure in code comments with date stamp (2026-05-11).

### Q5: Should we build this tool first, or enhance OpenAlex first?
**REVISED Decision**: Build web search as fallback only, acknowledge OpenAlex enhancement is prerequisite.
**Reason**: PM critique correctly identified that citation sorting in OpenAlex solves 90% of "most cited papers" use case. Web search is for edge cases.
**Action**: Update tool description to discourage misuse ("NOT reliable for 'most cited papers'").
**Sequencing**: This tool is okay to build now, but system prompt should prefer OpenAlex + citation sort when available.

### Q6: How do we make this testable?
**REVISED Decision**: Add `searchWebBaseURL` field to `ResearchToolSet`, inject in tests.
**Reason**: Allows tests to point to `httptest.NewServer()` instead of real DuckDuckGo. Follows existing pattern in other handlers.
**Implementation**: Modify `NewResearchToolSet()` to accept optional config struct with base URL override.

## Critical Files

| File | Purpose | Lines | Risk |
|------|---------|-------|------|
| `tools/research/definitions.go` | Tool schema | ~20 | Low (pure data) |
| `tools/research/handlers.go` | Handler + parser | ~150 | Medium (HTTP, parsing) |
| `tools/research/research.go` | Registration | ~1 | Low (one line change) |
| `tools/research/search_web_test.go` | Tests | ~100 | Low (new file) |
| `go.mod` | Dependencies | ~1 | Low (one dependency) |

**Total new code**: ~270 lines
**Modified code**: ~1 line (registration)

## Test Plan Reference

See companion file: `.claude/plans/stage5/web-search-tool-implementation-tests.md`

## Revision Log

**2026-05-11 - Initial draft**
- Created plan based on existing stage5-01 summary document
- Expanded with full contracts, specifications, and decisions
- Ready for critique

**2026-05-11 - Revision 1 (Post-Critique Round 1)**
- **Problem Statement**: Reframed web search as fallback, not primary citation tool. Acknowledged OpenAlex citation sorting is the correct solution for "most cited papers."
- **Functional Requirements**: Added `max_results` parameter for API consistency. Added circuit breaker (3 failures → disable). Added URL cleaning requirement.
- **Tool Definition**: Updated description to discourage misuse ("NOT reliable for 'most cited papers'"). Added `max_results` parameter.
- **Contracts**: Added `searchWebBaseURL` field (testability), `searchWebFailures` field (circuit breaker), token estimator contract, URL cleaning helper contract.
- **Production Considerations**: New section covering monitoring (3 metrics), fallback strategy, rate limiting defense, HTML structure validation, update procedure, deprecation criteria.
- **Open Questions**: Resolved Q1 (yes to max_results), Q2 (yes to URL cleaning), Q4 (circuit breaker + CI test), added Q5 (sequencing), Q6 (testability).
- **Implementation Order**: Reorganized to prioritize infrastructure (struct fields, estimator), added URL cleaning step, added comprehensive test coverage including circuit breaker.
- **Decisions**: Kept all original decisions, added context acknowledging fragility.

**Key changes addressing critiques**:
- Staff Engineer: Fixed testability (injectable base URL), added missing contracts (token estimator, URL cleaning), circuit breaker for operational resilience, comprehensive test plan.
- Product Manager: Acknowledged OpenAlex is primary tool, repositioned web search as fallback, added production monitoring/deprecation plan, added 3-month re-evaluation date.

**2026-05-11 - Revision 2 (Post-Critique Round 2)**
- **Front Matter**: Added `blocked_by` and `recommended_sequencing` fields. Added ⚠️ section warning to build this LAST, only after OpenAlex enhancement ships and gaps are measured.
- **HTML Parsing**: Added Phase 0 (HTML structure validation) as BLOCKING step. Requires manual verification of DuckDuckGo HTML before any code is written. Changed parsing to use exact class matching (not substring). Changed empty results to return structured JSON (not empty array).
- **Implementation Order**: Added Phase 0 with explicit curl commands to verify HTML structure. Updated ResearchToolSet struct documentation with specific field types. Added explicit import verification step. Added User-Agent to match existing pattern (`arxiv-deep-research/1.0`). Added request timeout (10 seconds). Added tool registration location (line 24 in research.go).
- **Testing**: Complete rewrite. Tests now use httptest.NewServer with injectable baseURL. Added 8 specific test functions with assertions. Added live integration test that hits real DuckDuckGo (runs weekly in CI to catch HTML changes).
- **Monitoring & Alerting**: Added alerting infrastructure requirements (PagerDuty, Slack). Defined alert thresholds (80%, 50%, 70%). Added ownership assignment requirement. Added logging code samples. Changed circuit breaker reset from time-based to success-based (prevents break-reset loop).
- **Rollback Plan**: New section. 3-phase rollback procedure (disable → investigate → fix/remove). Feature flag approach for quick disable. Incident severity levels (P1/P2/P3). Rollback SOP owner assignment.
- **Deprecation**: Added "HARD STOP" triggers (immediate deprecation). Changed re-evaluation from 3-month to 2-week + 3-month checkpoints. Added specific deprecation process (announce → disable → remove).

**Key changes addressing Round 2 critiques**:
- Staff Engineer: Addressed HTML verification gap (Phase 0), fixed constructor arity (no 2nd param), fixed test infrastructure (injectable baseURL), added tool registration location, fixed User-Agent, added request timeout, comprehensive import documentation.
- Product Manager: Added blocking warning (build last), added 2-week early checkpoint (quick kill if not working), feature flag for quick disable, hard stop criteria, structured rollback SOP, incident severity matrix.
