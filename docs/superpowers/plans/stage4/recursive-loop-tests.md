# Test Plan: Recursive Agentic Loop

Companion to `recursive-loop.md`.

## Test Framework & Conventions

- Standard `testing` package (no testify)
- Mock `MessageClient` via `scriptedMessageClient` (already exists in `loop_test.go`)
- Mock compaction client from `ctxmgr/compaction_test.go` pattern
- Table-driven tests where applicable
- File: `internal/agentic/loop_test.go` (modified) + `internal/agentic/spawn_test.go` (new)

## Tests

### Group 1: Rename Equivalence (Step 2)

| Test | Input | Expected |
|------|-------|----------|
| `TestLoopEndTurn` | Scripted client returns text with `end_turn` | `Run` returns that text |
| `TestLoopMaxTurns` | Scripted client returns 3 text responses | Loop stops at MaxTurns, returns last text |
| `TestLoopCostLimit` | Response with 1M input tokens | Error containing "cost limit exceeded" |
| `TestLoopFinishResult` | Response with `finish_loop` tool_use (summary: "answer") | `Run` returns `"answer"` (not empty string) |
| `TestLoopFinishResultFallback` | Response with text + `end_turn` (no finish tool) | `Run` returns the text via `ExtractFinalAnswer` |

### Group 2: Spawn Mechanics (Step 3)

| Test | Input | Expected |
|------|-------|----------|
| `TestSpawnCreatesChildManager` | Parent with 150k remaining, Spawn | Child budget = min(75k, 100k) = 75k |
| `TestSpawnDepthLimit` | Parent at depth 2, MaxDepth 3 | `Spawn` returns error |
| `TestSpawnDepthZero` | Root loop, MaxDepth 3 | `Spawn` succeeds, child.depth == 1 |
| `TestSpawnAutoRegistersFinishTool` | Spawn with empty registry | Child registry has `finish_loop` |
| `TestSpawnInheritsCompactionClient` | Parent with mock compaction client | Child manager has same client |
| `TestSpawnInheritsHooks` | Parent with OnTurnStart hook | Child hooks == parent hooks |
| `TestSpawnLoggerHasDepth` | Spawn at depth 1 | Child logger context includes `depth=2` |

### Group 3: Child Execution (Step 4-5)

| Test | Input | Expected |
|------|-------|----------|
| `TestChildRunReturnsFinishResult` | Child scripted: text → finish_loop(summary:"done") | child.Run returns "done" |
| `TestChildCostRollsUp` | Child uses 500 input + 200 output tokens | Parent `TotalCost()` includes child cost |
| `TestChildCostRollupMutex` | Two sequential children | Parent total = sum of both child costs + own |
| `TestChildIsolatedMessages` | Parent has 3 turns, spawns child that does 5 turns | Parent still has 3 turns after child returns (plus the tool result turn) |
| `TestResearchToolHandler` | Scripted child returns "research result" | Handler returns "research result" string |
| `TestResearchToolHandlerDepthExceeded` | Parent at MaxDepth-1 | Handler returns error string |

### Group 4: Integration (end-to-end)

| Test | Input | Expected |
|------|-------|----------|
| `TestRecursiveResearch` | Parent scripted: calls deep_research tool → child scripted: returns summary via finish_loop → parent uses result in final answer | Parent Run returns text incorporating child's summary |

## Verification Strategy

```bash
make check    # at every implementation step
make test-race  # after Step 3 (mutex correctness)
```

## What NOT to Test

- Anthropic API behavior (mocked)
- Token estimation accuracy (covered by ctxmgr tests)
- SSE streaming (integration test, not unit)
- Actual LLM responses (scripted)
