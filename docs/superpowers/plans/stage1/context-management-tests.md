# Test Plan: Context Window Management

Source plan: `.claude/plans/context-management.md`

## Test environment
- Language: Go
- Framework: `go test` (standard library `testing` package)
- Test location: Same package as implementation (e.g., `examples/03-context-management/` or a dedicated module). Each `_test.go` file alongside its source file.
- Run command: `make test`
- Mocking strategy: Interface-based dependency injection. Define interfaces for the Claude API client (`Messages.CountTokens`, `Messages.New`) and inject test doubles. No external mocking framework — use hand-rolled stubs implementing the interfaces.

## Components to test

### 1. TokenEstimator

**What it does:** Estimates token counts from text using per-content-type character ratios, with optional exact counting via the Anthropic API.
**Source plan section:** Section 1

| # | Behavior | Input | Expected Output | Type |
|---|----------|-------|-----------------|------|
| 1 | EstimateFast returns chars/ratio for prose | `"Hello world, this is a test."` (28 chars), `ContentProse` | `28 / 4.0 = 7` | unit |
| 2 | EstimateFast returns chars/ratio for JSON | `'{"key":"value","num":42}'` (24 chars), `ContentJSON` | `24 / 3.0 = 8` | unit |
| 3 | EstimateFast returns chars/ratio for code | `"func main() { fmt.Println() }"` (31 chars), `ContentCode` | `31 / 3.5 = 8` (truncated int) | unit |
| 4 | EstimateFast uses 3.5 fallback for unknown ContentType | `"test"` (4 chars), `ContentType(99)` | `4 / 3.5 = 1` (truncated int) | edge |
| 5 | EstimateFast on empty string | `""`, `ContentProse` | `0` | edge |
| 6 | CountExact returns error when hasDirect is false | Any valid messages | `0, error containing "unavailable on Bedrock"` | unit |
| 7 | CountExact calls API and returns token count | Messages with known content, mock client returns `InputTokens: 150` | `150, nil` | unit |
| 8 | CountExact propagates API errors | Mock client returns error | `0, error wrapping original` | unit |
| 9 | Calibrate updates ratio with EMA | text=`"abcdefghij"` (10 chars), actualTokens=`5`, ContentProse (old ratio 4.0) | new ratio = `0.8*4.0 + 0.2*(10/5) = 3.6` | unit |
| 10 | Calibrate is no-op when actualTokens is 0 | `"text"`, `0` | ratios unchanged | edge |
| 11 | Calibrate converges after many calls | 10 calls with consistent observed ratio of 3.0 for ContentProse | ratio approaches 3.0 (within 0.3 of target) | unit |

**Mocking needed:** `anthropic.Client.Messages.CountTokens` — mock the HTTP response or inject an interface that wraps this call.
**Setup/fixtures:** Pre-configured `TokenEstimator` with default ratios; one with `hasDirect=true` and mock client; one with `hasDirect=false`.

---

### 2. ContextBudget

**What it does:** Computes available token budget from model limits, output reservation, system tokens, and safety margin.
**Source plan section:** Section 2

| # | Behavior | Input | Expected Output | Type |
|---|----------|-------|-----------------|------|
| 1 | Available computes correct headroom | `ModelContextLimit=200000, MaxOutputTokens=16000, SystemTokens=1000, ToolDefTokens=500, SafetyMargin=8000` | `200000 - 16000 - 1000 - 500 - 8000 = 174500` | unit |
| 2 | Remaining subtracts current usage | Available=`174500`, currentTokens=`50000` | `124500` | unit |
| 3 | Remaining returns negative when over budget | Available=`174500`, currentTokens=`200000` | `-25500` | edge |
| 4 | Available with zero system/tool tokens | `SystemTokens=0, ToolDefTokens=0` | `200000 - 16000 - 0 - 0 - 8000 = 176000` | edge |

**Mocking needed:** None — pure arithmetic.
**Setup/fixtures:** Pre-built `ContextBudget` structs with known values.

---

### 3. OutputTracker

**What it does:** Tracks output token observations and recommends adaptive output reservation based on p95.
**Source plan section:** Section 2

