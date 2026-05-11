# Memory Store: PostgreSQL + pg_vector + Embedding Service

## Problem Statement

The agentic research loop produces valuable outputs (observations, decisions, findings, gotchas) that are discarded after each run. There is no way to recall past research context, avoid repeating mistakes, or build on prior sessions. The agent starts cold every time.

This plan designs a persistent memory store backed by PostgreSQL (with pg_vector for semantic search and tsvector for full-text search), an external embedding service in Python/FastAPI, and three Go tool calls that implement progressive disclosure so the agent can efficiently retrieve past memories without polluting its context window.

---

## Requirements

### Functional

1. **Store memories** — persist structured observations from agentic loop runs (type, title, content, source, timestamps, embedding)
2. **Full-text search** — tsvector/tsquery search over memory titles and content
3. **Semantic search** — pg_vector cosine similarity search using embeddings from the external service
4. **Hybrid search** — combine full-text and semantic scores with Reciprocal Rank Fusion (RRF)
5. **Progressive disclosure** — three-layer tool interface: index → details → source
6. **Embedding service** — FastAPI endpoint that accepts batches of text and returns embeddings, with input validation
7. **Context-manager integration** — all tools integrate with `PreToolGuardrail` / `PostToolCheck` / `ToolSizeEstimator` using existing mechanisms (no new contracts)
8. **Write path** — `store_memory` tool to persist new memories from the current run
9. **Schema migration** — Go-based migration that runs at startup, idempotent, works on existing volumes

### Non-Functional

10. **Embedding latency** — batch of 10 texts in < 2 seconds (local model)
11. **Search latency** — < 200ms for hybrid search over 10k memories
12. **Token efficiency** — Layer 1 results use ~15-20 tokens per row
13. **Docker Compose** — all infra runs alongside existing Langfuse stack

### Out of Scope

- Memory deduplication or merging
- Access control / multi-tenancy
- Embedding model fine-tuning
- Real-time memory streaming
- Migration from the old Python SQLite memory store

---

## Specs

### Architecture

```
┌──────────────────────────────────────────────────────────────┐
│                       Agent Loop (Go)                        │
│                                                              │
│  ┌────────────┐    ┌───────────────┐    ┌────────────────┐   │
│  │ Claude API  │◀──│ContextManager │───▶│ Memory Tools   │   │
│  └────────────┘    │ (guardrails)  │    │                │   │
│                    └───────────────┘    │ search_memories │   │
│                                        │ get_mem_details │   │
│                                        │ get_mem_source  │   │
│                                        │ store_memory    │   │
│                                        └───────┬────────┘   │
└────────────────────────────────────────────────┼────────────┘
                                                 │ HTTP / SQL
                    ┌────────────────────────────┼────────────┐
                    │         Docker Compose      │            │
                    │                             ▼            │
                    │  ┌──────────────────────────────────┐    │
                    │  │  PostgreSQL 15 + pg_vector        │    │
                    │  │  - memories table (separate DB)   │    │
                    │  │  - tsvector index (FTS)           │    │
                    │  │  - HNSW index (vector)            │    │
                    │  └──────────────────────────────────┘    │
                    │                             ▲            │
                    │  ┌──────────────┐           │            │
                    │  │ Embedding API │───────────┘            │
                    │  │ (FastAPI)     │  writes embeddings     │
                    │  │ :8100        │  on store_memory        │
                    │  └──────────────┘                         │
                    │                                           │
                    │  ┌──────────────┐                         │
                    │  │ Langfuse     │  (existing, unchanged)  │
                    │  │ :3000        │                         │
                    │  └──────────────┘                         │
                    └───────────────────────────────────────────┘
```

**Data flow:**

1. **Write path:** Agent calls `store_memory` tool → Go handler POSTs text to embedding service → embedding service returns vector → Go handler INSERTs row + vector into Postgres
2. **Read path (Layer 1):** Agent calls `search_memories` → Go handler runs hybrid SQL query (tsvector + pg_vector) → returns compact index table
3. **Read path (Layer 2):** Agent calls `get_memory_details` with IDs → Go handler SELECTs full content → returns details
4. **Read path (Layer 3):** Agent calls `get_memory_source` with ID → Go handler SELECTs raw source → returns original uncompacted content

### 1. PostgreSQL Schema

The existing `langfuse-db` container runs Postgres 15. We swap it for an image that includes pg_vector, and create a **separate database** (`memories`) so Langfuse's Prisma migrations never interact with our tables.

**Database isolation:** The Postgres container hosts two databases: `langfuse` (managed by Langfuse/Prisma) and `memories` (managed by our Go migration code). This prevents Langfuse upgrades from affecting memory data and avoids confusing `prisma db pull`.

**Migration strategy:** The Go binary is fully self-bootstrapping. On startup it: (1) connects to the default `langfuse` database, (2) creates the `memories` database if it doesn't exist, (3) reconnects to `memories`, (4) runs `golang-migrate/migrate` with migrations embedded via `embed.FS`. This is idempotent — safe to run on every startup, works on fresh volumes and existing ones. No init scripts in `docker-entrypoint-initdb.d`.

```
db/migrations/
├── 000001_create_memories.up.sql
├── 000001_create_memories.down.sql
```

**`000001_create_memories.up.sql`:**

```sql
CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE IF NOT EXISTS memories (
    id              BIGSERIAL PRIMARY KEY,
    session_id      TEXT NOT NULL,
    turn_index      INT,
    memory_type     TEXT NOT NULL,
    title           TEXT NOT NULL,
    content         TEXT NOT NULL,
    source          TEXT,
    token_count     INT NOT NULL,
    source_tokens   INT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    embedding       vector(384),

    search_vector   tsvector GENERATED ALWAYS AS (
        setweight(to_tsvector('english', title), 'A') ||
        setweight(to_tsvector('english', content), 'B')
    ) STORED
);

CREATE INDEX IF NOT EXISTS idx_memories_search ON memories USING GIN (search_vector);

-- HNSW: no training data required, works correctly from row 0.
-- At <100k rows, memory overhead vs ivfflat is negligible (~50MB).
CREATE INDEX IF NOT EXISTS idx_memories_embedding ON memories USING hnsw (embedding vector_cosine_ops)
    WITH (m = 16, ef_construction = 64);

CREATE INDEX IF NOT EXISTS idx_memories_session ON memories (session_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_memories_type ON memories (memory_type, created_at DESC);
```

**`000001_create_memories.down.sql`:**

```sql
DROP TABLE IF EXISTS memories;
```

**Go bootstrap + migration runner:**

