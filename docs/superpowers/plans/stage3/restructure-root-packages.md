# Restructure Root Go Files into Internal Packages

## Context

All 13 source files and 15 test files at the repo root are in `package contextmanager`. This is not idiomatic Go — the root package has become a monolith covering agentic loops, context management, tool registry, compaction, budgeting, guardrails, and estimation. There are no package boundaries, no encapsulation, and the flat namespace makes the codebase hard to navigate.

## Requirements

1. Split root `.go` files into domain-oriented `internal/` packages
2. Existing sub-packages (`cmd/server/`, `server/`, `tools/`, `memoryclient/`) remain unchanged
3. Update consumers (`cmd/server/main.go`, `server/server.go`, `server/server_test.go`) to import from new packages directly
4. All tests pass after restructuring
5. No behavioral changes — pure structural refactor

## Package Layout

```
internal/
  ctxmgr/          # Context management core
    manager.go      <- root manager.go
    turn.go         <- root turn.go
    budget.go       <- root budget.go
    estimator.go    <- root estimator.go
    compaction.go   <- root compaction.go
    guardrail.go    <- root guardrail.go
    toolsize.go     <- root toolsize.go
    (+ all corresponding _test.go files, package ctxmgr)
    integration_test.go  <- 4 of 5 integration tests

  registry/         # Tool registry and toolsets
    registry.go     <- root registry.go
    toolset.go      <- root toolset.go
    (+ corresponding _test.go files, package registry)

  agentic/          # Agentic loop and research loop
    agentic.go      <- root agentic.go
    types.go        <- root agentic_types.go (renamed)
    loop.go         <- root loop.go
    recall.go       <- root recall.go
    (+ corresponding _test.go files, package agentic)
    helpers_test.go <- root agentic_test_helpers_test.go (renamed)
    integration_test.go  <- TestIntegrationFullHappyPath
```

No root `.go` files remain. No facade — consumers update their imports directly.

## Why This Grouping

### `internal/ctxmgr`
- `ContextManager` is the central type; `Turn`, `MessageLog`, `ContextBudget`, `OutputTracker`, `TokenEstimator`, `ToolSizeEstimator` are all fields or collaborators of it
- Guardrail methods (`PreToolGuardrail`, `PostToolCheck`, `PreAPICheck`) are methods on `*ContextManager` — cannot separate without refactoring the type itself
- Compaction methods are also on `*ContextManager`
- All tightly coupled, minimal external surface

### `internal/registry`
- `ToolRegistry`, `ToolHandler`, `RegisteredTool`, `ToolSet` form a self-contained unit
- No dependency on `ctxmgr` types
- Clean, leaf-level package

### `internal/agentic`
- `AgenticLoop` orchestrates `*ctxmgr.ContextManager` + `*registry.ToolRegistry`
- `ResearchLoop` is a standalone function that uses `ContextManager`
- `recall.go` helpers are only used by `AgenticLoop`
- Depends on both `ctxmgr` and `registry` — sits at the top of the dependency graph
- Known coupling: `estimateCost`, `responseToAssistantParam`, and `jsonToMap` are shared between `loop.go` and `agentic.go` — they stay together in this package. If `loop.go` is ever extracted, these will need to move with it.

### Dependency graph
```
agentic -> ctxmgr
agentic -> registry
ctxmgr  (leaf)
registry (leaf)
```
No cycles.

## Pre-Move Refactoring

Before moving files, add exported accessor methods to `ContextManager` so `agentic` can use it across package boundaries. `agentic.go` currently accesses 3 unexported things:

1. `a.manager.estimateAll()` — unexported method (lines 28, 195, 196)
2. `a.manager.budget.Remaining(...)` — unexported field `budget` (lines 29, 196)
3. `a.manager.estimator.EstimateFast(...)` — unexported field `estimator` (lines 273, 286)

### New accessor methods on `ContextManager`

```go
// manager.go additions
func (m *ContextManager) EstimateAllTokens() int {
    return m.estimateAll()
}

func (m *ContextManager) EstimateText(text string, ct ContentType) int {
    return m.estimator.EstimateFast(text, ct)
}
```

For `budget.Remaining(...)`: `Budget()` already exists and returns `*ContextBudget`, and `Remaining()` is already exported on `ContextBudget`. So `a.manager.budget.Remaining(x)` becomes `a.manager.Budget().Remaining(x)`. No new accessor needed.

### Changes to `agentic.go`

