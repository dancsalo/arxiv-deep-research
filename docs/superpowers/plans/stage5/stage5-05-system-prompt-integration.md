---
name: System Prompt and Integration
description: Update system prompt, register tools, and test end-to-end workflow
type: feature
parent: stage5-master
---

## Context

This is the final integration plan that brings together all the new tools into a cohesive research workflow. It involves:
1. Updating the system prompt with structured instructions
2. Registering all 4 new tools
3. Ensuring dependencies are in place
4. End-to-end testing of the complete workflow

This plan depends on completion of plans 01-04.

## Implementation Steps

### Step 1: Register New Tools

**File**: `tools/research/research.go`

Update the `Register()` method to include all new tools:

```go
func (r *ResearchToolSet) Register(reg *registry.ToolRegistry) {
	// Existing tools
	reg.Register("search_arxiv", BuildSearchArxivTool(), r.handleSearchArxiv)
	reg.Register("search_openalex", BuildSearchOpenAlexTool(), r.handleSearchOpenAlex)
	reg.Register("fetch_arxiv_pdf", BuildFetchArxivPdfTool(), r.handleFetchArxivPdf)
	
	// New tools (Stage 5)
	reg.Register("search_web", BuildSearchWebTool(), r.handleSearchWeb)
	reg.Register("search_github_repos", BuildSearchGithubTool(), r.handleSearchGithub)
	reg.Register("fetch_arxiv_pdf_text", BuildFetchArxivPdfTextTool(), r.handleFetchArxivPdfText)
	reg.Register("create_research_plan", BuildCreateResearchPlanTool(), r.handleCreateResearchPlan)
}
```

### Step 2: Update System Prompt

**File**: `cmd/research-demo/main.go`

Replace the system prompt (around line 89-91) with the new structured version:

```go
systemBlocks := []anthropic.TextBlockParam{
	{Text: `You are an expert research assistant that conducts comprehensive, multi-section research on academic topics.

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
- If a tool fails (paper not found, rate limit), adapt and find alternatives`, Type: "text"},
}
```

### Step 3: Increase Max Turns

**File**: `cmd/research-demo/main.go`

The new workflow is more complex and may need more turns. Update default max turns (around line 35):

```go
maxTurns := flag.Int("max-turns", 30, "maximum agentic loop turns")
```

Update the budget if needed (around line 99-103):

```go
Budget: &ctxmgr.ContextBudget{
	ModelContextLimit: 200000,
	MaxOutputTokens:   8192,
	SafetyMargin:      2000,
},
```

### Step 4: Verify Dependencies

**File**: `go.mod`

Ensure all new dependencies are present:

```bash
go get github.com/ledongthuc/pdf
go get golang.org/x/net/html
go get golang.org/x/time/rate
go mod tidy
```

### Step 5: Update Documentation

