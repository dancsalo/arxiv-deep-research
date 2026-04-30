.PHONY: build vet lint test check

GO_EXAMPLES := $(wildcard examples/*/main.go)
GO_DIRS := $(dir $(GO_EXAMPLES))

build:
	@for d in $(GO_DIRS); do echo "==> build $$d"; (cd $$d && go build ./...); done

vet:
	@for d in $(GO_DIRS); do echo "==> vet $$d"; (cd $$d && go vet ./...); done

lint: vet

test:
	@for d in $(GO_DIRS); do echo "==> test $$d"; (cd $$d && go test ./...); done

check: lint build test
