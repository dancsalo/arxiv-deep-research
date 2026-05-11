# CLAUDE.md

## Project Overview

**arXiv Deep Research** is an AI research agent that autonomously searches academic literature, analyzes papers, and synthesizes findings. The core application is `research-demo`, which implements an agentic loop with context management, tool use, and tracing.

## Language & tooling

This project uses **Go** as a single module (`github.com/dancsalo/arxiv-deep-research`).

- Dependencies are managed via the root `go.mod`
- Do NOT use a global `GOPATH` workspace
- Main entry point: `cmd/research-demo/main.go`

## Common Commands

| Task | Command |
|------|---------|
| Run research agent | `make run` |
| Build all | `make build` |
| Build research-demo | `make build-research` |
| Build tools-cli | `make build-tools` |
| Lint (vet) | `make lint` |
| Run all tests | `make test` |
| Race detector | `make test-race` |
| All checks | `make check` |
| List traces | `make trace-list` |
| Show trace | `make trace-show RUN=<id>` |
| Show help | `make help` |

Always use `make` targets rather than running `go` commands directly.

## Project structure

- `cmd/research-demo/` — **main application**: agentic research loop with prompt variants
- `cmd/tools-cli/` — CLI for testing individual research tools
- `internal/ctxmgr/` — context management (turns, budget, compaction, estimator, guardrails)
- `internal/agentic/` — agentic loop implementation
- `internal/registry/` — tool registry and toolsets
- `internal/tracing/` — tracing and instrumentation
- `tools/research/` — research tool implementations (arXiv, OpenAlex, GitHub, web search)

## Development Workflow

This project uses a two-phase workflow: **planning** then **coding**. Each phase has its own pipeline.

- **Phase 1: Planning** — Use `/plan-all` skill to run the full planning pipeline autonomously (plan-begin → dual critique → revise × 2 → plan-tests → plan-finish)
- **Phase 2: Coding** — Use `/code-all` skill to run the full coding pipeline autonomously (code-checklist → implement/critique/revise/verify/checkpoint per section → code-finish)

See the individual skill documentation for detailed workflows:
- `/plan-all` — Phase 1 planning workflow
- `/code-all` — Phase 2 coding workflow
