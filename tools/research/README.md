# Research Tools

Research tool implementations for the arXiv Deep Research agent. These tools enable autonomous searching of academic literature, code repositories, and general web content.

## Available Tools

| Tool | Purpose | Rate Limits |
|------|---------|-------------|
| `search_arxiv` | Search arXiv for academic preprints | No limit |
| `search_openalex` | Search OpenAlex for published academic works | No limit |
| `fetch_arxiv_pdf` | Get PDF download URL for arXiv papers | 1 req/3 sec |
| `search_github_repos` | Find popular GitHub repositories with code implementations | 60 req/hr (no auth) |
| `search_web` | Search the general web using DuckDuckGo | No limit |
| `get_citations_and_references` | Get citation and reference data from OpenAlex | No limit |

## Tool Details

### search_arxiv

Searches arXiv for academic preprints.

**Input:**
- `query` (string, required): Search query
- `max_results` (integer, optional): Maximum results (default: 10)
- `search_field` (string, optional): Field to search: 'title' (default, most relevant) or 'abstract' (broader results)

**Output:**
- Array of papers with title, authors, abstract, arXiv ID, publication date, link (PDF URL)

**Example:**
```bash
tools-cli search-arxiv "attention mechanism" --max-results 5
tools-cli search-arxiv "neural networks" --search-field abstract
```

**Use case:** Finding recent papers on a specific topic

**Implementation:** `handlers.go:handleSearchArxiv()`

---

### search_openalex

Searches OpenAlex for published academic works with advanced filtering and sorting support.

**Input:**
- `query` (string, required): Search query
- `max_results` (integer, optional): Maximum results (default: 10)
- `filter` (string, optional): OpenAlex filter expression (e.g., "publication_year:>2022")
- `sort` (string, optional): Sort order: 'cited_by_count' (most cited first). Omit for default relevance ranking.

**Output:**
- Array of works with title, authors, DOI, abstract (when available), citation counts, publication year

**Example:**
```bash
tools-cli search-openalex "neural networks" --filter "publication_year:>2022"
tools-cli search-openalex "quantum computing" --max-results 20
tools-cli search-openalex "transformers" --sort cited_by_count
```

**Use case:** Finding highly-cited recent work with specific filters. Use `sort=cited_by_count` for most influential papers.

**Implementation:** `handlers.go:handleSearchOpenAlex()`

---

### fetch_arxiv_pdf

Gets the PDF download URL for an arXiv paper (with 3-second rate limiting).

**Input:**
- `arxiv_id` (string, required): arXiv identifier (e.g., "2301.00001", "arXiv:2301.00001", "astro-ph/9901234")

**Output:**
- Object with arxiv_id and pdf_url

**Example:**
```bash
tools-cli fetch-pdf "1706.03762"
```

**Use case:** Getting downloadable PDF URLs for papers

**Implementation:** `handlers.go:handleFetchArxivPDF()`

**Rate limiting:** 3-second delay enforced per request

---

### search_github_repos

Finds popular GitHub repositories related to a search query.

**Input:**
- `query` (string, required): GitHub search query (supports GitHub search syntax)
- `max_results` (integer, optional): Maximum results (default: 5, max: 5)

**Output:**
- Array of repositories with name, description, URL, stars, language, license, topics, last updated date

**Example:**
```bash
tools-cli search-github "diffusion models pytorch"
tools-cli search-github "transformer language:python stars:>100"
```

**Use case:** Finding reference implementations and popular codebases

**Implementation:** `handlers.go:handleSearchGithubRepos()`

**Important:** Results are automatically filtered to show only popular, actively-maintained repos (>100 stars, updated within 2 years, not archived). NOT suitable for finding experimental, niche, or small projects.

**GitHub Search Syntax:**
- `language:python` - Filter by programming language
- `stars:>100` - Filter by star count
- `pushed:>2023-01-01` - Filter by last push date
- `topic:machine-learning` - Filter by topic

---

### search_web

Searches the general web using DuckDuckGo, useful as a fallback when academic databases lack coverage.

**Input:**
- `query` (string, required): Web search query
- `max_results` (integer, optional): Maximum results (default: 10, max: 10)

**Output:**
- Array of web results with title, URL, and snippet

**Example:**
```bash
tools-cli search-web "quantum computing tutorials"
```

**Use case:** Finding tutorials, documentation, and general information when academic sources are insufficient. Use as fallback when arXiv/OpenAlex lack coverage.

**Important:** NOT reliable for 'most cited papers' - use search_openalex with citation sorting instead. WARNING: May be unreliable due to bot detection.

