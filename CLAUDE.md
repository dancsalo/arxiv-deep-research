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
- `src/tools/` — reusable tool modules (e.g. `arxiv_search.py`)
- `src/` — package root
