# Research Tools Testing, CLI, and Documentation

## Problem Statement

The research tools (`search_arxiv`, `search_openalex`, `fetch_arxiv_pdf`, `fetch_arxiv_pdf_text`, `search_github_repos`) have been implemented and merged into main, but lack:

1. **Comprehensive test coverage** - Some tools may have incomplete tests
2. **Interactive testing capability** - No easy way to manually test tools with different inputs
3. **User documentation** - No README explaining what tools exist and how to use them

**User need:** Developers and users need to:
- Verify all tools work correctly
- Manually test tools with various inputs during development
- Understand what tools are available and how to use them
- Trust that tools are well-tested

**Success criteria:**
- All 5 tools have comprehensive test coverage (>80% code coverage)
- CLI tool allows manual testing in both script and interactive modes
- README documents all tools with examples
- New developers can understand and test tools within 15 minutes

## Requirements

### Functional Requirements

**FR1: Test Coverage**
- All 5 tools have tests covering:
  - Input validation (missing fields, invalid formats)
  - Success cases (valid inputs, expected outputs)
  - Error handling (network failures, API errors, rate limits)
  - Edge cases (empty results, truncation, special characters)
- Tests use mocked HTTP responses where appropriate
- Integration tests call real APIs (marked appropriately for CI)

**FR2: CLI Tool - Script Mode**
- Binary: `cmd/tools-cli/main.go`
- Subcommands for each tool:
  - `search-arxiv <query> [--max-results=N]`
  - `search-openalex <query> [--max-results=N] [--filter=EXPR]`
  - `fetch-pdf <arxiv-id>`
  - `fetch-pdf-text <arxiv-id> [--max-pages=N]`
  - `search-github <query> [--max-results=N]`
- Output formats:
  - Human-readable (default)
  - JSON with `--json` flag for scripting
- Proper error messages and exit codes

**FR3: CLI Tool - Interactive Mode**
- Launched with `--interactive` or `-i` flag
- Menu-driven interface:
  1. Display numbered list of available tools
  2. Prompt for tool selection
  3. Prompt for required parameters
  4. Prompt for optional parameters (show defaults)
  5. Execute tool and display results
  6. Ask if user wants to run another command
- Clear error messages with examples
- Exit cleanly with Ctrl+C or "quit" command

**FR4: README Documentation**
- Location: `/README.md` (repo root)
- Sections:
  - Project overview
  - Research tools overview table
  - Detailed tool descriptions (purpose, parameters, examples)
  - Getting started (prerequisites, installation)
  - CLI usage examples (script + interactive)
  - Running tests
  - Architecture overview
  - Development workflow (link to CLAUDE.md)
- Examples use actual CLI commands
- Clear formatting with code blocks

### Non-Functional Requirements

**NFR1: Code Reuse**
- CLI reuses existing `ResearchToolSet` - no duplicated logic
- Test helpers shared across test files where appropriate
- Follow existing project patterns and conventions

**NFR2: User Experience**
- CLI commands follow standard UNIX conventions
- Error messages are actionable (suggest fixes)
- Interactive mode is intuitive for first-time users
- README examples are copy-pasteable

**NFR3: Maintainability**
- Tests are independent and can run in any order
- CLI code is organized by command (easy to add new tools)
- README stays in sync with actual tool behavior

## Specs

### Test Suite Structure

**File organization:**
```
tools/research/
├── arxiv_test.go                 # search_arxiv tests
├── openalex_test.go              # search_openalex tests
├── fetch_arxiv_pdf_test.go       # fetch_arxiv_pdf tests
├── fetch_arxiv_pdf_text_test.go  # fetch_arxiv_pdf_text tests
├── search_github_test.go         # search_github_repos tests
└── testutil.go (if needed)       # Shared test helpers
```

**Test naming convention:**
```go
func TestHandleSearchArxiv_InputValidation(t *testing.T)
func TestHandleSearchArxiv_Success(t *testing.T)
func TestHandleSearchArxiv_ErrorHandling(t *testing.T)
func TestHandleSearchArxiv_EdgeCases(t *testing.T)
```

