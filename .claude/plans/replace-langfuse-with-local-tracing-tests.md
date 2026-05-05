# Test Plan: Local Tracing (`internal/tracing/`)

## Conventions

- Standard library `testing` package (no testify)
- Table-driven tests where multiple inputs/outputs vary
- Mock/stub types defined in test files
- Direct `t.Fatalf` / `t.Errorf` assertions
- `context.Background()` via a local helper
- Package name: `tracing` (internal tests)

---

## 1. Unit Tests for Recorder

**File:** `internal/tracing/recorder_test.go`

### Test: `TestRecorder_TurnAccumulation`

**Inputs:** Create a Recorder, call `onTurnStart` then `onTurnEnd` three times with incrementing `TurnState.TurnIndex` values (0, 1, 2). Each TurnState has different token counts.

**Expected:**
- `recorder.trace.Turns` has length 3
- Each turn's `Index` matches the provided `TurnIndex`
- Each turn's `StartedAt` is before its `EndedAt`
- `DurationMs` is non-negative for each turn

### Test: `TestRecorder_ToolStackLIFO`

**Inputs:** Within a single turn, call `onToolCall` twice (tools "search_arxiv" and "fetch_pdf"), then call `onToolResult` twice. Provide `json.RawMessage` inputs of known byte lengths.

**Expected:**
- `currentTurn.ToolCalls` has length 2
- First `onToolResult` pops the second tool's start time (LIFO)
- Both tool calls have non-zero `DurationMs`
- `InputLength` equals `len(input)` for each tool call
- `ResultLength` equals `len(result)` for each tool result

### Test: `TestRecorder_ToolStackSingleTool`

**Inputs:** Single tool call + single tool result within a turn.

**Expected:**
- `toolStartStack` is empty after `onToolResult`
- The tool call has correct `DurationMs`, `InputLength`, `ResultLength`

### Test: `TestRecorder_CostDeltaComputation`

**Inputs:** Three turns with cumulative `TotalCostUSD` values: 0.001, 0.003, 0.007.

**Expected:**
- Turn 0: `CostUSD` = 0.001
- Turn 1: `CostUSD` = 0.002
- Turn 2: `CostUSD` = 0.004

### Test: `TestRecorder_FlushOutputFormat`

**Inputs:** Recorder with Config pointing to `t.TempDir()`. Add one turn via hooks. Call `Flush()`.

**Expected:**
- File exists at `<dir>/<session_id>.json`
- File contents are valid JSON
- Unmarshaled `Trace` matches: SessionID, Query, Model, Status == "ok", Turns length 1
- `EndedAt` is after `StartedAt`, `DurationMs` non-negative

### Test: `TestRecorder_FlushCreatesDirectory`

**Inputs:** Config with `Dir` set to a non-existent subdirectory within `t.TempDir()`.

**Expected:**
- `Flush()` returns nil
- Directory and file exist after flush

### Test: `TestRecorder_FlushError_BadPath`

**Inputs:** Config with `Dir` set to `/dev/null/impossible`.

**Expected:**
- `Flush()` returns a non-nil error

### Test: `TestRecorder_SetError`

**Inputs:** Call `SetError(fmt.Errorf("model not found"))` then `Flush()`.

**Expected:**
- `trace.Status == "error"`
- `trace.Error == "model not found"`

### Test: `TestRecorder_SetErrorNilIsNoOp`

**Inputs:** Call `SetError(nil)`.

**Expected:**
- `trace.Status` remains `"ok"`, `trace.Error` remains `""`

### Test: `TestRecorder_DisabledNoOp`

**Inputs:** Config with `Dir: ""`. Call all hook methods and `Flush()`.

**Expected:**
- No panics, `Flush()` returns nil, no files created

### Test: `TestRecorder_PendingLLMCallAttachedToTurn`

**Inputs:** Set `recorder.pendingLLMCall` to `&LLMCall{...}`, then call `onTurnEnd`.

**Expected:**
- Completed turn's `LLMCall` equals the pending call
- `recorder.pendingLLMCall` is nil after `onTurnEnd`

### Test: `TestRecorder_TotalTokensAccumulated`

**Inputs:** Two turns with pendingLLMCall having InputTokens: 100/200, OutputTokens: 50/100.

**Expected:**
- After flush: `trace.TotalInputTokens == 300`, `trace.TotalOutputTokens == 150`

