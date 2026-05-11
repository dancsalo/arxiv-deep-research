# Agentic Loop: Test Plan

Companion to `agentic-loop.md`. Follows the project's testing conventions: Go standard `testing` package, custom mock structs, `t.Fatal`/`t.Errorf` assertions, test helpers for setup.

## Test Conventions

- Files: `src/agentic_types_test.go`, `src/registry_test.go`, `src/recall_test.go`, `src/agentic_test.go`, `src/memoryclient/adapter_test.go`
- Package: same package as code under test (white-box)
- Mocks: custom structs implementing interfaces (no mock frameworks)
- All tests runnable via `make test-go`

## Mock Definitions

These mocks are shared across test files. Define in `src/agentic_test_helpers_test.go`:

### mockMemoryRecaller

```go
type mockMemoryRecaller struct {
    memories []RecalledMemory
    err      error
    calls    []recallCall // record calls for verification
}

type recallCall struct {
    query string
    mode  string
    limit int
}

func (m *mockMemoryRecaller) RecallMemories(ctx context.Context, query, mode string, limit int) ([]RecalledMemory, error) {
    m.calls = append(m.calls, recallCall{query, mode, limit})
    return m.memories, m.err
}
```

### scriptedMessageClient (extend existing)

The existing `scriptedMessageClient` from `loop_test.go` can be reused. It implements `MessageClient` with a scripted response sequence.

---

## Section 1: Core Types & Interfaces (`agentic_types_test.go`)

### Test: NewAgenticLoop applies defaults

| Input | Expected |
|-------|----------|
| `MemoryRecallConfig{Enabled: true}` (all zero fields) | `MaxResults=5, MaxTokens=2000, SearchMode="hybrid", RecallEveryN=1` |
| `MemoryRecallConfig{Enabled: true, MaxResults: 10}` | `MaxResults=10` (not overwritten) |
| `MemoryRecallConfig{Enabled: false}` | Defaults not applied (all zeros) |
| `DefaultPriority: 0` | Defaults to `PriorityCore` |
| `Logger: nil` | Uses `slog.Default()` (non-nil) |

### Test: NewAgenticLoop with nil recaller

- Construct with `recaller = nil`
- Verify the struct is created without error
- `Run()` should work (recall phase skipped)

### Test: NewAgenticLoop with nil hooks

- Construct with `cfg.Hooks = nil`
- Verify `Run()` doesn't panic on nil hook checks

---

## Section 2: ToolRegistry (`registry_test.go`)

### Test: Register and Definitions

- Register 3 tools in order A, B, C
- Call `Definitions()` → verify returns 3 definitions in registration order
- Verify each definition matches what was registered

### Test: Execute known tool

- Register tool "calc" with handler that returns `"42"`
- `Execute(ctx, "calc", []byte(`{"x":1}`))` → `("42", nil)`
- Verify handler received correct `json.RawMessage`

### Test: Execute unknown tool

- Empty registry
- `Execute(ctx, "nope", []byte("{}"))` → `("", error)` where error contains "unknown tool"

### Test: Execute passes context to handler

- Register tool with handler that checks `ctx.Value("key")`
- Pass context with value → handler receives it

### Test: WrapLegacyHandler

