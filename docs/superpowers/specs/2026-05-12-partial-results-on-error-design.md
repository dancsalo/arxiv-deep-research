# Partial Results on Error Design

**Date:** 2026-05-12  
**Status:** Approved  
**Owner:** Claude Code

## Problem Statement

When the research-demo agent hits an error (cost limit exceeded, context limit reached, API failure), it exits immediately without showing any of the research it gathered. Users see only an error message with no insight into what the agent discovered before failing.

In the provided example, the agent ran for 8 turns, made multiple API calls, and gathered research across arXiv, OpenAlex, GitHub, and web sources before hitting the $1.00 cost limit. All of that work was lost because the error handler couldn't access partial results.

## Solution

Track accumulated assistant text throughout loop execution and return it when errors occur, allowing the error handler to print partial research summaries instead of just error messages.

## Requirements

1. Accumulate all assistant text across turns during loop execution
2. Return accumulated text when any error terminates the loop
3. Print partial results with clear "Partial" labeling in main.go
4. Maintain backward compatibility with existing Loop API
5. Handle edge cases: empty results, finish tool called before error

## Architecture

### High-Level Flow

```
Loop.Run()
  → For each turn:
      → Call LLM
      → Append assistant text blocks to Loop.partialResult
      → Check for errors (cost limit, context limit, etc.)
      → If error: return (partialResult, error)
      → If finish_loop called: return (finishResult, nil)
  → End of loop: return (finishResult or partialResult, nil)

main.go error handler:
  → If result non-empty: print as "Partial Research Summary"
  → Print error
  → Print stats (cost, time, trace)
```

### Component Changes

**Modified files:**

1. **internal/agentic/types.go**
   - Add `partialResult string` field to `Loop` struct
   - This accumulates assistant text across all turns

2. **internal/agentic/agentic.go**
   - In `Run()`, after processing each LLM response, append `assistantText` to `l.partialResult`
   - On all error return paths, return `l.partialResult` instead of empty string
   - Priority: if both `finishResult` and `partialResult` exist, prefer `finishResult`

3. **cmd/research-demo/main.go**
   - Update error handler to check if `result` is non-empty
   - If non-empty, print with "=== Partial Research Summary ===" header
   - Print error after partial summary
   - Print stats (elapsed, cost, trace) as usual

### Data Flow

**Normal execution (no error):**
```
Turn 0: assistantText="Searching arXiv..." → partialResult="Searching arXiv..."
Turn 1: assistantText="Found papers..." → partialResult="Searching arXiv...Found papers..."
Turn N: finish_loop(summary="...") → return (finishResult, nil)
```

**Error execution:**
```
Turn 0: assistantText="Searching arXiv..." → partialResult="Searching arXiv..."
Turn 1: assistantText="Found papers..." → partialResult="Searching arXiv...Found papers..."
Turn 2: cost check fails → return (partialResult, costError)
main.go: prints partial result + error
```

### Implementation Details

**Accumulation logic:**
- After each LLM response, concatenate all text blocks into `assistantText`
- Append `assistantText` to `l.partialResult` (even if empty this turn)
- This happens before error checking, ensuring we capture text even on the failing turn

**Error return priority:**
1. If `finishResult != ""`: return `finishResult` (agent explicitly finished)
2. Else if `l.partialResult != ""`: return `l.partialResult` (accumulated text)
3. Else: return `""` (no text generated, likely failed on turn 0)

**Output formatting:**
- Success: `=== Research Summary ===`
- Partial: `=== Partial Research Summary ===` with note "Research incomplete due to error"
- Both cases show stats and trace location

### Edge Cases

**No text generated:** If the loop fails on turn 0 before any LLM response (e.g., pre-API check failure), `partialResult` will be empty. Error handler prints only the error message. This is correct behavior.

**Multiple text blocks per turn:** Assistant responses can interleave text and tool_use blocks. We concatenate all text blocks from each turn, preserving order.

**Finish tool called, then error:** If the agent calls `finish_loop` successfully but then the cost limit check fails, we have both `finishResult` and `partialResult`. Return `finishResult` because it's the agent's explicit summary, not just accumulated responses.

**Empty assistant text on a turn:** Some turns may have only tool calls with no text. Appending empty string to `partialResult` is harmless.

**Context compaction:** If turns get compacted during execution, their assistant text is already in `partialResult` and won't be lost.

## Testing Strategy

**Unit tests (internal/agentic/agentic_test.go):**
- Test that `partialResult` accumulates across successful turns
- Test that errors return `partialResult` instead of empty string
- Test that `finishResult` takes precedence over `partialResult`

**Integration test:**
- Run research-demo with artificially low cost limit (e.g., $0.05)
- Verify it prints partial results before exiting with cost error
- Verify the partial output contains text from early turns

**Manual verification:**
- Run the failing command from the issue: `./research-demo --query "RL for training agents" --prompt-variant B --max-turns 12`
- Confirm partial research summary prints before error
- Check that trace file still gets written correctly

## Success Criteria

1. When cost/context limits are exceeded, partial results print to stdout
2. Error message still appears (after partial summary)
3. Stats (time, cost, trace) print for both success and failure cases
4. No breaking changes to Loop API (still returns `(string, error)`)
5. Tests verify accumulation and error-path behavior
