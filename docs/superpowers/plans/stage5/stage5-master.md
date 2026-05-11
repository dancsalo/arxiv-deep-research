---
name: Enhanced Research Demo with Planning Step
description: Add structured research planning and expand tools for comprehensive topic analysis
type: feature
---

## Context

The current research demo (cmd/research-demo/main.go) uses a simple system prompt that encourages iterative searching with search_arxiv and search_openalex tools. The agent explores topics broadly and produces a summary at the end.

This enhancement transforms the research demo into a **structured, multi-section research agent** that:
1. **Plans first**: Maps out tasks before execution using a required tool call
2. **Seminal papers section**: Uses web search to find highly-cited papers, validates them via arXiv/OpenAlex, retrieves full text, and summarizes each
3. **Fundamental techniques section**: Deep-dives into algorithms using full-text papers and searches GitHub for implementations
4. **Survey papers section**: Finds 2-3 survey papers, extracts full text, and analyzes application areas, tradeoffs, and key insights

The user wants the agent to be more methodical and produce richer research outputs with multiple distinct sections.

## Implementation Plan

### Phase 1: New Tool Development

Build four new research tools to support the enhanced workflow:

#### 1.1 DuckDuckGo Search Tool
**File**: `tools/research/definitions.go`, `tools/research/handlers.go`

