# Partial Results on Error Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Track and return partial research summaries when the agent loop terminates with an error, so users see what was discovered before hitting cost/context limits.

**Architecture:** Add a `partialResult` field to the Loop struct that accumulates assistant text across all turns. On error paths, return this accumulated text instead of an empty string. Update main.go to print partial results with clear labeling.

**Tech Stack:** Go, Anthropic SDK, existing agentic loop implementation

---

## File Structure

**Modified files:**
- `internal/agentic/types.go` - Add `partialResult` field to Loop struct
- `internal/agentic/agentic.go` - Accumulate assistant text, return on error paths
- `cmd/research-demo/main.go` - Print partial results in error handler
- `internal/agentic/agentic_test.go` - Add tests for partial result behavior

**No new files created.** This is a focused enhancement to existing components.

---

### Task 1: Add partialResult Field to Loop Struct

**Files:**
- Modify: `internal/agentic/types.go:82-99`
- Test: `internal/agentic/agentic_test.go` (manual verification)

- [ ] **Step 1: Add partialResult field to Loop struct**

In `internal/agentic/types.go`, add the `partialResult` field to the Loop struct:

```go
type Loop struct {
	client   MessageClient
	manager  *ctxmgr.ContextManager
	registry *registry.ToolRegistry
	recaller MemoryRecaller
	cfg      LoopConfig
	system   []anthropic.TextBlockParam
	hooks    *LoopHooks

	query         string
	totalCostUSD  float64
	turnIndex     int
	finished      bool
	seenMemoryIDs map[int64]bool
	logger        *slog.Logger
	depth         int
	mu            sync.Mutex
	partialResult string  // Accumulated assistant text across turns
}
```

- [ ] **Step 2: Verify code compiles**

Run: `make lint`
Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add internal/agentic/types.go
git commit -m "feat: add partialResult field to Loop struct"
```

---

### Task 2: Accumulate Assistant Text During Loop Execution

**Files:**
- Modify: `internal/agentic/agentic.go:14-289`

- [ ] **Step 1: Initialize partialResult at start of Run()**

In `internal/agentic/agentic.go`, in the `Run()` function after line 19 where fields are initialized, add:

```go
func (l *Loop) Run(ctx context.Context, query string) (string, error) {
	l.query = query
	l.totalCostUSD = 0
	l.turnIndex = 0
	l.finished = false
	l.seenMemoryIDs = make(map[int64]bool)
	l.partialResult = ""  // Clear any previous partial result

	var finishResult string
```

- [ ] **Step 2: Accumulate assistantText after each LLM response**

After the assistantText is collected from response blocks (around line 112), append it to partialResult. Add after the tool processing loop (around line 235, before `assistantMsg := responseToAssistantParam(resp)`):

```go
		}
	}

	// Accumulate assistant text for partial results
	if assistantText != "" {
		l.partialResult += assistantText
	}

	assistantMsg := responseToAssistantParam(resp)