**Mock strategy:**
- Use `httptest.Server` for mocking HTTP responses
- Unit tests mock API responses
- Integration tests (marked with build tags if needed) hit real APIs

### CLI Architecture

**Package structure:**
```
cmd/tools-cli/
├── main.go           # Entry point, command routing
├── commands.go       # Command implementations
├── interactive.go    # Interactive mode logic
└── format.go         # Output formatting (human + JSON)
```

**Command interface pattern:**
```go
type Command struct {
    Name        string
    Description string
    Execute     func(args []string, flags map[string]string) error
}
```

**Flow diagram:**
```
main()
  ├─> Parse flags (--interactive, --json, --help)
  ├─> If --interactive: runInteractive()
  ├─> Else: parseCommand() -> executeCommand()
  └─> Exit with appropriate code

runInteractive()
  ├─> Display menu
  ├─> Read user selection
  ├─> Prompt for parameters
  ├─> Execute command
  ├─> Display results
  └─> Loop or exit
```

**Tool instantiation:**
```go
// Shared toolset instance
httpClient := &http.Client{Timeout: 30 * time.Second}
toolset := research.NewResearchToolSet(httpClient)

// Each command calls appropriate handler
result, err := toolset.handleSearchArxiv(ctx, inputJSON)
```

### CLI Commands Specification

**Command: search-arxiv**
```bash
tools-cli search-arxiv <query> [--max-results=N]

Arguments:
  query           Search query string (required)

Flags:
  --max-results   Maximum results to return (default: 10)
  --json          Output as JSON

Examples:
  tools-cli search-arxiv "attention mechanism"
  tools-cli search-arxiv "transformers" --max-results 5 --json
```

**Command: search-openalex**
```bash
tools-cli search-openalex <query> [--max-results=N] [--filter=EXPR]

Arguments:
  query           Search query string (required)

Flags:
  --max-results   Maximum results to return (default: 10)
  --filter        OpenAlex filter expression (e.g., "publication_year:>2022")
  --json          Output as JSON

Examples:
  tools-cli search-openalex "neural networks"
  tools-cli search-openalex "deep learning" --filter "publication_year:>2023"
```

**Command: fetch-pdf**
```bash
tools-cli fetch-pdf <arxiv-id>

Arguments:
  arxiv-id        arXiv identifier (e.g., "2301.00001", "arXiv:2301.00001")

Flags:
  --json          Output as JSON

Examples:
  tools-cli fetch-pdf "2301.00001"
  tools-cli fetch-pdf "arXiv:1706.03762"
```

**Command: fetch-pdf-text**
```bash
tools-cli fetch-pdf-text <arxiv-id> [--max-pages=N]

Arguments:
  arxiv-id        arXiv identifier

Flags:
  --max-pages     Maximum pages to extract (default: all pages)
  --json          Output as JSON

Examples:
  tools-cli fetch-pdf-text "2301.00001"
  tools-cli fetch-pdf-text "1706.03762" --max-pages 10
```

**Command: search-github**
```bash
tools-cli search-github <query> [--max-results=N]

Arguments:
  query           GitHub search query (supports GitHub search syntax)

Flags:
  --max-results   Maximum results to return (default: 5, max: 5)
  --json          Output as JSON

Examples:
  tools-cli search-github "diffusion models pytorch"
  tools-cli search-github "transformer language:python stars:>100"
```

### Output Format Specification

**Human-readable output (default):**
```
=== Search Results for "transformers" ===

1. Attention Is All You Need
   Authors: Vaswani et al.
   Published: 2017-06-12
   Abstract: The dominant sequence transduction models...
   URL: https://arxiv.org/abs/1706.03762

2. BERT: Pre-training of Deep Bidirectional Transformers
   [...]

Found 10 results (showing 10)
```

**JSON output (--json flag):**
```json
{
  "query": "transformers",
  "results": [
    {
      "title": "Attention Is All You Need",
      "authors": ["Vaswani, Ashish", "..."],
      "published": "2017-06-12",
      "abstract": "The dominant sequence transduction models...",
      "url": "https://arxiv.org/abs/1706.03762",
      "arxiv_id": "1706.03762"
    }
  ],
  "count": 10
}
```

