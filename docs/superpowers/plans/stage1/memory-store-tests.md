# Test Plan: Memory Store

Source plan: `.claude/plans/memory-store.md`

## Test environment

- **Language:** Go (memoryclient, tools packages) + Python (embedding service)
- **Go framework:** `go test` with `testing` package, table-driven tests
- **Python framework:** `pytest` with `httpx` (FastAPI TestClient)
- **Go test location:** `examples/03-memory-store/memoryclient/*_test.go`, `examples/03-memory-store/tools/*_test.go`
- **Python test location:** `services/embedding-api/test_main.py`
- **Run command:** `make test` (discovers example dirs automatically)
- **Database for tests:** Tests that need Postgres use a build tag `//go:build integration` and require a running `pgvector/pgvector:pg15` container. Unit tests mock the database via interfaces.
- **Mocking strategy:** Define a `DB` interface wrapping `*sql.DB` methods used by the client (`QueryContext`, `QueryRowContext`, `ExecContext`). Define an `Embedder` interface wrapping `Embed()`. Unit tests use test doubles; integration tests use real Postgres + real embedding service.

## Components to test

---

### 1. `EnsureDatabase` (bootstrap.go)

**What it does:** Connects to admin database, creates `memories` database if missing, returns connection to `memories`.
**Source plan section:** §1 (Migration strategy)

| # | Behavior | Input | Expected Output | Type |
|---|----------|-------|-----------------|------|
| 1 | Creates database on fresh Postgres | Admin DSN pointing to Postgres with no `memories` DB | Returns `*sql.DB` connected to `memories`; `SELECT 1 FROM pg_database WHERE datname = 'memories'` returns true | integration |
| 2 | Idempotent on existing database | Admin DSN, `memories` DB already exists | Returns `*sql.DB`, no error, no duplicate DB error | integration |
| 3 | Idempotent across rapid restarts | Call `EnsureDatabase` 3 times in sequence | All 3 succeed, database exists exactly once | integration |
| 4 | Returns error on bad admin DSN | `"postgres://bad:bad@localhost:9999/nope"` | Non-nil error wrapping connection failure | unit |
| 5 | Returns error if memories DSN unreachable | Valid admin DSN, memories DSN points to wrong port | Non-nil error from ping | integration |

**Mocking needed:** None for integration tests (real Postgres). For unit test #4, no mock needed — bad DSN fails at `sql.Open`/`Ping`.
**Setup/fixtures:** Docker container `pgvector/pgvector:pg15` on a random port. Each test uses a unique admin database name to avoid test interference.

---

### 2. `RunMigrations` (migrate.go)

**What it does:** Applies embedded SQL migrations to the connected database using golang-migrate.
**Source plan section:** §1

| # | Behavior | Input | Expected Output | Type |
|---|----------|-------|-----------------|------|
| 6 | Creates memories table on empty database | `*sql.DB` connected to empty `memories` DB | Table `memories` exists with all columns; `vector` extension enabled; all 4 indexes exist | integration |
| 7 | Idempotent on already-migrated database | `*sql.DB` where migrations already ran | No error (returns `migrate.ErrNoChange` internally, swallowed) | integration |
| 8 | HNSW index present | After migration | `SELECT indexname FROM pg_indexes WHERE tablename = 'memories' AND indexname = 'idx_memories_embedding'` returns 1 row; index type is `hnsw` | integration |
| 9 | search_vector generated column works | Insert a row with title="hook timeout" content="npm install fails" | `search_vector` is non-null; `to_tsvector('english', 'hook timeout')` matches | integration |

**Setup/fixtures:** Fresh `memories` database per test (create, migrate, drop).

---

### 3. `StoreMemory` + `validateTitle` + `estimateTokensFast` (store.go)

**What it does:** Validates title, gets embedding from service, inserts row into Postgres.
**Source plan section:** §5.4

