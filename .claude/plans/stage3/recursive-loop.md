# Plan: Recursive Agentic Loop

## Problem Statement

The codebase has two loop implementations (`AgenticLoop.Run` in `agentic.go` and `ResearchLoop` in `loop.go`). `ResearchLoop` is dead code (never called in production, only tests). The goal is to:

1. Delete the dead `ResearchLoop` code
2. Add a `Spawn` method to the existing loop that enables recursive sub-task execution
3. Each child loop gets its **own ContextManager** (private message history) but is budget-aware of the parent's remaining capacity
4. The child's result flows back to the parent as a single tool result string — no message pollution

This is a **feature-enablement refactor**: it unlocks recursive research patterns for future use while cleaning up dead code today.

## Requirements

1. Single `Loop` struct (rename `AgenticLoop` → `Loop`) that handles both top-level and sub-task execution
2. `Spawn` method creates a child loop with its own `ContextManager`, restricted tools, and sub-budget
3. Context isolation: child has private message history; result is a string returned to parent's tool handler
4. Budget awareness: child's `ContextBudget.ModelContextLimit` is capped at parent's remaining capacity
5. Priority: child turns are recorded in child's own manager at configurable priority (irrelevant to parent's compaction)
6. Hooks propagate to children (SSE streaming works across depth)
7. Memory recall only at depth 0
8. Depth guard prevents infinite recursion (default max depth: 3)
9. Cost rolls up: parent's total cost includes child costs (for accurate SSE reporting)
10. Existing tests pass; server wiring works

## Specs

### Core Type: `Loop`

```go
type Loop struct {
    client   MessageClient
    manager  *ctxmgr.ContextManager   // each loop owns its own
    registry *registry.ToolRegistry    // per-loop (children get subset)
    recaller MemoryRecaller            // nil for children
    cfg      LoopConfig
    system   []anthropic.TextBlockParam
    hooks    *LoopHooks
    logger   *slog.Logger
    depth    int
}
```

### `LoopConfig` (renamed from `AgenticLoopConfig`, absorbs old `LoopConfig`)

```go
type LoopConfig struct {
    MaxTurns        int
    MaxCostUSD      float64
    MaxDepth        int                    // default 3
    Model           anthropic.Model
    SessionID       string
    FinishTool      string
    DefaultPriority ctxmgr.TurnPriority
    MemoryRecall    MemoryRecallConfig
    Hooks           *LoopHooks
    Logger          *slog.Logger
}
```

### `Spawn` Method

```go
func (l *Loop) Spawn(cfg ChildConfig) (*Loop, error)
```

Returns error if `l.depth+1 >= l.cfg.MaxDepth`.

```go
type ChildConfig struct {
    Query      string                       // becomes the child's initial user message
    MaxTurns   int                          // child's turn limit
    MaxCostUSD float64                      // child's cost limit
    Priority   ctxmgr.TurnPriority          // priority for child's turns (in its own manager)
    Tools      *registry.ToolRegistry       // required — no inheritance to avoid FinishTool leaking
    System     []anthropic.TextBlockParam   // nil = inherit parent's
    Model      anthropic.Model              // zero = inherit parent's
}
```