- Create legacy handler: `func(input []byte) (string, error) { return string(input), nil }`
- Wrap with `WrapLegacyHandler`
- Call wrapped handler with `json.RawMessage(`{"a":1}`)` → returns `{"a":1}`
- Verify context argument is accepted (not used but doesn't panic)

### Test: Register duplicate name

- Register "foo" twice → second registration overwrites first
- `Execute("foo")` uses second handler
- `Definitions()` returns only one entry for "foo"

---

## Section 3: Memory Recall Logic (`recall_test.go`)

### Test: buildRecallQuery turn 0

- `buildRecallQuery("what is X?", "", 0)` → `"what is X?"`

### Test: buildRecallQuery turn N

- `buildRecallQuery("what is X?", "I found that Y is related to Z and the results show...", 3)` → contains both the original query and truncated assistant text
- Verify assistant text is truncated to 200 chars max

### Test: buildRecallQuery with empty assistant text

- `buildRecallQuery("what is X?", "", 5)` → falls back to original query only

### Test: buildMemoryBlock formats correctly

- Input: `[]RecalledMemory{{Type: "gotcha", Title: "pgvector needs HNSW"}, {Type: "decision", Title: "Use hybrid search"}}`
- Output contains `[Prior Knowledge]` header, `[End Prior Knowledge]` footer
- Each memory formatted as `- [type] "title"`

### Test: buildMemoryBlock empty list

- Input: empty slice → returns empty string

### Test: injectMemories copies messages

- Create messages slice with one user message containing one text block
- Call `injectMemories(messages, "memory block text")`
- Verify returned slice has the memory text appended to message[0]
- Verify **original** messages slice is unmodified (copy check)

### Test: injectMemories preserves existing content blocks

- User message has 2 text blocks already
- After injection → message[0] has 3 text blocks (original 2 + memory block)

### Test: deduplication filters seen IDs

- This is tested at the integration level in Section 4, but unit test the filtering:
- Given `seenMemoryIDs = {1: true, 3: true}` and memories `[{ID:1}, {ID:2}, {ID:3}, {ID:4}]`
- After filtering → `[{ID:2}, {ID:4}]`
- `seenMemoryIDs` now contains all 4 IDs

---

## Section 4: AgenticLoop.Run() (`agentic_test.go`)

### Test: Basic loop — single turn, no tools, end_turn

- Scripted client returns one text response with `StopReason: "end_turn"`
- No recaller (nil)
- Verify `Run()` returns the text content
- Verify `manager.AddTurn()` was called once

### Test: Basic loop — tool use then end_turn

- Turn 1: model calls tool "calc", handler returns "42"
- Turn 2: model returns text with `StopReason: "end_turn"`
- Verify both turns recorded
- Verify tool result was passed back to model

### Test: Finish tool terminates loop

- Configure `FinishTool: "done"`
- Register "done" handler that returns `"ok"`
- Model calls tool "done" with `{"summary": "all done"}`
- Verify loop returns and `finished` is true

### Test: Unknown tool returns error to model

- Model calls tool "nonexistent"
- Verify tool result block has `is_error: true` and contains "unknown tool"
- Loop continues (doesn't crash)

### Test: Cost limit exceeded

- `MaxCostUSD: 0.001`
- Scripted client returns response with high token counts
- Verify `Run()` returns error containing "cost limit"

### Test: Max turns exceeded

- `MaxTurns: 2`
- Scripted client always returns `StopReason: "tool_use"` with a tool call
- Verify loop stops after 2 turns (doesn't error, returns final answer)

### Test: Context cancellation stops loop

- Create context with cancel
- Cancel after first API response
- Verify `Run()` returns `context.Canceled` error

### Test: Memory recall — basic injection

- Mock recaller returns 2 memories
- Verify API call includes memory block text in messages
- Verify `TurnState.RecalledMemoryIDs` contains the 2 IDs

### Test: Memory recall — deduplication across turns

- Mock recaller returns `[{ID:1}, {ID:2}]` on turn 0, `[{ID:1}, {ID:3}]` on turn 1
- Verify turn 0 injects both (IDs 1, 2)
- Verify turn 1 injects only ID 3 (ID 1 is deduplicated)

### Test: Memory recall — SkipFirstN

- `SkipFirstN: 2`
- Verify recaller is NOT called on turns 0 and 1
- Verify recaller IS called on turn 2

### Test: Memory recall — RecallEveryN

- `RecallEveryN: 3, SkipFirstN: 0`
- Run 6 turns
- Verify recaller called on turns 0, 3 (every 3rd turn)

### Test: Memory recall — budget trimming

- Mock recaller returns 10 memories
- Manager budget is tight — `WillFit` returns false for all 10
- Verify loop reduces memory count (drops lowest-score) until fits or skips

### Test: Memory recall — recaller error degrades gracefully

- Mock recaller returns error
- Verify loop continues without injected memories
- Verify logger received warning

### Test: Memory recall — nil recaller skips phase

- `recaller = nil`
- Verify no panic, no recall-related log entries at Info level

### Test: Hook — OnTurnStart called each turn

- Set `OnTurnStart` hook that records calls
- Run 3-turn loop
- Verify hook called 3 times with correct `TurnIndex`

### Test: Hook — OnTurnEnd called each turn

- Same pattern as OnTurnStart

### Test: Hook — OnMemoryRecall filters memories

- Mock recaller returns `[{ID:1}, {ID:2}, {ID:3}]`
- `OnMemoryRecall` hook removes ID:2
- Verify only IDs 1 and 3 are injected

### Test: Hook — OnMemoryRecall error skips recall

- `OnMemoryRecall` hook returns error
- Verify no memories injected, loop continues

### Test: Hook — OnMemoryPersist called with full TurnState

- Set `OnMemoryPersist` hook that records the `TurnState`
- Run a turn where model returns text and calls a tool
- Verify `TurnState.AssistantText` is non-empty
- Verify `TurnState.ToolResultTexts` contains tool name and result

### Test: Hook — OnMemoryPersist error is swallowed

- `OnMemoryPersist` hook returns error
- Verify loop continues, error is logged

### Test: Hook — OnToolCall receives json.RawMessage

- Set `OnToolCall` hook that records the input
- Model calls tool with `{"x": 42}`
- Verify hook received `json.RawMessage` matching `{"x": 42}`

### Test: Hook — nil hooks don't panic

- `cfg.Hooks = nil`
- Run loop → no panic
- `cfg.Hooks = &LoopHooks{}` (all nil fields) → no panic

### Test: PreToolGuardrail integration

- Set up tight token budget
- Model calls a tool that would exceed budget
- Verify guardrail rejects or compacts (existing behavior, just verify it's wired)

### Test: Run resets state

- Construct AgenticLoop, manually set `seenMemoryIDs` and `totalCostUSD`
- Call `Run()` → verify they are reset to zero

### Test: DefaultPriority used in AddTurn

- Set `DefaultPriority: PrioritySupplementary`
- Run a turn
- Verify `manager.GetTurn(0).Priority == PrioritySupplementary`

### Test: TurnState token counts use fast estimation

- Run a turn
- Capture `TurnState` in a hook
- Verify `TokensUsed` and `TokensRemaining` are populated and non-zero
- Verify no exact count API call was made (scripted client call count unchanged)

---

## Section 5: MemoryRecaller Adapter (`memoryclient/adapter_test.go`)

### Test: Adapter maps MemoryIndexRow to RecalledMemory

- Create a `memoryclient.Client` mock (or use `httptest.NewServer` per existing patterns)
- Call `RecallMemories` → verify fields map correctly:
  - `MemoryIndexRow.ID` → `RecalledMemory.ID`
  - `MemoryIndexRow.Type` → `RecalledMemory.Type` (emoji form from DB)
  - `MemoryIndexRow.Title` → `RecalledMemory.Title`
  - `MemoryIndexRow.Score` → `RecalledMemory.Score`

### Test: Adapter passes search parameters

- Call `RecallMemories(ctx, "query", "semantic", 10)`
- Verify underlying `SearchMemories` called with `query="query", mode="semantic", limit=10, memType=""`

### Test: Adapter propagates errors

- Mock `SearchMemories` returns error
- Verify `RecallMemories` returns the same error

---

## Integration Test Scenario

### Test: Full loop with memory recall and persist hook

End-to-end test simulating a 3-turn conversation:

1. **Setup:** Scripted client with 3 responses. Mock recaller with canned memories. Persist hook that collects stored data.
2. **Turn 0:** Recaller returns memories A, B. Model calls tool "search". Tool returns result. Persist hook fires with assistant text + tool result.
3. **Turn 1:** Recaller returns memories A, C (A is deduplicated, only C injected). Model returns text.
4. **Turn 2:** Model calls finish tool.
5. **Verify:**
   - Recaller called 3 times with evolving queries
   - Turn 0 messages included memories A, B
   - Turn 1 messages included only memory C
   - Persist hook called 3 times with correct `TurnState`
   - Final answer returned correctly
   - Total cost tracked across all turns
   - Manager has 3 recorded turns
