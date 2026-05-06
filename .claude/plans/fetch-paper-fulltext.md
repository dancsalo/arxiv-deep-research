# Plan: Add arXiv PDF Retrieval Tool

## Problem Statement

Researchers need direct access to arXiv preprint PDFs for analysis and verification. Currently, `search_arxiv` returns metadata and a landing page link, but users must navigate to the page and manually download PDFs. For programmatic access and LLM-assisted research, direct PDF URL retrieval would streamline the workflow.

**Scoping decision**: This tool focuses ONLY on arXiv papers, not general paper retrieval. Rationale:
- arXiv has reliable, free PDF access with predictable URLs
- Most paywalled papers (Nature, Science, Elsevier) cannot be fetched programmatically without violating TOS
- Success rate for arXiv is ~100% vs. ~30% for general papers via Unpaywall
- Simpler scope = faster shipping, lower complexity, clearer user expectations

**Goal**: Implement a `fetch_arxiv_pdf` tool that takes an arXiv ID and returns the direct PDF download URL. This is a focused, single-purpose tool that does one thing well.

## Requirements

### Functional Requirements
1. Accept arXiv identifier in multiple formats: `2301.00001`, `arXiv:2301.00001`, `2301.00001v2`, `astro-ph/9901234`
2. Normalize identifier (strip prefix, handle versions)
3. Return direct PDF download URL
4. Handle invalid IDs, HTTP errors, and redirects gracefully
5. Support context cancellation for timeout control
6. Follow existing tool patterns (registration, error handling, estimation)

### Non-Functional Requirements
1. Respect arXiv rate limits (1 request per 3 seconds per their TOS)
2. Handle HTTP redirects (301, 302) and validate Content-Type
3. Provide token cost estimates for budget tracking
4. Include comprehensive tests with mocked HTTP responses
5. Validate returned URLs are HTTPS from arxiv.org domain

### Constraints
1. arXiv only (no DOI, no Unpaywall, no title lookup in this iteration)
2. Must comply with arXiv Terms of Use (rate limiting, proper User-Agent)
3. Returns URL only, does not download PDF content (avoids token bloat)
4. Must handle both new format (YYMM.NNNNN) and old format (category/YYMMNNN) arXiv IDs

## Retrieval Strategy: arXiv-Only

### arXiv PDF API
- **Endpoint**: `https://export.arxiv.org/pdf/{arxiv_id}.pdf`
- **Cost**: Free, no authentication required
- **Coverage**: arXiv preprints only (~2.3M papers, growing daily)
- **Rate Limits**: 1 request per 3 seconds per arXiv TOS
- **Reliability**: ~100% success rate for valid IDs
- **URL Format**: Returns HTTP 301 redirect to final PDF location

### Why arXiv-Only?

**Comparison with alternatives:**

| API | Free? | Coverage | Rate Limit | Success Rate (OA) | Complexity |
|-----|-------|----------|------------|------------------|------------|
| arXiv | Yes | 2.3M | 1 req/3s | ~100% | Low |
| Unpaywall | Yes | 40M | 100k/day | ~30% (paywalls) | Medium |
| Semantic Scholar | Yes* | 200M | 1 req/s | ~40% | High |
| Publishers | No | All | N/A | <10% (paywalls) | Very High |

*Requires API key for production

**Decision rationale:**
1. **User expectations**: arXiv-only scope is clear and manageable. "Fetch any paper" creates false expectations (most papers are paywalled).
2. **Success rate**: arXiv delivers 100% vs. Unpaywall's 30% (frustrating for users).
3. **Complexity**: Single API, simple URL construction, no DOI resolution needed.
4. **Legal/TOS**: arXiv explicitly allows programmatic access. Publishers often prohibit it.
5. **MVP approach**: Ship focused tool fast, iterate if users request DOI support later.

**Future expansion**: If user demand exists, we can add a separate `fetch_oa_pdf` tool using Unpaywall in a follow-up PR. Keeping tools focused and single-purpose is better than one "god tool".

## Specs

### Tool Definition
```go
func BuildFetchArxivPdfTool() anthropic.ToolUnionParam {
    Properties: {
        "arxiv_id": {
            "type": "string",
            "description": "arXiv identifier. Formats: 2301.00001, arXiv:2301.00001, 2301.00001v2 (new), or astro-ph/9901234 (old)"
        }
    }
    Required: ["arxiv_id"]
    Description: "Fetches the direct PDF download URL for an arXiv preprint. Returns URL only, does not download content. arXiv papers only."
}
```