**Implementation:** `search_web.go:handleSearchWeb()`

---

### get_citations_and_references

Gets citations and references for an academic work from OpenAlex.

**Input:**
- `work_id` (string, required): OpenAlex work ID (format: W2741809807). Use search_openalex to find work IDs.
- `direction` (string, required): Direction: 'references' (papers cited BY this work) or 'cited_by' (papers that cite this work)
- `max_results` (integer, optional): Maximum number of results to return (default: 10, max: 50)

**Output:**
- Lightweight metadata for citations or references: title, authors, year, citation count, DOI

**Example:**
```bash
tools-cli get-citations "W2741809807" --direction references
tools-cli get-citations "W2741809807" --direction cited_by --max-results 20
```

**Use case:** Use direction='references' to get the bibliography (papers cited BY this work), or direction='cited_by' to get forward citations (papers that cite this work). For abstracts, use search_openalex with the DOI.

**Implementation:** `handlers.go:handleGetCitationsAndReferences()`

## Architecture

### Package Structure

```
tools/research/
├── README.md              - This file
├── research.go            - Tool registration and setup
├── definitions.go         - Tool definitions for Claude API
├── handlers.go            - Tool handler implementations
├── estimators.go          - Token size estimation for tools
├── search_web.go          - Web search implementation
└── *_test.go              - Test files
```

### Key Components

**Tool Registration** (`research.go`)
- `NewResearchToolSet()` - Creates toolset with all research tools
- Registers tool definitions, handlers, and size estimators
- Configures HTTP client for external API calls

**Tool Definitions** (`definitions.go`)
- `BuildSearchArxivTool()` - Claude API tool definition for arXiv search
- `BuildSearchOpenAlexTool()` - Claude API tool definition for OpenAlex search
- `BuildFetchArxivPDFTool()` - Claude API tool definition for PDF fetching
- `BuildSearchGithubReposTool()` - Claude API tool definition for GitHub search
- `BuildSearchWebTool()` - Claude API tool definition for web search
- `BuildGetCitationsAndReferencesTool()` - Claude API tool definition for citations

**Handlers** (`handlers.go`)
- Implement the actual tool logic
- Parse JSON input and return JSON output
- Handle HTTP requests to external APIs
- Manage rate limiting and error handling

**Size Estimators** (`estimators.go`)
- Estimate output token size before tool execution
- Used by context manager for budget planning
- Helps prevent context overflow

## Testing

### Running Tests

```bash
# All research tool tests
go test ./tools/research

# With coverage
go test ./tools/research -cover

# Specific test
go test ./tools/research -run TestHandleSearchArxiv

# Verbose output
go test ./tools/research -v
```

### Test Coverage

- Unit tests for each tool handler
- HTTP client mocking for external API tests
- Edge cases: empty results, malformed input, API errors
- Rate limiting verification

## Adding New Tools

To add a new research tool:

1. **Add definition** in `definitions.go`:
   ```go
   func BuildMyNewTool() anthropic.ToolUnionParam {
       // Define input schema and description
   }
   ```

2. **Implement handler** in `handlers.go`:
   ```go
   func handleMyNewTool(input []byte) (string, error) {
       // Parse input, call external API, return JSON
   }
   ```

3. **Add size estimator** in `estimators.go`:
   ```go
   "my_new_tool": func(args map[string]any) int {
       // Estimate output token size
   }
   ```

4. **Register tool** in `research.go`:
   ```go
   registry.RegisterTool("my_new_tool", BuildMyNewTool(), handlers["my_new_tool"])
   ```

5. **Write tests** in `my_new_tool_test.go`:
   ```go
   func TestHandleMyNewTool(t *testing.T) {
       // Test handler logic
   }
   ```

6. **Add CLI command** in `cmd/tools-cli/commands.go`:
   ```go
   case "my-new-tool":
       // Handle CLI arguments and call tool
   ```

7. **Update documentation** in this README

## External APIs

- **arXiv API**: http://export.arxiv.org/api/query
- **OpenAlex API**: https://api.openalex.org/works
- **GitHub API**: https://api.github.com/search/repositories
- **DuckDuckGo**: HTML scraping (no official API)

## Rate Limits

- **arXiv**: No enforced limits, but be respectful
- **OpenAlex**: No enforced limits, polite usage encouraged
- **GitHub**: 60 requests/hour (unauthenticated), 5000/hour (authenticated)
- **DuckDuckGo**: No official limits, may block on abuse

Set `GITHUB_TOKEN` environment variable for higher GitHub API limits.