**Error output:**
```
Error: Invalid arXiv ID format

Expected format:
  - 2301.00001 (new format)
  - arXiv:2301.00001
  - astro-ph/9901234 (old format)

Try: tools-cli fetch-pdf "2301.00001"
```

### Interactive Mode Specification

**Flow:**
```
$ tools-cli --interactive

=== Research Tools CLI ===

Select a tool:
  [1] search-arxiv        - Search arXiv for preprints
  [2] search-openalex     - Search academic literature
  [3] fetch-pdf           - Get arXiv PDF download URL
  [4] fetch-pdf-text      - Extract text from arXiv PDF
  [5] search-github       - Find GitHub repositories
  [0] Exit

Enter selection: 1

--- search-arxiv ---
Query: attention mechanism
Max results [10]: 5

Searching arXiv...

=== Results ===
[Display formatted results]

Run another command? [y/n]: y
[Return to menu]
```

**Input handling:**
- Required parameters: prompt until valid input received
- Optional parameters: show default in brackets, accept empty input
- Invalid selections: show error, re-prompt
- Ctrl+C: graceful exit with cleanup

### README Structure

**Section 1: Header**
```markdown
# arXiv Deep Research

A Go-based research agent with tool support for searching academic literature, fetching papers, and finding code implementations.
```

**Section 2: Research Tools Overview**
```markdown
## Research Tools

| Tool | Purpose | Rate Limits |
|------|---------|-------------|
| `search_arxiv` | Search arXiv preprints | No limit |
| `search_openalex` | Search academic works | No limit |
| `fetch_arxiv_pdf` | Get PDF download URL | 1 req/3 sec |
| `fetch_arxiv_pdf_text` | Extract full paper text | 1 req/3 sec |
| `search_github_repos` | Find code repositories | 60 req/hr (no auth) |
```

**Section 3: Detailed Tool Descriptions**
For each tool:
```markdown
### search_arxiv

Searches arXiv for academic preprints.

**Input:**
- `query` (string, required): Search query
- `max_results` (integer, optional): Maximum results (default: 10)

**Output:**
- Array of papers with title, authors, abstract, publication date, arXiv ID, PDF URL

**Example:**
\```bash
tools-cli search-arxiv "attention mechanism" --max-results 5
\```

**Use case:** Finding recent papers on a specific topic
```

**Section 4: Getting Started**
```markdown
## Getting Started

### Prerequisites
- Go 1.21 or later
- Internet connection (tools call external APIs)
- (Optional) GitHub personal access token for higher rate limits

### Installation
\```bash
git clone <repo>
cd arxiv-deep-research
go build ./cmd/tools-cli
\```

### Quick Start
\```bash
# Search for papers
./tools-cli search-arxiv "transformers"

# Get PDF URL
./tools-cli fetch-pdf "1706.03762"

# Interactive mode
./tools-cli --interactive
\```
```

**Section 5: CLI Usage**
```markdown
## CLI Usage

### Script Mode
[Examples for each command]

### Interactive Mode
[Screenshot or example session]

### JSON Output
[Scripting examples with jq]
```

**Section 6: Running Tests**
```markdown
## Running Tests

\```bash
# All tests
make test

# Go tests only
make test-go

# With coverage
go test ./tools/research -cover

# Specific tool
go test ./tools/research -run TestHandleSearchArxiv
\```
```

**Section 7: Architecture**
```markdown
## Architecture

- `internal/ctxmgr/` - Context management and token budgets
- `internal/agentic/` - Agentic loop and research workflow
- `internal/registry/` - Tool registration system
- `tools/research/` - Research tool implementations
- `cmd/tools-cli/` - Interactive CLI for testing tools
- `cmd/research-demo/` - Full research agent demo

See [CLAUDE.md](./CLAUDE.md) for development workflow.
```

**Section 8: Development**
```markdown
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
[Brief overview of key directories]
```

## Implementation Strategy