```
Line 28:  a.manager.estimateAll()                    → a.manager.EstimateAllTokens()
Line 29:  a.manager.budget.Remaining(tokensUsed)     → a.manager.Budget().Remaining(tokensUsed)
Line 195: a.manager.estimateAll()                    → a.manager.EstimateAllTokens()
Line 196: a.manager.budget.Remaining(...)            → a.manager.Budget().Remaining(a.manager.EstimateAllTokens())
Line 273: a.manager.estimator.EstimateFast(block, ContentProse) → a.manager.EstimateText(block, ctxmgr.ContentProse)
Line 286: a.manager.estimator.EstimateFast(block, ContentProse) → a.manager.EstimateText(block, ctxmgr.ContentProse)
```

Verify all tests pass after this step, before any file moves.

## Cross-Package Reference Updates (all `internal/agentic/` files)

After moving to `internal/agentic/`, every file that references types from `ctxmgr` or `registry` needs: (a) new import lines, and (b) package-qualified names. This section catalogs ALL such references.

### `agentic/types.go` (from `agentic_types.go`)

New imports needed:
```go
"github.com/dancsalo/arxiv-deep-research/internal/ctxmgr"
"github.com/dancsalo/arxiv-deep-research/internal/registry"
```

References to qualify:
- `TurnPriority` → `ctxmgr.TurnPriority` (line 57, `AgenticLoopConfig.DefaultPriority`)
- `PriorityCore` → `ctxmgr.PriorityCore` (line 104, default value in `NewAgenticLoop`)
- `*ContextManager` → `*ctxmgr.ContextManager` (line 65, `AgenticLoop.manager` field)
- `*ToolRegistry` → `*registry.ToolRegistry` (line 66, `AgenticLoop.registry` field)

### `agentic/agentic.go` (from `agentic.go`)

New imports needed:
```go
"github.com/dancsalo/arxiv-deep-research/internal/ctxmgr"
```

References to qualify:
- `ContentProse` → `ctxmgr.ContentProse` (lines 273, 286)
- `a.manager.estimateAll()` → `a.manager.EstimateAllTokens()` (lines 28, 195, 196)
- `a.manager.budget.Remaining(...)` → `a.manager.Budget().Remaining(...)` (lines 29, 196)
- `a.manager.estimator.EstimateFast(...)` → `a.manager.EstimateText(...)` (lines 273, 286)

### `agentic/loop.go` (from `loop.go`)

New imports needed:
```go
"github.com/dancsalo/arxiv-deep-research/internal/ctxmgr"
```

References to qualify:
- `*ContextManager` → `*ctxmgr.ContextManager` (line 36, `ResearchLoop` parameter)
- `PriorityResearch` → `ctxmgr.PriorityResearch` (line 135, `manager.AddTurn` call)
- `GuardrailDecision` — used implicitly via return type of `manager.PreToolGuardrail`, but since it's returned from an exported method, the agentic package doesn't need to name the type explicitly. No change needed.

### `agentic/recall.go` (from `recall.go`)

No cross-package references. `RecalledMemory` and `AgenticLoop` are both in the `agentic` package after the move. No imports needed.

### `agentic/helpers_test.go` (from `agentic_test_helpers_test.go`)

New imports needed:
```go
"github.com/dancsalo/arxiv-deep-research/internal/ctxmgr"
"github.com/dancsalo/arxiv-deep-research/internal/registry"
```

References to qualify:
- `*ContextManager` → `*ctxmgr.ContextManager` (line 56, return type of `newAgenticLoopManager`)
- `NewTokenEstimator(...)` → `ctxmgr.NewTokenEstimator(...)` (line 57)
- `&ContextBudget{...}` → `&ctxmgr.ContextBudget{...}` (line 58)
- `NewContextManager(...)` → `ctxmgr.NewContextManager(...)` (line 66)
- `ContextManagerConfig{...}` → `ctxmgr.ContextManagerConfig{...}` (line 66)
- `NewToolRegistry()` → `registry.NewToolRegistry()` (line 74)

### `agentic/agentic_test.go` (from `agentic_test.go`)

New imports needed:
```go
"github.com/dancsalo/arxiv-deep-research/internal/registry"
```

References to qualify:
- `NewToolRegistry()` → `registry.NewToolRegistry()` (lines 39, 131, 217, 260, 304, 397, 440, 552, 628)
- `minimalToolDef(...)` — **PROBLEM**: defined in `registry_test.go` (moving to `internal/registry/`), not accessible from `agentic` tests. **Solution**: duplicate as a local helper in `helpers_test.go`.
- `PrioritySupplementary` → `ctxmgr.PrioritySupplementary` (line 713)