```go
import (
    "embed"
    "database/sql"
    "fmt"

    "github.com/golang-migrate/migrate/v4"
    "github.com/golang-migrate/migrate/v4/database/postgres"
    "github.com/golang-migrate/migrate/v4/source/iofs"
    _ "github.com/lib/pq"
)

//go:embed db/migrations/*.sql
var migrationsFS embed.FS

// EnsureDatabase connects to the default database and creates the memories
// database if it doesn't exist. Returns a connection to the memories database.
// This makes the Go binary fully self-bootstrapping — no init scripts needed.
func EnsureDatabase(defaultDSN, memoriesDSN string) (*sql.DB, error) {
    admin, err := sql.Open("postgres", defaultDSN)
    if err != nil {
        return nil, fmt.Errorf("open admin db: %w", err)
    }
    defer admin.Close()

    var exists bool
    err = admin.QueryRow("SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = 'memories')").Scan(&exists)
    if err != nil {
        return nil, fmt.Errorf("check memories db: %w", err)
    }
    if !exists {
        if _, err := admin.Exec("CREATE DATABASE memories"); err != nil {
            return nil, fmt.Errorf("create memories db: %w", err)
        }
    }

    db, err := sql.Open("postgres", memoriesDSN)
    if err != nil {
        return nil, fmt.Errorf("open memories db: %w", err)
    }
    if err := db.Ping(); err != nil {
        db.Close()
        return nil, fmt.Errorf("ping memories db: %w", err)
    }
    return db, nil
}

func RunMigrations(db *sql.DB) error {
    source, err := iofs.New(migrationsFS, "db/migrations")
    if err != nil {
        return fmt.Errorf("migration source: %w", err)
    }
    driver, err := postgres.WithInstance(db, &postgres.Config{})
    if err != nil {
        return fmt.Errorf("migration driver: %w", err)
    }
    m, err := migrate.NewWithInstance("iofs", source, "postgres", driver)
    if err != nil {
        return fmt.Errorf("migration init: %w", err)
    }
    if err := m.Up(); err != nil && err != migrate.ErrNoChange {
        return fmt.Errorf("migration up: %w", err)
    }
    return nil
}
```

**Memory types (stored as text keys, displayed as emoji):**

| Key | Emoji | Meaning |
|-----|-------|---------|
| `session-goal` | `🎯` | Original research query |
| `gotcha` | `🔴` | Critical edge case or pitfall |
| `problem-fix` | `🟡` | Bug fix or workaround |
| `how-it-works` | `🔵` | Technical explanation |
| `what-changed` | `🟢` | Code/architecture change |
| `discovery` | `🟣` | Learning or insight |
| `why-it-exists` | `🟠` | Design rationale |
| `decision` | `🟤` | Architecture decision |
| `trade-off` | `⚖️` | Deliberate compromise |

### 2. FastAPI Embedding Service

A lightweight Python service that wraps `sentence-transformers` with batching support and input validation.

```
services/embedding-api/
├── Dockerfile
├── requirements.txt
├── main.py
└── test_main.py
```

**`main.py`:**

```python
from fastapi import FastAPI, HTTPException
from pydantic import BaseModel, field_validator

app = FastAPI()

MAX_TEXTS = 100
MAX_CHARS_PER_TEXT = 10_000

class EmbedRequest(BaseModel):
    texts: list[str]

    @field_validator("texts")
    @classmethod
    def validate_texts(cls, v):
        if len(v) == 0:
            raise ValueError("texts must not be empty")
        if len(v) > MAX_TEXTS:
            raise ValueError(f"max {MAX_TEXTS} texts per request, got {len(v)}")
        return [t[:MAX_CHARS_PER_TEXT] for t in v]  # truncate, don't reject

class EmbedResponse(BaseModel):
    embeddings: list[list[float]]
    dimensions: int

# Load model at module level — once, at startup
from sentence_transformers import SentenceTransformer
model = SentenceTransformer("all-MiniLM-L6-v2")

@app.post("/embed", response_model=EmbedResponse)
def embed(req: EmbedRequest):
    vectors = model.encode(req.texts, batch_size=32, normalize_embeddings=True)
    return EmbedResponse(
        embeddings=vectors.tolist(),
        dimensions=vectors.shape[1],  # 384 for MiniLM
    )

@app.get("/health")
def health():
    return {"status": "ok", "model": "all-MiniLM-L6-v2", "dimensions": 384}
```

**`Dockerfile`:**

```dockerfile
FROM python:3.12-slim
WORKDIR /app
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
COPY main.py .
EXPOSE 8100
CMD ["uvicorn", "main:app", "--host", "0.0.0.0", "--port", "8100"]
```

**`requirements.txt`:**

```
fastapi==0.115.0
uvicorn[standard]==0.30.0
sentence-transformers==3.0.0
pydantic==2.9.0
```

**Batching design:** The `/embed` endpoint accepts up to 100 texts, each truncated at 10,000 characters. `sentence-transformers` handles internal batching via `batch_size=32`. The Go client sends all texts in a single HTTP request rather than one request per text. For `store_memory` (single text), the batch size is 1.

### 3. Docker Compose Additions

Modifications to `docker-compose.langfuse.yml`:

```yaml
services:
  langfuse-db:
    image: pgvector/pgvector:pg15    # swap from postgres:15
    restart: unless-stopped
    environment:
      POSTGRES_USER: langfuse
      POSTGRES_PASSWORD: langfuse
      POSTGRES_DB: langfuse
    ports:
      - "5432:5432"
    volumes:
      - langfuse-db-data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U langfuse"]
      interval: 5s
      timeout: 5s
      retries: 5

  embedding-api:
    build:
      context: ./services/embedding-api
    restart: unless-stopped
    ports:
      - "8100:8100"
    healthcheck:
      test: ["CMD", "python", "-c", "import urllib.request; urllib.request.urlopen('http://localhost:8100/health')"]
      interval: 10s
      timeout: 5s
      retries: 3
    deploy:
      resources:
        limits:
          memory: 2G    # sentence-transformers + model weights
```