```

- [ ] **Step 3: Verify code compiles**

Run: `make lint`
Expected: No errors

- [ ] **Step 4: Commit**

```bash
git add internal/agentic/agentic.go
git commit -m "feat: accumulate assistant text in partialResult during loop execution"
```

---

### Task 3: Return Partial Results on Error Paths

**Files:**
- Modify: `internal/agentic/agentic.go:14-289`

- [ ] **Step 1: Return partialResult on cost limit error**

Around line 100-102 where the cost limit check happens, modify the error return:

```go
if l.TotalCost() > l.cfg.MaxCostUSD {
	return l.partialResult, fmt.Errorf("cost limit exceeded: $%.4f > $%.4f", l.TotalCost(), l.cfg.MaxCostUSD)
}
```

- [ ] **Step 2: Return partialResult on context cancellation errors**

Find the two places where context cancellation is checked (around lines 95-98 and 263-266). Update both:

First occurrence (around line 95):
```go
if ctx.Err() != nil {
	l.logger.Info("loop.cancelled", "turn", l.turnIndex, "reason", ctx.Err())
	return l.partialResult, ctx.Err()
}
```

Second occurrence (around line 263):
```go
if ctx.Err() != nil {
	l.logger.Info("loop.cancelled", "turn", l.turnIndex, "reason", ctx.Err())
	return l.partialResult, ctx.Err()
}
```

- [ ] **Step 3: Update end-of-loop return logic**

At the end of Run() (around lines 285-289), update the return logic to prefer finishResult but fall back to partialResult:

```go
if finishResult != "" {
	return finishResult, nil
}
if l.partialResult != "" {
	return l.partialResult, nil
}
return l.manager.ExtractFinalAnswer(), nil
```

- [ ] **Step 4: Verify code compiles**

Run: `make lint`
Expected: No errors

- [ ] **Step 5: Commit**

```bash
git add internal/agentic/agentic.go
git commit -m "feat: return partial results on error paths"
```

---

### Task 4: Update main.go Error Handler to Print Partial Results

**Files:**
- Modify: `cmd/research-demo/main.go:179-204`

- [ ] **Step 1: Update error handler to print partial results**

In `cmd/research-demo/main.go`, modify the error handler section (around lines 182-189):

```go
if err != nil {
	recorder.SetError(err)
	if flushErr := recorder.Flush(); flushErr != nil {
		logger.Error("failed to flush trace", "err", flushErr)
	}
	
	// Print partial results if available
	if result != "" {
		fmt.Println("=== Partial Research Summary ===")
		fmt.Println("(Research incomplete due to error)")
		fmt.Println()
		fmt.Println(result)
		fmt.Println()
	}
	
	fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	fmt.Println()
	fmt.Printf("--- Stats ---\n")
	fmt.Printf("Elapsed: %s\n", elapsed.Round(time.Millisecond))
	fmt.Printf("Cost:    $%.4f\n", loop.TotalCost())
	if traceCfg.Enabled() {
		fmt.Printf("Trace:   %s/%s.json\n", traceCfg.Dir, sessionID)
	}
	os.Exit(1)
}
```

- [ ] **Step 2: Verify code compiles**

Run: `make build-research`
Expected: SUCCESS

- [ ] **Step 3: Commit**

```bash
git add cmd/research-demo/main.go
git commit -m "feat: print partial results in error handler"
```

---

### Task 5: Add Unit Test for Partial Result Accumulation

**Files:**
- Modify: `internal/agentic/agentic_test.go`

- [ ] **Step 1: Write failing test for partial result accumulation**

Add this test at the end of `internal/agentic/agentic_test.go`:

```go
func TestPartialResultAccumulation(t *testing.T) {
	client := &scriptedMessageClient{
		responses: []*anthropic.Message{
			makeTextResponse("First turn text"),
			makeTextResponse("Second turn text"),
			makeTextResponse("Third turn text"),
		},
	}

	loop := newBasicAgenticLoop(client, nil, nil)
	result, err := loop.Run(bgctx(), "test")
	
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	
	expected := "First turn textSecond turn textThird turn text"
	if result != expected {
		t.Errorf("result = %q, want %q", result, expected)
	}
}
```

- [ ] **Step 2: Run test to verify it passes**

Run: `make test`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/agentic/agentic_test.go
git commit -m "test: add unit test for partial result accumulation"
```

---

### Task 6: Add Unit Test for Partial Result on Cost Limit Error

**Files:**
- Modify: `internal/agentic/agentic_test.go`

- [ ] **Step 1: Write failing test for partial result on cost error**

Add this test to `internal/agentic/agentic_test.go`:

```go
func TestPartialResultOnCostLimitError(t *testing.T) {
	client := &scriptedMessageClient{
		responses: []*anthropic.Message{
			{
				Content:    []anthropic.ContentBlockUnion{{Type: "text", Text: "Some research findings before hitting cost limit"}},
				StopReason: "tool_use",
				Usage:      anthropic.Usage{InputTokens: 100000, OutputTokens: 100000},
			},
		},
	}

	loop := newBasicAgenticLoop(client, nil, nil)
	loop.cfg.MaxCostUSD = 0.001

	result, err := loop.Run(bgctx(), "test")
	
	if err == nil {
		t.Fatal("expected cost limit error")
	}
	if !contains(err.Error(), "cost limit") {
		t.Errorf("expected cost limit error, got: %v", err)
	}
	
	expected := "Some research findings before hitting cost limit"
	if result != expected {
		t.Errorf("result = %q, want %q", result, expected)
	}
}
```

- [ ] **Step 2: Run test to verify it passes**

