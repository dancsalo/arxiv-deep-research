# arxiv-deep-research

A deep research tool that searches arXiv for academic papers, critiques them using Claude, performs follow-on web searches, and produces a comprehensive research synthesis.

## Setup

```bash
pip install -e .
```

## Running the server

```bash
make run-server
```

Then open http://localhost:8080 in your browser.

**Flags** (pass via `go run ./cmd/server/`):
- `--addr :9090` — change listen port
- `--bedrock=false` — use direct Anthropic API instead of AWS Bedrock
- `--model <id>` — override the model ID

By default the server uses AWS Bedrock (requires AWS credentials). To use the Anthropic API directly:

```bash
export ANTHROPIC_API_KEY="sk-ant-..."
go run ./cmd/server/ --bedrock=false
```

## Langfuse Observability

The example script logs traces to [Langfuse](https://langfuse.com) for observability into each pipeline step: arXiv search, Claude critique, web searches, page fetches, and final synthesis.

### Run Langfuse locally with Docker

```bash
docker compose -f docker-compose.langfuse.yml up -d
```

Open http://localhost:3000 in your browser. On first launch:

1. Create an account (local only, no email verification needed)
2. Create a new project
3. Go to **Settings > API Keys** and copy the **Secret Key** and **Public Key**

### Configure environment variables

```bash
export LANGFUSE_SECRET_KEY="sk-lf-..."
export LANGFUSE_PUBLIC_KEY="pk-lf-..."
export LANGFUSE_HOST="http://localhost:3000"
```

## Testing

### Run all tests

```bash
make test          # runs Go + Python tests
make check         # runs lint + build + all tests
make test-race     # all Go tests with race detector
```

### Go tests

```bash
make test-go       # all Go packages
make test-race     # all Go packages with -race flag
```

Go tests live alongside source files in the same package (standard Go convention). Packages:

| Package | Tests | What they cover |
|---------|-------|-----------------|
| `contextmanager` | 100+ | Context manager, budget, compaction, estimator, guardrails, loop, turns, agentic loop (memory recall, hooks, cost/turn limits, tool execution), tool registry, toolset interface, recall logic |
| `memoryclient` | 27 | Title validation, token estimation, emoji mapping, embed HTTP client, vector literal formatting, null string handling, RecallerAdapter |
| `server` | 30+ | SSE handler (event format, level filtering, groups, attrs, concurrency), HTTP server (routing, single-flight, streaming, error handling) |
| `tools` | 15 | Tool schema definitions, size estimator math, arg reduction logic |

### Python tests

```bash
make test-python   # embedding service tests
```

Requires dependencies installed (`pip install -r services/embedding-api/requirements.txt`). The first run downloads the `all-MiniLM-L6-v2` model (~80MB).

| Service | Tests | What they cover |
|---------|-------|-----------------|
| `embedding-api` | 11 | Single/batch embedding, normalization, input validation (empty, >100, truncation), health endpoint, determinism |

### Integration tests (future)

Integration tests requiring Docker (Postgres + pgvector + embedding service) will use the `//go:build integration` build tag:

```bash
docker compose -f docker-compose.langfuse.yml up -d langfuse-db embedding-api
go test ./... -tags integration -count=1
```

### Tear down

```bash
docker compose -f docker-compose.langfuse.yml down     # stop containers, keep data
docker compose -f docker-compose.langfuse.yml down -v   # stop containers and delete data
```