**Key changes:**
- `langfuse-db` image swapped to `pgvector/pgvector:pg15` (drop-in replacement, Langfuse unaffected)
- No init scripts — the Go binary creates the `memories` database and runs migrations at startup via `EnsureDatabase()` + `RunMigrations()`
- New `embedding-api` service on port 8100 (healthcheck uses Python's `urllib` — no `curl` dependency needed in `python:3.12-slim`)
- Langfuse connects to `langfuse` database, memory client connects to `memories` database — complete isolation

### 4. Go Module Structure

This lives under `examples/03-memory-store/` as its own Go module, consistent with the project convention ("each example under `examples/` is its own Go module").

```
examples/03-memory-store/
├── go.mod                  # module memory-store
├── go.sum
├── main.go                 # demo: store + search + progressive disclosure
├── memoryclient/
│   ├── client.go           # Client struct, Config, New(), Close(), Embed()
│   ├── search.go           # SearchMemories, searchFTS, searchSemantic, searchHybrid
│   ├── details.go          # GetMemoryDetails, GetMemorySource, PeekSourceSize
│   ├── store.go            # StoreMemory, validateTitle, estimateTokensFast
│   ├── bootstrap.go        # EnsureDatabase (creates memories DB if needed)
│   └── migrate.go          # RunMigrations with embedded SQL
├── tools/
│   ├── definitions.go      # build*Tool() functions
│   ├── handlers.go         # NewMemoryToolHandlers (session UUID + turn callback)
│   └── estimators.go       # RegisterMemoryToolEstimators (PeekSourceSize with timeout)
└── db/
    └── migrations/
        ├── 000001_create_memories.up.sql
        └── 000001_create_memories.down.sql
```

**`go.mod` dependencies:**
- `github.com/anthropics/anthropic-sdk-go` — tool definitions, API types
- `github.com/lib/pq` — Postgres driver
- `github.com/golang-migrate/migrate/v4` — schema migrations
- `github.com/charmbracelet/lipgloss` + `log` — consistent with other examples

**Memory client:**

```go
package memoryclient

import (
    "context"
    "database/sql"
    "encoding/json"
    "fmt"
    "net/http"
    "bytes"

    _ "github.com/lib/pq"
)

type Client struct {
    db           *sql.DB
    embedURL     string
    httpClient   *http.Client
}

type Config struct {
    AdminDSN     string // "postgres://langfuse:langfuse@localhost:5432/langfuse?sslmode=disable"
    MemoriesDSN  string // "postgres://langfuse:langfuse@localhost:5432/memories?sslmode=disable"
    EmbedURL     string // "http://localhost:8100"
}

func New(cfg Config) (*Client, error) {
    db, err := EnsureDatabase(cfg.AdminDSN, cfg.MemoriesDSN)
    if err != nil {
        return nil, fmt.Errorf("ensure database: %w", err)
    }

    if err := RunMigrations(db); err != nil {
        db.Close()
        return nil, fmt.Errorf("migrations: %w", err)
    }

    return &Client{
        db:         db,
        embedURL:   cfg.EmbedURL,
        httpClient: &http.Client{},
    }, nil
}

func (c *Client) Close() error {
    return c.db.Close()
}
```

**Embedding helper:**

```go
type embedRequest struct {
    Texts []string `json:"texts"`
}

type embedResponse struct {
    Embeddings [][]float32 `json:"embeddings"`
    Dimensions int         `json:"dimensions"`
}

func (c *Client) Embed(ctx context.Context, texts []string) ([][]float32, error) {
    body, err := json.Marshal(embedRequest{Texts: texts})
    if err != nil {
        return nil, fmt.Errorf("marshal embed request: %w", err)
    }
    req, err := http.NewRequestWithContext(ctx, "POST", c.embedURL+"/embed", bytes.NewReader(body))
    if err != nil {
        return nil, err
    }
    req.Header.Set("Content-Type", "application/json")

    resp, err := c.httpClient.Do(req)
    if err != nil {
        return nil, fmt.Errorf("embed request failed: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != 200 {
        return nil, fmt.Errorf("embed returned %d", resp.StatusCode)
    }

    var out embedResponse
    if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
        return nil, fmt.Errorf("decode embed response: %w", err)
    }
    return out.Embeddings, nil
}
```

### 5. Tool Definitions (Go)

#### 5.1 Layer 1 — `search_memories` (Index)

Returns a compact table. Supports three search modes: `text` (FTS), `semantic` (vector), `hybrid` (both via RRF).

```go
func buildSearchMemoriesTool() anthropic.ToolUnionParam {
    t := anthropic.ToolUnionParamOfTool(
        anthropic.ToolInputSchemaParam{
            Type: "object",
            Properties: map[string]any{
                "query": map[string]any{
                    "type":        "string",
                    "description": "Search query for memories",
                },
                "mode": map[string]any{
                    "type":        "string",
                    "enum":        []string{"text", "semantic", "hybrid"},
                    "description": "Search mode: 'text' (full-text), 'semantic' (vector similarity), 'hybrid' (both, default)",
                    "default":     "hybrid",
                },
                "limit": map[string]any{
                    "type":        "integer",
                    "description": "Max results to return (default 20)",
                    "default":     20,
                },
                "memory_type": map[string]any{
                    "type":        "string",
                    "description": "Filter by memory type (e.g. 'gotcha', 'decision'). Omit for all types.",
                },
            },
            Required: []string{"query"},
        },
        "search_memories",
    )
    t.OfTool.Description = anthropic.String(
        "Search past research memories. Returns a compact index with IDs, types, titles, dates, and token costs. " +
        "Use get_memory_details to fetch full content of specific memories.",
    )
    return t
}
```

**Handler:**

```go
type MemoryIndexRow struct {
    ID         int64   `json:"id"`
    Type       string  `json:"type"`
    Title      string  `json:"title"`
    Date       string  `json:"date"`
    Tokens     int     `json:"tokens"`
    Score      float64 `json:"score"`
}

type SearchResult struct {
    Matches      []MemoryIndexRow `json:"matches"`
    SearchMode   string           `json:"search_mode"`
    Instructions string           `json:"instructions"`
}

func (c *Client) SearchMemories(ctx context.Context, query string, mode string, limit int, memType string) (*SearchResult, error) {
    if mode == "" {
        mode = "hybrid"
    }
    if limit == 0 {
        limit = 20
    }

    var rows []MemoryIndexRow
    var err error

    switch mode {
    case "text":
        rows, err = c.searchFTS(ctx, query, limit, memType)
    case "semantic":
        rows, err = c.searchSemantic(ctx, query, limit, memType)
    case "hybrid":
        rows, err = c.searchHybrid(ctx, query, limit, memType)
    default:
        return nil, fmt.Errorf("unknown search mode: %s", mode)
    }
    if err != nil {
        return nil, err
    }

    return &SearchResult{
        Matches:    rows,
        SearchMode: mode,
        Instructions: "Use get_memory_details with IDs from this index to fetch full content. " +
            "Types: 🎯=session-goal 🔴=gotcha 🟡=problem-fix 🔵=how-it-works 🟢=what-changed 🟣=discovery 🟠=why-it-exists 🟤=decision ⚖️=trade-off",
    }, nil
}
```

**Full-text search query (fully parameterized):**

```go
func (c *Client) searchFTS(ctx context.Context, query string, limit int, memType string) ([]MemoryIndexRow, error) {
    var args []any
    args = append(args, query) // $1

    typeClause := ""
    if memType != "" {
        typeClause = " AND memory_type = $2"
        args = append(args, memType)
    }

    limitPlaceholder := fmt.Sprintf("$%d", len(args)+1)
    args = append(args, limit)

    q := fmt.Sprintf(`
        SELECT id, memory_type, title, created_at, token_count,
               ts_rank_cd(search_vector, websearch_to_tsquery('english', $1)) AS rank
        FROM memories
        WHERE search_vector @@ websearch_to_tsquery('english', $1)
        %s
        ORDER BY rank DESC
        LIMIT %s
    `, typeClause, limitPlaceholder)

    rows, err := c.db.QueryContext(ctx, q, args...)
    if err != nil {
        return nil, fmt.Errorf("fts query: %w", err)
    }
    defer rows.Close()

    var results []MemoryIndexRow
    for rows.Next() {
        var r MemoryIndexRow
        var createdAt time.Time
        var memoryType string
        if err := rows.Scan(&r.ID, &memoryType, &r.Title, &createdAt, &r.Tokens, &r.Score); err != nil {
            return nil, fmt.Errorf("scan fts row: %w", err)
        }
        r.Type = memoryTypeToEmoji(memoryType)
        r.Date = createdAt.Format("2006-01-02")
        results = append(results, r)
    }
    return results, rows.Err()
}
```

**Semantic-only search (vector similarity):**

```go
func (c *Client) searchSemantic(ctx context.Context, query string, limit int, memType string) ([]MemoryIndexRow, error) {
    embeddings, err := c.Embed(ctx, []string{query})
    if err != nil {
        return nil, fmt.Errorf("embedding failed (required for semantic mode): %w", err)
    }

    vecStr := pgVectorLiteral(embeddings[0])

    var args []any
    args = append(args, vecStr) // $1 = embedding vector

    typeClause := ""
    if memType != "" {
        typeClause = " AND memory_type = $2"
        args = append(args, memType)
    }

    limitPlaceholder := fmt.Sprintf("$%d", len(args)+1)
    args = append(args, limit)

    q := fmt.Sprintf(`
        SELECT id, memory_type, title, created_at, token_count,
               1 - (embedding <=> $1::vector) AS score
        FROM memories
        WHERE embedding IS NOT NULL
        %s
        ORDER BY embedding <=> $1::vector
        LIMIT %s
    `, typeClause, limitPlaceholder)

    rows, err := c.db.QueryContext(ctx, q, args...)
    if err != nil {
        return nil, fmt.Errorf("semantic query: %w", err)
    }
    defer rows.Close()

    var results []MemoryIndexRow
    for rows.Next() {
        var r MemoryIndexRow
        var createdAt time.Time
        var memoryType string
        if err := rows.Scan(&r.ID, &memoryType, &r.Title, &createdAt, &r.Tokens, &r.Score); err != nil {
            return nil, fmt.Errorf("scan semantic row: %w", err)
        }
        r.Type = memoryTypeToEmoji(memoryType)
        r.Date = createdAt.Format("2006-01-02")
        results = append(results, r)
    }
    return results, rows.Err()
}
```

Note: unlike `searchHybrid`, semantic mode does *not* fall back to FTS if the embedding service is down — it returns an error. If you want fallback behavior, use `hybrid` mode.

**Hybrid search (RRF, fully parameterized):**

```go
func (c *Client) searchHybrid(ctx context.Context, query string, limit int, memType string) ([]MemoryIndexRow, error) {
    embeddings, err := c.Embed(ctx, []string{query})
    if err != nil {
        return c.searchFTS(ctx, query, limit, memType)
    }

    vecStr := pgVectorLiteral(embeddings[0])

    // Build parameterized query — no string interpolation of user input
    var args []any
    args = append(args, query)  // $1 = FTS query
    args = append(args, vecStr) // $2 = embedding vector

    typeClause := ""
    if memType != "" {
        typeClause = " AND memory_type = $3"
        args = append(args, memType)
    }

    limitPlaceholder := fmt.Sprintf("$%d", len(args)+1)
    args = append(args, limit)

    // RRF constant k=60 is a literal, not user input
    q := fmt.Sprintf(`
        WITH fts AS (
            SELECT id, ROW_NUMBER() OVER (
                ORDER BY ts_rank_cd(search_vector, websearch_to_tsquery('english', $1)) DESC
            ) AS rank
            FROM memories
            WHERE search_vector @@ websearch_to_tsquery('english', $1)
            %s
            LIMIT 100
        ),
        sem AS (
            SELECT id, ROW_NUMBER() OVER (
                ORDER BY embedding <=> $2::vector
            ) AS rank
            FROM memories
            WHERE embedding IS NOT NULL
            %s
            LIMIT 100
        ),
        combined AS (
            SELECT COALESCE(f.id, s.id) AS id,
                   COALESCE(1.0 / (60 + f.rank), 0) + COALESCE(1.0 / (60 + s.rank), 0) AS rrf_score
            FROM fts f
            FULL OUTER JOIN sem s ON f.id = s.id
        )
        SELECT m.id, m.memory_type, m.title, m.created_at, m.token_count, c.rrf_score
        FROM combined c
        JOIN memories m ON m.id = c.id
        ORDER BY c.rrf_score DESC
        LIMIT %s
    `, typeClause, typeClause, limitPlaceholder)

    rows, err := c.db.QueryContext(ctx, q, args...)
    if err != nil {
        return nil, fmt.Errorf("hybrid query: %w", err)
    }
    defer rows.Close()

    var results []MemoryIndexRow
    for rows.Next() {
        var r MemoryIndexRow
        var createdAt time.Time
        var memoryType string
        if err := rows.Scan(&r.ID, &memoryType, &r.Title, &createdAt, &r.Tokens, &r.Score); err != nil {
            return nil, fmt.Errorf("scan hybrid row: %w", err)
        }
        r.Type = memoryTypeToEmoji(memoryType)
        r.Date = createdAt.Format("2006-01-02")
        results = append(results, r)
    }
    return results, rows.Err()
}
```

**Result format (what Claude sees):**

```json
{
  "matches": [
    {"id": 42, "type": "🔴", "title": "Hook timeout: 60s too short for npm install", "date": "2026-04-28", "tokens": 155, "score": 0.032},
    {"id": 38, "type": "🟤", "title": "Manager-first over agent-first for context control", "date": "2026-04-27", "tokens": 203, "score": 0.028}
  ],
  "search_mode": "hybrid",
  "instructions": "Use get_memory_details with IDs from this index to fetch full content. Types: 🎯=session-goal ..."
}
```

At ~15-20 tokens per row, 20 results = ~300-400 tokens. Compact.

#### 5.2 Layer 2 — `get_memory_details` (Details)

```go
func buildGetMemoryDetailsTool() anthropic.ToolUnionParam {
    t := anthropic.ToolUnionParamOfTool(
        anthropic.ToolInputSchemaParam{
            Type: "object",
            Properties: map[string]any{
                "ids": map[string]any{
                    "type":        "array",
                    "items":       map[string]any{"type": "integer"},
                    "description": "Memory IDs to fetch (from search_memories results)",
                },
            },
            Required: []string{"ids"},
        },
        "get_memory_details",
    )
    t.OfTool.Description = anthropic.String(
        "Fetch full content of specific memories by ID. Use after search_memories to drill into relevant results. " +
        "Returns content, metadata, and whether raw source is available for deeper inspection via get_memory_source.",
    )
    return t
}
```

**Handler:**

```go
type MemoryDetail struct {
    ID           int64  `json:"id"`
    SessionID    string `json:"session_id"`
    TurnIndex    *int   `json:"turn_index,omitempty"`
    Type         string `json:"type"`
    Title        string `json:"title"`
    Content      string `json:"content"`
    Tokens       int    `json:"tokens"`
    HasSource    bool   `json:"has_source"`
    SourceTokens *int   `json:"source_tokens,omitempty"`
    CreatedAt    string `json:"created_at"`
}

type DetailsResult struct {
    Memories     []MemoryDetail `json:"memories"`
    Instructions string         `json:"instructions"`
}

func (c *Client) GetMemoryDetails(ctx context.Context, ids []int64) (*DetailsResult, error) {
    query := `
        SELECT id, session_id, turn_index, memory_type, title, content,
               token_count, source IS NOT NULL AS has_source, source_tokens, created_at
        FROM memories
        WHERE id = ANY($1)
        ORDER BY created_at DESC
    `
    rows, err := c.db.QueryContext(ctx, query, pq.Array(ids))
    if err != nil {
        return nil, fmt.Errorf("details query: %w", err)
    }
    defer rows.Close()

    var details []MemoryDetail
    for rows.Next() {
        var d MemoryDetail
        var createdAt time.Time
        var memoryType string
        var turnIndex sql.NullInt32
        var sourceTokens sql.NullInt32
        if err := rows.Scan(&d.ID, &d.SessionID, &turnIndex, &memoryType, &d.Title, &d.Content,
            &d.Tokens, &d.HasSource, &sourceTokens, &createdAt); err != nil {
            return nil, fmt.Errorf("scan detail row: %w", err)
        }
        d.Type = memoryTypeToEmoji(memoryType)
        d.CreatedAt = createdAt.Format(time.RFC3339)
        if turnIndex.Valid {
            v := int(turnIndex.Int32)
            d.TurnIndex = &v
        }
        if sourceTokens.Valid {
            v := int(sourceTokens.Int32)
            d.SourceTokens = &v
        }
        details = append(details, d)
    }

    return &DetailsResult{
        Memories:     details,
        Instructions: "Use get_memory_source with an ID to fetch the original raw content (if has_source is true).",
    }, rows.Err()
}
```

#### 5.3 Layer 3 — `get_memory_source` (Deep Dive)

```go
func buildGetMemorySourceTool() anthropic.ToolUnionParam {
    t := anthropic.ToolUnionParamOfTool(
        anthropic.ToolInputSchemaParam{
            Type: "object",
            Properties: map[string]any{
                "id": map[string]any{
                    "type":        "integer",
                    "description": "Memory ID to fetch raw source for",
                },
            },
            Required: []string{"id"},
        },
        "get_memory_source",
    )
    t.OfTool.Description = anthropic.String(
        "Fetch the original uncompacted raw content that produced a memory. " +
        "This is the largest retrieval — only use when you need the full original context.",
    )
    return t
}
```

**Handler:**

```go
type SourceResult struct {
    ID           int64  `json:"id"`
    Title        string `json:"title"`
    Source       string `json:"source"`
    SourceTokens int    `json:"source_tokens"`
}

func (c *Client) GetMemorySource(ctx context.Context, id int64) (*SourceResult, error) {
    query := `
        SELECT id, title, source, source_tokens
        FROM memories
        WHERE id = $1 AND source IS NOT NULL
    `
    var r SourceResult
    err := c.db.QueryRowContext(ctx, query, id).Scan(&r.ID, &r.Title, &r.Source, &r.SourceTokens)
    if err == sql.ErrNoRows {
        return nil, fmt.Errorf("memory %d not found or has no source", id)
    }
    if err != nil {
        return nil, fmt.Errorf("source query: %w", err)
    }
    return &r, nil
}
```

**Lightweight size peek (used by ToolSizeEstimator before guardrail decision):**

```go
func (c *Client) PeekSourceSize(ctx context.Context, id int64) (int, error) {
    var tokens sql.NullInt32
    err := c.db.QueryRowContext(ctx,
        "SELECT source_tokens FROM memories WHERE id = $1", id,
    ).Scan(&tokens)
    if err == sql.ErrNoRows {
        return 0, fmt.Errorf("memory %d not found", id)
    }
    if err != nil {
        return 0, err
    }
    if !tokens.Valid {
        return 0, fmt.Errorf("memory %d has no source", id)
    }
    return int(tokens.Int32), nil
}
```

#### 5.4 Write — `store_memory`

```go
func buildStoreMemoryTool() anthropic.ToolUnionParam {
    t := anthropic.ToolUnionParamOfTool(
        anthropic.ToolInputSchemaParam{
            Type: "object",
            Properties: map[string]any{
                "memory_type": map[string]any{
                    "type":        "string",
                    "enum":        []string{"session-goal", "gotcha", "problem-fix", "how-it-works", "what-changed", "discovery", "why-it-exists", "decision", "trade-off"},
                    "description": "Category of this memory",
                },
                "title": map[string]any{
                    "type":        "string",
                    "description": "Compressed title (3-15 words). Must name the specific entity and finding. Bad: 'Observation about search'. Good: 'Hook timeout: 60s too short for npm install'.",
                },
                "content": map[string]any{
                    "type":        "string",
                    "description": "Full observation content",
                },
                "source": map[string]any{
                    "type":        "string",
                    "description": "Raw uncompacted original content (optional, for Layer 3 retrieval)",
                },
            },
            Required: []string{"memory_type", "title", "content"},
        },
        "store_memory",
    )
    t.OfTool.Description = anthropic.String(
        "Store an observation, decision, or finding as a persistent memory for future sessions. " +
        "Choose the most specific memory_type. Write a title that would let a future agent decide whether to fetch the full content. " +
        "Title MUST be 3-15 words and include the specific entity (function, tool, config) and the key finding.",
    )
    return t
}
```

**Handler with title validation:**

```go
type StoreMemoryInput struct {
    MemoryType string `json:"memory_type"`
    Title      string `json:"title"`
    Content    string `json:"content"`
    Source     string `json:"source,omitempty"`
}

func validateTitle(title string) error {
    words := strings.Fields(title)
    if len(words) < 3 {
        return fmt.Errorf("title too short (%d words, minimum 3): %q", len(words), title)
    }
    if len(words) > 15 {
        return fmt.Errorf("title too long (%d words, maximum 15): %q", len(words), title)
    }
    return nil
}

func (c *Client) StoreMemory(ctx context.Context, sessionID string, turnIndex int, mem StoreMemoryInput) (int64, error) {
    if err := validateTitle(mem.Title); err != nil {
        return 0, err
    }

    embedText := mem.Title + ". " + mem.Content
    embeddings, err := c.Embed(ctx, []string{embedText})
    if err != nil {
        return c.insertWithoutEmbedding(ctx, sessionID, turnIndex, mem)
    }

    tokenCount := estimateTokensFast(mem.Content)
    var sourceTokens *int
    if mem.Source != "" {
        st := estimateTokensFast(mem.Source)
        sourceTokens = &st
    }

    vecStr := pgVectorLiteral(embeddings[0])

    query := `
        INSERT INTO memories (session_id, turn_index, memory_type, title, content, source, token_count, source_tokens, embedding)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::vector)
        RETURNING id
    `
    var id int64
    err = c.db.QueryRowContext(ctx, query,
        sessionID, turnIndex, mem.MemoryType, mem.Title, mem.Content, nullString(mem.Source),
        tokenCount, sourceTokens, vecStr,
    ).Scan(&id)
    if err != nil {
        return 0, fmt.Errorf("insert memory: %w", err)
    }

    return id, nil
}

func (c *Client) insertWithoutEmbedding(ctx context.Context, sessionID string, turnIndex int, mem StoreMemoryInput) (int64, error) {
    tokenCount := estimateTokensFast(mem.Content)
    var sourceTokens *int
    if mem.Source != "" {
        st := estimateTokensFast(mem.Source)
        sourceTokens = &st
    }

    query := `
        INSERT INTO memories (session_id, turn_index, memory_type, title, content, source, token_count, source_tokens)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
        RETURNING id
    `
    var id int64
    err := c.db.QueryRowContext(ctx, query,
        sessionID, turnIndex, mem.MemoryType, mem.Title, mem.Content, nullString(mem.Source),
        tokenCount, sourceTokens,
    ).Scan(&id)
    if err != nil {
        return 0, fmt.Errorf("insert memory (no embedding): %w", err)
    }
    return id, nil
}

// estimateTokensFast uses the same chars-per-token heuristic as context-management's EstimateFast.
// This is a local copy to avoid a cross-module import — both use len(text)/4 as the baseline.
// If TokenEstimator is later extracted to a shared package, this should be replaced with a call to it.
func estimateTokensFast(text string) int {
    return len(text) / 4
}
```