### Phase 1: Test Coverage Audit
1. Review existing tests for all 5 tools
2. Identify gaps in coverage
3. Add missing tests (prioritize critical paths)
4. Verify all tests pass with `make test`

### Phase 2: CLI Foundation
1. Create `cmd/tools-cli/main.go` with basic structure
2. Implement flag parsing (--interactive, --json, --help)
3. Create command registry and routing
4. Implement one command (search-arxiv) end-to-end
5. Test and refine

### Phase 3: CLI Complete
1. Implement remaining 4 commands
2. Add JSON output formatting
3. Add error handling and validation
4. Test all commands in script mode

### Phase 4: Interactive Mode
1. Implement menu system
2. Add parameter prompting
3. Implement command loop
4. Add graceful exit handling
5. Test interactive flow

### Phase 5: Documentation
1. Write README sections
2. Add examples (tested from CLI)
3. Review for clarity and completeness
4. Commit and push

## Error Handling

### CLI Errors
- Invalid command: Show help and available commands
- Missing arguments: Show command usage with example
- Invalid flags: Show valid flags for command
- Network errors: Suggest retry with friendly message
- Rate limit errors: Show wait time and suggest delay

### Test Errors
- Flaky network tests: Use retry logic or skip in CI
- Missing test data: Clear error about setup requirements
- Mock server errors: Include response body in error message

## Edge Cases

### CLI Edge Cases
- Empty query string: Reject with error
- Very long query: Truncate with warning
- Invalid arXiv ID: Show format examples
- Ctrl+C during execution: Graceful shutdown
- Invalid JSON output request: Fall back to human-readable

### Test Edge Cases
- Empty result sets: Verify graceful handling
- Malformed API responses: Verify error handling
- Network timeouts: Verify timeout handling
- Rate limit hits: Verify backoff behavior
- Concurrent requests: Verify rate limiter correctness

## Testing Strategy

### Unit Tests
- Input validation logic
- Output formatting (human + JSON)
- Error message generation
- Command parsing

### Integration Tests
- Full command execution (mocked HTTP)
- Tool handler calls with real toolset
- JSON output correctness
- Error handling end-to-end

### Manual Tests
- Interactive mode user flow
- All script commands with real APIs
- Error scenarios (invalid inputs, network failures)
- JSON output piped to jq

## Dependencies

**New dependencies needed:**
- None (use standard library for CLI)

**Optional enhancements:**
- `github.com/spf13/cobra` - CLI framework (if needed for complex commands)
- `github.com/chzyer/readline` - Better interactive input (if needed)

**Recommendation:** Start with standard library (`flag` package) for simplicity. Add frameworks only if needed.

## Success Metrics

### Test Coverage
- All 5 tools have test files
- Code coverage >80% for tools/research package
- All tests pass in CI

### CLI Usability
- First-time user can run a command in <2 minutes
- Interactive mode is intuitive (no documentation needed)
- Error messages are actionable

### Documentation Quality
- README includes working examples for all tools
- New developers can understand architecture in <15 minutes
- Examples are copy-pasteable and work

## Open Questions

**Q: Should the CLI be its own module?**
**A:** No, keep it in the same module for simplicity. It's a utility for testing, not a separate product.

**Q: Should we add shell completion?**
**A:** Defer to future work. Nice-to-have but not critical for testing.

**Q: Should interactive mode save command history?**
**A:** Defer to future work. Standard up/down arrow history is sufficient for now.

**Q: Should we test with real GitHub API (requires auth)?**
**A:** Yes for integration tests, but mark appropriately. Unit tests should mock. Document that integration tests need GITHUB_TOKEN env var.

**Q: Should JSON output include error details?**
**A:** Yes. Format: `{"error": "message", "recoverable": bool}` matching tool error format.

## Implementation Checklist

- [ ] Phase 1: Test coverage audit and gap filling
- [ ] Phase 2: CLI foundation (main.go, flags, one command)
- [ ] Phase 3: CLI complete (all 5 commands, JSON output)
- [ ] Phase 4: Interactive mode
- [ ] Phase 5: README documentation
- [ ] Phase 6: Manual testing and refinement
- [ ] Phase 7: Code review and merge
