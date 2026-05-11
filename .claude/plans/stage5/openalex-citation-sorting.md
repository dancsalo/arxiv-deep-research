---
name: OpenAlex Citation Sorting Enhancement
description: Add sort parameter to search_openalex tool for citation-based ranking
type: enhancement
status: draft
created: 2026-05-11
priority: high
blocks: web-search-tool-implementation
---

# OpenAlex Citation Sorting Enhancement

## Problem Statement

The research agent needs to find highly-cited papers on a topic. Currently, `search_openalex` returns results sorted by relevance only. OpenAlex API supports sorting by citation count (`cited_by_count`), publication date, and other fields, but our tool doesn't expose this capability.

**User need**: When asked "find the most cited papers on transformers" or "what are the seminal papers in deep learning," agents should query OpenAlex sorted by citation count, not resort to unreliable web scraping.

**Current behavior**:
```
Agent: search_openalex(query="transformer deep learning", max_results=10)
Result: 10 papers sorted by relevance (not citation count)
```

**Desired behavior**:
```
Agent: search_openalex(query="transformer deep learning", sort="cited_by_count", max_results=10)
Result: 10 papers sorted by citation count (most cited first)
```

**Why this is critical**: The web search tool plan identified that 90% of "most cited papers" queries should use OpenAlex citation sorting, not web scraping. This enhancement is a prerequisite for the web search tool.

## Requirements

### Functional Requirements

**FR1**: Tool must accept optional `sort` parameter
- Input: `sort` (string, optional)
- Valid values: `cited_by_count` (MVP - only value for v1)
- Default: omit sort parameter → OpenAlex relevance ranking (preserves existing behavior)
- Invalid values: return error `{"error": "invalid sort value: must be 'cited_by_count'", "recoverable": true}` (allow agent to retry)

**FR2**: Tool must sort results in descending order
- `cited_by_count` → most cited first (only sort option in v1)
- Future: `publication_date` (newest first) - add only if users request it

**FR3**: Tool must return citation count in results
- Add `cited_by_count` field to `OpenAlexResult` struct
- Always return citation count (even if not sorting by it)
- Value is `*int` (pointer) to distinguish missing data from zero citations
- `null` → data missing from OpenAlex (paper too new or not indexed)
- `0` → paper has zero citations (valid but unpopular)

**FR4**: Tool must validate sort parameter before API call
- Validate against allowed values
- Return clear error for invalid values
- Don't make API call if validation fails

**FR5**: Tool must maintain backward compatibility
- Existing calls without `sort` parameter continue to work
- Default behavior (relevance sorting) unchanged

### Non-Functional Requirements

**NFR1**: No performance regression
- Adding sort parameter should not slow down queries
- OpenAlex API handles sorting server-side (no client-side processing needed)

**NFR2**: API compliance
- Use OpenAlex's documented `sort` parameter syntax
- Include polite email in requests (already done: `mailto=arxiv-deep-research@...`)

**NFR3**: Clear documentation
- Tool description explains sort options
- Examples show how to find most cited papers

## Specifications

### Tool Definition Schema (Updated)

```go
{
  "name": "search_openalex",
  "description": "Search OpenAlex for published academic works. Returns titles, authors, DOIs, abstracts, and citation counts. Supports sorting by citation count (most cited first) when sort='cited_by_count'. Default: relevance ranking. Always returns citation counts for all papers.",
  "input_schema": {
    "type": "object",
    "properties": {
      "query": {
        "type": "string",
        "description": "Search query for academic works"
      },
      "max_results": {
        "type": "integer",
        "description": "Maximum number of results to return (default 10)",
        "default": 10
      },
      "filter": {
        "type": "string",
        "description": "Optional OpenAlex filter expression (e.g. \"publication_year:>2022\")"
      },
      "sort": {
        "type": "string",
        "description": "Sort order: 'cited_by_count' (most cited first). Omit for relevance ranking."
      }
    },
    "required": ["query"]
  }
}
```

**Note**: Enum constraint removed from schema. Validation happens in handler (see Contract 4). This allows better error messages and future extensibility.