**Session ID:** Generated as a UUID v4 at `ResearchLoop` start. The current turn index is provided by a callback from the agentic loop, so `turn_index` in the database matches the actual loop turn (not a per-tool call counter):

```go
func NewMemoryToolHandlers(client *memoryclient.Client, sessionID string, currentTurn func() int) map[string]ToolHandler {
    return map[string]ToolHandler{
        "store_memory": func(input []byte) (string, error) {
            var mem memoryclient.StoreMemoryInput
            if err := json.Unmarshal(input, &mem); err != nil {
                return "", err
            }
            id, err := client.StoreMemory(context.Background(), sessionID, currentTurn(), mem)
            if err != nil {
                return "", err
            }
            result, _ := json.Marshal(map[string]any{"id": id, "stored": true})
            return string(result), nil
        },
        // ... other handlers
    }
}

// Called from ResearchLoop setup:
// sessionID := uuid.New().String()
// turn := 0
// handlers := NewMemoryToolHandlers(memClient, sessionID, func() int { return turn })
// ... in loop body: turn++
```

### 6. Context Manager Integration

#### 6.1 ToolSizeEstimator Registration

These registrations go alongside the tool definitions, following the pattern from context-management.md section 7. They use only the existing `RegisterTool` interface — no new contracts.

```go
func RegisterMemoryToolEstimators(estimator *ToolSizeEstimator, memClient *memoryclient.Client) {
    estimator.RegisterTool("search_memories", func(args map[string]any) int {
        limit, _ := args["limit"].(float64)
        if limit == 0 { limit = 20 }
        // ~20 tokens per result row + ~50 tokens overhead (instructions, metadata)
        return 50 + int(limit)*20
    })

    estimator.RegisterTool("get_memory_details", func(args map[string]any) int {
        ids, _ := args["ids"].([]any)
        if len(ids) == 0 { return 200 }
        // Use average from history if available, else estimate ~200 tokens per memory
        return 50 + len(ids)*200
    })

    estimator.RegisterTool("get_memory_source", func(args map[string]any) int {
        // Deliberate deviation from the pure-function estimator pattern: this does
        // a database round-trip to get actual source size. Justified because source
        // size is unbounded (could be 20k tokens) and a fixed estimate would mislead
        // the guardrail. The query is a single-row indexed SELECT (<1ms typical).
        // A 50ms timeout prevents a slow DB from blocking the guardrail.
        id, _ := args["id"].(float64)
        if id == 0 {
            return 2000
        }
        ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
        defer cancel()
        tokens, err := memClient.PeekSourceSize(ctx, int64(id))
        if err != nil {
            return 2000
        }
        return tokens + 50
    })

    estimator.RegisterTool("store_memory", func(args map[string]any) int {
        return 50 // returns just {id, stored: true}
    })
}
```