### `agentic/agentic_types_test.go` (from `agentic_types_test.go`)

New imports needed:
```go
"github.com/dancsalo/arxiv-deep-research/internal/ctxmgr"
"github.com/dancsalo/arxiv-deep-research/internal/registry"
```

Check needed — read the file during implementation to catalog exact references (likely `NewToolRegistry`, `ContextManager`, `PriorityCore`).

### `agentic/loop_test.go` (from `loop_test.go`)

New imports needed:
```go
"github.com/dancsalo/arxiv-deep-research/internal/ctxmgr"
```

References to qualify:
- `*ContextManager` → `*ctxmgr.ContextManager` (in `newLoopManager` return type)
- `NewTokenEstimator(...)` → `ctxmgr.NewTokenEstimator(...)`
- `&ContextBudget{...}` → `&ctxmgr.ContextBudget{...}`
- `NewContextManager(...)` → `ctxmgr.NewContextManager(...)`
- `ContextManagerConfig{...}` → `ctxmgr.ContextManagerConfig{...}`

### `agentic/recall_test.go` (from `recall_test.go`)

No cross-package references expected — only tests `buildRecallQuery`, `buildMemoryBlock`, `injectMemories`, `filterNewMemories`, all staying in `agentic`.

### `agentic/integration_test.go` (TestIntegrationFullHappyPath only)

New imports needed:
```go
"github.com/dancsalo/arxiv-deep-research/internal/ctxmgr"
```

References to qualify:
- `NewTokenEstimator(...)` → `ctxmgr.NewTokenEstimator(...)`
- `&ContextBudget{...}` → `&ctxmgr.ContextBudget{...}`
- `NewContextManager(...)` → `ctxmgr.NewContextManager(...)`
- `ContextManagerConfig{...}` → `ctxmgr.ContextManagerConfig{...}`

### Key issue: `minimalToolDef` shared across packages

`minimalToolDef` is defined in `registry_test.go` (→ `internal/registry/registry_test.go`) but used 9 times in `agentic_test.go`. After the split, this helper is inaccessible from the `agentic` package.

**Solution**: Duplicate `minimalToolDef` into `agentic/helpers_test.go`. It's a 13-line helper function — duplication is better than creating a shared test utilities package for one function.

## Consumer Updates

### `cmd/server/main.go`
Replace:
```go
contextmanager "github.com/dancsalo/arxiv-deep-research"
```
With:
```go
"github.com/dancsalo/arxiv-deep-research/internal/agentic"
"github.com/dancsalo/arxiv-deep-research/internal/ctxmgr"
"github.com/dancsalo/arxiv-deep-research/internal/registry"
```
Update type references:
- `contextmanager.NewContextManager(...)` → `ctxmgr.NewContextManager(...)`
- `contextmanager.ContextManagerConfig{...}` → `ctxmgr.ContextManagerConfig{...}`
- `contextmanager.NewTokenEstimator(...)` → `ctxmgr.NewTokenEstimator(...)`
- `contextmanager.ContextBudget{...}` → `ctxmgr.ContextBudget{...}`
- `contextmanager.NewToolRegistry()` → `registry.NewToolRegistry()`
- `contextmanager.BuildFinishTool()` → `agentic.BuildFinishTool()`
- `contextmanager.NewAgenticLoop(...)` → `agentic.NewAgenticLoop(...)`
- `contextmanager.AgenticLoopConfig{...}` → `agentic.AgenticLoopConfig{...}`
- `contextmanager.PriorityCore` → `ctxmgr.PriorityCore`

### `server/server.go`
Replace import and update `LoopFactory` type:
```go
*contextmanager.AgenticLoop → *agentic.AgenticLoop
```

### `server/server_test.go`
Same pattern — replace import, update all `contextmanager.X` references to their new package.

## Integration Test Splitting

### Tests → `internal/ctxmgr/integration_test.go` (package ctxmgr)
- `TestIntegrationBudgetPressureTriggersCompaction` — uses only ctxmgr types
- `TestIntegrationCalibrationConverges` — accesses `estimator.ratios` (unexported field), must be in same package
- `TestIntegrationConversationHistoryCompaction` — uses `mockCompactionClient` from `compaction_test.go` (same package)
- `TestIntegrationGuardrailCompactionChain` — uses only ctxmgr types

### Tests → `internal/agentic/integration_test.go` (package agentic)
- `TestIntegrationFullHappyPath` — uses `scriptedMessageClient`/`makeTextResponse` from `loop_test.go` (same package after move), plus ctxmgr types. Needs import of `internal/ctxmgr` for `NewContextManager`, `NewTokenEstimator`, `ContextBudget`, `ContextManagerConfig`.

