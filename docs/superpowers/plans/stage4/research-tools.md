# Plan: Research Tools (arXiv + OpenAlex) + Demo Binary

## Context

The project currently has only memory-related tools in `tools/`. We need real research tools that query external academic APIs so the agentic loop can do actual literature research. This adds two search tools (arXiv, OpenAlex) and a demo binary that runs them end-to-end with the real Anthropic API.

**DuckDuckGo is deferred** — it requires HTML scraping of an undocumented endpoint that actively blocks bots, adds fragility for minimal differentiated value in an academic research assistant. arXiv + OpenAlex provide a compelling multi-source academic search demo.

## Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Package location | `tools/research/` sub-package | Implements `registry.ToolSet` interface (the newer pattern); memory tools still use the legacy handler style |
| HTTP client | Inject `*http.Client` with 30s timeout | Testable via `httptest.Server`; timeout prevents stalling the loop |
| Demo API | Direct Anthropic (`ANTHROPIC_API_KEY`) | Works without AWS creds; simplest for anyone to run |
| Error format | Return JSON `{"error": "...", "recoverable": true}` as tool result string (nil Go error) | Model interprets and decides; Go errors reserved for programmer bugs only |
| arXiv rate limiting | System prompt instructs model to not call search_arxiv more than once; handler logs a warning if called within 3s of last call (no sleep/block) | Simple for v1; can add `x/time/rate` later |
| Reducer integration | Add `case "search_openalex"` to existing `reduceToolArgs` in `guardrail.go` | The existing switch is the only callsite; no pluggable mechanism exists yet |

## File Structure

```
tools/research/
    research.go       -- ResearchToolSet struct, ToolSet interface impl, error helpers
    definitions.go    -- BuildSearchArxivTool(), BuildSearchOpenAlexTool()
    handlers.go       -- Handler methods + API client logic (arXiv XML, OpenAlex JSON)
    estimators.go     -- Token size estimators
    arxiv_test.go     -- Tests with httptest + canned Atom XML
    openalex_test.go  -- Tests with httptest + canned JSON

cmd/research-demo/
    main.go           -- End-to-end demo binary

internal/ctxmgr/guardrail.go  -- Add search_openalex case to reduceToolArgs
```

## Specs

### `tools/research/research.go`

```go
package research

import (
    "context"
    "encoding/json"
    "net/http"
    "github.com/dancsalo/arxiv-deep-research/internal/registry"
)

type ResearchToolSet struct {
    client *http.Client
}

func NewResearchToolSet(client *http.Client) *ResearchToolSet

// Implements registry.ToolSet
func (r *ResearchToolSet) Register(reg *registry.ToolRegistry) {
    reg.Register("search_arxiv", BuildSearchArxivTool(), r.handleSearchArxiv)
    reg.Register("search_openalex", BuildSearchOpenAlexTool(), r.handleSearchOpenAlex)
}

// toolError returns a JSON string for tool error results.
// Go error return is always nil — the model sees the error as a tool result.
func toolError(msg string, recoverable bool) string
```

### `tools/research/definitions.go`

```go
func BuildSearchArxivTool() anthropic.ToolUnionParam
// Params: query (required string), max_results (int, default 10)
// Description: "Search arXiv for academic preprints. Returns titles, authors, abstracts, and links."

func BuildSearchOpenAlexTool() anthropic.ToolUnionParam
// Params: query (required string), max_results (int, default 10),
//         filter (optional string, e.g. "publication_year:>2022")
// Description: "Search OpenAlex for published academic works. Returns titles, authors, DOIs, and abstracts."
```

### `tools/research/handlers.go`

All HTTP requests use `http.NewRequestWithContext(ctx, ...)` for cancellation.

**arXiv** (`search_arxiv`):
```go
type ArxivResult struct {
    Title     string   `json:"title"`
    Authors   []string `json:"authors"`
    Abstract  string   `json:"abstract"`
    Link      string   `json:"link"`
    Published string   `json:"published"`
}

// XML parsing structs (unexported):
type atomFeed struct {
    XMLName xml.Name    `xml:"feed"`
    Entries []atomEntry `xml:"entry"`
}
type atomEntry struct {
    Title   string       `xml:"title"`
    Summary string       `xml:"summary"`
    ID      string       `xml:"id"`
    Authors []atomAuthor `xml:"author"`
    Published string     `xml:"published"`
}
type atomAuthor struct {
    Name string `xml:"name"`
}

func (r *ResearchToolSet) handleSearchArxiv(ctx context.Context, input json.RawMessage) (string, error)
```
- URL: `https://export.arxiv.org/api/query?search_query=all:{query}&max_results={n}`
- Note: Go's `encoding/xml` handles the default namespace without explicit namespace tags in struct fields when using `xml:"entry"` (the feed declares `xmlns="..."` as default). Verified pattern.