#### 6.2 reduceToolArgs Integration

Add cases to the existing `reduceToolArgs` function from context-management.md section 5:

```go
case "search_memories":
    if v, ok := args["limit"]; ok {
        if n, ok := v.(float64); ok && n > 10 {
            reduced["limit"] = 10
            return reduced, true
        }
    }
    reduced["limit"] = 10
    return reduced, true

case "get_memory_details":
    if ids, ok := args["ids"].([]any); ok && len(ids) > 3 {
        reduced["ids"] = ids[:3]
        return reduced, true
    }
```

#### 6.3 Compaction Compatibility

Memory tool results are plain JSON strings returned as `tool_result` content. The `ContextManager` compacts them through the standard levels using the same mechanisms as any other tool result — no special handling needed:

- **Level 0 → 1 (Extractive):** `extractiveTruncate` truncates the JSON string to ~50% size. For search results, this preserves the first N rows.
- **Level 1 → 2 (Summary):** Haiku summarization. The JSON structure gives Haiku enough signal to produce "Searched memories for [query], found [count] results including [key topics]."
- **Level 2 → 3 (Tombstone):** One-line tombstone generated by `EvictTurn`.

No `_meta` field or custom compaction code is required. The existing `PostToolCheck` calls `EstimateFast(resultText, ContentJSON)` on the raw result string, which works correctly for these JSON responses.

