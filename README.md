# arXiv Deep Research

A Go-based research agent with tool support for searching academic literature, fetching papers, and finding code implementations.

## Research Tools

| Tool | Purpose | Rate Limits |
|------|---------|-------------|
| `search_arxiv` | Search arXiv for academic preprints | No limit |
| `search_openalex` | Search OpenAlex for published academic works | No limit |
| `fetch_arxiv_pdf` | Get PDF download URL for arXiv papers | 1 req/3 sec |
| `search_github_repos` | Find popular GitHub repositories with code implementations | 60 req/hr (no auth) |
| `search_web` | Search the general web using DuckDuckGo | No limit |

## Tool Details

### search_arxiv

Searches arXiv for academic preprints.

**Input:**
- `query` (string, required): Search query
- `max_results` (integer, optional): Maximum results (default: 10)

**Output:**
- Array of papers with title, authors, abstract, publication date, arXiv ID, PDF URL

**Example:**
```bash
tools-cli search-arxiv "attention mechanism" --max-results 5
```

**Use case:** Finding recent papers on a specific topic

### search_openalex

Searches OpenAlex for published academic works with advanced filtering support.

**Input:**
- `query` (string, required): Search query
- `max_results` (integer, optional): Maximum results (default: 10)
- `filter` (string, optional): OpenAlex filter expression (e.g., "publication_year:>2023")

**Output:**
- Array of works with title, authors, abstract, publication date, DOI, citations count, PDF URL

**Example:**
```bash
tools-cli search-openalex "neural networks" --filter "publication_year:>2023"
```

**Use case:** Finding highly-cited recent work with specific filters

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

### search_github_repos

Finds popular GitHub repositories related to a search query.

**Input:**
- `query` (string, required): GitHub search query (supports GitHub search syntax)
- `max_results` (integer, optional): Maximum results (default: 5, max: 5)

**Output:**
- Array of repositories with name, description, URL, stars, language, topics

**Example:**
```bash
tools-cli search-github "diffusion models pytorch"
tools-cli search-github "transformer language:python stars:>100"
```

**Use case:** Finding reference implementations and popular codebases

### search_web

Searches the general web using DuckDuckGo, useful as a fallback when academic databases lack coverage.

**Input:**
- `query` (string, required): Web search query
- `max_results` (integer, optional): Maximum results (default: 10)

**Output:**
- Array of web results with title, URL, and snippet

**Example:**
```bash
tools-cli search-web "quantum computing tutorials"
```

**Use case:** Finding tutorials, documentation, and general information when academic sources are insufficient

## Getting Started

### Prerequisites
- Go 1.21 or later
- Internet connection (tools call external APIs)
- (Optional) GitHub personal access token for higher rate limits

### Installation

```bash
git clone https://github.com/dancsalo/arxiv-deep-research.git
cd arxiv-deep-research
go build ./cmd/tools-cli
```

### Quick Start

```bash
# Search for papers
./tools-cli search-arxiv "transformers"

# Get PDF URL
./tools-cli fetch-pdf "1706.03762"

# Search web
./tools-cli search-web "machine learning tutorials"

# Interactive mode
./tools-cli --interactive
```

## CLI Usage

### Script Mode

Run specific commands with arguments:

```bash
# Search arXiv
tools-cli search-arxiv "attention mechanism" --max-results 5

# Search OpenAlex with filter
tools-cli search-openalex "neural networks" --filter "publication_year:>2023"

# Fetch PDF URL
tools-cli fetch-pdf "1706.03762"

# Search GitHub
tools-cli search-github "diffusion models pytorch" --max-results 3

# Search web
tools-cli search-web "quantum computing tutorials" --max-results 10

# JSON output (for scripting)
tools-cli search-arxiv "transformers" --json
```

### Interactive Mode

Launch the interactive menu:

```bash
tools-cli --interactive
# or
tools-cli -i
```

The interactive mode will prompt you for:
1. Tool selection (numbered menu)
2. Required parameters (query, arXiv ID, etc.)
3. Optional parameters (with defaults shown)

Example session:
```
=== Research Tools CLI ===

Select a tool:
  [1] search-arxiv        - Search arXiv for preprints
  [2] search-openalex     - Search academic literature
  [3] fetch-pdf           - Get arXiv PDF download URL
  [4] search-github       - Find GitHub repositories
  [5] search-web          - Search the general web
  [0] Exit

Enter selection: 1

--- search-arxiv ---
Query: attention mechanism
Max results [10]: 5

Searching arXiv...

=== Results ===
[... formatted results ...]

Run another command? [y/n]:
```

### JSON Output

Use `--json` flag for machine-readable output (useful for scripting):

```bash
tools-cli search-arxiv "transformers" --json | jq '.[:2]'
```

## Running Tests

```bash
# All tests
make test

# Go tests only
make test-go

# With coverage
go test ./tools/research -cover

# Specific tool
go test ./tools/research -run TestHandleSearchArxiv
```

## Architecture

- `internal/ctxmgr/` - Context management and token budgets
- `internal/agentic/` - Agentic loop and research workflow
- `internal/registry/` - Tool registration system
- `tools/research/` - Research tool implementations
- `cmd/tools-cli/` - Interactive CLI for testing tools
- `cmd/research-demo/` - Full research agent demo

See [CLAUDE.md](./CLAUDE.md) for development workflow.

## Development

This project uses a two-phase workflow (planning → coding). See [CLAUDE.md](./CLAUDE.md) for details.

### Adding a New Tool

1. Add tool definition in `tools/research/definitions.go`
2. Implement handler in `tools/research/handlers.go`
3. Register in `tools/research/research.go`
4. Add tests in `tools/research/<tool>_test.go`
5. Add CLI subcommand in `cmd/tools-cli/commands.go`
6. Update this README

### Project Structure

```
.
├── cmd/
│   ├── research-demo/      # Full research agent
│   ├── server/             # HTTP server with SSE streaming
│   └── tools-cli/          # CLI for testing tools
├── internal/
│   ├── agentic/            # Agentic loop
│   ├── ctxmgr/             # Context management
│   └── registry/           # Tool registry
├── tools/
│   └── research/           # Research tool implementations
└── README.md               # This file
```