| # | Behavior | Input | Expected Output | Type |
|---|----------|-------|-----------------|------|
| 1 | RecommendedReservation defaults to 16000 with no observations | No `Record` calls | `16000` | unit |
| 2 | Record stores observations | `Record(500)`, `Record(1000)` | `len(observations) == 2` | unit |
| 3 | RecommendedReservation returns 4096 when p95 is below it | 10 observations all `100` | `4096` | unit |
| 4 | RecommendedReservation returns p95 when above 4096 | 20 observations: 19 at `5000`, 1 at `15000` | p95 value (the 15000 observation at index 19) | unit |
| 5 | Record drops oldest when exceeding 20 | 25 `Record` calls with values 1..25 | `len(observations) == 20`, first value is `6` | unit |
| 6 | maxObserved tracks lifetime max | `Record(100)`, `Record(500)`, `Record(200)` | `maxObserved == 500` | unit |

**Mocking needed:** None — pure data structure.
**Setup/fixtures:** Fresh `OutputTracker` instances.

---

### 4. Turn and BuildMessages

**What it does:** Represents a turn (assistant response + tool results) and assembles the full message history.
**Source plan section:** Section 3

| # | Behavior | Input | Expected Output | Type |
|---|----------|-------|-----------------|------|
| 1 | BuildMessages returns initial user message when no turns | Initial message = `"What is quantum computing?"` | Slice with 1 message (the user query) | unit |
| 2 | BuildMessages includes assistant + tool result for a turn with tools | 1 turn: assistant with tool_use, tool result with tool_result | Slice: [user, assistant, toolresult] — 3 messages | unit |
| 3 | BuildMessages omits tool result when nil | 1 turn: assistant only (text response, no tool use) | Slice: [user, assistant] — 2 messages | unit |
| 4 | BuildMessages preserves ordering across multiple turns | 3 turns with various tool/no-tool combos | Messages in strict index order, tool results immediately after their assistant message | integration |
| 5 | Turn with CompactionLevel 0 has Compacted=false | Freshly created turn | `Compacted == false, CompactionLevel == 0` | unit |

**Mocking needed:** None — struct assembly. Use `anthropic.NewAssistantMessage`, `anthropic.NewUserMessage` to build test data.
**Setup/fixtures:** Helper function to create turns with known content.

---

### 5. ContextManager — Token Counting and Caching

**What it does:** Manages cached token counts, falls back to heuristic, and triggers calibration from exact counts.
**Source plan section:** Sections 1 and 4

| # | Behavior | Input | Expected Output | Type |
|---|----------|-------|-----------------|------|
| 1 | GetTokenCount returns cached value when not dirty and recent | `cached = {tokens: 5000, dirty: false, countedAt: now}` | `5000, nil` — no API call made | unit |
| 2 | GetTokenCount calls CountExact when dirty and hasDirect | `cached.dirty = true`, mock returns 6000 | `6000, nil`, cache updated, `Calibrate` called | unit |
| 3 | GetTokenCount falls back to heuristic when CountExact errors | `hasDirect=true`, mock returns error | Heuristic estimate (non-zero int), `nil` error | unit |
| 4 | GetTokenCount uses heuristic when hasDirect is false | `hasDirect=false`, dirty cache | Heuristic estimate, nil | unit |
| 5 | GetTokenCount recalculates when cache older than 30s | `countedAt = 31 seconds ago`, `dirty=false` | Fresh count (exact or heuristic) | unit |
| 6 | AddTurn marks cache dirty | Add a turn | `cached.dirty == true` | unit |

**Mocking needed:** Mock the token counting API client (interface-based).
**Setup/fixtures:** `ContextManager` with mock estimator, pre-populated turns.

---

### 6. Compaction — Levels and Strategies

**What it does:** Compacts turns through 4 levels: full → extractive → summary → tombstone.
**Source plan section:** Sections 4 and 6

| # | Behavior | Input | Expected Output | Type |
|---|----------|-------|-----------------|------|
| 1 | extractiveTruncate returns original when under limit | `"short text"`, maxTokens=`100` | `"short text"` (unchanged) | unit |
| 2 | extractiveTruncate truncates and appends marker | 1000-char string, maxTokens=`50` (→ 175 chars) | String of 175 chars + `"\n[...truncated]"` | unit |
| 3 | CompactTurn at level 0 advances to level 1 | Turn with level 0, 2000 estimated tokens | Level becomes 1, EstimatedTokens decreases, Compacted=true, ToolResults=nil | unit |
| 4 | CompactTurn at level 2 advances to level 3 (tombstone) | Turn with level 2 | Level becomes 3, content is a one-line string | unit |
| 5 | CompactTurn at level 3 is a no-op | Turn already at level 3 | No change, no error | edge |
| 6 | summarize calls Haiku API with correct prompt | Content string, level 1 | Mock Haiku client receives call with target chars = `len(content) / (2*(1+1))` | unit |
| 7 | summarize returns error when API fails | Mock returns error | Error wrapping `"summarization API call failed"` | unit |
| 8 | summarizeChunked splits large content | Content of 200,000 chars (exceeds `maxCompactionInputTokens * 3.5`) | `summarize` called multiple times, then combined summary | integration |
| 9 | summarizeChunked with single chunk delegates to summarize | Content of 50,000 chars (under threshold) | Single summarize call | edge |