## Implementation Order

1. **Add accessor methods** to `manager.go` (`EstimateAllTokens`, `EstimateText`)
2. **Update `agentic.go`** to use new accessors + `Budget()` instead of direct field access
3. **Run `make check`** — verify everything passes before any file moves
4. **Create `internal/ctxmgr/`** — move 7 source + 7 test files, change `package` to `ctxmgr`
5. **Create `internal/registry/`** — move 2 source + 2 test files, change `package` to `registry`
6. **Create `internal/agentic/`** — move 4 source + 5 test files, change `package` to `agentic`, add imports for `ctxmgr` and `registry`, rename `agentic_types.go` → `types.go`, `agentic_test_helpers_test.go` → `helpers_test.go`
7. **Split `integration_test.go`** — 4 tests to `internal/ctxmgr/`, 1 to `internal/agentic/` (with `ctxmgr` import)
8. **Update consumers** — `cmd/server/main.go`, `server/server.go`, `server/server_test.go`
9. **Run `make check`** — verify clean build + all tests pass
10. **Update CLAUDE.md** project structure section

Note: Steps 4-7 are the file moves. Moving a file IS deleting the original — there is no separate "delete originals" step. Between steps 3 and 9, the repo may not compile (the move is atomic in a single commit).

## Verification

1. `make check` (build + lint + test) — must pass
2. `make test-race` — verify no race conditions introduced
3. `go vet ./...` — verify internal packages are properly resolved
4. Confirm `go test ./internal/...` discovers and runs all tests
5. Confirm `make run-server` starts successfully

Makefile uses `./...` for test and lint commands, which automatically recurses into `internal/`. No Makefile changes needed.

## Decisions & Tradeoffs

| Decision | Tradeoff |
|----------|----------|
| Use `internal/` not top-level packages | Standard Go practice; prevents accidental external imports even though no external consumers exist today |
| No facade — update consumers directly | 3 files to update vs. maintaining 50+ lines of aliases/vars forever. Facade hurts godoc, allows runtime reassignment of constructors, and adds a layer for zero benefit. |
| Keep guardrail.go in ctxmgr | Guardrail methods are on `*ContextManager` — splitting requires a wrapper or interface refactor. Not worth it for this change. |
| Add accessor methods, not interfaces | `agentic.go` accesses 3 unexported things. Two new methods + one existing `Budget()` accessor is simpler than defining interfaces for the sake of a structural refactor. Interfaces can come later if there's a testing/decoupling need. |
| `ctxmgr` not `context` | Avoids collision with stdlib `context` package |
| All test files use internal package name | e.g. `package ctxmgr` not `package ctxmgr_test`, preserving access to unexported fields in tests like `estimator.ratios` |
| Rename `agentic_types.go` → `types.go` | Redundant prefix removed since file is in `agentic/` package |

## Revision Log

- v1: Initial plan
- v2: Revised based on round 1 dual critique (staff engineer + PM)
  - Dropped facade — consumers update imports directly (PM critique #2)
  - Dropped `RemainingFrom` accessor — `Budget()` already exists (engineer critique #1)
  - Explicitly stated test files use internal package names (engineer critique #2)
  - Fixed implementation ordering — move IS delete (engineer critique #12)
  - Added `agentic/integration_test.go` needs `ctxmgr` import (engineer critique #3)
  - Documented `estimateCost`/`responseToAssistantParam`/`jsonToMap` coupling (engineer critique #8)
  - Confirmed Makefile unaffected (engineer critique #9)
  - Removed `CachedCount` and `RollingAvg` from public surface — no facade means no decision needed
  - Added ContentType constants note — imported from `ctxmgr` (engineer critique #4)
- v3: Revised based on round 2 dual critique
  - Added comprehensive "Cross-Package Reference Updates" section for ALL files in `internal/agentic/` (engineer round 2 blocking issue)
  - Cataloged every type, constant, and constructor that needs package qualification in: `types.go`, `agentic.go`, `loop.go`, `helpers_test.go`, `agentic_test.go`, `loop_test.go`, `integration_test.go`
  - Identified `minimalToolDef` cross-package test helper issue — solution: duplicate into `helpers_test.go`
  - Qualified `ContentProse` as `ctxmgr.ContentProse` in changes table (PM round 2 cosmetic fix)
  - Noted `PriorityResearch` in `loop.go` needs `ctxmgr.` prefix (engineer round 2 item #2)