---

## 2. Unit Tests for TracedClient

**File:** `internal/tracing/client_test.go`

### Mock

```go
type mockMessageClient struct {
    response *anthropic.Message
    err      error
    delay    time.Duration
}
```

### Test: `TestTracedClient_RecordsTiming`

**Inputs:** Inner client with 50ms delay.

**Expected:** `recorder.pendingLLMCall.DurationMs >= 50`

### Test: `TestTracedClient_RecordsTokens`

**Inputs:** Inner client returns `Usage{InputTokens: 500, OutputTokens: 200}`, `StopReason: "end_turn"`.

**Expected:**
- `pendingLLMCall.InputTokens == 500`
- `pendingLLMCall.OutputTokens == 200`
- `pendingLLMCall.StopReason == "end_turn"`

### Test: `TestTracedClient_RecordsModel`

**Inputs:** Params with `Model: "claude-3-5-haiku"`.

**Expected:** `pendingLLMCall.Model == "claude-3-5-haiku"`

### Test: `TestTracedClient_RecordsError`

**Inputs:** Inner client returns `nil, fmt.Errorf("rate limited")`.

**Expected:**
- `pendingLLMCall.Error == "rate limited"`
- `pendingLLMCall.InputTokens == 0`
- TracedClient returns `nil, err`

### Test: `TestTracedClient_PassthroughOnSuccess`

**Inputs:** Inner client returns a valid message.

**Expected:** Returned `*anthropic.Message` is the exact same pointer.

### Test: `TestTracedClient_PassthroughOnError`

**Inputs:** Inner client returns error.

**Expected:** Returned error is the exact same error value.

---

## 3. Integration Tests

**File:** `internal/tracing/integration_test.go`

### Test: `TestHooksIntegration_FullLoopSequence`

Simulate a 2-turn run (turn 0 with tool use, turn 1 with text response):

1. `OnTurnStart(ctx, state{TurnIndex: 0})`
2. `OnToolCall(ctx, "search_arxiv", input, state)`
3. `OnToolResult(ctx, "search_arxiv", result, state)`
4. Set `recorder.pendingLLMCall`
5. `OnTurnEnd(ctx, state{TurnIndex: 0, TotalCostUSD: 0.0015})`
6. `OnTurnStart(ctx, state{TurnIndex: 1})`
7. Set `recorder.pendingLLMCall`
8. `OnTurnEnd(ctx, state{TurnIndex: 1, TotalCostUSD: 0.0032})`
9. `recorder.Flush()`

**Expected:**
- Valid JSON trace file, 2 turns
- Turn 0: 1 tool call, LLMCall populated, CostUSD = 0.0015
- Turn 1: 0 tool calls, LLMCall populated, CostUSD = 0.0017
- `trace.TotalCostUSD == 0.0032`, `trace.Status == "ok"`

### Test: `TestHooksIntegration_WithTracedClient`

Wire up TracedClient + hooks together end-to-end.

**Expected:** Turn has LLMCall from TracedClient, tokens correct, single file written.

### Test: `TestHooksIntegration_ErrorPath`

Same as above but `SetError(err)` before flush.

**Expected:** `status: "error"` in trace file.

### Test: `TestNewTracingHooks_DisabledReturnsNil`

**Inputs:** `Config{Dir: ""}`.

**Expected:** Hooks pointer is nil, recorder non-nil, `Flush()` no-op.

---

## 4. Manual Verification Checklist

### Makefile targets

- [ ] `make trace-list` with no `.traces/` prints "No traces" (exit 0)
- [ ] `make trace-list` after demo run shows session, status, query, cost
- [ ] `make trace-show RUN=<id>` pretty-prints JSON
- [ ] `make trace-errors` with no errors prints "No errors"

### End-to-end

- [ ] `go run ./cmd/research-demo -query "transformers" -max-turns 2` → `.traces/demo-*.json` exists
- [ ] JSON has correct schema (session_id, turns, timestamps, tokens, cost)
- [ ] Running without trace dir produces no file, no error
- [ ] Tool calls have positive `duration_ms`
- [ ] `llm_call.duration_ms` ≈ wall-clock API time

### Build

- [ ] `make check` passes
- [ ] `go mod tidy` removed all OTel deps
- [ ] No `go.opentelemetry.io` or `langfuse` imports remain