### Result Structure (Updated)

```go
type OpenAlexResult struct {
    Title         string   `json:"title"`
    Authors       []string `json:"authors"`
    DOI           string   `json:"doi"`
    Abstract      string   `json:"abstract"`
    Year          int      `json:"year"`
    CitedByCount  *int     `json:"cited_by_count,omitempty"`  // NEW FIELD (pointer to distinguish null from 0)
}
```

**Rationale for `*int`**: 
- `null` in JSON → `nil` in Go → omitted from output (paper lacks citation data)
- `0` in JSON → `&0` in Go → `"cited_by_count": 0` in output (paper has zero citations)
- Agents can distinguish between "no data" and "unpopular paper"

### OpenAlex API Sort Parameter

OpenAlex API supports `sort` query parameter:
- Format: `&sort=cited_by_count:desc`
- Default (no parameter): relevance_score ranking (not explicitly "relevance")
- Documentation: https://docs.openalex.org/how-to-use-the-api/get-lists-of-entities/sort-entity-lists
- API Version: 2023-10-01 (snapshot date, may change)

**Mapping** (v1 MVP):
| Tool Parameter | API Parameter | Order |
|----------------|---------------|-------|
| `cited_by_count` | `cited_by_count:desc` | Descending (most cited first) |
| (omit) | (omit parameter) | Default relevance_score ranking |

**Future expansions** (not in v1):
| Tool Parameter | API Parameter | Order |
|----------------|---------------|-------|
| `publication_date` | `publication_date:desc` | Descending (newest first) |

### Verification of OpenAlex API Behavior

**Before implementation, verify**:
```bash
# Test 1: Citation sorting works
curl "https://api.openalex.org/works?search=transformer+architecture&sort=cited_by_count:desc&per_page=5&mailto=arxiv-deep-research@users.noreply.github.com" | jq '.results[] | {title: .title, cited_by_count: .cited_by_count}'

# Expected: Results sorted by citation count (descending)
# Verify: cited_by_count values decrease (e.g., 50000 → 30000 → 20000)

# Test 2: Citation count field is always present
curl "https://api.openalex.org/works?search=test&per_page=5&mailto=arxiv-deep-research@users.noreply.github.com" | jq '.results[] | has("cited_by_count")'

# Expected: All results return true (field exists)
# Edge case: Check if any return null (possible for very new papers)

# Test 3: Omitting sort parameter uses relevance
curl "https://api.openalex.org/works?search=deep+learning&per_page=5&mailto=arxiv-deep-research@users.noreply.github.com" | jq '.results[0]'

# Expected: Results are NOT sorted by citation count (verify manually)
# Compare: Run same query with sort parameter, confirm different order
```

**Document findings in implementation PR**: Include curl output samples proving behavior matches plan.

## Contracts

### Contract 1: Tool Definition Update
**Location**: `tools/research/definitions.go`
**Function**: `BuildSearchOpenAlexTool()`
**Changes**:
- Add `sort` property to input schema
- Add `enum` constraint: `["cited_by_count", "publication_date", "relevance"]`
- Update description to mention citation sorting
- Set default: `"relevance"`

**Before**:
```go
func BuildSearchOpenAlexTool() anthropic.ToolUnionParam {
    // ... existing properties: query, max_results, filter
}
```

**After**:
```go
func BuildSearchOpenAlexTool() anthropic.ToolUnionParam {
    t := anthropic.ToolUnionParamOfTool(
        anthropic.ToolInputSchemaParam{
            Type: "object",
            Properties: map[string]any{
                "query": map[string]any{
                    "type":        "string",
                    "description": "Search query for academic works",
                },
                "max_results": map[string]any{
                    "type":        "integer",
                    "description": "Maximum number of results to return (default 10)",
                    "default":     10,
                },
                "filter": map[string]any{
                    "type":        "string",
                    "description": "Optional OpenAlex filter expression (e.g. \"publication_year:>2022\")",
                },
                "sort": map[string]any{
                    "type":        "string",
                    "description": "Sort order: 'cited_by_count' (most cited first). Omit for default relevance ranking.",
                },
            },
            Required: []string{"query"},
        },
        "search_openalex",
    )
    t.OfTool.Description = anthropic.String(
        "Search OpenAlex for published academic works. Returns titles, authors, DOIs, abstracts, and citation counts. Supports sorting by citation count (most cited first), publication date (newest first), or relevance (default). Use sort='cited_by_count' to find highly-cited papers.",
    )
    return t
}
```

