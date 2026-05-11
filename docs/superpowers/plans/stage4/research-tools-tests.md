# Test Plan: Research Tools (arXiv + OpenAlex) + Demo Binary

## Testing Strategy

Unit tests use `httptest.NewServer` with canned responses — no real network calls in CI. Each test file covers one external API.

## `tools/research/arxiv_test.go`

### Test: `TestSearchArxiv_Success`
- **Setup**: httptest server returning canned Atom XML (2 entries with title, authors, summary, id, published)
- **Input**: `query="attention", max_results=2`
- **Assert**: Returns 2 `ArxivResult` with correct fields parsed from XML; abstract trimmed of whitespace

### Test: `TestSearchArxiv_EmptyResults`
- **Setup**: httptest server returning valid Atom feed with 0 entries
- **Input**: `query="xyznonexistent", max_results=5`
- **Assert**: Returns empty slice, no error

### Test: `TestSearchArxiv_HTTPError`
- **Setup**: httptest server returning 500
- **Input**: `query="test", max_results=5`
- **Assert**: Returns JSON error string with `recoverable: true`, nil Go error

### Test: `TestSearchArxiv_MalformedXML`
- **Setup**: httptest server returning invalid XML
- **Assert**: Returns JSON error string, nil Go error

### Test: `TestSearchArxiv_ContextCancelled`
- **Setup**: httptest server with 5s delay; context cancelled immediately
- **Assert**: Returns JSON error string mentioning context

## `tools/research/openalex_test.go`

### Test: `TestSearchOpenAlex_Success`
- **Setup**: httptest server returning canned JSON response with 3 works (including inverted index abstracts)
- **Input**: `query="RAG", max_results=3`
- **Assert**: Returns 3 `OpenAlexResult` with correct title, authors, DOI, year, reconstructed abstract

### Test: `TestSearchOpenAlex_NullAbstract`
- **Setup**: httptest response with `abstract_inverted_index: null`
- **Assert**: Abstract field is empty string, no error

### Test: `TestSearchOpenAlex_WithFilter`
- **Setup**: httptest server that asserts URL contains `filter=publication_year:>2022`
- **Input**: `query="test", filter="publication_year:>2022"`
- **Assert**: Correct URL formed, results parsed

### Test: `TestSearchOpenAlex_HTTPError`
- **Setup**: httptest server returning 429
- **Assert**: Returns JSON error string with `recoverable: true`

### Test: `TestReconstructAbstract`
- **Input**: `{"the": [0,3], "cat": [1], "sat": [2], "mat": [4]}`
- **Assert**: Returns `"the cat sat the mat"`

### Test: `TestReconstructAbstract_Nil`
- **Input**: nil map
- **Assert**: Returns `""`

## `tools/research/` — additional unit tests

### Test: `TestToolError_Format`
- **Assert**: `toolError("api down", true)` returns valid JSON `{"error":"api down","recoverable":true}`

### Test: `TestResearchToolEstimators`
- **Assert**: `search_arxiv` with `max_results=5` returns `50 + 5*400 = 2050`
- **Assert**: `search_openalex` with `max_results=3` returns `50 + 3*350 = 1100`
- **Assert**: defaults (no max_results key) use n=10

### Test: `TestToolSetRegister`
- **Setup**: Create `ResearchToolSet`, call `Register` on a fresh `ToolRegistry`
- **Assert**: Registry has 2 tools: `search_arxiv`, `search_openalex`
- **Assert**: Can execute each via `registry.Execute` (with httptest backends)

## `internal/ctxmgr/guardrail_test.go` — addition

### Test: `TestReduceToolArgs_SearchOpenAlex`
- **Input**: `toolName="search_openalex", args={"max_results": 10.0}, remainingTokens=2000`
- **Assert**: Returns reduced args with `max_results=5`, `ok=true`

### Test: `TestReduceToolArgs_SearchOpenAlexNoReduction`
- **Input**: `args={"max_results": 3.0}`
- **Assert**: Returns `nil, false` (already under threshold)

## Integration-level test (in `tools/research/`)

### Test: `TestFullHandlerRoundTrip`
- **Setup**: httptest servers for both arXiv and OpenAlex; inject custom `*http.Client`
- Create `ResearchToolSet`, register on a `ToolRegistry`
- Call `registry.Execute(ctx, "search_arxiv", jsonInput)` and `registry.Execute(ctx, "search_openalex", jsonInput)`
- **Assert**: Both return valid JSON arrays parseable into result structs

## Test Fixtures

Canned responses stored as string constants in test files (not separate fixture files — keeps tests self-contained):
- `arxivAtomSample` — minimal valid Atom feed with 2 entries
- `openAlexJSONSample` — minimal valid response with 3 works (one with null abstract)

## What is NOT tested here

- Real network calls (deferred to manual e2e via `go run ./cmd/research-demo/`)
- The demo binary itself (it's a thin wiring layer over tested components)
- The agentic loop's integration with these tools (already tested in `internal/agentic/` with mocks)
