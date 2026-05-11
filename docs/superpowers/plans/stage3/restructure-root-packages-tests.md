# Test Plan: Restructure Root Go Files into Internal Packages

Companion to: `restructure-root-packages.md`

## Strategy

This is a pure structural refactor — no behavioral changes. The test strategy is:
1. All existing tests must pass in their new locations
2. No new tests needed (behavior is unchanged)
3. Verification is primarily compilation + existing test suite

## Pre-Move Verification

| Check | Command | Expected |
|-------|---------|----------|
| Accessor methods compile | `make build` | Clean build |
| Accessor usage in agentic.go compiles | `make build` | Clean build |
| All existing tests pass | `make test` | All pass |
| No race conditions | `make test-race` | All pass |

## Post-Move Verification

### 1. Package compilation
| Package | Command | Verifies |
|---------|---------|----------|
| `internal/ctxmgr` | `go build ./internal/ctxmgr/` | Package declarations, imports, no cycles |
| `internal/registry` | `go build ./internal/registry/` | Package declarations, imports |
| `internal/agentic` | `go build ./internal/agentic/` | Cross-package references qualified correctly |
| Root (empty) | `go build ./...` | No leftover root .go files causing conflicts |

### 2. Test suite by package
| Package | Command | Test count (approx) |
|---------|---------|---------------------|
| `internal/ctxmgr` | `go test ./internal/ctxmgr/` | ~50 tests (manager, turn, budget, estimator, compaction, guardrail, toolsize + 4 integration) |
| `internal/registry` | `go test ./internal/registry/` | ~10 tests (registry, toolset) |
| `internal/agentic` | `go test ./internal/agentic/` | ~30 tests (agentic, types, loop, recall + 1 integration) |
| `cmd/server` | `go build ./cmd/server/` | Compilation only (binary) |
| `server` | `go test ./server/` | Existing server tests with updated imports |

### 3. Full suite
| Check | Command | Expected |
|-------|---------|----------|
| All tests | `make test` | All pass, same count as before |
| Lint/vet | `make lint` | Clean |
| Race detector | `make test-race` | No races |
| Full checks | `make check` | All pass |
| Server starts | `make run-server` | Starts without error (manual check) |

### 4. Key things to watch for
- **Unexported field access in tests**: Tests that access `estimator.ratios`, `loop.finished`, `loop.seenMemoryIDs` etc. must be in the same package (not `_test` suffix package)
- **`minimalToolDef` duplication**: After splitting, verify `agentic` tests define their own copy and it matches the original
- **`bgctx()` availability**: Defined in `loop_test.go` → `agentic/loop_test.go`. Used in `agentic_test.go` → `agentic/agentic_test.go`. Same package, no issue.
- **`scriptedMessageClient` / `makeTextResponse`**: Defined in `loop_test.go`, used in `agentic_test.go` and `integration_test.go`. All move to `agentic/`, so no issue.
- **`mockCompactionClient` / `newMockTextResponse`**: Defined in `compaction_test.go` → `ctxmgr/`. Used in `ctxmgr/integration_test.go`. Same package, no issue.

### 5. Import path verification
After restructuring, grep for any remaining references to the old root import:
```bash
grep -rn '"github.com/dancsalo/arxiv-deep-research"' --include='*.go' .
```
Expected: zero results (no files should import the root package anymore).
