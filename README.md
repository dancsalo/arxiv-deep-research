# arXiv Deep Research

An autonomous AI research agent that searches academic literature, analyzes papers, and synthesizes findings. Built in Go with Claude 4.x integration, context management, and tracing.

## Quick Start

```bash
# Build and run the research agent
make run

# Or with custom query and variant
./research-demo --query "quantum computing algorithms" --prompt-variant A --max-turns 15

# Test individual research tools
./tools-cli search-arxiv "transformers" --max-results 5
./tools-cli --interactive
```

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

## Research Agent Features

- **Autonomous research loop** - Multi-turn agentic workflow with tool use
- **Context management** - Token budgeting, guardrails, and automatic compaction
- **Multiple prompt variants** - Compare different prompting strategies
- **Comprehensive tracing** - JSON traces for analysis and debugging
- **Research tools** - arXiv, OpenAlex, GitHub, web search

## Getting Started

### Prerequisites
- Go 1.25 or later
- Claude API key (set `ANTHROPIC_API_KEY` env var)
- Internet connection for research tools

### Installation

```bash
git clone https://github.com/dancsalo/arxiv-deep-research.git
cd arxiv-deep-research
make build
```

### Running the Research Agent

The research agent (`research-demo`) implements an autonomous research loop with multiple prompt variants:

**Variant A (Explicit)** - Prescriptive turn-by-turn guidance
- Enforces parallel tool usage for breadth
- Structured phases with explicit examples
- Targets 10-15 turns

**Variant B (Metacognitive)** - Strategy planning and self-reflection
- Pre-turn planning and post-turn reflection
- Adaptive tool selection based on findings
- Quality gates for stopping criteria

**Variant C (Reward-Shaping)** - Quality metrics and scoring
- Evaluates research on 5 dimensions
- Target score: 90+ points for "Excellent" quality
- Self-assessment before completion

**Variant D (Survey-Driven)** - Comprehensive literature review
- Systematic survey methodology
- Multiple sources and triangulation
- Citation analysis

### Basic Usage

```bash
# Default run (variant A)
make run

# Custom query and variant
./research-demo --query "neural architecture search" --prompt-variant B --max-turns 12

# Use different Claude model
./research-demo --query "diffusion models" --model opus --prompt-variant C
```

### Analyzing Results

Traces are saved to `.traces/` as JSON files:

```bash
# List recent traces
make trace-list

# Show specific trace
make trace-show RUN=<session_id>

# Show failed traces
make trace-errors
```

Prompt files are in `cmd/research-demo/prompts/variant-{a,b,c,d}.txt` and can be customized.

## Testing Individual Tools

The `tools-cli` binary lets you test research tools independently:

```bash
# Build tools CLI
make build-tools

# Search arXiv
./tools-cli search-arxiv "attention mechanism" --max-results 5

# Search OpenAlex with filter
./tools-cli search-openalex "neural networks" --filter "publication_year:>2023"

# Get PDF URL
./tools-cli fetch-pdf "1706.03762"

# Search GitHub
./tools-cli search-github "diffusion models pytorch"

# Search web
./tools-cli search-web "quantum computing tutorials"

# Interactive mode
./tools-cli --interactive

# JSON output for scripting
./tools-cli search-arxiv "transformers" --json | jq '.[:2]'
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

**Core Components:**
- `cmd/research-demo/` - Main application: autonomous research agent
- `cmd/tools-cli/` - CLI for testing individual research tools

**Internal Packages:**
- `internal/agentic/` - Agentic loop implementation and research workflow
- `internal/ctxmgr/` - Context management, token budgets, and guardrails
- `internal/registry/` - Tool registration system
- `internal/tracing/` - JSON tracing and instrumentation

**Tools:**
- `tools/research/` - Research tool implementations (arXiv, OpenAlex, GitHub, web)

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
│   ├── research-demo/      # Main: autonomous research agent
│   └── tools-cli/          # CLI for testing individual tools
├── internal/
│   ├── agentic/            # Agentic loop and workflow
│   ├── ctxmgr/             # Context management and budgeting
│   ├── registry/           # Tool registration
│   └── tracing/            # JSON tracing
├── tools/
│   └── research/           # Research tool implementations
└── .traces/                # Research run traces (gitignored)
```