| # | Behavior | Input | Expected Output | Type |
|---|----------|-------|-----------------|------|
| 10 | Stores memory with embedding | `StoreMemoryInput{MemoryType: "gotcha", Title: "Hook timeout breaks npm install", Content: "Default 60s timeout..."}`, embedding service up | Returns positive `int64` ID; row in DB has non-null `embedding` (384 dims), `token_count > 0` | integration |
| 11 | Stores memory without embedding (graceful degradation) | Same input, embedding service down | Returns positive ID; row in DB has `embedding IS NULL`; `token_count > 0` | integration |
| 12 | Stores memory with source | Input with `Source: "full raw turn content..."` | Row has `source` non-null, `source_tokens` computed as `len(source)/4` | integration |
| 13 | Stores memory without source | Input with `Source: ""` | Row has `source IS NULL`, `source_tokens IS NULL` | integration |
| 14 | Title too short rejected | `Title: "two words"` (2 words) | Error containing "title too short (2 words, minimum 3)" | unit |
| 15 | Title too long rejected | `Title: "one two three four five six seven eight nine ten eleven twelve thirteen fourteen fifteen sixteen"` (16 words) | Error containing "title too long (16 words, maximum 15)" | unit |
| 16 | Title at lower boundary accepted | `Title: "three word title"` (3 words) | No validation error | unit |
| 17 | Title at upper boundary accepted | `Title: "one two three four five six seven eight nine ten eleven twelve thirteen fourteen fifteen"` (15 words) | No validation error | unit |
| 18 | Empty title rejected | `Title: ""` | Error containing "title too short (0 words" | unit |
| 19 | Title with extra whitespace | `Title: "  spaced   out   title  "` (3 real words) | Accepted — `strings.Fields` handles extra whitespace | unit |
| 20 | `estimateTokensFast` basic | `"hello world test string"` (24 chars) | Returns 6 (`24/4`) | unit |
| 21 | `estimateTokensFast` empty | `""` | Returns 0 | unit |
| 22 | `token_count` matches estimate | Store memory with content "a" repeated 400 times | `token_count` in DB equals 100 (`400/4`) | integration |
| 23 | `session_id` persisted | Store with `sessionID = "abc-123"` | Row has `session_id = "abc-123"` | integration |
| 24 | `turn_index` persisted | Store with `turnIndex = 7` | Row has `turn_index = 7` | integration |