`Spawn` does:
1. Depth check (return error if exceeded)
2. Create a new `ContextManager` for the child:
   - Same `TokenEstimator` (stateless, safe to share)
   - Budget: `ModelContextLimit` = min(parent's remaining tokens, some cap like 100k)
   - Initial message = `anthropic.NewUserMessage(anthropic.NewTextBlock(cfg.Query))`
3. Create child `Loop` with `depth: l.depth + 1`, `recaller: nil`
4. Register `"finish_loop"` tool in child's registry automatically
5. Inherit parent's hooks and logger (with `depth` added to logger context)

### The "research" Tool Pattern (chicken-and-egg solved)

The loop exposes a `SpawnTool` helper that returns a tool definition + handler. The handler captures `l` via method receiver:

```go
func (l *Loop) ResearchToolHandler(ctx context.Context, input json.RawMessage) (string, error) {
    var args struct{ Query string `json:"query"` }
    json.Unmarshal(input, &args)

    childReg := registry.NewToolRegistry()
    // register research-relevant tools (search, fetch, etc.)
    
    child, err := l.Spawn(ChildConfig{
        Query:    args.Query,
        MaxTurns: 10,
        MaxCostUSD: 0.20,
        Priority: ctxmgr.PriorityResearch,
        Tools:    childReg,
    })
    if err != nil {
        return "", err
    }

    result, err := child.Run(ctx)
    if err != nil {
        return fmt.Sprintf("research failed: %v", err), nil
    }
    
    // Roll child cost up to parent
    l.addChildCost(child.TotalCost())
    return result, nil
}
```

Construction order in server: create registry → create loop → register the research tool using `loop.ResearchToolHandler`. This works because `Register` is called on the registry that the loop already holds a pointer to. The loop is constructed first, then the tool that references it is added to its own registry.

```go
loop := agentic.NewLoop(client, manager, reg, nil, cfg, system)
reg.Register("deep_research", deepResearchToolDef, loop.ResearchToolHandler)
```

### `Run` Method

```go
func (l *Loop) Run(ctx context.Context) (string, error)
```

No `query` parameter. The query is already in the `ContextManager` as the initial user message (set at construction time, or by `Spawn`). The original query string is also stored in `l.query` for memory recall (`buildRecallQuery` needs it).

Logic:
1. If depth == 0 and memory enabled: set up memory recall state
2. Main loop (turn < MaxTurns && !finished):
   - Context cancellation check
   - Memory recall (if depth == 0 and applicable)
   - `PreAPICheck`
   - `BuildMessages` (+ inject memories)
   - `CreateMessage` API call
   - Cost tracking + limit check (includes accumulated child costs)
   - Output tracker recording
   - Process response blocks (same as current `agentic.go`):
     - Text → accumulate
     - ToolUse:
       - FinishTool → mark finished, extract `"summary"` from tool input, store as `finishResult`
       - Hook: OnToolCall
       - PreToolGuardrail → proceed/modify/reject
       - Execute via registry (may invoke `ResearchToolHandler` → recursive spawn)
       - PostToolCheck
       - Hook: OnToolResult
   - `AddTurn` with `cfg.DefaultPriority`
   - Hooks: OnMemoryPersist, OnTurnEnd
   - Stop: end_turn, finished, max turns
3. If `finishResult != ""`, return it. Otherwise return `manager.ExtractFinalAnswer()`.

The `finishResult` path ensures the model's explicit `finish_loop(summary: "...")` answer is returned, even when the last assistant message is a bare tool_use block with no text.

## Contracts

### Public API

```go
func NewLoop(client MessageClient, manager *ctxmgr.ContextManager, reg *registry.ToolRegistry,
    recaller MemoryRecaller, cfg LoopConfig, system []anthropic.TextBlockParam) *Loop

func (l *Loop) Run(ctx context.Context) (string, error)
func (l *Loop) Spawn(cfg ChildConfig) (*Loop, error)
func (l *Loop) TotalCost() float64
func (l *Loop) ResearchToolHandler(ctx context.Context, input json.RawMessage) (string, error)
```

### Server Factory Change

```go
// Before
type LoopFactory func(query string, logger *slog.Logger) (*agentic.AgenticLoop, error)

// After  
type LoopFactory func(query string, logger *slog.Logger) (*agentic.Loop, error)
```

## Decisions & Tradeoffs

1. **Private ContextManager per child (not shared)**: Resolves the message pollution problem. The child's internal deliberation never appears in the parent's `BuildMessages()`. The parent only sees the child's final result as a tool result string. Trade-off: no global compaction across parent/child. Acceptable because the child has its own budget cap based on parent's remaining capacity.

2. **Cost rolls up to parent (mutex-protected)**: Child cost IS added to parent total. `addChildCost` uses `sync.Mutex` to protect `totalCostUSD`. This is safe from day one even if children are later parallelized. The parent's cost-limit check fires after `addChildCost` returns, so runaway children are caught on the parent's next iteration.

3. **`Spawn` returns error (not just `*Loop`)**: Depth guard lives in `Spawn`, not `Run`. Fail fast.

4. **Tools are required in ChildConfig (no inheritance)**: This avoids the FinishTool leaking problem and makes tool scope explicit. `Spawn` always auto-registers `"finish_loop"` in the child's registry with `FinishTool: "finish_loop"` in the child's LoopConfig.

5. **Post-construction tool registration solves the chicken-and-egg**: The loop is created with the registry, then tools that reference the loop are added to the same registry. This works because Go closures capture the pointer, not the value. **Important**: the parent registry is frozen after construction (no Register calls during Run). This is already the contract per `registry.go` documentation.

6. **`Run` takes no `query` parameter**: The query is in the ContextManager's initial message. The `Loop.query` field stores the string for memory recall. For the root loop, this is set by `NewLoop`. For children, `Spawn` sets it from `cfg.Query`.

7. **Logger with depth context**: Child loops add `slog.Int("depth", l.depth)` to their logger, making recursive calls traceable in logs/SSE.

8. **Child budget cap: 50% of parent's remaining or 100k, whichever is smaller**: `Spawn` computes `min(parentRemaining * 0.5, 100_000)` as the child's `ModelContextLimit`. This reserves capacity for the parent to continue after the child returns.

9. **Child compaction client**: Children share the parent's compaction client (passed through `Spawn`). If parent has no compaction client, child gets nil (extractive truncation fallback, which is fine for short-lived children).

10. **`finish_loop` tool schema for children**: `{ "type": "object", "properties": { "summary": { "type": "string" } }, "required": ["summary"] }`. Handler returns the `summary` value. `Run` extracts this as `finishResult` and returns it to the parent's tool handler.

11. **`ResearchToolHandler` is experimental**: Marked with a godoc comment noting the contract may change. Not exposed in public API docs until exercised by a real prompt.

## Implementation Order

### Step 1: Relocate shared symbols and delete dead code
- Move `MessageClient`, `estimateCost`, `responseToAssistantParam`, `jsonToMap`, `BuildFinishTool` from `loop.go` → new `internal/agentic/helpers.go`
- Delete `loop.go` (only `ResearchLoop` and `LoopConfig` and `FinishResult` remain — all dead)
- Remove `ResearchLoop`-specific test cases from `loop_test.go`
- **Verify: `make check` passes**

### Step 2: Rename types in place
- Rename `AgenticLoop` → `Loop` in `types.go`
- Rename `AgenticLoopConfig` → `LoopConfig` in `types.go`
- Rename `NewAgenticLoop` → `NewLoop` in `types.go`
- Add `query string` field to `Loop` struct (extracted from initial message text at construction)
- Add `MaxDepth int` field to `LoopConfig` (default 3 in constructor)
- Update `cmd/server/main.go` and `server/server.go` references
- Store `finishResult` in `Run`: when FinishTool is called, extract `"summary"` from input and return it instead of `ExtractFinalAnswer()`
- **Verify: `make check` passes**

### Step 3: Add `Spawn` and `ChildConfig`
- Add `ChildConfig` type to `types.go`
- Add `Spawn` method: depth check, create child ContextManager (budget = min(parent remaining * 0.5, 100k)), auto-register `finish_loop`, inherit compaction client + hooks + logger (with depth field)
- Add `TotalCost() float64` accessor
- Add `addChildCost(float64)` with `sync.Mutex` protection on `totalCostUSD`
- **Verify: `make check` passes**

### Step 4: Add `ResearchToolHandler`
- Create `internal/agentic/research.go`
- Add `ResearchToolHandler` method to `Loop` (experimental godoc)
- Add `BuildResearchTool()` helper (tool definition)
- Wire it up in `cmd/server/main.go` post-loop-construction
- **Verify: `make check` passes**

### Step 5: Add tests
- Test: spawn creates child with correct budget cap
- Test: depth limit returns error at `MaxDepth`
- Test: child finishResult flows back as string to parent tool handler
- Test: child cost rolls up to parent total
- Test: recursive spawn (depth 2) works
- Test: `finishResult` returned by `Run` instead of `ExtractFinalAnswer` when finish tool called
- **Verify: `make check` passes**

## Files Modified

| File | Action |
|------|--------|
| `internal/agentic/helpers.go` | **Create** — relocated `MessageClient`, `estimateCost`, `responseToAssistantParam`, `jsonToMap`, `BuildFinishTool` |
| `internal/agentic/types.go` | **Modify** — rename types, add `ChildConfig`, `query` field, `Spawn`, `TotalCost`, mutex |
| `internal/agentic/agentic.go` | **Modify** — rename, `finishResult` logic, `addChildCost` |
| `internal/agentic/loop.go` | **Delete** — dead code (ResearchLoop, old LoopConfig, FinishResult) |
| `internal/agentic/research.go` | **Create** — `ResearchToolHandler`, `BuildResearchTool` |
| `internal/agentic/loop_test.go` | **Modify** — remove ResearchLoop tests, add Spawn/depth/finishResult tests |
| `cmd/server/main.go` | **Modify** — use `NewLoop`, register research tool post-construction |
| `server/server.go` | **Modify** — update `LoopFactory` type |

## Verification

```bash
make check           # lint + all tests pass at every step
go run ./cmd/server/ # server starts and accepts queries
```

## Revision Log

- v1: Initial draft with shared ContextManager
- v2: Revised based on round 1 critique. Key changes:
  - Private ContextManager per child (fixes message pollution)
  - Spawn returns error (depth guard at creation time)
  - Tools required in ChildConfig (fixes FinishTool leak)
  - Post-construction registration (fixes chicken-and-egg)
  - Cost rolls up (fixes SSE visibility)
  - Rename-in-place implementation order (compiles at every step)
  - Deleted dead `ResearchLoop` as step 1 (PM recommendation)
- v3: Revised based on round 2 critique. Key changes:
  - `l.query` field preserved for memory recall (fixes recall regression)
  - `finishResult` logic: Run returns finish tool's summary, not ExtractFinalAnswer (fixes empty-string bug)
  - Symbol relocation step before loop.go deletion (fixes compile breakage)
  - `addChildCost` uses sync.Mutex (future-proofs for concurrent children)
  - Concrete budget cap: min(parent remaining * 0.5, 100k)
  - Child inherits parent's compaction client
  - Explicit `finish_loop` schema documented
  - `ResearchToolHandler` marked experimental
  - Parent registry frozen after construction (documented contract)
