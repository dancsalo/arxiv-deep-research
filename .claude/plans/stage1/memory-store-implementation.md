# Implementation Summary: Memory Store

Source plan: `.claude/plans/memory-store.md`
Test plan: `.claude/plans/memory-store-tests.md`

**PR:** [#13](https://github.com/dancsalo/arxiv-deep-research/pull/13) on branch `feat/memory-store`
**Date:** 2026-04-30
**Status:** Complete — all `make check` passing (vet + build + test)

---

## What was built

| Section | Files | Status |
|---------|-------|--------|
| Docker infra | `docker-compose.langfuse.yml` | Done — pgvector image swap, embedding-api service |
| Embedding service | `services/embedding-api/{main.py,Dockerfile,requirements.txt,test_main.py,.dockerignore}` | Done — FastAPI with model pre-download |
| Go module scaffold | `src/{go.mod,go.sum,main.go}`, `Makefile` | Done — `src/` module, Makefile integration |
| Migrations | `src/memoryclient/{bootstrap.go,migrate.go,migrations/*.sql}` | Done — HNSW + GIN indexes, self-bootstrapping |
| Memory client core | `src/memoryclient/{client.go,store.go,client_test.go,store_test.go}` | Done — 27 unit tests passing |
| Search + details | `src/memoryclient/{search.go,details.go,search_test.go}` | Done — FTS, semantic, hybrid RRF |
| Tool layer | `src/tools/{definitions.go,handlers.go,estimators.go,definitions_test.go,estimators_test.go}` | Done — 4 tools, handlers, estimators |

**30 files changed, ~1900 lines added.**

---

## Deviations from plan

| Deviation | Reason |
|-----------|--------|
| Code in `src/` instead of `examples/03-memory-store/` | User direction — this is library code for the agentic loop, not a standalone example |
| Migrations in `src/memoryclient/migrations/` instead of `db/migrations/` | Go `embed` directive cannot use `../` paths; migrations must be inside the package |
| No demo `main.go` (plan step 8) | Dropped — no standalone example needed for library code |
| No integration tests (plan step 9) | Deferred — requires Docker Compose running; unit tests cover all pure logic |
| Added `Degraded` field to `SearchResult` | Critique feedback — hybrid fallback to FTS should be visible to the caller |
| Added `validMemoryTypes` set + validation | Critique feedback — reject invalid memory types at the boundary |
| Limit capped at 100 in `SearchMemories` | Critique feedback — prevent unbounded queries |
| `estimateTokensFast` returns min 1 for non-empty | Critique feedback — zero token count for short strings was semantically wrong |
| HTTP client timeout (10s) on embedding requests | Critique feedback — prevent hangs if embedding service is unresponsive |
| TOCTOU race handling in `EnsureDatabase` | Critique feedback — concurrent startup could fail on `CREATE DATABASE` |
| Embedding-api bound to `127.0.0.1` | Critique feedback — internal-only service shouldn't be externally accessible |
| Defensive slice copy in `ReduceToolArgs` | Critique feedback — prevent aliasing between original and reduced args |

---

## Known gaps (deferred)

| Gap | Why deferred |
|-----|-------------|
| Handler `context.Context` propagation | Depends on agentic loop `ToolHandler` signature — not yet defined |
| Connection pool configuration (`SetMaxOpenConns`, etc.) | Depends on usage patterns under load — premature to tune now |
| Integration tests against real Postgres | Requires Docker infra running; will add with `//go:build integration` tag |
| Silent embedding failure logging in `StoreMemory` | Needs structured logging setup — adding `charmbracelet/log` is a separate concern |
| `pgVectorLiteral` precision (`%f` vs `%g`) | Works correctly for cosine similarity at 6 decimal places; optimize later if needed |
