.PHONY: build vet lint test test-go test-python check

GO_EXAMPLES := $(wildcard examples/*/main.go)
GO_SRC := $(wildcard src/go.mod)
GO_DIRS := $(dir $(GO_EXAMPLES)) $(dir $(GO_SRC))

PYTHON_TEST_DIRS := services/embedding-api

build:
	@for d in $(GO_DIRS); do echo "==> build $$d"; (cd $$d && go build ./...); done

vet:
	@for d in $(GO_DIRS); do echo "==> vet $$d"; (cd $$d && go vet ./...); done

lint: vet

test-go:
	@for d in $(GO_DIRS); do echo "==> test $$d"; (cd $$d && go test ./...); done

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