**Mocking needed:** Mock the Haiku compaction client (`compactionClient.Messages.New`). Return deterministic summary strings.
**Setup/fixtures:** `ContextManager` with mock compaction client. Turns pre-populated with known content of specific lengths.

---

### 7. Auto-Compaction Policy

**What it does:** Frees tokens by compacting turns in priority/age order until the requested space is recovered.
**Source plan section:** Section 6.4

| # | Behavior | Input | Expected Output | Type |
|---|----------|-------|-----------------|------|
| 1 | autoCompact frees enough tokens from low-priority turns | 5 turns (3 supplementary, 2 core), need 5000 tokens freed | Returns indices of compacted supplementary turns, nil error | integration |
| 2 | autoCompact compacts oldest first within same priority | 3 turns all PrioritySupplementary (indices 0,1,2), need 2000 tokens | Compacts index 0 first, then 1 if needed | integration |
| 3 | autoCompact returns error when insufficient space | 2 turns already at level 3, need 10000 tokens | Error containing `"could only free"` | integration |
| 4 | autoCompact skips turns already at level 3 | Mix of levels: one at 3, two at 0 | Only level-0 turns are compacted | unit |
| 5 | autoCompact escalates levels progressively | Turn at level 0, need more tokens than level 1 frees | Turn advances from 0→1, then 1→2 on second pass if needed | integration |
| 6 | compactionCandidates returns sorted order | Turns with mixed priorities and ages | Sorted: lowest priority first, then oldest first within priority | unit |

**Mocking needed:** Mock compaction client for level-2 summarization calls.
**Setup/fixtures:** `ContextManager` with multiple turns of known sizes and priorities.

---

### 8. CompactConversationHistory

**What it does:** Collapses all turns except the most recent 3 into a single summary turn.
**Source plan section:** Section 4.4

| # | Behavior | Input | Expected Output | Type |
|---|----------|-------|-----------------|------|
| 1 | Compacts old turns, keeps recent 3 | 6 turns total | `len(turns) == 4` (1 summary + 3 recent), summary turn has `CompactionLevel=2, Priority=PriorityCore` | integration |
| 2 | No-op when 3 or fewer turns | 3 turns | No change, nil error | edge |
| 3 | No-op when all old turns are tombstones | 5 turns, first 2 at level 3 | No change (no compactable turns) | edge |
| 4 | Summary turn index is 0 | 6 turns, compact | `turns[0].Index == 0` | unit |
| 5 | Cache marked dirty after compaction | 6 turns, compact | `cached.dirty == true` | unit |

**Mocking needed:** Mock compaction client for the summarize call.
**Setup/fixtures:** `ContextManager` with 6+ turns of known content.

---

### 9. ToolSizeEstimator

**What it does:** Estimates expected token count for a tool's result using static estimates and rolling historical averages.
**Source plan section:** Section 7

| # | Behavior | Input | Expected Output | Type |
|---|----------|-------|-----------------|------|
| 1 | Estimate uses static estimate when no history | `"search_arxiv"`, `{"max_results": 10}` | `500 + 10*800 = 8500` (per registration function) | unit |
| 2 | Estimate uses historical average when >= 3 observations | `"search_arxiv"` with 5 recorded values `[1000, 2000, 3000, 4000, 5000]` | `3000` (average) | unit |
| 3 | Estimate returns 5000 for unknown tool | `"unknown_tool"`, no registration, no history | `5000` | unit |
| 4 | RegisterTool stores estimator function | Register `"my_tool"` with `func(args) int { return 42 }`, then Estimate | `42` | unit |
| 5 | RollingAvg drops oldest past 20 | Record 25 values (1..25) | Avg of 6..25 = `15` (actually `(6+7+...+25)/20 = 310/20 = 15`) | unit |
| 6 | RollingAvg returns 0 with no values | Fresh RollingAvg | `Avg() == 0` | edge |
| 7 | Estimate prefers history over static after 3 observations | Register static=100, record 3 values of 500 | `500` (history wins) | unit |

**Mocking needed:** None — pure data structures and functions.
**Setup/fixtures:** Fresh `ToolSizeEstimator` with registered tools.