**Mocking needed:** Unit tests (#14-21) need no DB. Integration tests (#10-13, 22-24) need Postgres + optionally embedding service.
**Setup/fixtures:** For embedding-down tests, use a `Config.EmbedURL` pointing to `http://localhost:1` (connection refused).

---

### 4. `SearchMemories` / `searchFTS` / `searchSemantic` / `searchHybrid` (search.go)

**What it does:** Searches memories using FTS, vector similarity, or hybrid RRF.
**Source plan section:** §5.1

**Seed data for search tests:** Insert 5 memories before each search test:

| ID | Type | Title | Content |
|----|------|-------|---------|
| S1 | gotcha | Hook timeout breaks npm install | Default 60s timeout insufficient for npm... |
| S2 | decision | Manager-first context control model | Agent does not see budget information... |
| S3 | how-it-works | tsvector search weights title over content | Title gets weight A, content gets weight B... |
| S4 | discovery | RRF outperforms weighted sum for hybrid | Reciprocal rank fusion is rank-based... |
| S5 | gotcha | Embedding nulls break semantic-only search | Rows without embeddings are excluded... |

| # | Behavior | Input | Expected Output | Type |
|---|----------|-------|-----------------|------|
| 25 | FTS finds matching memory | `query="hook timeout", mode="text"` | Results include S1; S1.Score > 0 | integration |
| 26 | FTS respects limit | `query="timeout", mode="text", limit=1` | Exactly 1 result | integration |
| 27 | FTS returns empty on no match | `query="xylophone quantum entanglement", mode="text"` | `Matches` is empty slice, no error | integration |
| 28 | FTS filters by memory_type | `query="hook timeout", mode="text", memType="decision"` | Results do NOT include S1 (which is "gotcha") | integration |
| 29 | FTS title weighted higher than content | Insert memory with "hook" in title vs "hook" in content only | Title-match memory ranks higher | integration |
| 30 | Semantic finds similar memory | `query="npm timeout configuration", mode="semantic"` | Results include S1 (semantically similar); embedding service must be up | integration |
| 31 | Semantic errors when embed service down | `query="anything", mode="semantic"`, embed service down | Non-nil error containing "embedding failed" | integration |
| 32 | Semantic skips null-embedding rows | Store a memory without embedding (embed service was down), then `mode="semantic"` | That memory does NOT appear in results | integration |
| 33 | Hybrid returns results | `query="hook timeout", mode="hybrid"` | Non-empty results; results include S1 | integration |
| 34 | Hybrid falls back to FTS when embed down | `query="hook timeout", mode="hybrid"`, embed service down | Non-empty results (FTS-only); `SearchMode` is still "hybrid" | integration |
| 35 | Hybrid RRF boosts dual-match items | Insert a memory matching both FTS query terms AND semantically similar | That memory ranks higher than FTS-only or semantic-only matches | integration |
| 36 | Default mode is hybrid | `mode=""` | `SearchResult.SearchMode == "hybrid"` | unit |
| 37 | Default limit is 20 | `limit=0` | At most 20 results (with enough seed data) | unit |
| 38 | Unknown mode returns error | `mode="magic"` | Error containing "unknown search mode: magic" | unit |
| 39 | Result format correct | Any successful search | Each row has: `ID > 0`, `Type` is emoji, `Date` matches `YYYY-MM-DD`, `Tokens > 0`, `Score > 0` | integration |
| 40 | Instructions field present | Any successful search | `Instructions` contains "get_memory_details" | integration |
| 41 | memoryTypeToEmoji maps all 9 types | Each of the 9 type keys | Returns correct emoji for each: `gotcha→🔴`, `decision→🟤`, etc. | unit |

**Mocking needed:** Unit tests mock DB interface. Integration tests need Postgres + embedding service.

---

### 5. `GetMemoryDetails` (details.go)

**What it does:** Fetches full content of specific memories by ID.
**Source plan section:** §5.2

| # | Behavior | Input | Expected Output | Type |
|---|----------|-------|-----------------|------|
| 42 | Returns requested memories | `ids=[S1.ID, S2.ID]` (from seed data) | 2 memories returned with correct Title, Content, Type (as emoji) | integration |
| 43 | Skips nonexistent IDs silently | `ids=[S1.ID, 99999]` | 1 memory returned (S1), no error | integration |
| 44 | Empty IDs returns empty result | `ids=[]` | `Memories` is empty slice, no error | integration |
| 45 | `has_source` true when source present | Memory stored with source | `HasSource == true`, `SourceTokens` non-nil | integration |
| 46 | `has_source` false when no source | Memory stored without source | `HasSource == false`, `SourceTokens` is nil | integration |
| 47 | `turn_index` omitted when nil | Memory stored with `turnIndex = 0` (nullable) | JSON output has `turn_index` omitted or null | integration |
| 48 | CreatedAt is RFC3339 format | Any memory | `CreatedAt` parses as `time.RFC3339` without error | integration |
| 49 | Instructions field present | Any call | `Instructions` contains "get_memory_source" | integration |

---

### 6. `GetMemorySource` + `PeekSourceSize` (details.go)

**What it does:** Fetches raw source content; peeks at source token count without loading content.
**Source plan section:** §5.3

| # | Behavior | Input | Expected Output | Type |
|---|----------|-------|-----------------|------|
| 50 | Returns source for memory with source | ID of memory stored with `Source: "raw content..."` | `SourceResult` with matching `Source` string and `SourceTokens > 0` | integration |
| 51 | Errors for memory without source | ID of memory stored without source | Error containing "has no source" | integration |
| 52 | Errors for nonexistent ID | `id=99999` | Error containing "not found" | integration |
| 53 | PeekSourceSize returns correct count | ID of memory with `source_tokens = 250` | Returns `250, nil` | integration |
| 54 | PeekSourceSize errors for no source | ID of memory without source | Error containing "has no source" | integration |
| 55 | PeekSourceSize errors for bad ID | `id=99999` | Error containing "not found" | integration |
| 56 | PeekSourceSize respects context timeout | ID with source, context with 1ns timeout | Error (context deadline exceeded) | integration |

---

### 7. `Embed` (client.go)

**What it does:** POSTs texts to embedding service, returns float32 vectors.
**Source plan section:** §4 (Embedding helper)

| # | Behavior | Input | Expected Output | Type |
|---|----------|-------|-----------------|------|
| 57 | Single text returns 384-dim vector | `texts=["hello world"]` | `[][]float32` with 1 element of length 384; all values finite | integration |
| 58 | Batch of 5 returns 5 vectors | `texts=["a","b","c","d","e"]` | 5 vectors, each 384-dim | integration |
| 59 | Returns error when service down | Embed URL pointing to `http://localhost:1` | Non-nil error containing "embed request failed" | unit |
| 60 | Returns error on non-200 status | Mock HTTP server returning 500 | Non-nil error containing "embed returned 500" | unit |
| 61 | Returns error on malformed JSON response | Mock HTTP server returning `{"garbage": true}` | Non-nil error or empty embeddings | unit |
| 62 | Respects context cancellation | Cancelled context | Error (context cancelled) | unit |

**Mocking needed:** Unit tests use `httptest.NewServer`. Integration tests use the real embedding service container.

---

### 8. Embedding Service (Python — `main.py`)

**What it does:** FastAPI endpoint that generates embeddings with input validation.
**Source plan section:** §2

| # | Behavior | Input | Expected Output | Type |
|---|----------|-------|-----------------|------|
| 63 | Single text embedding | `POST /embed {"texts": ["hello"]}` | 200; `embeddings` has 1 vector of 384 floats; `dimensions == 384` | unit |
| 64 | Batch of 10 texts | `POST /embed {"texts": ["a","b",...,"j"]}` | 200; 10 vectors, each 384-dim | unit |
| 65 | Normalized embeddings | `POST /embed {"texts": ["hello"]}` | L2 norm of returned vector is approximately 1.0 (±0.01) | unit |
| 66 | Empty texts rejected | `POST /embed {"texts": []}` | 422; error message contains "must not be empty" | unit |
| 67 | Over 100 texts rejected | `POST /embed {"texts": ["a"] * 101}` | 422; error message contains "max 100" | unit |
| 68 | Exactly 100 texts accepted | `POST /embed {"texts": ["a"] * 100}` | 200; 100 vectors | unit |
| 69 | Long text truncated (not rejected) | `POST /embed {"texts": ["x" * 20000]}` | 200; 1 vector returned (text silently truncated to 10,000 chars) | unit |
| 70 | Health endpoint | `GET /health` | 200; `{"status": "ok", "model": "all-MiniLM-L6-v2", "dimensions": 384}` | unit |
| 71 | Missing body | `POST /embed` with empty body | 422 | unit |
| 72 | Same text produces same embedding | Embed "hello" twice | Both vectors are identical (deterministic) | unit |

**Framework:** `pytest` + `httpx.AsyncClient` via `from fastapi.testclient import TestClient`.
**Setup:** `client = TestClient(app)` — no server needed.

---

### 9. Tool Definitions (tools/definitions.go)

**What it does:** Builds `anthropic.ToolUnionParam` for each tool.
**Source plan section:** §5.1-5.4

| # | Behavior | Input | Expected Output | Type |
|---|----------|-------|-----------------|------|
| 73 | `buildSearchMemoriesTool` schema | Call function | Tool name is `"search_memories"`; has `query` in required; properties include `query`, `mode`, `limit`, `memory_type` | unit |
| 74 | `buildGetMemoryDetailsTool` schema | Call function | Tool name is `"get_memory_details"`; has `ids` in required; `ids` type is array of integer | unit |
| 75 | `buildGetMemorySourceTool` schema | Call function | Tool name is `"get_memory_source"`; has `id` in required | unit |
| 76 | `buildStoreMemoryTool` schema | Call function | Tool name is `"store_memory"`; required includes `memory_type`, `title`, `content`; `memory_type` has enum with 9 values | unit |
| 77 | All tools have descriptions | Call all 4 build functions | Each `.OfTool.Description` is non-nil and non-empty | unit |

---

### 10. Tool Handlers (tools/handlers.go)

**What it does:** `NewMemoryToolHandlers` returns handler map with session UUID and turn callback.
**Source plan section:** §5.4

| # | Behavior | Input | Expected Output | Type |
|---|----------|-------|-----------------|------|
| 78 | store_memory handler returns ID | Valid JSON input | JSON output with `"id"` > 0 and `"stored": true` | integration |
| 79 | store_memory uses session ID | Store, then query DB for session_id | Matches the sessionID passed to `NewMemoryToolHandlers` | integration |
| 80 | store_memory uses turn callback | `currentTurn` returns 5, then 8 | First memory has `turn_index=5`, second has `turn_index=8` | integration |
| 81 | store_memory returns error on bad JSON | `[]byte("{invalid}")` | Non-empty error string | unit |
| 82 | Handler map has all 4 tools | Call `NewMemoryToolHandlers` | Map has keys: `store_memory`, `search_memories`, `get_memory_details`, `get_memory_source` | unit |

---

### 11. ToolSizeEstimator Registrations (tools/estimators.go)

**What it does:** Registers size estimation functions for the 4 memory tools.
**Source plan section:** §6.1

| # | Behavior | Input | Expected Output | Type |
|---|----------|-------|-----------------|------|
| 83 | search_memories estimate scales with limit | `args={"limit": 20}` | Returns `50 + 20*20 = 450` | unit |
| 84 | search_memories default limit | `args={}` (no limit key) | Returns `50 + 20*20 = 450` (default 20) | unit |
| 85 | get_memory_details estimate scales with IDs | `args={"ids": [1,2,3]}` | Returns `50 + 3*200 = 650` | unit |
| 86 | get_memory_details empty IDs | `args={"ids": []}` | Returns `200` | unit |
| 87 | get_memory_source uses PeekSourceSize | `args={"id": <real ID with source_tokens=500>}` | Returns `500 + 50 = 550` | integration |
| 88 | get_memory_source falls back on timeout | `args={"id": <real ID>}`, but DB artificially slow (e.g. `pg_sleep`) | Returns `2000` (fallback) | integration |
| 89 | get_memory_source falls back on missing ID | `args={"id": 0}` | Returns `2000` | unit |
| 90 | store_memory estimate is fixed | `args={}` | Returns `50` | unit |

---

### 12. `reduceToolArgs` cases (tools/estimators.go)

**What it does:** Reduces tool args when context budget is tight.
**Source plan section:** §6.2

| # | Behavior | Input | Expected Output | Type |
|---|----------|-------|-----------------|------|
| 91 | search_memories limit reduced from 20 to 10 | `args={"limit": 20}` | `reduced["limit"] == 10`, `ok == true` | unit |
| 92 | search_memories limit already <= 10 | `args={"limit": 5}` | `reduced["limit"] == 10`, `ok == true` (always reduces to 10) | unit |
| 93 | search_memories no limit key | `args={}` | `reduced["limit"] == 10`, `ok == true` | unit |
| 94 | get_memory_details IDs truncated | `args={"ids": [1,2,3,4,5]}` | `reduced["ids"]` has length 3 (first 3), `ok == true` | unit |
| 95 | get_memory_details IDs <= 3 | `args={"ids": [1,2]}` | `ok == false` (no reduction possible) | unit |

---

## Integration tests

These require Docker Compose (`pgvector/pgvector:pg15` + `embedding-api`) running.

| # | Scenario | Components involved | Setup | Verification |
|---|----------|-------------------|-------|-------------|
| I1 | Full bootstrap from scratch | EnsureDatabase, RunMigrations | Fresh Postgres container, no `memories` DB | DB created, table exists, all indexes present, `vector` extension enabled |
| I2 | Store → FTS search → Details → Source (full progressive disclosure) | StoreMemory, SearchMemories(text), GetMemoryDetails, GetMemorySource | Store 3 memories (1 with source, 2 without) | FTS finds them; details returns content + `has_source`; source returns raw content for the one that has it |
| I3 | Store → Hybrid search (with embedding service) | StoreMemory, Embed, SearchMemories(hybrid) | Store 5 memories with diverse content, embedding service up | Hybrid returns results; results include semantically similar matches that pure FTS might miss |
| I4 | Hybrid fallback when embedding service down | SearchMemories(hybrid) | Store memories, then stop embedding-api container | Search still returns results (FTS-only); no error |
| I5 | Store without embedding, then search | StoreMemory (embed service down), SearchMemories(text), SearchMemories(semantic) | Stop embed service, store memory, restart | FTS finds it; semantic does NOT find it (null embedding) |
| I6 | Token budget claim: 20 rows ~300-400 tokens | SearchMemories(limit=20) | Store 20+ memories | JSON-serialize result; `len(json) / 4` is between 300 and 500 |
| I7 | PeekSourceSize matches actual source | StoreMemory with known source, PeekSourceSize, GetMemorySource | Store memory with 2000-char source | `PeekSourceSize` returns 500 (2000/4); `GetMemorySource.SourceTokens` also 500 |
| I8 | Migration idempotency | RunMigrations x3 | Run migrations, insert data, run migrations again x2 | Data survives; no errors; table structure unchanged |

---

## Test gaps

These behaviors are specified in the plan but are **too vague or infrastructure-dependent to write precise tests for** at this stage:

1. **`pgVectorLiteral` helper function** — referenced in search and store code but never defined. Need to know the exact format it produces (e.g., `"[0.1,0.2,...]"` vs `"{0.1,0.2,...}"`) before writing tests.

2. **`nullString` helper function** — referenced in store code, converts empty string to `sql.NullString`. Trivial but needs to exist.

3. **`memoryTypeToEmoji` mapping** — test #41 covers the contract, but the function isn't defined in the plan. Need to know if it handles unknown types (return type key as-is? error?).

4. **Docker Compose Langfuse compatibility** — the plan claims swapping to `pgvector/pgvector:pg15` won't break Langfuse. This needs a manual smoke test: `docker compose up`, verify Langfuse UI loads on `:3000`, create a trace. Not automatable in unit tests.

5. **Search latency < 200ms at 10k rows** — performance test. Requires seeding 10k rows (with embeddings) and benchmarking. Should be a separate benchmark, not part of `make test`.

6. **Embedding latency < 2s for batch of 10** — depends on hardware. Should be a benchmark in `test_main.py`, skipped by default.

---

## Verification strategy

**Unit tests (no external deps):**
```bash
cd examples/03-memory-store && go test ./... -short
```
Runs tests #14-21, 36-38, 41, 60-62, 73-77, 81-86, 89-95. Should pass in < 5 seconds, no Docker needed.

**Python tests (no external deps beyond pip):**
```bash
cd services/embedding-api && pip install -r requirements.txt && pytest test_main.py -v
```
Runs tests #63-72. Requires model download on first run (~80MB).

**Integration tests (require Docker Compose):**
```bash
docker compose -f docker-compose.langfuse.yml up -d langfuse-db embedding-api
cd examples/03-memory-store && go test ./... -tags integration -count=1
```
Runs all tests including #1-13, 22-35, 42-58, 78-80, 87-88, I1-I8.

**What passing looks like:**
- All unit tests pass with `-short` flag
- All Python tests pass
- All integration tests pass with `-tags integration`
- `make check` (lint + build + test) passes for the `examples/03-memory-store` directory

**What to watch for:**
- Flaky integration tests from port conflicts (use random ports or Docker networking)
- Slow first run of Python tests (model download)
- Integration tests leaving behind test databases — each test should clean up or use a unique DB name