**File**: `cmd/research-demo/README.md` (create if doesn't exist)

Create documentation for the new workflow:

```markdown
# Research Demo

Comprehensive research assistant that produces structured, multi-section research reports on academic topics.

## Features

- **Planning Phase**: Creates and validates research plan before execution
- **Seminal Papers**: Finds highly-cited foundational papers via web search, validates with OpenAlex
- **Fundamental Techniques**: Deep-dives into core algorithms with GitHub implementation search
- **Survey Papers**: Analyzes review papers for applications, tradeoffs, and future directions

## Tools

- `search_web` - DuckDuckGo web search for finding "most cited" papers
- `search_arxiv` - arXiv API search for academic papers
- `search_openalex` - OpenAlex API for citation counts and metadata
- `fetch_arxiv_pdf` - Get PDF download URL for arXiv papers
- `fetch_arxiv_pdf_text` - Download and extract full text from arXiv PDFs
- `search_github_repos` - GitHub API search for repository implementations
- `create_research_plan` - Validates research plan structure (required first step)

## Usage

Basic usage:
```bash
./bin/research-demo --query "attention mechanisms in transformers"
```

With GitHub token for higher rate limits:
```bash
export GITHUB_TOKEN="your_github_token"
./bin/research-demo --query "transformers"
```

Options:
- `--query` - Research topic (default: "retrieval augmented generation")
- `--max-turns` - Maximum agentic loop turns (default: 30)
- `--trace-dir` - Directory for trace files (default: ".traces")
- `--bedrock` - Use AWS Bedrock (default: true)
- `--model` - Model ID override

## Environment Variables

- `GITHUB_TOKEN` - GitHub personal access token (optional, recommended)
  - Without: 60 API requests/hour
  - With: 5000 API requests/hour
  - Get token at: https://github.com/settings/tokens

## Output

The research demo produces a structured markdown report with three sections:

1. **Seminal Papers** (3-5 papers)
   - Title, authors, citation count
   - 2-3 paragraph summary per paper
   - Core contributions and impact

2. **Fundamental Techniques**
   - Algorithm/technique explanation
   - Top 2 GitHub implementations
   - Implementation details and differences

3. **Survey Insights**
   - Application areas
   - Tradeoffs vs alternatives
   - Open challenges and future directions

## Rate Limits

- **arXiv**: 1 request per 3 seconds (enforced automatically)
- **GitHub**: 60/hour (unauthenticated) or 5000/hour (authenticated)
- **DuckDuckGo**: Best effort throttling to avoid blocks
- **OpenAlex**: 100,000 requests/day (polite pool)

## Examples

Transformers:
```bash
./bin/research-demo --query "attention mechanisms in transformers"
```

Retrieval Augmented Generation:
```bash
./bin/research-demo --query "retrieval augmented generation"
```

Graph Neural Networks:
```bash
./bin/research-demo --query "graph neural networks"
```

## Trace Files

Trace files are saved to `.traces/` directory with session ID filenames. Each trace contains:
- All tool calls with input/output
- Token usage per turn
- Cost tracking
- Timing information

View trace:
```bash
cat .traces/demo-<timestamp>.json | jq
```
```

## Verification

### Step 1: Build

```bash
make build
```

### Step 2: Run Unit Tests

```bash
make test-go
```

Verify all tests pass for:
- `tools/research/search_web_test.go`
- `tools/research/search_github_test.go`
- `tools/research/fetch_arxiv_pdf_text_test.go`
- `tools/research/create_plan_test.go`

### Step 3: Integration Test (Short)

Run a quick test with a simple query:

```bash
export GITHUB_TOKEN="your_token"  # Optional but recommended
./bin/research-demo --query "attention mechanisms" --max-turns 10
```

Expected behavior:
- Turn 0-1: `create_research_plan` called, returns "approved"
- Turn 2-3: `search_web` called for "most cited papers"
- Turn 4+: Mix of `search_arxiv`, `search_openalex`, `fetch_arxiv_pdf_text`, `search_github_repos`
- Final turn: `finish_loop` with structured markdown output

### Step 4: Full Integration Test

Run the complete workflow:

```bash
./bin/research-demo --query "transformer architecture" --max-turns 30 --trace-dir .traces
```

Verify output includes:
1. **Planning confirmation**: "Research plan approved..."
2. **Seminal Papers section**: 3-5 papers with summaries
3. **Fundamental Techniques section**: Algorithm explanation + 2 GitHub repos
4. **Survey Insights section**: Applications, tradeoffs, future directions

Check trace file:
```bash
cat .traces/demo-*.json | jq '.turns | length'  # Should see 15-30 turns
cat .traces/demo-*.json | jq '.turns[].tool_calls[] | .tool'  # Should see all tools used
```

### Step 5: Verify Rate Limiting

Test arXiv rate limiter:
```bash
# Run demo and watch logs - should see 3-second delays between PDF fetches
./bin/research-demo --query "transformers" 2>&1 | grep -i "rate\|wait"
```

Test GitHub rate limit handling:
```bash
# Run without GITHUB_TOKEN and make 61+ requests to trigger limit
unset GITHUB_TOKEN
# Should see clear error message about rate limits
```

### Step 6: Error Handling Tests

Test with invalid/missing data:
```bash
# Non-existent paper
./bin/research-demo --query "fake paper that does not exist xyz123"
# Should gracefully handle and adapt
```

### Step 7: Cost and Performance Check

Review trace for:
- Total cost < $0.50 per query
- Token usage < 200k context limit
- Execution time < 5 minutes for typical query

```bash
cat .traces/demo-*.json | jq '.cost_usd, .total_tokens'
```

## Critical Files

- `cmd/research-demo/main.go` - System prompt and setup
- `tools/research/research.go` - Tool registration
- `cmd/research-demo/README.md` - Documentation
- `go.mod` - Dependencies

## Rollback Plan

If issues occur:
1. Revert system prompt to previous version
2. Comment out new tool registrations
3. Keep new tool implementations for future use

## Success Criteria

✅ All 7 tools registered successfully
✅ System prompt loads without errors
✅ `create_research_plan` called first in every run
✅ All three sections present in output
✅ Full-text summaries (not just abstracts)
✅ GitHub repos included with star counts
✅ Rate limiting works (no API blocks)
✅ Tests pass
✅ Cost < $0.50 per query
✅ Trace files show complete tool usage

## Notes

- The system prompt is ~1500 tokens but cached after first turn
- Expect 15-30 turns for a complete research report
- PDF text extraction is the slowest operation (~3-5 seconds per paper)
- Web search may occasionally fail due to DuckDuckGo blocks (agent should adapt)
- GitHub API rate limits are the main constraint without token