### 7. Progressive Disclosure Flow

What the agent sees in practice:

**Step 1 — Search (Layer 1):**
```
Agent calls: search_memories(query="hook timeout configuration", mode="hybrid", limit=20)

Returns (~400 tokens):
{
  "matches": [
    {"id": 42, "type": "🔴", "title": "Hook timeout: 60s too short for npm install", "date": "2026-04-28", "tokens": 155, "score": 0.032},
    {"id": 38, "type": "🟤", "title": "Configured hook timeout to 120s after npm failures", "date": "2026-04-27", "tokens": 93, "score": 0.028},
    {"id": 15, "type": "🔵", "title": "Hook lifecycle: init → execute → cleanup with timeout", "date": "2026-04-25", "tokens": 312, "score": 0.019}
  ],
  "search_mode": "hybrid",
  "instructions": "Use get_memory_details with IDs to fetch full content..."
}
```

**Step 2 — Details (Layer 2):**
```
Agent decides: IDs 42 and 38 look relevant (🔴 gotcha + 🟤 decision)

Agent calls: get_memory_details(ids=[42, 38])

Returns (~250 tokens):
{
  "memories": [
    {
      "id": 42,
      "type": "🔴",
      "title": "Hook timeout: 60s too short for npm install",
      "content": "Default 60-second hook timeout insufficient for npm install with large dependency trees or slow network. Causes silent failure of SessionStart hook, preventing context injection.",
      "tokens": 155,
      "has_source": true,
      "source_tokens": 890,
      "created_at": "2026-04-28T14:22:00Z"
    },
    ...
  ],
  "instructions": "Use get_memory_source with an ID for raw original content..."
}
```