**OpenAlex** (`search_openalex`):
```go
type OpenAlexResult struct {
    Title    string   `json:"title"`
    Authors  []string `json:"authors"`
    DOI      string   `json:"doi"`
    Abstract string   `json:"abstract"`
    Year     int      `json:"year"`
}

// JSON response structs (unexported):
type openAlexResponse struct {
    Results []openAlexWork `json:"results"`
}
type openAlexWork struct {
    Title              string                 `json:"title"`
    DOI                string                 `json:"doi"`
    PublicationYear    int                    `json:"publication_year"`
    Authorships        []openAlexAuthorship   `json:"authorships"`
    AbstractInvertedIndex map[string][]int    `json:"abstract_inverted_index"`
}
type openAlexAuthorship struct {
    Author struct {
        DisplayName string `json:"display_name"`
    } `json:"author"`
}

// reconstructAbstract places words at their positions from the inverted index.
// Returns "" if index is nil.
func reconstructAbstract(index map[string][]int) string

func (r *ResearchToolSet) handleSearchOpenAlex(ctx context.Context, input json.RawMessage) (string, error)
```
- URL: `https://api.openalex.org/works?search={query}&per_page={n}&filter={filter}&mailto=arxiv-deep-research@users.noreply.github.com`

### `tools/research/estimators.go`

```go
func ResearchToolEstimators() map[string]func(args map[string]any) int
// search_arxiv:    50 + n*400 (title+authors+abstract per result)
// search_openalex: 50 + n*350
```

### `internal/ctxmgr/guardrail.go` — modification

Add to `reduceToolArgs` switch:
```go
case "search_openalex":
    if v, ok := args["max_results"]; ok {
        switch n := v.(type) {
        case float64:
            if n > 5 { reduced["max_results"] = float64(5); return reduced, true }
        case int:
            if n > 5 { reduced["max_results"] = 5; return reduced, true }
        }
    }
```

### `cmd/research-demo/main.go`

**Flags**: `-query` (default: "retrieval augmented generation"), `-model` (default: `anthropic.ModelClaudeHaiku4_5`), `-max-turns` (default: 10)

**Behavior**: Runs 3-6 turns (~15-30s). Searches both arXiv and OpenAlex. Model calls `finish_loop` with a markdown summary. Prints: final answer, turn count, input/output token counts, elapsed time.

**Wiring** (uses actual types: `agentic.Loop`, `agentic.LoopConfig`, `agentic.NewLoop`):
```go
func main() {
    // Parse flags
    // Validate ANTHROPIC_API_KEY (fail fast with clear error)
    // httpClient := &http.Client{Timeout: 30 * time.Second}
    // toolSet := research.NewResearchToolSet(httpClient)
    // reg := registry.NewToolRegistry()
    // registry.RegisterToolSets(reg, toolSet)
    // reg.Register("finish_loop", agentic.BuildFinishTool(), finishHandler)
    //   finishHandler just returns the input as-is (same as cmd/server)
    // tse := ctxmgr.NewToolSizeEstimator()
    // for name, fn := range research.ResearchToolEstimators() { tse.RegisterTool(name, fn) }
    // manager := ctxmgr.NewContextManager(config, initialMsg)
    // manager.SetToolSizeEstimator(tse)
    // loop := agentic.NewLoop(client, manager, reg, nil, config, system)
    //   nil recaller is safe — MemoryRecall.Enabled defaults false
    // Run loop, print stats
}
```

**System prompt**:
```
You are a research assistant. Search arXiv for preprints and OpenAlex for published works
to find relevant papers on the given topic. Search both sources, then synthesize a summary
with the most important findings. Include paper titles and authors in your summary.
Do not call search_arxiv more than once.
Call finish_loop with your final markdown summary when done.
```

## Implementation Order

1. `tools/research/research.go` — struct, constructor, `toolError` helper
2. `tools/research/definitions.go` — tool schemas for both tools
3. `tools/research/handlers.go` — arXiv handler with XML parsing
4. `tools/research/arxiv_test.go` — test with canned Atom XML via httptest
5. `tools/research/handlers.go` — OpenAlex handler with JSON + abstract reconstruction
6. `tools/research/openalex_test.go` — test with canned JSON via httptest
7. `tools/research/estimators.go` — token estimators
8. Wire `Register()` in `research.go`
9. **Midpoint: `make build && make test`** — confirm compilation and tests pass
10. `internal/ctxmgr/guardrail.go` — add `search_openalex` reduction case
11. `cmd/research-demo/main.go` — demo binary
12. End-to-end manual test: `go run ./cmd/research-demo/ -query "transformer attention mechanisms"`

## Verification

1. `make build` — all packages compile
2. `make test` — httptest-based unit tests pass
3. `go run ./cmd/research-demo/ -query "transformer attention mechanisms"` — prints markdown summary with papers from both sources, plus stats (turns, tokens, time)
4. Error path: `HTTP_PROXY=http://localhost:1 go run ./cmd/research-demo/` — tools return recoverable error JSON, demo completes gracefully (model sees errors, finishes with partial results or "could not reach APIs" message)

## Critical Files

| File | Role |
|------|------|
| `internal/registry/registry.go` | `ToolHandler` signature, `Register()` |
| `internal/registry/toolset.go` | `ToolSet` interface |
| `internal/agentic/types.go` | `Loop`, `LoopConfig`, `NewLoop` |
| `internal/agentic/helpers.go` | `BuildFinishTool()` |
| `internal/ctxmgr/guardrail.go` | `reduceToolArgs` switch to extend |
| `internal/ctxmgr/toolsize.go` | `ToolSizeEstimator.RegisterTool()` |
| `cmd/server/main.go` | Reference for wiring pattern |
| `cmd/ctxmgr-demo/main.go` | Reference for demo structure |
