.PHONY: build vet lint test test-go test-race test-python check run-server

PYTHON_TEST_DIRS := services/embedding-api

build:
	go build ./...

vet:
	go vet ./...

lint: vet

test-go:
	go test ./...

test-race:
	go test -race ./... -count=1

test-python:
	@if ! python -m pytest --version >/dev/null 2>&1; then \
		echo "pytest not found — install with: pip install -r services/embedding-api/requirements.txt pytest"; \
		exit 1; \
	fi
	@for d in $(PYTHON_TEST_DIRS); do echo "==> pytest $$d"; (cd $$d && python -m pytest -v); done

test: test-go
	@if python -m pytest --version >/dev/null 2>&1; then \
		$(MAKE) test-python; \
	else \
		echo "==> skipping Python tests (pytest not installed)"; \
	fi

check: lint build test

run-server:
	go run ./cmd/server/