**Step 3 — Source (Layer 3, only if needed):**
```
Agent decides: Need the full debugging session that found this

Agent calls: get_memory_source(id=42)

ToolSizeEstimator peeks: PeekSourceSize(42) → 890 tokens
PreToolGuardrail: 890 + 50 overhead = 940 estimated, approved

Returns (~890 tokens):
{
  "id": 42,
  "title": "Hook timeout: 60s too short for npm install",
  "source": "... full raw turn content from the original agentic loop ...",
  "source_tokens": 890
}
```

**Total budget spent:** 400 + 250 + 890 = 1,540 tokens (all relevant)
**Without progressive disclosure:** Would need to load all 3 memories at full source = ~2,400 tokens (33% wasted on irrelevant ID 15)

---

## Contracts

### Public API — Go Client

```go
type Client struct { /* unexported */ }

func New(cfg Config) (*Client, error)  // bootstraps DB + migrations, then returns connected client
func (c *Client) Close() error

// Bootstrap
func EnsureDatabase(adminDSN, memoriesDSN string) (*sql.DB, error)
func RunMigrations(db *sql.DB) error

// Embedding
func (c *Client) Embed(ctx context.Context, texts []string) ([][]float32, error)

// Read (progressive disclosure)
func (c *Client) SearchMemories(ctx context.Context, query string, mode string, limit int, memType string) (*SearchResult, error)
func (c *Client) GetMemoryDetails(ctx context.Context, ids []int64) (*DetailsResult, error)
func (c *Client) GetMemorySource(ctx context.Context, id int64) (*SourceResult, error)
func (c *Client) PeekSourceSize(ctx context.Context, id int64) (int, error)

// Write
func (c *Client) StoreMemory(ctx context.Context, sessionID string, turnIndex int, mem StoreMemoryInput) (int64, error)
```

### Public API — Embedding Service

```
POST /embed
  Request:  { "texts": ["string", ...] }
  Constraints: max 100 texts, each truncated at 10,000 chars
  Response: { "embeddings": [[float, ...], ...], "dimensions": 384 }
  Errors:   422 (validation — empty texts, >100 texts), 500 (model error)

GET /health
  Response: { "status": "ok", "model": "all-MiniLM-L6-v2", "dimensions": 384 }
```

### Input/Output Contracts

| Tool | Input | Output | Error |
|------|-------|--------|-------|
| `search_memories` | query (string, required), mode (string), limit (int), memory_type (string) | `SearchResult` JSON | Empty matches if no results; FTS fallback if embedding service down |
| `get_memory_details` | ids (int[], required) | `DetailsResult` JSON | Skips IDs that don't exist (no error) |
| `get_memory_source` | id (int, required) | `SourceResult` JSON | Error if ID not found or source is NULL |
| `store_memory` | memory_type, title, content (required), source (optional) | `{"id": N, "stored": true}` | Stores without embedding if embed service down; rejects titles <3 or >15 words |

### Invariants

1. Every memory has a `title`, `content`, `memory_type`, and `token_count` — never NULL
2. `embedding` may be NULL (if embedding service was down at write time) — FTS still works
3. `source` may be NULL — `has_source` in Layer 2 is always accurate
4. `token_count` is estimated at write time via `len(text)/4` and immutable
5. Layer 1 results never exceed `limit` rows
6. The embedding service is a graceful dependency — all write and read paths degrade to FTS-only if it's unavailable
7. The `memories` database is fully isolated from Langfuse's `langfuse` database — Langfuse migrations cannot affect memory data
8. `session_id` is a UUID v4 generated once per `ResearchLoop` invocation
9. Titles are validated: 3-15 words, enforced at the Go handler level before database insert
10. HNSW index works correctly from row 0 — no minimum data requirement

### Dependencies

**This plan requires from context-management.md:**
- `ToolSizeEstimator.RegisterTool()` for all 4 memory tools
- `reduceToolArgs()` extended with `search_memories` and `get_memory_details` cases
- Standard `PostToolCheck()` with `EstimateFast(resultText, ContentJSON)` — no modifications needed
- Standard `autoCompact()` compaction levels — no modifications needed

**This plan provides to the agentic loop:**
- 4 tool definitions (`buildSearchMemoriesTool`, `buildGetMemoryDetailsTool`, `buildGetMemorySourceTool`, `buildStoreMemoryTool`)
- 4 tool handlers via `NewMemoryToolHandlers()`
- `memoryclient.Client` for database + embedding access
- `RegisterMemoryToolEstimators()` to wire up size estimation
- `RunMigrations()` for schema setup

---

## Decisions & Tradeoffs