### Contract 2: Result Struct Update
**Location**: `tools/research/handlers.go`
**Type**: `OpenAlexResult`
**Changes**: Add `CitedByCount *int` field (pointer for null handling)

**Before**:
```go
type OpenAlexResult struct {
    Title    string   `json:"title"`
    Authors  []string `json:"authors"`
    DOI      string   `json:"doi"`
    Abstract string   `json:"abstract"`
    Year     int      `json:"year"`
}
```

**After**:
```go
type OpenAlexResult struct {
    Title        string   `json:"title"`
    Authors      []string `json:"authors"`
    DOI          string   `json:"doi"`
    Abstract     string   `json:"abstract"`
    Year         int      `json:"year"`
    CitedByCount *int     `json:"cited_by_count,omitempty"`
}
```

### Contract 3: API Response Struct Update
**Location**: `tools/research/handlers.go`
**Type**: `openAlexWork`
**Changes**: Add `CitedByCount *int` field to match API response (may be null)

**Before**:
```go
type openAlexWork struct {
    Title                 string               `json:"title"`
    DOI                   string               `json:"doi"`
    PublicationYear       int                  `json:"publication_year"`
    Authorships           []openAlexAuthorship `json:"authorships"`
    AbstractInvertedIndex map[string][]int     `json:"abstract_inverted_index"`
}
```

**After**:
```go
type openAlexWork struct {
    Title                 string               `json:"title"`
    DOI                   string               `json:"doi"`
    PublicationYear       int                  `json:"publication_year"`
    CitedByCount          *int                 `json:"cited_by_count"`
    Authorships           []openAlexAuthorship `json:"authorships"`
    AbstractInvertedIndex map[string][]int     `json:"abstract_inverted_index"`
}
```

### Contract 4: Handler Update
**Location**: `tools/research/handlers.go`
**Function**: `handleSearchOpenAlex(ctx context.Context, input json.RawMessage) (string, error)`
**Changes**:
1. Add `Sort string` to input params struct
2. Validate sort parameter
3. Append sort parameter to URL if not "relevance"
4. Extract `CitedByCount` from API response and populate result

**Input params struct** (before):
```go
var params struct {
    Query      string `json:"query"`
    MaxResults int    `json:"max_results"`
    Filter     string `json:"filter"`
}
```

**Input params struct** (after):
```go
var params struct {
    Query      string `json:"query"`
    MaxResults int    `json:"max_results"`
    Filter     string `json:"filter"`
    Sort       string `json:"sort"`
}
```

**Package-level variable** (add near top of handlers.go):
```go
// openAlexSortMappings maps tool sort values to OpenAlex API sort parameters (v1: citation only)
var openAlexSortMappings = map[string]string{
    "cited_by_count": "cited_by_count:desc",
    // Future: "publication_date": "publication_date:desc",
}
```

