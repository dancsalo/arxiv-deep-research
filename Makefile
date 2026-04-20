.PHONY: install test lint typecheck check

install:
	uv sync --all-extras

test:
	uv run pytest

lint:
	uv run ruff check .

typecheck:
	uv run pyright

check: lint typecheck test