| # | Decision | Why | Alternative considered |
|---|----------|-----|----------------------|
| 1 | **Postgres over SQLite** | pg_vector is mature, runs in existing container, scales to 100k+ memories | SQLite + sqlite-vss: simpler but vss is experimental, no concurrent writes |
| 2 | **Separate `memories` database** | Isolates from Langfuse's Prisma migrations; survives Langfuse upgrades; clean `pg_dump` | Same database: simpler but Prisma may complain about unknown tables, `pg_dump`/`pg_restore` risks |
| 3 | **Separate embedding service** | Decouples Python ML dependencies from Go agent; can scale/restart independently | Embed in Go (no good MiniLM bindings), or call OpenAI API (costs money, external dependency) |
| 4 | **all-MiniLM-L6-v2** | 384 dims is compact, fast on CPU, good enough for retrieval | Larger models (768/1024 dims) are better but 2-4x slower and need more storage per row |
| 5 | **RRF over weighted sum** | RRF is rank-based, doesn't need score normalization between FTS and vector | Weighted sum requires calibrating ts_rank vs cosine — fragile |
| 6 | **HNSW over ivfflat** | Works correctly from row 0 (no training data needed); at <100k rows memory overhead is negligible (~50MB) | ivfflat: cheaper memory but requires training data, garbage recall on small tables |
| 7 | **Graceful embedding degradation** | Embedding service down shouldn't block memory writes or searches | Require embedding service healthy — too fragile for a dev tool |
| 8 | **Store token counts at write time** | Avoids re-estimating on every search; immutable, cacheable | Compute on read: adds ~1ms per row, drifts if estimator changes |
| 9 | **pgvector/pgvector:pg15 image** | Drop-in replacement for postgres:15, Langfuse unaffected | Install extension manually: more Dockerfile complexity |
| 10 | **golang-migrate for schema** | Runs at Go startup, works on existing volumes, supports up/down migrations | `docker-entrypoint-initdb.d`: only runs on empty volumes — breaks on existing setups |
| 11 | **Title validation (3-15 words)** | Progressive disclosure depends on title quality; validation catches the worst cases | Trust Claude to write good titles — it won't consistently |
| 12 | **PeekSourceSize for Layer 3 estimation** | Source size is unbounded; fixed estimate (2000) would mislead the guardrail | Fixed estimate: simple but allows 20k-token results to slip past the guardrail |
| 13 | **No `_meta` field** | Context-management's `PostToolCheck` uses `EstimateFast` on raw text; adding a new contract would require changes to that plan | Custom `_meta` field: more accurate but invents a contract the other plan doesn't support |
| 14 | **`estimateTokensFast` as local function** | Avoids cross-module import for a one-liner; documented as replaceable when TokenEstimator is extracted to shared package | Import from context-management module: cleaner but modules aren't structured for sharing yet |

---

## Implementation Order

1. **Docker infra** — Swap Postgres image to `pgvector/pgvector:pg15`, add `embedding-api` service. Verify `docker compose up` starts cleanly and Langfuse still works on port 3000.
2. **Embedding service** — `services/embedding-api/` with FastAPI, Dockerfile, health check, input validation. Verify `/embed` and `/health` endpoints. Write `test_main.py`.
3. **Go module scaffold** — `examples/03-memory-store/` with `go.mod`, dependencies (`lib/pq`, `golang-migrate`, `anthropic-sdk-go`).
4. **Migrations** — `db/migrations/000001_create_memories.up.sql` with HNSW index. `memoryclient/migrate.go` with embedded SQL. Verify migrations run idempotently.
5. **Memory client core** — `memoryclient/client.go` (New, Close, Embed), `memoryclient/store.go` (StoreMemory with title validation, insertWithoutEmbedding). Unit tests with test Postgres.
6. **Search** — `memoryclient/search.go` (searchFTS, searchSemantic, searchHybrid with parameterized queries). `memoryclient/details.go` (GetMemoryDetails, GetMemorySource, PeekSourceSize). Unit tests.
7. **Tool definitions** — `tools/definitions.go` (4 build functions), `tools/handlers.go` (NewMemoryToolHandlers with session UUID), `tools/estimators.go` (RegisterMemoryToolEstimators with PeekSourceSize).
8. **Demo main.go** — store 10 sample memories, run search + progressive disclosure flow, print results.
9. **Integration test** — end-to-end: Docker Compose up → store memories → hybrid search → get details → get source → verify progressive disclosure token budget claims.

---

## Resolved Critique Items

### Round 1

| # | Issue | Resolution |
|---|-------|------------|
| 1 | Init script only runs on empty volumes | `golang-migrate` runs at Go startup, idempotent, works on existing volumes (§1) |
| 2 | Sharing Langfuse's database risks Prisma conflicts | Separate `memories` database in same container (§1, §3) |
| 3 | ivfflat with empty table has garbage recall | Switched to HNSW — no training data needed, works from row 0 (§1) |
| 4 | `_meta` field is a phantom contract | Removed. Uses standard `EstimateFast(resultText, ContentJSON)` (§6.3) |
| 5 | SQL injection via `fmt.Sprintf` of `typeFilter` | All queries fully parameterized with `$N` placeholders (§5.1) |
| 6 | Embedding service has no input validation | Added `MAX_TEXTS=100`, `MAX_CHARS_PER_TEXT=10000` with truncation (§2) |
| 7 | `get_memory_source` fixed 2000 estimate is wrong | `PeekSourceSize` does a lightweight SELECT of `source_tokens` before guardrail decision (§5.3, §6.1) |
| 8 | `session_id` origin undefined | UUID v4 generated at `ResearchLoop` start, passed via handler closure (§5.4) |
| 9 | `count(*)` subquery doubles FTS cost | Removed `total_results` field — agent doesn't need it for progressive disclosure decisions (§5.1) |
| 10 | Go module structure undefined | `examples/03-memory-store/` with its own `go.mod`, matching project convention (§4) |
| 11 | Claude writes bad titles | Title validation: 3-15 words enforced at handler level; tool description includes good/bad examples (§5.4) |
| 12 | `estimateTokens` function undefined | `estimateTokensFast` defined locally as `len(text)/4`, documented as replaceable (§5.4) |

### Round 2

| # | Issue | Resolution |
|---|-------|------------|
| 13 | `create-databases.sh` has same first-init-only problem | Removed init script. Go binary self-bootstraps via `EnsureDatabase()` — connects to default DB, creates `memories` if needed, reconnects (§1, §4) |
| 14 | `PeekSourceSize` does I/O in estimator hot path | Added 50ms timeout with 2000-token fallback. Documented as deliberate deviation from pure-function pattern (§6.1) |
| 15 | `turnCounter` doesn't match actual loop turn index | Replaced with `currentTurn func() int` callback from the agentic loop (§5.4) |
| 16 | `searchSemantic` referenced but never defined | Full implementation added — vector-only search, no FTS fallback, errors if embedding service is down (§5.1) |
| 17 | Healthcheck uses `curl` but `python:3.12-slim` lacks it | Switched to `python -c "import urllib.request; ..."` — zero-dependency (§3) |
| 18 | `json.Marshal` error swallowed in `Embed` | Error handled properly (§4) |

---

## Open Questions

1. **Embedding model upgrade path** — all-MiniLM-L6-v2 is good enough to start. If we later swap to a larger model (different dimensions), existing embeddings become incompatible. Options: (a) re-embed everything (simple batch job), (b) store model name per row and query only matching embeddings. Defer until it matters.

2. **Memory retention policy** — should old memories be pruned? Archived? The plan has no TTL or cleanup mechanism. For a dev tool this is probably fine — Postgres can handle millions of rows. Revisit if storage becomes an issue.

3. **Bulk import from prior sessions** — if we want to backfill memories from existing research outputs, we need a one-time migration script. Not designed here but straightforward (read old outputs, call `store_memory` in a loop).
