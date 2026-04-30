# CLAUDE.md

## Package management

This project uses **uv** for dependency and virtualenv management. All Python commands should be run via `uv run`.

- Install/sync dependencies: `make install` (runs `uv sync --all-extras`)
- Do NOT use `pip install` directly

## Testing

- Run tests: `make test`
- Do NOT run `uv run pytest` or `pytest` directly; always use the Makefile target
- Test framework: **pytest** with **pytest-asyncio** for async tests
- Tests live in `tests/`

## Verification commands

| Task | Command |
|------|---------|
| Install deps | `make install` |
| Run tests | `make test` |
| Lint | `make lint` |
| Type check | `make typecheck` |
| All checks | `make check` |

Always use `make` targets rather than running the underlying tools directly.

## Project structure

- `example.py` — main research workflow script
- `src/` — package root
  - `tools/` — reusable tool modules (e.g. `arxiv_search.py`)
  - `mcp_servers/` — MCP server implementations
  - `skills/` — skill modules
- `examples/` — standalone examples (Go-based tool use demos)
- `data/` — local data (SQLite research memory DB)
- `tests/` — pytest test suite
- `.claude/plans/` — implementation plans (markdown)
- `.claude/skills/` — Claude Code skill definitions

## Workflow

This project follows a plan → test → implement → critique → finish workflow:

1. **Plan** — write an implementation plan before coding. Plans live in `.claude/plans/`.
2. **Test** — write TDD tests from the plan before implementation.
3. **Implement** — build against the failing tests.
4. **Critique** — review the code or plan for structural issues, error handling, and second-order effects.
5. **Checkpoint** — commit locally at meaningful progress points. Do not push.
6. **Finish** — run checks, push to a feature branch, and open a PR.