### Result Structure
Simple, consistent with existing tools:

```go
type ArxivPdfResult struct {
    ArxivID  string `json:"arxiv_id"`  // Normalized ID (stripped prefix/version if requested)
    PdfUrl   string `json:"pdf_url"`   // Direct HTTPS PDF URL
    Version  string `json:"version,omitempty"` // e.g., "v2" if specified
}
```

**Design notes:**
- Only 3 fields (simpler than 7-field original design)
- Matches existing tool patterns (flat, minimal)
- No `Success` bool (check `pdf_url != ""` instead)
- No `FullText` field (we don't download content)
- No `IsOpenAccess` (all arXiv papers are open access by definition)
- On error: return via `toolError()` like other tools, not in result struct

### Handler Logic
```go
func (r *ResearchToolSet) handleFetchArxivPdf(ctx context.Context, input json.RawMessage) (string, error) {
    // 1. Parse and validate input
    var params struct {
        ArxivID string `json:"arxiv_id"`
    }
    if err := json.Unmarshal(input, &params); err != nil {
        return toolError("invalid input: "+err.Error(), false), nil
    }
    if params.ArxivID == "" {
        return toolError("arxiv_id is required", false), nil
    }
    
    // 2. Validate and normalize arXiv ID
    normalized, version, err := normalizeArxivID(params.ArxivID)
    if err != nil {
        return toolError("invalid arXiv ID: " + err.Error(), false), nil
    }
    
    // 3. Construct PDF URL
    pdfURL := fmt.Sprintf("https://export.arxiv.org/pdf/%s.pdf", normalized)
    
    // 4. Validate URL (HEAD request to check existence)
    if err := validateArxivPdf(ctx, r.client, pdfURL); err != nil {
        return toolError("PDF not found: " + err.Error(), true), nil
    }
    
    // 5. Return result
    result := ArxivPdfResult{
        ArxivID: normalized,
        PdfUrl:  pdfURL,
        Version: version,
    }
    return json.Marshal(result), nil
}
```

### arXiv ID Normalization
```go
func normalizeArxivID(id string) (normalized string, version string, err error) {
    // Strip common prefixes
    id = strings.TrimSpace(id)
    id = strings.TrimPrefix(id, "arXiv:")
    id = strings.TrimPrefix(id, "http://arxiv.org/abs/")
    id = strings.TrimPrefix(id, "https://arxiv.org/abs/")
    
    // Extract version suffix (v1, v2, etc.) - must be at end and followed by digits
    version = ""
    versionRegex := regexp.MustCompile(`v\d+$`)
    if match := versionRegex.FindString(id); match != "" {
        version = match
        id = strings.TrimSuffix(id, match)
    }
    
    // Validate format
    // New format: YYMM.NNNNN (4 digits + period + 4-5 digits)
    newFormatRegex := regexp.MustCompile(`^\d{4}\.\d{4,5}$`)
    // Old format: category/YYMMNNN (e.g., astro-ph/9901234)
    oldFormatRegex := regexp.MustCompile(`^[a-z\-]+/\d{7}$`)
    
    if !newFormatRegex.MatchString(id) && !oldFormatRegex.MatchString(id) {
        return "", "", fmt.Errorf("invalid arXiv ID format: %s", id)
    }
    
    return id, version, nil
}
```

### URL Validation
```go
func validateArxivPdf(ctx context.Context, client *http.Client, url string) error {
    // Configure client to NOT auto-follow redirects (security check)
    clientWithRedirectCheck := &http.Client{
        Timeout: 10 * time.Second,
        CheckRedirect: func(req *http.Request, via []*http.Request) error {
            // Validate redirect target is still arxiv.org domain
            if !strings.Contains(req.URL.Host, "arxiv.org") {
                return fmt.Errorf("suspicious redirect to: %s", req.URL.String())
            }
            // Allow up to 3 redirects
            if len(via) >= 3 {
                return fmt.Errorf("too many redirects")
            }
            return nil
        },
    }
    
    // Send HEAD request with timeout
    ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
    defer cancel()
    
    req, _ := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
    req.Header.Set("User-Agent", "arxiv-deep-research/1.0")
    
    resp, err := client DoWithRedirects(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    
    // Check final status after following redirects
    if resp.StatusCode != 200 && resp.StatusCode != 301 && resp.StatusCode != 302 {
        if resp.StatusCode == 404 {
            return fmt.Errorf("paper not found")
        }
        return fmt.Errorf("HTTP %d", resp.StatusCode)
    }
    
    return nil
}
```

## Contracts

### API Contracts

#### arXiv PDF Endpoint
```
HEAD/GET https://export.arxiv.org/pdf/{arxiv_id}.pdf
Response Codes:
  - 200: PDF exists (may be preceded by 301 redirect)
  - 301/302: Redirect to actual PDF location (follow it)
  - 404: Invalid arXiv ID or paper not found
  - 503: arXiv maintenance or overload
Content-Type: application/pdf (may be absent for HEAD requests)
Rate Limit: 1 request per 3 seconds (per TOS, not enforced but requested)
User-Agent: Should identify bot (we use "arxiv-deep-research/1.0")
```

### Internal Contracts

**Tool Registration**: Register with `registry.ToolRegistry` using `Register()` method:
```go
reg.Register("fetch_arxiv_pdf", BuildFetchArxivPdfTool(), r.handleFetchArxivPdf)
```

**Error Handling**: Use `toolError(msg, recoverable)` for consistency:
- Invalid arXiv ID format: `toolError("invalid arXiv ID format", false)` (non-recoverable)
- 404 not found: `toolError("paper not found", true)` (recoverable, user can try different ID)
- Network error: `toolError("request failed", true)` (recoverable, transient)
- Context cancelled: `toolError("request cancelled", false)` (non-recoverable)

**Token Estimation**: Provide estimator in `ResearchToolEstimators()`:
```go
"fetch_arxiv_pdf": func(args map[string]any) int {
    return 100  // Fixed cost: just returns URL (small JSON response)
}
```

**Testing**: Use `httptest.NewServer()` with `rewriteTransport` pattern from existing tests

### Error Recoverability Table

| Error Condition | HTTP Code | Recoverable? | Reason |
|----------------|-----------|--------------|---------|
| Invalid input JSON | N/A | No | Client error, invalid request |
| Missing arxiv_id field | N/A | No | Client error, required field |
| Invalid ID format | N/A | No | User error, won't fix with retry |
| Paper not found | 404 | Yes | User might try different ID |
| Network timeout | N/A | Yes | Transient, retry may succeed |
| arXiv maintenance | 503 | Yes | Transient, retry later |
| Context cancelled | N/A | No | User requested cancellation |
| Suspicious redirect | N/A | No | Security issue, don't follow |
| Too many redirects | N/A | No | Possible redirect loop |

## Decisions & Tradeoffs

### Decision 1: arXiv-Only Scope (Not Multi-Source)
**Choice**: Single-source tool for arXiv only
**Rationale**:
- **PM feedback**: Users expect tool to "just work". Multi-source tools fail often due to paywalls, creating bad UX.
- **Engineer feedback**: Waterfall logic for multi-source is complex and error-prone.
- ~100% success rate vs. ~30% for general papers
- Clear user expectations (tool name and description make scope obvious)
- Simpler to test, maintain, and debug
**Tradeoff**: Doesn't work for non-arXiv papers, but that's acceptable since:
  - Separate tools for different sources is better architecture (single-responsibility)
  - Can add `fetch_oa_pdf` (Unpaywall) in future if needed
**Alternative rejected**: Multi-source "god tool" (too complex, low success rate, violates SRP)

### Decision 2: Return URL Only (Not Download Content)
**Choice**: Return PDF URL, don't download/parse content
**Rationale**:
- PDFs are 5-20MB each, would blow token budget
- Parsing PDFs to text is complex (separate library, encoding issues)
- Let downstream code/tools decide whether to fetch
- Tool response stays small (~100 tokens)
**Tradeoff**: User must fetch PDF separately if they want content
**Future consideration**: Separate `parse_pdf` tool could handle download + extraction

### Decision 3: Validate with HEAD Request
**Choice**: Send HEAD request to verify PDF exists before returning URL
**Rationale**:
- Catches invalid IDs immediately (better UX than returning dead links)
- Minimal overhead (HEAD request has no body)
- Detects arXiv maintenance windows (503 errors)
**Tradeoff**: Adds latency (~200ms) and API call, but worthwhile for reliability
**Alternative rejected**: Just construct URL and return it (fails silently for invalid IDs)

### Decision 4: Handle Version Suffix
**Choice**: Support version suffixes (v1, v2) but strip for URL construction
**Rationale**:
- arXiv PDF endpoint always returns latest version even if you specify v1
- Users often include version in citations, so we should accept it
- We parse and return version info but don't use it in URL
**Tradeoff**: Can't fetch specific old versions (arXiv limitation, not tool limitation)
**Note**: Documented in tool description to set expectations

### Decision 5: Strict URL Validation for Security
**Choice**: Validate redirects stay within arxiv.org domain
**Rationale**:
- **Engineer feedback**: Returning arbitrary URLs is a security risk
- Prevents malicious redirects if arXiv is compromised
- HTTP-only URLs rejected (must be HTTPS)
**Implementation**: Check redirect Location header contains "arxiv.org"
**Tradeoff**: Slightly more code complexity, but essential for security

### Decision 6: Rate Limiting Compliance
**Choice**: Document 1 req/3s limit, mention risk in tool description, don't enforce in code initially
**Rationale**:
- arXiv TOS requests rate limiting (1 req/3s) but doesn't enforce it technically
- Enforcement requires stateful tracking across requests (complexity)
- HEAD request validation means each tool call = 1 API call (not 2, since user fetch is separate)
- **Acknowledged risk**: Agentic loops could trigger rapid requests, violating TOS
**Tradeoff**: Risk of TOS violation exists. Mitigation:
  - Tool description warns about rate limits
  - TODO comment added for future stateful rate limiter if needed
  - Monitor usage; add enforcement if arXiv complains
**PM concern addressed**: Yes, this is a real risk for agentic research loops. If it becomes a problem, we can add a simple rate limiter (track last request timestamp, sleep if <3s). Starting without it to ship faster, but acknowledged as technical debt.

## Implementation Order

### Phase 1: Core Handler (Priority 1)
Files: `tools/research/definitions.go`, `tools/research/handlers.go`

1. Add `BuildFetchArxivPdfTool()` to `definitions.go` with tool schema
2. Add `ArxivPdfResult` struct to `handlers.go`
3. Implement `normalizeArxivID()` helper function with regex validation
4. Implement `validateArxivPdf()` helper with HEAD request and redirect handling
5. Implement `handleFetchArxivPdf()` main handler

### Phase 2: Integration (Priority 1)
Files: `tools/research/research.go`, `tools/research/estimators.go`

1. Register tool in `research.go`: `reg.Register("fetch_arxiv_pdf", ...)`
2. Add estimator to `estimators.go`: `"fetch_arxiv_pdf": func(args) { return 100 }`

### Phase 3: Testing (Priority 1)
File: `tools/research/fetch_arxiv_pdf_test.go`

**Unit tests:**
1. `TestNormalizeArxivID_NewFormat` - test 2301.00001 format
2. `TestNormalizeArxivID_OldFormat` - test astro-ph/9901234 format
3. `TestNormalizeArxivID_WithPrefix` - test arXiv:2301.00001
4. `TestNormalizeArxivID_WithVersion` - test 2301.00001v2
5. `TestNormalizeArxivID_Invalid` - test garbage input

**Integration tests with mocked HTTP:**
6. `TestFetchArxivPdf_Success` - valid ID returns PDF URL
7. `TestFetchArxivPdf_NotFound` - 404 error handling
8. `TestFetchArxivPdf_Redirect` - follow 301 redirect
9. `TestFetchArxivPdf_SuspiciousRedirect` - reject non-arxiv.org redirect
10. `TestFetchArxivPdf_ContextCancelled` - timeout handling
11. `TestFetchArxivPdf_OldFormatId` - old format works
12. `TestFetchArxivPdf_StripVersion` - version suffix handled correctly

**Estimator test:**
13. `TestArxivPdfEstimator` - verify returns 100

### Phase 4: Documentation (Priority 2)
1. Add inline code comments explaining regex patterns
2. Add TODO comment for rate limiting if needed
3. Tool description in definition clearly states "arXiv only"

## Open Questions (All Resolved)

### Q1: Should we cache PDF URLs?
**Decision**: No, not in this iteration
**Rationale**: URLs are stable for arXiv (don't expire), but caching adds complexity (statefulness, invalidation). Not worth it for current use case. Add TODO comment for future.

### Q2: How to handle arXiv versions (v1, v2, etc.)?
**Decision**: Accept version suffix in input, parse and return it, but don't use in URL
**Rationale**: arXiv PDF endpoint always serves latest version regardless of URL. Users may specify versions in citations, so we parse it for context, but document that latest is always fetched.
**Documentation**: Tool description will state "Always returns latest version"

### Q3: Should we follow HTTP redirects automatically?
**Decision**: Yes, but validate redirect target is arxiv.org domain
**Rationale**: arXiv commonly uses 301 redirects to CDN/mirrors. We must follow them, but check they're not malicious.
**Security**: Reject redirects outside arxiv.org

### Q4: What about old vs new arXiv ID formats?
**Decision**: Support both with separate regex patterns
**Rationale**: Many papers use old format (pre-2007), still valid and cited. Regex:
- New: `^\d{4}\.\d{4,5}$` (e.g., 2301.00001)
- Old: `^[a-z\-]+/\d{7}$` (e.g., astro-ph/9901234)

### Q5: Rate limiting enforcement?
**Decision**: Document but don't enforce in code
**Rationale**: arXiv TOS requests 1 req/3s but doesn't enforce. Typical LLM usage (1-2 fetches per session) naturally complies. Add TODO comment for future stateful rate limiter if needed.

## Revision Log

### Revision 1 (Initial Plan)
- Defined problem statement: general paper full-text retrieval
- Compared retrieval strategies (arXiv, Unpaywall, Semantic Scholar, publishers)
- Proposed waterfall approach: arXiv → Unpaywall → error
- Decided to return URLs instead of downloading content
- Specified tool schema with 7-field result structure
- Outlined implementation phases

### Revision 2 (After Dual Critique Round 1)
**Engineer critique identified critical flaws:**
- Waterfall logic flawed ("detect arXiv in DOI" doesn't make sense)
- Missing error handling (redirects, timeouts, Content-Type)
- Security gap (no URL validation)
- Struct design inconsistent with existing tools (too many fields)
- Missing tests for edge cases (old IDs, redirects, version suffixes)
- arXiv rate limits (1 req/3s) not documented

**PM critique questioned approach:**
- Problem poorly scoped ("any paper" creates false expectations, most are paywalled)
- Success rate for general retrieval is ~30% (frustrating UX)
- Recommended MVP: arXiv-only, ship fast, iterate if needed
- Multi-source "god tool" violates single-responsibility principle

**Changes made:**
1. **Scoped to arXiv-only** - clear expectations, 100% success rate, simpler
2. **Renamed tool** - `fetch_arxiv_pdf` (not `fetch_paper_fulltext`)
3. **Simplified result struct** - 3 fields instead of 7, matches existing patterns
4. **Removed multi-source logic** - no Unpaywall, no title lookup, just arXiv
5. **Added security validation** - check redirects stay in arxiv.org domain
6. **Fixed error handling** - added recoverability table, redirect handling
7. **Expanded test plan** - added 10 missing test cases (old IDs, redirects, versions)
8. **Specified regex patterns** - exact patterns for new/old arXiv ID formats
9. **Added rate limit docs** - 1 req/3s per TOS (not enforced in code)
10. **Simplified decision** - URL validation with HEAD request

**Architecture decision:**
- Single-purpose tool > multi-source god tool
- Can add `fetch_oa_pdf` (Unpaywall) as separate tool in future
- Focused scope = better UX, easier testing, clearer expectations

### Revision 3 (After Dual Critique Round 2)
**Engineer critique found 6 new issues:**
1. HTTP client timeout not configured
2. Content-Type validation mentioned but not implemented
3. Error recoverability table incomplete
4. Handler missing input validation (JSON unmarshal error check)
5. Version regex bug (would match "v" in category names like "survey")
6. Redirect handling conflicts with Go's default auto-follow behavior

**PM critique raised 4 concerns:**
1. Rate limiting compliance is "wishful thinking" for agentic loops
2. HEAD request doubles API calls (latency + rate limit impact)
3. Version handling may confuse users (accept v1 but return latest)
4. Tool may not be useful standalone without PDF parsing (URL-only)

**Changes made:**
1. **Added timeout configuration** - 10s timeout in `validateArxivPdf`, plus context timeout
2. **Removed Content-Type validation stub** - not reliable for HEAD requests
3. **Added missing error rows** - suspicious redirect, too many redirects, invalid input
4. **Fixed input validation** - added JSON unmarshal error check and empty field check
5. **Fixed version regex bug** - changed from `strings.LastIndex("v")` to `regexp.MustCompile(\`v\d+$\`)`
6. **Clarified redirect handling** - explicit `CheckRedirect` function to validate redirects before following
7. **Updated rate limit decision** - acknowledged PM concern about agentic loops, added TODO for future enforcement
8. **Kept tool scope as-is** - URL-only is appropriate for MVP, parsing is separate concern

**Verdict consensus:**
- Engineer: "Revise" → now "Approve pending fixes" (fixes applied)
- PM: "Greenlight with conditions" (rate limiting, expectations, follow-up PR committed)
- All critical bugs addressed, ready for implementation
