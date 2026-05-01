# Implementation Summary: Context Window Management

Source plan: `.claude/plans/context-management.md`

**PR:** [#12](https://github.com/dancsalo/arxiv-deep-research/pull/12) on branch `feat/context-management`
**Location:** `src/` — standalone Go module (`module context-management`)
**Stats:** 2,702 lines of Go across 17 files (8 source, 9 test), 95 tests passing

---

## Files

| File | Lines | Purpose |
|------|-------|---------|
| `estimator.go` | 123 | `TokenEstimator` — per-content-type heuristic ratios, optional exact counting via Anthropic API, EMA calibration, `classifyContent` |
| `budget.go` | 64 | `ContextBudget` (Available/Remaining arithmetic), `OutputTracker` (p95-based adaptive output reservation, rolling window of 20) |
| `turn.go` | 64 | `Turn` struct, `MessageLog` with `AddTurn`/`GetTurn`/`BuildMessages` — maps directly to Claude API message pairs |
| `manager.go` | 196 | `ContextManager` — orchestrates estimator, budget, message log; cached token counts with dirty flag and 30s TTL; text extraction from SDK message types |
| `compaction.go` | 256 | 3-level compaction (extractive → Haiku summarization → tombstone), `autoCompact` by priority/age, `CompactConversationHistory`, chunked summarization with depth limit |
| `toolsize.go` | 65 | `ToolSizeEstimator` with static registration and `RollingAvg` history (prefers history after 3 observations) |
| `guardrail.go` | 155 | `PreToolGuardrail` (check → reduce args → compact → reject), `PostToolCheck`, `PreAPICheck`, `reduceToolArgs` for fetch_page_text and search_arxiv |
| `loop.go` | 213 | `ResearchLoop` with 4 termination conditions, `BuildFinishTool`, `estimateCost`, `responseToAssistantParam` |

---

## Commits

1. **Foundation** (sections 1–4): TokenEstimator, ContextBudget, OutputTracker, Turn, MessageLog, ContextManager
2. **Compaction** (sections 5–6): extractive (L1), summarization via Haiku (L2), tombstone (L3), auto-compaction policy, ToolSizeEstimator
3. **Critique fixes**: recursion depth limit, error reporting in autoCompact, turn re-indexing, applyCompaction helper
4. **Guardrails** (section 7): PreToolGuardrail, PostToolCheck, PreAPICheck, reduceToolArgs
5. **ResearchLoop** (sections 8–9): agentic loop, finish_research tool, cost tracking, responseToAssistantParam
6. **Integration tests** (section 10): budget pressure, calibration convergence, conversation compaction, full happy path, guardrail+compaction chain

---

## Deviations from plan

| Plan spec | Actual implementation | Reason |
|-----------|----------------------|--------|
| `Calibrate(text, actualTokens)` auto-classifies | `Calibrate(text, actualTokens, ContentType)` — caller specifies type | Code review: auto-classification silently miscalibrates when mixed content is passed |
| `EstimateFast` uses `len(text)` (byte length) | Uses `utf8.RuneCountInString(text)` (rune count) | Code review: byte length overestimates for Unicode-heavy scientific papers |
| `classifyContent` checks code before JSON | Checks JSON first (structural), then code (keyword) | Code review: JSON containing code-like strings was misclassified |
| `CountExact` accepts `[]ToolUnionParam` | Accepts `[]MessageCountTokensToolUnionParam` | SDK uses a different type for the count_tokens endpoint |
| `resp.ToParam()` for assistant message conversion | Custom `responseToAssistantParam()` | `ToParam()` requires raw JSON which test mocks don't have |
| `summarizeChunked` recurses freely | Capped at `maxSummarizeDepth = 3` | Code review: unbounded recursion if model returns oversized summaries |
| `autoCompact` silently skips errors | Captures first error and includes in final error message | Code review: caller had no visibility into why compaction failed |
| Turn indices preserved after compaction | Re-indexed sequentially after `CompactConversationHistory` | Code review: stale indices caused inconsistencies |

---

## What's not built yet

- **Real tool implementations** (search_arxiv, fetch_page_text) — the loop accepts a `ToolExecutor` function, tools plug in externally
- **Bedrock client wiring** — the module defines interfaces (`TokenCounterClient`, `CompactionClient`, `MessageClient`), concrete clients are injected by the caller
- **Example 03** — a runnable demo that wires everything together with real API calls
- **Concurrency safety** — single-goroutine design; add a mutex if the manager is ever shared across goroutines