Run: `make test`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/agentic/agentic_test.go
git commit -m "test: add unit test for partial result on cost limit error"
```

---

### Task 7: Add Unit Test for finishResult Priority Over partialResult

**Files:**
- Modify: `internal/agentic/agentic_test.go`

- [ ] **Step 1: Write test for finishResult priority**

Add this test to `internal/agentic/agentic_test.go`:

```go
func TestFinishResultTakesPrecedenceOverPartialResult(t *testing.T) {
	client := &scriptedMessageClient{
		responses: []*anthropic.Message{
			makeTextResponse("Some preliminary research"),
			makeToolUseResponse("finish", "call1", json.RawMessage(`{"summary":"Final polished summary"}`)),
		},
	}

	loop := newBasicAgenticLoop(client, nil, nil)
	result, err := loop.Run(bgctx(), "test")
	
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	
	// Should return finishResult, not partialResult
	expected := "Final polished summary"
	if result != expected {
		t.Errorf("result = %q, want %q (finishResult should take precedence)", result, expected)
	}
}
```

- [ ] **Step 2: Run test to verify it passes**

Run: `make test`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/agentic/agentic_test.go
git commit -m "test: verify finishResult takes precedence over partialResult"
```

---

### Task 8: Manual Integration Test

**Files:**
- None (manual testing)

- [ ] **Step 1: Build the research-demo binary**

Run: `make build-research`
Expected: Binary created at `./research-demo`

- [ ] **Step 2: Test with artificially low cost limit**

Run with low cost limit to trigger error:
```bash
./research-demo --query "machine learning" --max-turns 5 --prompt-variant A --trace-dir .traces 2>&1 | tee test-output.txt
```

Wait for cost limit error to occur.

Expected output should include:
```
=== Partial Research Summary ===
(Research incomplete due to error)

[some research text here]

Error: cost limit exceeded: $X.XXXX > $1.0000

--- Stats ---
Elapsed: ...
Cost:    $...
Trace:   ...
```

- [ ] **Step 3: Verify partial result contains actual research text**

Check that `test-output.txt` contains:
- The "Partial Research Summary" header
- Non-empty research text (mentions of papers, arXiv, searches, etc.)
- The error message
- Stats showing elapsed time and cost

Run: `grep -A 5 "Partial Research Summary" test-output.txt`
Expected: Should show research summary text

- [ ] **Step 4: Verify trace file was written**

Run: `ls -l .traces/*.json | tail -1`
Expected: Recent trace file exists

- [ ] **Step 5: Clean up test artifacts**

Run: `rm -f test-output.txt ./research-demo`

- [ ] **Step 6: Document manual test results**

No commit needed - this is manual verification only.

---

### Task 9: Run Full Test Suite

**Files:**
- None (verification step)

- [ ] **Step 1: Run all unit tests**

Run: `make test`
Expected: All tests PASS

- [ ] **Step 2: Run linter**

Run: `make lint`
Expected: No errors or warnings

- [ ] **Step 3: Run full check**

Run: `make check`
Expected: All checks PASS (lint + test)

- [ ] **Step 4: Commit if any fixes were needed**

If any issues were found and fixed in previous steps:
```bash
git add <modified-files>
git commit -m "fix: address issues found in full test suite"
```

Otherwise, no commit needed.

---

## Self-Review Checklist

**Spec coverage:**
- ✅ Requirement 1 (Accumulate assistant text): Task 2
- ✅ Requirement 2 (Return on error): Task 3
- ✅ Requirement 3 (Print with labeling): Task 4
- ✅ Requirement 4 (Backward compatibility): Maintained - Run() signature unchanged
- ✅ Requirement 5 (Edge cases): Handled in Task 3 Step 3 (finishResult priority)
- ✅ Testing Strategy: Unit tests (Tasks 5-7), Integration test (Task 8)

**Placeholder scan:**
- ✅ No TBD, TODO, or "implement later" markers
- ✅ All code blocks are complete
- ✅ All test expectations are explicit

**Type consistency:**
- ✅ `partialResult` is `string` type throughout
- ✅ `finishResult` is `string` type (existing)
- ✅ `assistantText` is `string` type (existing)
- ✅ All consistent across tasks

**Commands and paths:**
- ✅ All file paths are exact
- ✅ All commands are complete with expected output
- ✅ Test commands use `make` targets per CLAUDE.md