---

### 10. PreToolGuardrail

**What it does:** Decides whether a tool call should proceed, be modified, be deferred (after compaction), or be rejected.
**Source plan section:** Section 5

| # | Behavior | Input | Expected Output | Type |
|---|----------|-------|-----------------|------|
| 1 | Proceed when result fits comfortably | Estimated=1000, remaining=50000, safety=8000 | `{Proceed: true}` | unit |
| 2 | Reduce args when tight (fetch_page_text) | Estimated=40000, remaining=20000, tool=`"fetch_page_text"` | `{Proceed: true, ModifiedArgs: {max_chars: reduced value}}` | unit |
| 3 | Reject fetch_page_text when reduced max_chars < 1000 | remaining=500 | `{Proceed: false}` with reason | unit |
| 4 | Reduce search_arxiv max_results to 5 | tool=`"search_arxiv"`, args `{max_results: 20}`, tight budget | `ModifiedArgs["max_results"] == 5` | unit |
| 5 | Compact then proceed when reduction insufficient | Estimated=30000, remaining=10000, compaction frees 25000 | `{Proceed: true, Compacted: [indices]}` | integration |
| 6 | Reject when compaction insufficient | Estimated=100000, remaining=5000, compaction can only free 10000 | `{Proceed: false, Reason contains "only ... available after compaction"}` | integration |
| 7 | Propagate GetTokenCount error | Mock GetTokenCount returns error | Returns error | unit |

**Mocking needed:** Mock token counting, mock compaction client.
**Setup/fixtures:** `ContextManager` configured with known budget, turns, and tool size estimator.

---

### 11. PostToolCheck

**What it does:** After tool execution, records result size and triggers emergency compaction if budget is breached.
**Source plan section:** Section 5

| # | Behavior | Input | Expected Output | Type |
|---|----------|-------|-----------------|------|
| 1 | Records tool result size in history | tool=`"search_arxiv"`, result=3500-char string | `toolHistory["search_arxiv"]` has new entry | unit |
| 2 | No compaction when within budget | Remaining > SafetyMargin after recording | No compaction triggered, nil error | unit |
| 3 | Triggers compaction when remaining < safety margin | Remaining=1000, SafetyMargin=8000 | autoCompact called with `SafetyMargin*2`, returns nil error | integration |
| 4 | Returns error from GetTokenCount | Mock returns error | Error propagated | unit |

**Mocking needed:** Mock token counting, mock compaction client.
**Setup/fixtures:** `ContextManager` with tight budget and pre-existing turns.

---

### 12. PreAPICheck

**What it does:** Emergency compaction if token count exceeds available budget before an API call.
**Source plan section:** Section 5

| # | Behavior | Input | Expected Output | Type |
|---|----------|-------|-----------------|------|
| 1 | No-op when under budget | currentTokens=50000, available=174500 | nil | unit |
| 2 | Compacts to 80% of available when over | currentTokens=180000, available=174500 | autoCompact called, nil if successful | integration |
| 3 | Returns error when compaction fails | currentTokens=180000, compaction fails | Error containing `"pre-API compaction failed"` | integration |
| 4 | Propagates GetTokenCount error | Mock returns error | Error returned | unit |

**Mocking needed:** Mock token counting, mock compaction client.
**Setup/fixtures:** `ContextManager` near budget limit.

---

### 13. ResearchLoop

**What it does:** Orchestrates the full agentic loop: API calls, tool execution, guardrails, compaction, termination.
**Source plan section:** Section 8

| # | Behavior | Input | Expected Output | Type |
|---|----------|-------|-----------------|------|
| 1 | Terminates on finish_research tool | Mock API returns tool_use for finish_research with summary `"Quantum computing is..."` | Returns `"Quantum computing is..."`, nil | integration |
| 2 | Terminates at maxTurns | maxTurns=3, mock API never calls finish_research | Loop runs exactly 3 iterations, returns extracted answer | integration |
| 3 | Terminates on cost limit | maxCostUSD=0.01, mock API usage exceeds limit on turn 2 | Error containing `"cost limit exceeded"` | integration |
| 4 | Rejected tool returns error as tool_result | Mock API requests tool, guardrail rejects | Tool result contains `"REJECTED:"`, loop continues | integration |
| 5 | Modified tool args used for execution | Mock API requests `fetch_page_text`, guardrail reduces args | Tool executed with reduced `max_chars` | integration |
| 6 | Terminates when budget exhausted and compaction insufficient | Budget nearly full, compaction fails | Error from PreAPICheck | integration |
| 7 | Tool execution error returned as is_error tool_result | Mock tool executor returns error | Tool result has `is_error: true`, loop continues | integration |
| 8 | end_turn stop reason breaks loop | Mock API returns `StopReason: "end_turn"` with no tool use | Loop breaks, returns final answer | integration |
| 9 | Multiple tool uses in single response | Mock API returns 2 tool_use blocks | Both tools executed, both results recorded | integration |