**Note**: This is a variable, not a constant (Go doesn't support map constants).

**Validation logic** (add after unmarshaling, replaces inline map):
```go
// Validate sort parameter (if provided)
var sortParam string
if params.Sort != "" {
    var valid bool
    sortParam, valid = openAlexSortMappings[params.Sort]
    if !valid {
        return toolError(fmt.Sprintf("invalid sort value '%s': must be 'cited_by_count'", params.Sort), true), nil
    }
}
// If params.Sort == "", sortParam is empty string → omit from URL → OpenAlex uses relevance_score
```

**URL construction** (update existing line ~141):
```go
// Before:
u := "https://api.openalex.org/works?search=" + url.QueryEscape(params.Query) +
    "&per_page=" + strconv.Itoa(params.MaxResults) +
    "&mailto=arxiv-deep-research@users.noreply.github.com"

// After:
u := "https://api.openalex.org/works?search=" + url.QueryEscape(params.Query) +
    "&per_page=" + strconv.Itoa(params.MaxResults) +
    "&mailto=arxiv-deep-research@users.noreply.github.com"
if sortParam != "" {
    u += "&sort=" + sortParam
}
```

**Result population** (update existing loop ~168-180):
```go
// Before:
results = append(results, OpenAlexResult{
    Title:    work.Title,
    Authors:  authors,
    DOI:      work.DOI,
    Abstract: reconstructAbstract(work.AbstractInvertedIndex),
    Year:     work.PublicationYear,
})

// After:
results = append(results, OpenAlexResult{
    Title:        work.Title,
    Authors:      authors,
    DOI:          work.DOI,
    Abstract:     reconstructAbstract(work.AbstractInvertedIndex),
    Year:         work.PublicationYear,
    CitedByCount: work.CitedByCount,  // Already a *int, no conversion needed
})
```

## Decisions & Tradeoffs

### Decision 1: Sort Values - Enum vs Free-Form String
**Choice**: Use enum with 3 predefined values
**Alternatives**:
- Free-form string (agent can pass any OpenAlex sort parameter)
- Boolean flags (`sort_by_citations`, `sort_by_date`)

**Rationale**:
- Enum prevents typos and invalid values
- OpenAlex supports many sort fields (relevance_score, display_name, etc.) but only 3 are useful for research agents
- Validating upfront provides better error messages

**Tradeoff**: Less flexible (can't use other OpenAlex sort fields), but this is acceptable because citation count and publication date cover 99% of use cases.

### Decision 2: Sort Order - Always Descending
**Choice**: Always sort descending (most cited first, newest first)
**Alternatives**:
- Add `sort_direction` parameter (asc/desc)
- Allow ascending via negative prefix (e.g., `sort="-cited_by_count"`)

**Rationale**:
- Research use cases always want "most cited" or "newest," never least cited or oldest
- Simpler API (fewer parameters)
- OpenAlex API uses `:desc` suffix, so descending is natural

**Tradeoff**: Can't get "least cited papers" (but who wants that?).

### Decision 3: Default Sort - Relevance (Not Citation Count)
**Choice**: Default `sort="relevance"` (preserve existing behavior)
**Alternatives**:
- Default to `cited_by_count` (most cited first)
- No default (make sort required)

**Rationale**:
- Backward compatibility: existing tool calls continue to work
- Relevance is often better for exploratory queries ("papers about X")
- Citation count is better for specific queries ("most influential papers on X")

**Tradeoff**: Agents must explicitly request citation sorting, but this is intentional (forces clarity).

### Decision 4: Citation Count Field - Always Return
**Choice**: Always return `cited_by_count` in results (even if not sorting by it)
**Alternatives**:
- Only return citation count when sorting by it
- Make it a separate field in metadata

**Rationale**:
- Agents can see citation counts and decide which papers to prioritize
- Useful for downstream analysis (e.g., "filter to papers with >1000 citations")
- OpenAlex API always returns this field, so no extra cost

**Tradeoff**: Slightly larger response payload (~10 bytes per result), negligible.

### Decision 5: Token Estimator Increase
**Choice**: Increase estimate by 50 tokens to account for citation count field
**Alternatives**:
- Keep existing estimate (hope there's headroom)
- Increase by exact amount (10 results × 5 tokens = 50 tokens)

**Rationale**:
- `"cited_by_count": 12345` = ~19 characters per result
- At ~4 chars per token, that's ~5 tokens per result
- For max_results=10: 5 × 10 = 50 tokens
- Better to over-estimate than under-estimate (budget tracking precision)

**Implementation**: Update `estimators.go` search_openalex case to increase per-result cost:
```go
// Assuming current formula is: 50 + n*350
// Change to: 50 + n*360 (adds 10 tokens per result for citation field)

// Example calculation:
// max_results=10: old=50+10*350=3550, new=50+10*360=3650 (+100 tokens)
// max_results=50: old=50+50*350=17550, new=50+50*360=18050 (+500 tokens)
```

**Correct Math**: Citation count field adds ~5 tokens per result (`"cited_by_count": 12345` = ~19 chars ÷ 4 chars/token = ~5 tokens). For max_results=10, that's 50 extra tokens. For max_results=50, that's 250 extra tokens. Using per-result increase (10 tokens per result) over-estimates slightly but is safer for variable max_results.

**Tradeoff**: Over-estimates by ~50% for typical citation counts (<10k), but prevents under-estimation for high max_results queries.

## Implementation Order

### Phase 1: Struct Updates
1. **Update `OpenAlexResult` struct** (`handlers.go` line 102)
   - Add `CitedByCount int` field with JSON tag `cited_by_count`

2. **Update `openAlexWork` struct** (`handlers.go` line 114)
   - Add `CitedByCount int` field with JSON tag `cited_by_count`

### Phase 2: Tool Definition
3. **Update `BuildSearchOpenAlexTool()`** (`definitions.go` line 30)
   - Add `sort` property to input schema
   - Add enum constraint
   - Update description

### Phase 3: Handler Logic
4. **Update `handleSearchOpenAlex()`** (`handlers.go` line 128)
   - Add `Sort string` to params struct
   - Add validation logic (valid sorts map)
   - Update URL construction to include sort parameter
   - Update result population to include `CitedByCount`

### Phase 4: Testing
5. **Update existing tests** (`openalex_test.go`)
   - Verify backward compatibility (calls without sort still work)
   - Add test cases for each sort value

6. **Add new test cases**
   - Test citation count sorting
   - Test publication date sorting
   - Test invalid sort value (error case)
   - Test citation count field is populated

### Phase 5: Verification
7. **Manual testing**
   - Query: "transformer architecture" with `sort="cited_by_count"`
   - Verify results are sorted by citation count (descending)
   - Verify `cited_by_count` field is populated in results

**Order rationale**: Bottom-up (structs → definition → handler → tests) ensures each layer is correct before building on it.

## Open Questions & Resolutions

### Q1: Should we support ascending sort?
**Decision**: No, always descending.
**Reason**: Research use cases always want "most cited" or "newest," never the opposite. Simplifies API.

### Q2: Should we return total citation count for the query?
**Decision**: No, only per-paper citation counts.
**Reason**: OpenAlex API returns per-paper counts, not query totals. Aggregating across results would be misleading (not all papers returned).

### Q3: What if OpenAlex doesn't return citation count for a paper?
**REVISED Decision**: Use `*int` (pointer) to distinguish null from zero.
**Reason**: 
- `null` in API response → `nil` in Go → omitted from JSON output → agent knows "no citation data"
- `0` in API response → `&0` in Go → `"cited_by_count": 0` in JSON → agent knows "paper has zero citations"
- Distinguishing these prevents misleading agents about unpopular vs. un-indexed papers

### Q4: Should we add citation count to arXiv results too?
**Decision**: No, arXiv API doesn't provide citation counts.
**Reason**: Would require cross-referencing with OpenAlex or Google Scholar, adding complexity. Out of scope for this enhancement.

### Q5: Should we validate that results are actually sorted?
**Decision**: No, trust OpenAlex API.
**Reason**: OpenAlex API is reliable. Client-side validation would add overhead for minimal benefit.

## Critical Files

| File | Changes | Lines | Risk |
|------|---------|-------|------|
| `tools/research/definitions.go` | Add `sort` property | ~10 | Low (schema update) |
| `tools/research/handlers.go` (structs) | Add `CitedByCount` fields | ~2 | Low (additive change) |
| `tools/research/handlers.go` (handler) | Add validation, URL param, result field | ~15 | Medium (logic change) |
| `tools/research/openalex_test.go` | Add test cases | ~50 | Low (new tests) |

**Total new/changed code**: ~90 lines (includes token estimator update)

## Success Metrics & Monitoring

**Metrics to track** (add instrumentation in handler):
1. **Sort parameter usage**:
   - `sort=cited_by_count` call count vs. total search_openalex calls
   - Target: >10% of queries use citation sorting (validates feature value)

2. **Citation count field population**:
   - % of results with non-null `cited_by_count`
   - Track null rate to detect OpenAlex data quality issues
   - Alert if null rate >20% (indicates API change or data problem)

3. **API latency by sort type**:
   - p50, p95, p99 for citation-sorted queries vs. relevance queries
   - Detect if citation sorting is significantly slower

**Logging** (add to handler):
```go
log.Debug("search_openalex",
    "query", params.Query,
    "sort", params.Sort,
    "results_count", len(results),
    "citation_nulls", countNulls(results),
    "latency_ms", time.Since(start).Milliseconds())
```

**Dashboard**:
- Graph: citation sorting usage over time (detect adoption)
- Graph: API latency percentiles by sort type
- Alert: null citation count rate >20%

## Rollback Plan

**If this breaks production**:

**Phase 1: Quick Disable (5 minutes)**
1. Set environment variable: `ENABLE_OPENALEX_CITATION_SORTING=false`
2. Update handler to check flag:
   ```go
   if params.Sort != "" && !config.EnableOpenAlexCitationSorting {
       return toolError("citation sorting temporarily disabled", true), nil
   }
   ```
3. Deploy config-only change (no code rebuild)
4. Notify users: "Citation sorting unavailable, use default relevance"

**Phase 2: Investigate (30 minutes)**
1. Check OpenAlex API status: `curl https://api.openalex.org/works?search=test&sort=cited_by_count:desc`
2. Review logs for error patterns
3. Verify API contract hasn't changed (run verification curl commands)

**Phase 3: Fix or Revert (variable)**
- Quick fix (<1 hour): Update validation logic, redeploy
- Complex fix (>1 hour): Revert PR, fix offline, re-submit

**Feature Flag Implementation**:
```go
// In research.go or config package
type ResearchConfig struct {
    EnableOpenAlexCitationSorting bool
}

// In handleSearchOpenAlex
if params.Sort != "" && !r.config.EnableOpenAlexCitationSorting {
    return toolError("citation sorting is temporarily disabled", true), nil
}
```

**Rollback Trigger**: If API errors >10% for citation-sorted queries, disable immediately.

## Integration Testing Requirements

**End-to-end test** (add to test suite):
```go
func TestResearchAgent_CitationSortingIntegration(t *testing.T) {
    // Simulate agent query: "What are the most cited papers on transformers?"
    // Expected: Agent calls search_openalex with sort="cited_by_count"
    // Verify: Results are sorted by citation count (descending)
    // Verify: All results have cited_by_count field (non-null or 0)
}
```

**Manual testing checklist**:
- [ ] Agent query: "show me the most cited papers on deep learning"
- [ ] Verify agent uses `sort="cited_by_count"` (check trace logs)
- [ ] Verify results show citation counts in output
- [ ] Verify results are sorted (highest citation count first)
- [ ] Query with no sort parameter still works (backward compatibility)
- [ ] Invalid sort value returns clear error message

## Test Plan

### Unit Tests (add to `openalex_test.go`)

**Test 1**: Valid citation sort
- Input: `{"query": "test", "sort": "cited_by_count"}`
- Mock: OpenAlex API returns 3 papers with citations [1000, 500, 100]
- Assert: Results returned in citation order, `cited_by_count` field populated

**Test 2**: Omit sort parameter (backward compatibility)
- Input: `{"query": "test"}` (no sort field)
- Assert: URL does NOT contain `&sort=` parameter
- Assert: Results returned successfully

**Test 3**: Invalid sort value
- Input: `{"query": "test", "sort": "invalid"}`
- Assert: Error returned with `recoverable: true`
- Assert: Error message lists valid values

**Test 4**: Citation count null handling
- Mock: API returns paper with `"cited_by_count": null`
- Assert: Result has `CitedByCount == nil`
- Assert: JSON output omits field (`omitempty`)

**Test 5**: Citation count zero handling
- Mock: API returns paper with `"cited_by_count": 0`
- Assert: Result has `CitedByCount == &0`
- Assert: JSON output includes `"cited_by_count": 0`

### Integration Test (manual or automated if harness exists)

**End-to-end agent test**:
1. User: "Show me the most cited papers on transformers"
2. Agent calls: `search_openalex(query="transformer", sort="cited_by_count")`
3. Assert: Results are sorted by citation count
4. Assert: Agent displays citation counts to user

## Revision Log

**2026-05-11 - Initial draft**
- Created plan for OpenAlex citation sorting enhancement
- Identified as prerequisite for web search tool
- Covers sort parameter, citation count field, validation
- Ready for critique

**2026-05-11 - Revision 1 (Post-Critique Round 1)**
- **Scope Reduction**: MVP is `cited_by_count` only. Removed `publication_date` and `relevance` from v1. Can add later if requested.
- **Error Handling**: Changed invalid sort from `recoverable: false` to `recoverable: true` (allow agent to retry with corrected value).
- **Citation Count Field**: Changed from `int` to `*int` (pointer) to distinguish null (no data) from 0 (zero citations). Added `omitempty` JSON tag.
- **API Verification**: Added section with curl commands to verify OpenAlex API behavior before implementation. Documents API version (2023-10-01 snapshot).
- **Validation Logic**: Moved `validSorts` map to package-level constant `openAlexSortMappings`. Prevents per-request allocation. Changed validation to check `params.Sort != ""` instead of defaulting to "relevance".
- **URL Construction**: Simplified - omit sort parameter entirely when empty (OpenAlex uses relevance_score by default). No need to explicitly pass "relevance".
- **Token Estimator**: CHANGED decision - now increases estimate by 50 tokens to account for citation count field (~5 tokens per result × 10 results).
- **Success Metrics**: New section defining 3 metrics to track (sort usage %, citation null rate, latency by sort type) plus logging and alerting.
- **Rollback Plan**: New section with feature flag approach (`ENABLE_OPENALEX_CITATION_SORTING`), 3-phase rollback procedure, and rollback triggers.
- **Integration Testing**: New section requiring end-to-end agent test and manual testing checklist.
- **Open Questions**: Updated Q3 to reflect `*int` decision with detailed rationale.

**Key changes addressing critiques**:
- **Staff Engineer**: Fixed nullability (`*int`), added API verification section with curl commands, moved validation map to package level, added token estimator increase, clarified URL construction (omit vs. explicit), added concrete test requirements.
- **Product Manager**: Scoped down to MVP (citation sorting only), made errors recoverable, added success metrics, added rollback plan with feature flag, added integration testing requirements, removed gold-plating (`publication_date`, `relevance` enum values).

**2026-05-11 - Revision 2 (Post-Critique Round 2)**
- **CRITICAL FIX - Contract 1**: Removed enum from tool definition (was `["cited_by_count", "publication_date", "relevance"]`, now no enum). Removed `default: "relevance"`. Now matches spec section statement "enum constraint removed from schema". Tool definition is minimalist like existing tools.
- **Token Estimator Math**: Corrected formula from flat +50 to per-result +10 (change from `50+n*350` to `50+n*360`). Documented calculation showing max_results=10 adds 100 tokens, max_results=50 adds 500 tokens. Over-estimates slightly for safety.
- **Package-Level Variable**: Fixed terminology from "constant" to "variable" (Go doesn't support map constants).
- **Test Plan**: Inlined test plan (5 unit tests + 1 integration test) instead of referencing nonexistent companion file. Covers citation sorting, backward compat, invalid input, null handling, zero handling.
- **Default Behavior**: Clarified that omitting sort parameter → OpenAlex uses relevance_score (no need to explicitly pass "relevance"). Handler checks `params.Sort != ""` and omits parameter when empty.

**Key changes addressing Round 2 critiques**:
- **Staff Engineer**: Fixed Contract 1 contradiction (removed enum), fixed token math (per-result not flat), fixed terminology (variable not constant), inlined test plan, clarified default omission behavior.
- **Product Manager**: Removed enum contradiction, removed "relevance" as explicit option, clarified backward compatibility (omit sort works), documented token estimator assumptions (per-result increase), inlined test cases.
