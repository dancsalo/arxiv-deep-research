.PHONY: build vet lint test test-go test-race test-python check run-server trace-list trace-show trace-errors

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

TRACE_DIR ?= .traces

trace-list: ## List recent traces
	@find $(TRACE_DIR) -name '*.json' -print0 2>/dev/null | xargs -0 ls -t 2>/dev/null | head -20 | xargs -I{} jq -r '[.session_id, .status, .query[0:40], "\(.total_cost_usd)"] | join("\t")' {} || echo "No traces in $(TRACE_DIR)/"

trace-show: ## Show a trace: make trace-show RUN=<session_id>
	@jq . $(TRACE_DIR)/$(RUN).json

trace-errors: ## Show failed traces
	@find $(TRACE_DIR) -name '*.json' -print0 2>/dev/null | xargs -0 jq -r 'select(.status=="error") | [.session_id, .error[0:60]] | join("\t")' 2>/dev/null || echo "No errors"