**Mocking needed:** Full mock of `anthropic.Client.Messages.New` (returns scripted responses per turn). Mock tool executor function. Mock compaction client.
**Setup/fixtures:** `LoopConfig` with known limits. Scripted API responses that simulate multi-turn research conversations.

---

## Integration tests

| # | Scenario | Components involved | Setup | Verification |
|---|----------|-------------------|-------|-------------|
| 1 | Budget pressure triggers compaction mid-loop | ResearchLoop, ContextManager, autoCompact, ToolSizeEstimator | LoopConfig with small budget (e.g. ModelContextLimit=20000). Mock API returns large tool results that force compaction. | After loop completes: turns with low priority are compacted, final answer is returned, no panic. |
| 2 | Calibration improves estimates over time | TokenEstimator, ContextManager | Start with default ratios. Feed 10 exact counts where true ratio is 3.0 for prose. | `ratios[ContentProse]` converges toward 3.0 (within ±0.3). |
| 3 | Conversation history compaction fires when turns accumulate | ResearchLoop, CompactConversationHistory | 10+ turns, budget pressure | Old turns collapsed into summary, recent 3 preserved, message history still valid (user/assistant alternation). |
| 4 | Full happy path: query → research → finish | ResearchLoop, all components | Mock 3-turn conversation: search → fetch → finish_research | Returns summary string, cost tracked, turns recorded. |
| 5 | Guardrail + compaction + reduced args chain | PreToolGuardrail, autoCompact, reduceToolArgs | Near-budget state, large tool request | Args reduced first, if still tight compaction triggers, tool eventually proceeds or is rejected. |

## Test gaps

Behaviors in the plan that are **too vague to test** without further specification:

- **`classifyContent` function** (Section 1, Calibration): The plan says "heuristic: check for {}, code patterns, etc." but does not specify the classification rules. Tests for `Calibrate` can pass a known `ContentType`, but tests for automatic classification need the rules defined first.
- **`extractTurnText` function**: Referenced by `CompactTurn` and `autoCompact` but not defined in the plan. Need to know what text is extracted from a `Turn` (just assistant text? tool results too? JSON-serialized content blocks?).
- **`extractMultiTurnText` function**: Referenced by `CompactConversationHistory`, not defined. Same question as above but across multiple turns.
- **`estimateAll` function**: Referenced in `GetTokenCount` fallback path. Presumably sums `EstimateFast` over all turn content, but not specified.
- **`messagesText` function**: Referenced in `GetTokenCount` for calibration. Not defined.
- **`estimateCost` function**: Referenced in `ResearchLoop`. Pricing formula per model not specified.
- **`ExtractFinalAnswer` function**: Referenced as the return value of `ResearchLoop`. Not defined — is it the last text block? The finish_research summary?
- **`jsonToMap` and `copyArgs` utility functions**: Referenced but not defined. Presumably straightforward, but edge cases (nested objects, nil input) are unspecified.
- **`turnsOlderThan` function**: Used in `CompactConversationHistory`. Presumably returns turns with index < threshold, but the plan could mean age-based or position-based.
- **`SystemBlocks` function**: Referenced in `ResearchLoop` and `BuildMessages`. Not defined in the plan.
- **Priority assignment**: `AddTurn` is said to "assign priority" but the plan doesn't specify how priority is determined (caller passes it? heuristic based on tool names?).

## Verification strategy

How to confirm the implementation is correct:

- Run `make check` — this runs `go vet`, `go build`, and `go test` across all modules.
- All unit tests pass with `go test -v -race ./...` to catch data races (particularly in `OutputTracker`, `RollingAvg`, `CachedCount` if ever accessed concurrently).
- Integration tests use deterministic mock responses — no real API calls in CI.
- Coverage target: all public methods of `TokenEstimator`, `ContextBudget`, `OutputTracker`, `ContextManager`, `ToolSizeEstimator`, and `ResearchLoop` have at least one happy-path and one error-path test.
- Watch for: flaky tests from time-dependent logic (`CachedCount.countedAt` — use a clock interface or inject `time.Now` for deterministic testing of the 30-second cache expiry).
