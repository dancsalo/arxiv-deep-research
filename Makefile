.PHONY: build build-research build-tools build-viewer vet lint test test-go test-race check run trace-list trace-show trace-view trace-errors help

help: ## Show this help
	@echo "arXiv Deep Research - Research Agent"
	@echo ""
	@echo "Main commands:"
	@echo "  make run              Run research-demo with prompt variant A"
	@echo "  make build            Build all binaries"
	@echo "  make check            Run all checks (lint, build, test)"
	@echo ""
	@echo "Build commands:"
	@echo "  make build-research   Build research-demo binary"
	@echo "  make build-tools      Build tools-cli binary"
	@echo "  make build-viewer     Build trace-viewer binary"
	@echo ""
	@echo "Test commands:"
	@echo "  make test             Run all tests"
	@echo "  make test-race        Run tests with race detector"
	@echo ""
	@echo "Trace commands:"
	@echo "  make trace-list       List recent trace files"
	@echo "  make trace-show       Show trace: make trace-show RUN=<session_id>"
	@echo "  make trace-view       View most recent trace in browser"
	@echo "  make trace-errors     Show failed traces"

build: build-research build-tools ## Build all binaries

build-research: ## Build research-demo binary
	go build -o research-demo ./cmd/research-demo

build-tools: ## Build tools-cli binary
	go build -o tools-cli ./cmd/tools-cli

build-viewer: ## Build trace-viewer binary
	go build -o trace-viewer ./cmd/trace-viewer

vet:
	go vet ./...

lint: vet

test-go:
	go test ./...

test-race:
	go test -race ./... -count=1

test: test-go

check: lint build test

run: build-research ## Run research-demo with default query
	@echo "Running research-demo (variant A)..."
	@./research-demo --query "transformer attention mechanisms" --prompt-variant A --max-turns 12

TRACE_DIR ?= .traces

trace-list: ## List recent traces
	@find $(TRACE_DIR) -name '*.json' -print0 2>/dev/null | xargs -0 ls -t 2>/dev/null | head -20 | xargs -I{} jq -r '[.session_id, .status, .query[0:40], "\(.total_cost_usd)"] | join("\t")' {} || echo "No traces in $(TRACE_DIR)/"

trace-show: ## Show a trace: make trace-show RUN=<session_id>
	@jq . $(TRACE_DIR)/$(RUN).json

trace-view: build-viewer ## View most recent trace in browser
	@LATEST=$$(ls -t $(TRACE_DIR)/*.json 2>/dev/null | head -1); \
	if [ -z "$$LATEST" ]; then \
		echo "No traces found in $(TRACE_DIR)/"; \
		exit 1; \
	fi; \
	echo "Opening $$LATEST in browser..."; \
	./trace-viewer --open "$$LATEST"

trace-errors: ## Show failed traces
	@find $(TRACE_DIR) -name '*.json' -print0 2>/dev/null | xargs -0 jq -r 'select(.status=="error") | [.session_id, .error[0:60]] | join("\t")' 2>/dev/null || echo "No errors"