Add `BuildSearchWebTool()` and `handleSearchWeb()`:
- Tool name: `search_web`
- Input schema: `query` (string, required)
- Uses DuckDuckGo HTML search API (https://html.duckduckgo.com/html/)
- Handler parses HTML results and extracts titles, snippets, and URLs
- Returns JSON array of search results (max 10)
- Error handling: Retry once on failure, return "recoverable" error

#### 1.2 GitHub Repository Search Tool
**File**: `tools/research/definitions.go`, `tools/research/handlers.go`

Add `BuildSearchGithubTool()` and `handleSearchGithub()`:
- Tool name: `search_github_repos`
- Input schema: `query` (string, required), `max_results` (int, default 10)
- Uses GitHub REST API: `GET /search/repositories?q={query}&sort=stars&order=desc`
- Requires GitHub token from environment variable `GITHUB_TOKEN` (optional but recommended for rate limits)
- Handler returns JSON array with: name, full_name, description, stars, url, language
- Rate limit handling: Respect GitHub API rate limits (5000/hour authenticated, 60/hour unauthenticated)
- Error handling: Return clear error if rate limited or token invalid

#### 1.3 arXiv PDF Text Extraction Tool
**File**: `tools/research/definitions.go`, `tools/research/handlers.go`

Add `BuildFetchArxivPdfTextTool()` and `handleFetchArxivPdfText()`:
- Tool name: `fetch_arxiv_pdf_text`
- Input schema: `arxiv_id` (string, required), `max_pages` (int, optional, default all)
- Downloads PDF using existing `fetch_arxiv_pdf` logic
- Extracts text using Go PDF library (recommend: `github.com/ledongthuc/pdf`)
- Handler flow:
  1. Reuse `normalizeArxivID()` and `validateArxivPdf()` helpers
  2. Download PDF to temporary file
  3. Extract text page by page
  4. Clean up: remove excessive whitespace, handle multi-column layouts
  5. Return JSON: `{arxiv_id, text, page_count, truncated}`
- Truncation: If text exceeds ~100k chars, truncate and set `truncated: true`
- Rate limiting: Add 3-second delay per arXiv TOS (shared with existing fetch tool)
- Error handling: Return "recoverable" error for network issues, "non-recoverable" for invalid ID

#### 1.4 Research Plan Tool
**File**: `tools/research/definitions.go`, `tools/research/handlers.go`

Add `BuildCreateResearchPlanTool()` and `handleCreateResearchPlan()`:
- Tool name: `create_research_plan`
- Input schema: `topic` (string), `sections` (array of objects with `name` and `description`)
- Validates that required sections are present:
  - "Seminal Papers" (or similar: pivotal, highly-cited, foundational)
  - "Fundamental Techniques" (or similar: algorithms, methods, math)
  - "Survey Papers" (or similar: review, landscape, applications)
- Handler returns JSON confirmation: `{status: "approved", sections: [list of section names]}`
- If validation fails (missing section), returns error with guidance
- This tool is **required** as the first step in the research workflow

### Phase 2: System Prompt Enhancement

**File**: `cmd/research-demo/main.go` (line 89)

Replace the current system prompt with a new structured version:

```
You are an expert research assistant that conducts comprehensive, multi-section research on academic topics.

WORKFLOW:
1. PLANNING PHASE (Required First Step):
   - Call create_research_plan tool with topic and planned sections
   - Required sections: Seminal Papers, Fundamental Techniques, Survey Papers
   - The tool will validate your plan before you proceed

2. SEMINAL PAPERS SECTION:
   Goal: Find and summarize 3-5 pivotal, highly-cited papers that define this topic
   Steps:
   a) Use search_web with query: "most cited papers related to: <topic>"
   b) Extract paper titles/authors from the web search results
   c) For each candidate paper:
      - Search arXiv using title search to find the exact paper
      - Verify citation count using search_openalex (filter by DOI or title)
   d) Rank papers by citation count and select top 3-5
   e) For each selected paper:
      - Call fetch_arxiv_pdf_text to get full text
      - Generate a 2-3 paragraph summary covering:
        * Core contribution
        * Key techniques/methods
        * Impact on the field
   
3. FUNDAMENTAL TECHNIQUES SECTION:
   Goal: Deep dive into the core algorithm/technique/math
   Steps:
   a) Based on seminal papers, identify the key technique to analyze
   b) Extract how it works from the full-text papers
   c) Call search_github_repos to find implementations (query: "<technique-name> implementation")
   d) Select top 2 repos by stars
   e) For each repo, provide:
      - Repository name, stars, language
      - Summary of implementation approach
      - Notable features or differences from paper

4. SURVEY PAPERS SECTION:
   Goal: Find 2-3 survey/review papers and analyze the broader landscape
   Steps:
   a) Use search_arxiv with query: "survey <topic>" or "<topic> review"
   b) Identify 2-3 recent survey papers (prefer post-2020)
   c) For each survey:
      - Call fetch_arxiv_pdf_text to get full text
      - Extract and summarize:
        * Application areas for this topic
        * Tradeoffs compared to alternative techniques
        * Open challenges or future directions
   
5. FINAL OUTPUT:
   When all sections are complete, call finish_loop with a markdown summary containing:
   - Section 1: Seminal Papers (with paper summaries)
   - Section 2: Fundamental Techniques (algorithm explanation + GitHub repos)
   - Section 3: Survey Insights (applications, tradeoffs, future directions)

TOOL USAGE NOTES:
- search_web: Use for finding "most cited" papers via general web search
- search_arxiv: Use for finding specific papers by title or topic keywords
- search_openalex: Use for verifying citation counts and metadata
- fetch_arxiv_pdf_text: Use to read full paper content for deep analysis
- search_github_repos: Use to find implementations, sort by stars
- create_research_plan: MUST be called first before any searches

RESEARCH QUALITY:
- Always verify papers exist in arXiv before claiming to analyze them
- Use full-text content (not just abstracts) for summaries
- Provide specific details: citation counts, author names, repo stars
- If a tool fails (paper not found, rate limit), adapt and find alternatives
```

### Phase 3: Tool Registration

**File**: `tools/research/research.go` (line 18-21)

Update the `Register()` method to add the new tools:

```go
func (r *ResearchToolSet) Register(reg *registry.ToolRegistry) {
    reg.Register("search_arxiv", BuildSearchArxivTool(), r.handleSearchArxiv)
    reg.Register("search_openalex", BuildSearchOpenAlexTool(), r.handleSearchOpenAlex)
    reg.Register("fetch_arxiv_pdf", BuildFetchArxivPdfTool(), r.handleFetchArxivPdf)
    
    // New tools
    reg.Register("search_web", BuildSearchWebTool(), r.handleSearchWeb)
    reg.Register("search_github_repos", BuildSearchGithubTool(), r.handleSearchGithub)
    reg.Register("fetch_arxiv_pdf_text", BuildFetchArxivPdfTextTool(), r.handleFetchArxivPdfText)
    reg.Register("create_research_plan", BuildCreateResearchPlanTool(), r.handleCreateResearchPlan)
}
```

### Phase 4: Dependencies and Configuration

**File**: `go.mod`

Add new dependency for PDF text extraction:
- `github.com/ledongthuc/pdf` (for PDF parsing)

**File**: `cmd/research-demo/main.go` (line 40-51)

Add environment variable handling for GitHub token:
- Check for `GITHUB_TOKEN` env var (optional)
- Pass to ResearchToolSet constructor: `research.NewResearchToolSet(httpClient, os.Getenv("GITHUB_TOKEN"))`
- Update ResearchToolSet struct to accept and store token

**File**: `tools/research/research.go` (line 10-16)

Update `ResearchToolSet` struct:
```go
type ResearchToolSet struct {
    client      *http.Client
    githubToken string
}

func NewResearchToolSet(client *http.Client, githubToken string) *ResearchToolSet {
    return &ResearchToolSet{
        client:      client,
        githubToken: githubToken,
    }
}
```

### Phase 5: Rate Limiting and Robustness

**File**: `tools/research/handlers.go`

Add rate limiting mechanisms:
1. **arXiv rate limiter**: Create a shared rate limiter (1 request per 3 seconds) for both `fetch_arxiv_pdf` and `fetch_arxiv_pdf_text`
2. **GitHub rate limiter**: Track remaining API quota and return clear errors when exhausted
3. **DuckDuckGo throttling**: Add small delays between requests to avoid blocks

Implementation approach:
- Use `golang.org/x/time/rate` for rate limiting
- Store limiters on `ResearchToolSet` struct
- Initialize in `NewResearchToolSet()`

### Phase 6: Testing

**Files**: `tools/research/*_test.go`

Create test files for new tools:

1. `tools/research/search_web_test.go`:
   - Test HTML parsing of DuckDuckGo results
   - Test error handling for network failures
   - Use httptest.Server for mocked responses

2. `tools/research/search_github_test.go`:
   - Test GitHub API response parsing
   - Test rate limit handling
   - Test authenticated vs unauthenticated requests
   - Mock GitHub API with httptest

3. `tools/research/fetch_arxiv_pdf_text_test.go`:
   - Test PDF text extraction with sample PDF
   - Test truncation logic for large papers
   - Test rate limiting coordination with fetch_arxiv_pdf
   - Use embedded test PDF or httptest for PDF download

4. `tools/research/create_plan_test.go`:
   - Test validation logic for required sections
   - Test various section name formats (e.g., "Seminal Papers" vs "Foundational Papers")
   - Test error messages for missing sections

### Phase 7: Documentation

**File**: `README.md` (if exists, or create `cmd/research-demo/README.md`)

Document the new research demo workflow:
- Tool descriptions and usage
- Environment variables (GITHUB_TOKEN)
- Example queries and expected output structure
- Rate limiting considerations

## Critical Files

Primary implementation files:
- `tools/research/definitions.go` - Tool schemas (4 new tools)
- `tools/research/handlers.go` - Tool implementations (4 new handlers)
- `tools/research/research.go` - Registration and struct updates
- `cmd/research-demo/main.go` - System prompt and setup

## Verification

End-to-end testing steps:

1. **Build and run**:
   ```bash
   make build
   ./bin/research-demo --query "attention mechanisms in transformers" --max-turns 30
   ```

2. **Verify planning phase**:
   - Check trace logs for `create_research_plan` tool call in turn 0 or 1
   - Verify tool returns `status: approved`

3. **Verify seminal papers section**:
   - Check for `search_web` call with "most cited papers" query
   - Verify `search_arxiv` calls for specific paper titles
   - Verify `search_openalex` calls for citation verification
   - Verify `fetch_arxiv_pdf_text` calls for full-text retrieval
   - Count: Should see 3-5 papers with summaries

4. **Verify techniques section**:
   - Check for `search_github_repos` calls
   - Verify at least 2 repository results in final output

5. **Verify survey section**:
   - Check for `search_arxiv` with "survey" keyword
   - Verify `fetch_arxiv_pdf_text` calls for surveys
   - Verify output includes applications, tradeoffs, future directions

6. **Run tests**:
   ```bash
   make test-go
   ```

7. **Check trace output**:
   - Verify all tool calls succeeded or failed gracefully
   - Check token usage is within budget (200k context)
   - Verify cost is under $0.50 threshold

## Implementation Notes

### PDF Extraction Considerations
- The `github.com/ledongthuc/pdf` library is simple but may struggle with complex layouts
- Alternative: Call out to Python script if Go library insufficient
- Add truncation early to avoid context window issues with long papers

### GitHub API Rate Limits
- Unauthenticated: 60 requests/hour per IP
- Authenticated: 5000 requests/hour
- Tool should gracefully degrade if no token provided
- Clear error messages when rate limited

### DuckDuckGo Search
- DuckDuckGo HTML endpoint doesn't require API keys
- May require user-agent spoofing to avoid blocks
- Fallback: If blocked, tool should return error and agent can skip web search step

### System Prompt Length
- The enhanced prompt is longer (~1200 tokens)
- This is acceptable since it's cached and stable across turns
- Consider adding a shorter "reminder" version if token budget becomes tight

### Extensibility
- The section-based approach can be extended in the future
- New sections (e.g., "Historical Context", "Industrial Applications") can be added
- The `create_research_plan` tool validation can be made configurable

## Dependency Changes

New Go dependencies required:
- `github.com/ledongthuc/pdf` - PDF text extraction
- `golang.org/x/time/rate` - Rate limiting (may already be indirect dependency)

Check with:
```bash
go get github.com/ledongthuc/pdf
go mod tidy
```
