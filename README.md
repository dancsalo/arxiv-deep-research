## Prerequisites

- **Go** (1.21+)
- **Docker & Docker Compose** — the server depends on PostgreSQL (with pgvector) and the embedding API, both of which run as Docker containers
- **Python 3.10+** (optional, for running Python tests or the embedding service locally)

## Running the server

Start the infrastructure (Postgres + embedding API):

```bash
docker compose -f docker-compose.langfuse.yml up -d langfuse-db embedding-api
```

Then start the Go server:

```bash
make run-server
```

Open http://localhost:8080 in your browser.

**Flags** (pass via `go run ./cmd/server/`):
- `--addr :9090` — change listen port
- `--bedrock=false` — use direct Anthropic API instead of AWS Bedrock
- `--model <id>` — override the model ID

By default the server uses AWS Bedrock (requires AWS credentials). To use the Anthropic API directly:

```bash
export ANTHROPIC_API_KEY="sk-ant-..."
go run ./cmd/server/ --bedrock=false
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

### Python tests

```bash
make test-python   # embedding service tests
```

Requires dependencies installed (`pip install -r services/embedding-api/requirements.txt`).

## Tear down

```bash
docker compose -f docker-compose.langfuse.yml down     # stop containers, keep data
docker compose -f docker-compose.langfuse.yml down -v   # stop containers and delete data
```
