# Agentic Loop: Context Manager + Memory Store Integration

## Problem Statement

The codebase has two powerful subsystems that currently operate independently:

1. **Context Manager** (`src/` package `contextmanager`) — manages conversation context with token budgets, compaction, guardrails, and turn tracking.
2. **Memory Store** (`src/memoryclient/`) — persistent PostgreSQL-backed memory with embedding, hybrid search, and structured memory types.

The existing loop in `loop.go` runs a tool-use loop with context management but has no memory integration. The memory tools exist (`src/tools/`) but are only wired up as passthrough tool handlers — the loop itself never proactively queries or stores memories.

**Goal:** Build an `AgenticLoop` struct that composes both subsystems into a general-purpose agentic framework. The agent should recall relevant memories before each LLM call and provide a hook point for persisting observations after tool execution — making memory a first-class participant in the loop rather than an optional tool the model may or may not invoke. The framework is domain-agnostic: callers register their own tools via a `ToolRegistry` and customize behavior via hooks.

## Requirements

### Functional

1. **Memory Recall (pre-LLM):** Before each API call, the loop queries the memory store for memories relevant to the current conversation state. Retrieved memories are injected into the initial user message as additional context blocks so the model has access to prior knowledge without using a tool call.

2. **Memory Persist Hook Point (post-turn):** After each complete turn, the loop calls an `OnMemoryPersist` hook. The loop itself does **not** decide what to persist — that logic is the caller's responsibility via the hook. v1 provides the hook point; callers wire in their own persist strategy (heuristic, LLM-driven, or none).

3. **Context-Aware Memory Budget:** Memory recall results consume context tokens. The context manager's existing budget system (`WillFit`) accounts for injected memories so they don't crowd out conversation history or tool results.

4. **Hook System:** The loop exposes named hook points that callers can customize. Hooks are synchronous functions called at specific points in the loop. They receive loop state and can modify behavior (e.g., skip memory recall on turn 0, filter recalled memories, implement custom persist logic).

5. **ToolRegistry:** The loop accepts a `ToolRegistry` of domain-specific tools. The registry maps tool names to definitions and handlers, and supports registration at setup time. The framework handles tool dispatch, guardrails, and result tracking generically. Callers register whatever tools their domain requires.

6. **Backward Compatibility:** The existing loop function signature and `ContextManager` API remain unchanged. The new `AgenticLoop` is a separate struct that composes them.

### Non-Functional

1. The loop must remain testable with mock clients (no real DB or API required for unit tests).
2. Memory operations must not block the critical path if the memory store is unavailable — degrade gracefully.
3. The design must support future addition of new hook points without breaking existing callers.

## Specs

### Loop Lifecycle

Each iteration of the `AgenticLoop` follows this sequence:

```
┌─────────────────────────────────────────────────────────┐
│                    AgenticLoop Turn N                     │
├─────────────────────────────────────────────────────────┤
│                                                          │
│  1. Hook: OnTurnStart(turnState)                         │
│                                                          │
│  2. MEMORY RECALL                                        │
│     a. Check turn % RecallEveryN == 0                    │
│     b. Build recall query from recent context            │
│     c. Search memory store (hybrid mode)                 │
│     d. Hook: OnMemoryRecall(memories, turnState)         │
│     e. Budget check via manager.WillFit()                │
│     f. Inject memories into initial user message copy    │
│                                                          │
│  3. CONTEXT CHECK                                        │
│     a. manager.PreAPICheck(ctx) — compact if needed      │
│                                                          │
│  4. LLM CALL                                             │
│     a. Copy messages from context manager                │
│     b. Insert memory block into copied messages          │
│     c. Call Claude API                                   │
│     d. Track cost, output tokens                         │
│     e. Check ctx.Err() for cancellation                  │
│                                                          │
│  5. TOOL EXECUTION (for each tool_use block)             │
│     a. Check for finish tool → set finished flag         │
│     b. Hook: OnToolCall(toolName, input, turnState)      │
│     c. PreToolGuardrail — budget check, arg reduction    │
│     d. Execute tool via ToolRegistry                     │
│     e. PostToolCheck — update size estimator, compact    │
│     f. Hook: OnToolResult(toolName, result, turnState)   │
│                                                          │
│  6. TURN RECORDING                                       │
│     a. manager.AddTurn(assistant, toolResults, priority) │
│        (priority from cfg.DefaultPriority)               │
│                                                          │
│  7. Hook: OnMemoryPersist(turnState)                     │
│     (caller decides what, if anything, to persist)       │
│                                                          │
│  8. TERMINATION CHECK                                    │
│     a. ctx.Err() != nil → return context error           │
│     b. finish tool called → break                        │
│     c. stop_reason == "end_turn" → break                 │
│     d. cost limit exceeded → error                       │
│     e. max turns exceeded → break                        │
│                                                          │
│  9. Hook: OnTurnEnd(turnState)                           │
│                                                          │
└─────────────────────────────────────────────────────────┘
```

### Core Types

```go
package contextmanager

// AgenticLoop is a stateful agentic loop that composes
// context management, memory recall, tool execution, and hooks.
// Instances are single-use: call Run() once per instance.
type AgenticLoop struct {
    client    MessageClient
    manager   *ContextManager
    registry  *ToolRegistry
    recaller  MemoryRecaller   // nil = no recall
    cfg       AgenticLoopConfig
    system    []anthropic.TextBlockParam
    hooks     *LoopHooks

    // internal state — reset at start of Run()
    query         string
    totalCostUSD  float64
    turnIndex     int
    finished      bool
    seenMemoryIDs map[int64]bool // deduplication
    logger        *slog.Logger
}

// NewAgenticLoop constructs the loop. All dependencies are injected here.
// Applies defaults to cfg: if MemoryRecall.Enabled is true and MaxResults is 0,
// defaults to 5 results, 2000 max tokens, "hybrid" mode, RecallEveryN=1.
// If cfg.DefaultPriority is 0, defaults to PriorityCore.
// If cfg.Logger is nil, uses slog.Default().
func NewAgenticLoop(
    client MessageClient,
    manager *ContextManager,
    registry *ToolRegistry,
    recaller MemoryRecaller,
    cfg AgenticLoopConfig,
    system []anthropic.TextBlockParam,
) *AgenticLoop

// Run executes the loop until completion, returning the final answer.
// Resets internal state (dedup set, cost, turn index) at the start.
// Safe to call only once per instance. For a new conversation, construct
// a new AgenticLoop.
func (a *AgenticLoop) Run(ctx context.Context, query string) (string, error)
```

```go
// AgenticLoopConfig holds all configuration for the loop.
type AgenticLoopConfig struct {
    MaxTurns        int
    MaxCostUSD      float64
    Model           anthropic.Model
    SessionID       string
    FinishTool      string       // tool name that signals completion
    DefaultPriority TurnPriority // priority for AddTurn, default PriorityCore

    // Memory recall settings
    MemoryRecall MemoryRecallConfig

    // Hooks — all optional, nil means no-op
    Hooks *LoopHooks

    // Logger — if nil, uses slog.Default()
    Logger *slog.Logger
}

type MemoryRecallConfig struct {
    Enabled      bool
    MaxResults   int    // default 5 (applied in constructor when 0 and Enabled)
    MaxTokens    int    // max tokens for injected memories, default 2000
    SearchMode   string // "hybrid", "semantic", "text" — default "hybrid"
    SkipFirstN   int    // skip recall for first N turns (default 0)
    RecallEveryN int    // recall every N turns after SkipFirstN (default 1 = every turn)
}
```

```go
// TurnState is passed to all hooks, giving them read access to loop state.
// Token counts use fast estimation (no API round-trip).
type TurnState struct {
    TurnIndex         int
    TotalCostUSD      float64
    TokensUsed        int               // fast estimate via estimator
    TokensRemaining   int               // fast estimate via budget
    LastToolCalls     []string           // tool names called this turn
    RecalledMemoryIDs []int64            // memory IDs injected this turn
    AssistantText     string             // assistant response text (post-turn hooks only)
    ToolResultTexts   map[string]string  // tool name → result text (post-turn hooks only)
}
```

```go
// LoopHooks defines all hook points. Each is optional (nil = no-op).
type LoopHooks struct {
    // Notification hooks — errors are logged, don't stop the loop.
    OnTurnStart  func(ctx context.Context, state TurnState) error
    OnTurnEnd    func(ctx context.Context, state TurnState) error
    OnToolCall   func(ctx context.Context, toolName string, input json.RawMessage, state TurnState) error
    OnToolResult func(ctx context.Context, toolName string, result string, state TurnState) error

    // Filter hook — can modify the recalled memories list.
    // Errors cause recall to be skipped (graceful degradation).
    OnMemoryRecall func(ctx context.Context, memories []RecalledMemory, state TurnState) ([]RecalledMemory, error)

    // Persist hook — caller implements their own persist logic here.
    // Receives full turn state including AssistantText and ToolResultTexts.
    // Errors are logged, don't stop the loop.
    OnMemoryPersist func(ctx context.Context, state TurnState) error
}
```

```go
// RecalledMemory is a memory retrieved for injection into context.
// Contains index-level data only — no full content (avoids detail-fetch round trip).
type RecalledMemory struct {
    ID    int64
    Type  string
    Title string
    Score float64
}
```

Note: `RecalledMemory` intentionally omits a `Tokens` field. Token budgeting is done on the rendered injection text (which includes formatting overhead), not on per-memory stored token counts.

### MemoryRecaller Interface

The loop depends on an interface, not the concrete `memoryclient.Client`. The interface returns index-level data only (title + type), not full content. This avoids the two-round-trip problem.

```go
// MemoryRecaller abstracts memory search for the loop.
type MemoryRecaller interface {
    RecallMemories(ctx context.Context, query string, mode string, limit int) ([]RecalledMemory, error)
}
```

The adapter (Section 5) wraps `memoryclient.Client.SearchMemories` and maps `MemoryIndexRow` → `RecalledMemory`. No detail fetch needed.

### ToolRegistry

```go
// ToolRegistry maps tool names to their definitions and handlers.
type ToolRegistry struct {
    tools map[string]RegisteredTool
    order []string // preserves registration order for Definitions()
}

type RegisteredTool struct {
    Definition anthropic.ToolUnionParam
    Handler    ToolHandler
}

// ToolHandler is the standard handler signature for registered tools.
// Receives context and raw JSON input (matching the Anthropic SDK's
// ToolUseBlock.Input field, which is json.RawMessage).
type ToolHandler func(ctx context.Context, input json.RawMessage) (string, error)

func NewToolRegistry() *ToolRegistry
func (r *ToolRegistry) Register(name string, def anthropic.ToolUnionParam, handler ToolHandler)
func (r *ToolRegistry) Definitions() []anthropic.ToolUnionParam

// Execute runs the handler for the named tool. Returns an error if the tool
// name is not registered. The loop is responsible for formatting the error
// as a tool result block for the model.
func (r *ToolRegistry) Execute(ctx context.Context, name string, input json.RawMessage) (string, error)

// WrapLegacyHandler adapts a func([]byte) (string, error) to ToolHandler.
// Since json.RawMessage is []byte, this is a direct pass-through.
func WrapLegacyHandler(h func([]byte) (string, error)) ToolHandler {
    return func(ctx context.Context, input json.RawMessage) (string, error) {
        return h([]byte(input))
    }
}
```

**`Execute` error contract:** When `name` is not found, `Execute` returns `("", fmt.Errorf("unknown tool: %s", name))`. The `AgenticLoop.Run()` method catches this error and formats it as `anthropic.NewToolResultBlock(id, "error: unknown tool: <name>", true)`. The loop does not crash.

### Memory Recall Strategy

The recall query is built from the conversation state, not a static string:

1. **Turn 0:** Use the original user query as the recall query.
2. **Turn N > 0:** Extract the last assistant response text (truncated to 200 chars) + the original query, combined. This keeps recall relevant as the conversation evolves. The original query is stored on the `AgenticLoop` struct (the `query` field), so `buildRecallQuery` doesn't need to extract it from the `ContextManager`'s private state.
3. **Frequency:** Recall runs when `turnIndex >= SkipFirstN` and `(turnIndex - SkipFirstN) % RecallEveryN == 0`. This gives callers control over recall frequency without writing a hook.
4. **Deduplication:** The `seenMemoryIDs` set on the struct tracks which memory IDs have been injected. Don't re-inject the same memory. The set is reset at the start of `Run()`.
5. **Injection format:** Memories are injected as **additional text blocks appended to a copy of the initial user message**. This preserves the Anthropic API's strict user/assistant alternation (no consecutive user messages). The format:

```
[Prior Knowledge]
- [gotcha] "pgvector index needs HNSW for >100k rows"
- [decision] "Use hybrid search as default mode"
[End Prior Knowledge]
```

**Critical: the loop copies the messages slice returned by `manager.BuildMessages()` before injecting.** The memory block is appended as a `TextBlock` to message index 0 (the initial user message) in the copy. The `ContextManager`'s internal state is never mutated.

6. **Token accounting:** Before injecting, render the memory block text and estimate tokens via `manager.estimator.EstimateFast(block, ContentProse)`. Check `manager.WillFit(memoryTokens)`. If it doesn't fit, reduce the number of memories (drop lowest-score first) until it fits or skip recall entirely. Since `AgenticLoop` is in the `contextmanager` package, it has access to the unexported `estimator` field. If `AgenticLoop` moves to its own package in the future, a public `EstimateTokens(text string) int` method should be added to `ContextManager`.

### Interaction: Proactive Recall vs. Memory Tools

The model has both proactive recall (loop-injected) and `search_memories` as a callable tool. The interaction model:

- **Proactive recall** provides compact one-liners (title + type only). It's a "here's what you might want to know" nudge.
- **`search_memories` tool** provides full search results and supports drill-down via `get_memory_details` / `get_memory_source`. It's for when the model wants to actively investigate.
- The system prompt should include a note: "You have been provided with relevant prior knowledge in the user message. Use the search_memories tool if you need deeper detail on any recalled item or want to search for something specific."
- Title quality is critical for proactive recall to be useful. The `store_memory` tool definition enforces 3-15 word titles that "name the specific entity and finding." If stored titles are low-quality, proactive recall degrades to noise. This is an upstream data quality concern, not something the loop can fix — but callers should be aware.

### Finish Tool Handling

The `AgenticLoopConfig.FinishTool` field names the tool that signals loop completion. The loop special-cases this tool by name in the tool execution phase:

1. When a `tool_use` block matches `cfg.FinishTool`, set `a.finished = true`.
2. Extract the tool input and return it as a tool result to the model.
3. After processing all tool_use blocks for this turn, the termination check sees `finished == true` and breaks.

This keeps the finish mechanism configurable — different applications can use different finish tool names and schemas. The finish tool's definition and handler are registered in the `ToolRegistry` like any other tool.

### Context Cancellation

The loop checks `ctx.Err()` at three points:
1. At the top of each iteration (before OnTurnStart hook).
2. After the LLM API call returns.
3. In the termination check (step 8).

If the context is cancelled, `Run()` returns `("", ctx.Err())`. Hooks also receive the context and should respect cancellation. Tool handlers receive it too. The loop does not attempt to save partial state on cancellation — the `ContextManager` already has all recorded turns from prior iterations.

### Graceful Degradation

If memory operations fail:
- **Recall failure:** Log via `a.logger`, proceed without injected memories. Don't fail the loop.
- **Nil recaller:** Skip the recall phase entirely. The loop works without memory.
- **Hook errors on filter hooks (`OnMemoryRecall`):** Skip recall for this turn. Log the error.
- **Hook errors on notification hooks:** Log, continue.
- **Hook errors on `OnMemoryPersist`:** Log, continue. Persistence is best-effort.

### Logging

The `AgenticLoop` uses `slog.Logger` (Go stdlib structured logging) for all operational output:

- **Turn start:** `logger.Info("turn.start", "turn", n, "tokens_used", x, "tokens_remaining", y)`
- **Memory recall:** `logger.Info("memory.recall", "turn", n, "query", q, "results", len(memories), "injected", len(filtered))`
- **Memory recall skip:** `logger.Debug("memory.recall.skip", "reason", "...")`
- **LLM call:** `logger.Info("llm.call", "turn", n, "model", model, "input_tokens", x, "output_tokens", y, "cost_usd", c)`
- **Tool execution:** `logger.Info("tool.execute", "turn", n, "tool", name, "latency_ms", d)`
- **Tool error (unknown):** `logger.Warn("tool.unknown", "tool", name)`
- **Hook errors:** `logger.Warn("hook.error", "hook", name, "err", err)`
- **Graceful degradation:** `logger.Warn("memory.recall.failed", "err", err)`
- **Context cancelled:** `logger.Info("loop.cancelled", "turn", n, "reason", ctx.Err())`

## Contracts

### AgenticLoop ↔ ContextManager

- Calls `manager.PreAPICheck()` before every LLM call (unchanged from existing loop).
- Calls `manager.AddTurn()` after every turn with `cfg.DefaultPriority` (configurable, default `PriorityCore`).
- Calls `manager.PreToolGuardrail()` / `manager.PostToolCheck()` around tool execution (unchanged).
- Calls `manager.WillFit(memoryTokens)` before injecting recalled memories. (`WillFit` already exists on `ContextManager` — this is its first use in the loop context.)
- Calls `manager.BuildMessages()` to get the message history. **The returned slice is copied before any injection.** The `ContextManager`'s internal state is never modified by the loop's memory injection.
- Accesses `manager.estimator.EstimateFast()` for token estimation (same-package access to unexported field). If `AgenticLoop` is extracted to its own package in the future, add a public `EstimateTokens(text string) int` to `ContextManager`.

### AgenticLoop ↔ MemoryRecaller

- Called once per qualifying turn (respecting `SkipFirstN` and `RecallEveryN`), before the LLM call.
- Input: query string, search mode, limit.
- Output: slice of `RecalledMemory` or error.
- The loop handles errors by skipping recall, never propagating.
- Returns index-level data only (title, type, score) — no content field, no second round trip.

### AgenticLoop ↔ ToolRegistry

- `registry.Definitions()` is called once at loop start to build the tool list for the API.
- `registry.Execute(ctx, name, input)` is called for each `tool_use` block.
- `Execute` returns an error for unknown tool names. The loop formats the error as a tool result block with `is_error: true`.
- The finish tool is registered like any other tool but is also special-cased by name for control flow.

### AgenticLoop ↔ LoopHooks

- All hooks are optional (nil check before call).
- Hooks receive `context.Context` and can respect cancellation.
- `OnMemoryRecall` is a filter hook — returns a modified slice. Errors cause recall to be skipped.
- `OnMemoryPersist` is a delegate hook — the caller implements persist logic. Errors are logged.
- `OnTurnStart`, `OnTurnEnd`, `OnToolCall`, `OnToolResult` are notification hooks — errors are logged but don't stop the loop.
- Hooks must not call `AgenticLoop` methods (no re-entrancy).
- `OnToolCall` receives `json.RawMessage` (not `map[string]any`) for consistency with `ToolHandler`.

## Examples

### Caller Setup

```go
// 1. Create Anthropic client
client := anthropic.NewClient(bedrock.WithLoadDefaultConfig(ctx))

// 2. Create context manager (existing setup)
initialMsg := anthropic.NewUserMessage(anthropic.NewTextBlock(query))
manager := contextmanager.NewContextManager(contextmanager.ContextManagerConfig{
    Estimator: contextmanager.NewTokenEstimator(nil),
    Budget:    &contextmanager.ContextBudget{ModelContextLimit: 200000, MaxOutputTokens: 8192, SafetyMargin: 2000},
    System:    systemBlocks,
    NowFunc:   time.Now,
}, initialMsg)

// 3. Build tool registry
registry := contextmanager.NewToolRegistry()
registry.Register("search_arxiv", searchArxivDef, searchArxivHandler)
registry.Register("fetch_page", fetchPageDef, fetchPageHandler)
registry.Register("finish", finishDef, finishHandler)
// Adapt existing memory tool handlers
for name, handler := range tools.NewMemoryToolHandlers(memClient, sessionID, turnCounter) {
    registry.Register(name, memToolDefs[name], contextmanager.WrapLegacyHandler(handler))
}

// 4. Create memory recaller adapter
recaller := memoryclient.NewRecallerAdapter(memClient)

// 5. Create and run the agentic loop
loop := contextmanager.NewAgenticLoop(
    &apiClient,    // MessageClient
    manager,
    registry,
    recaller,      // nil to disable recall
    contextmanager.AgenticLoopConfig{
        MaxTurns:        30,
        MaxCostUSD:      0.50,
        Model:           anthropic.ModelClaudeSonnet4_6,
        SessionID:       sessionID,
        FinishTool:      "finish",
        DefaultPriority: contextmanager.PriorityCore,
        MemoryRecall: contextmanager.MemoryRecallConfig{
            Enabled:      true,
            // MaxResults, MaxTokens, SearchMode, RecallEveryN
            // all default when zero-valued and Enabled is true
        },
        Hooks: &contextmanager.LoopHooks{
            OnMemoryPersist: myPersistHook,
        },
    },
    systemBlocks,
)

answer, err := loop.Run(ctx, query)
```

### Minimal Persist Hook

This example stores a memory whenever the assistant text contains a decision or finding. Callers can implement any strategy — this is the simplest useful version:

```go
func makePersistHook(memClient *memoryclient.Client, sessionID string) func(context.Context, contextmanager.TurnState) error {
    return func(ctx context.Context, state contextmanager.TurnState) error {
        if len(state.AssistantText) < 200 {
            return nil // skip short responses
        }

        // Simple keyword heuristic — callers can use LLM-based evaluation instead
        keywords := []string{"decided to", "key finding", "important:", "gotcha:", "discovered that"}
        found := ""
        for _, kw := range keywords {
            if strings.Contains(strings.ToLower(state.AssistantText), kw) {
                found = kw
                break
            }
        }
        if found == "" {
            return nil
        }

        // Truncate for title (caller is responsible for 3-15 word compliance)
        title := state.AssistantText
        if len(title) > 80 {
            title = title[:80]
        }

        _, err := memClient.StoreMemory(ctx, sessionID, state.TurnIndex, memoryclient.StoreMemoryInput{
            MemoryType: "discovery",
            Title:      title,
            Content:    state.AssistantText,
        })
        return err // logged by the loop, doesn't stop execution
    }
}
```

## Decisions & Tradeoffs

1. **Proactive recall vs. tool-only memory access:** We chose proactive recall (the loop queries memory on a configurable schedule) in addition to keeping memory tools available. Proactive recall provides a lightweight index (titles only); tools provide full retrieval. Tradeoff: uses context tokens even when memories aren't useful. Mitigated by `MaxTokens` cap, `SkipFirstN`, `RecallEveryN`, and the `OnMemoryRecall` filter hook.

2. **Persist as hook vs. built-in heuristic:** v1 provides only a hook point for persistence — no built-in heuristic. The caller decides what to persist. Tradeoff: more work for callers, but avoids shipping an untested heuristic that pollutes the memory store. The `TurnState` passed to `OnMemoryPersist` includes `AssistantText` and `ToolResultTexts` so the caller has full context.

3. **Interface-based memory access vs. direct `memoryclient` import:** The loop depends on a `MemoryRecaller` interface, not the concrete client. This keeps the loop testable and allows alternative backends. Tradeoff: requires a thin adapter to wrap `memoryclient.Client`.

4. **Struct with `Run()` vs. standalone function:** `AgenticLoop` is a struct, not a function. Dependencies are injected at construction, and `Run()` executes the loop. This avoids a 9+ parameter function, gives a natural home for loop state (deduplication set, cost accumulator, turn index), and supports future mid-loop inspection. Instances are single-use.

5. **Recalled memories as user message blocks vs. system prompt:** We inject memories as additional text blocks appended to a copy of the initial user message. This preserves Anthropic's system prompt caching (the system prompt stays stable across turns) and respects the API's strict user/assistant alternation rule (no consecutive user messages). Tradeoff: the model sees memories as "user-provided context" rather than "system knowledge," but the `[Prior Knowledge]` delimiter makes the role clear.

6. **ToolHandler signature:** The new `ToolHandler` takes `(ctx, json.RawMessage)` — compatible with the SDK's `ToolUseBlock.Input` field and supports context propagation. Existing handlers need a one-line wrapper via `WrapLegacyHandler`. Tradeoff: slight friction adapting old handlers, but cleaner going forward.

7. **Configurable finish tool:** The finish tool name is a config field (`FinishTool`), not hardcoded. Different applications can use different tool names and schemas. The loop special-cases it for control flow only.

8. **Configurable turn priority:** `DefaultPriority` in config (default `PriorityCore`) replaces any domain-specific hardcoded priority. Callers set the priority appropriate to their use case.

9. **Token counts in TurnState use fast estimation:** `TurnState.TokensUsed` and `TokensRemaining` are populated via the fast estimator, not the exact API-based counter. This avoids an API round-trip per hook call. Values are approximate (~15% margin). If callers need exact counts, they can call `manager.GetTokenCount(ctx)` in their hook.

10. **Same-package placement (for now):** `AgenticLoop` lives in the `contextmanager` package to access the unexported `estimator` field. This overloads the package's responsibilities (context management + orchestration). If the package grows further, `AgenticLoop` should move to its own `agenticloop` package, at which point a public `EstimateTokens` method should be added to `ContextManager`.

11. **Existing loop untouched:** We don't modify the existing loop function in `loop.go`. The `AgenticLoop` is additive. Existing callers are unaffected.

## Implementation Order

### Section 1: Core Types & Interfaces
- Define `AgenticLoopConfig`, `MemoryRecallConfig`, `TurnState`, `LoopHooks`
- Define `MemoryRecaller` interface and `RecalledMemory` type
- Define `AgenticLoop` struct and `NewAgenticLoop` constructor (with defaults)
- File: `src/agentic_types.go`

### Section 2: ToolRegistry
- Implement `ToolRegistry` struct with `Register`, `Definitions`, `Execute`
- Define `ToolHandler` type: `func(ctx context.Context, input json.RawMessage) (string, error)`
- Implement `WrapLegacyHandler` adapter
- File: `src/registry.go`

### Section 3: Memory Recall Logic
- Implement `buildRecallQuery(query string, lastAssistantText string, turnIndex int) string`
- Implement `buildMemoryBlock(memories []RecalledMemory) string` — formats the injection text
- Implement `injectMemories(messages []anthropic.MessageParam, block string) []anthropic.MessageParam` — copies messages slice, appends text block to message[0]
- Deduplication logic uses `a.seenMemoryIDs`
- File: `src/recall.go`

### Section 4: AgenticLoop.Run()
- Implement the main loop method, composing context manager, recall, registry, and hooks
- Wire hooks at each hook point with nil checks
- Handle finish tool detection via `cfg.FinishTool`
- Handle context cancellation checks at top of loop, after API call, and in termination
- Handle graceful degradation for nil recaller
- Wire structured logging via `slog.Logger`
- File: `src/agentic.go`

### Section 5: MemoryRecaller Adapter
- Implement adapter that wraps `memoryclient.Client.SearchMemories` to satisfy `MemoryRecaller`
- Maps `memoryclient.MemoryIndexRow` → `RecalledMemory` (direct field mapping, no detail fetch)
- File: `src/memoryclient/adapter.go`

## Open Questions

1. **Recall query quality:** "Last assistant text truncated to 200 chars + original query" is a reasonable starting point but untested against real memory stores. May need tuning after integration testing.

2. **Memory message position:** Currently appended as blocks to the initial user message. Could also be prepended. Worth testing whether position affects model attention to recalled memories.

3. **Concurrency:** The loop is single-threaded (one LLM call at a time). Parallel tool execution (like example 02) could be added later. Not in scope for v1.

## Revision Log

- v1 (initial): Plan created.
- v2 (revision 1): Major scope reduction and architectural changes based on dual critique.
  - **Cut:** Heuristic memory persist logic. v1 provides only the `OnMemoryPersist` hook point. Callers implement their own persist strategy. Rationale: untested heuristics risk polluting the memory store.
  - **Cut:** Stubbed domain tools. These are application-specific and don't belong in the generic framework.
  - **Changed:** `AgenticLoop` is now a struct with `Run()` method instead of a standalone function.
  - **Changed:** `RecalledMemory` no longer includes `Content` field. Recall returns index-level data only.
  - **Changed:** Memory injection moved from system prompt to user message blocks. Preserves prompt caching.
  - **Changed:** `ToolHandler` signature is `func(ctx, json.RawMessage)` — compatible with SDK's `ToolUseBlock.Input`.
  - **Changed:** Finish tool is configurable via `AgenticLoopConfig.FinishTool`.
  - **Added:** Structured logging spec via `slog.Logger`.
  - **Added:** Interaction model for proactive recall vs. memory tools.
  - **Added:** `WrapLegacyHandler` adapter for existing `tools.ToolHandler` functions.
  - **Removed:** `MemoryPersister` interface and `MemoryCandidate` type. Persistence fully delegated to hook.
  - **Removed:** All application-specific references. Framework is domain-agnostic.
- v3 (revision 2): Bug fixes and specification tightening from Round 2 critique.
  - **Fixed:** Memory injection now appends text blocks to a *copy* of the initial user message instead of inserting a separate user message. Avoids violating Anthropic API's strict user/assistant alternation.
  - **Fixed:** Turn priority is now configurable via `cfg.DefaultPriority` (default `PriorityCore`) instead of hardcoded domain-specific value.
  - **Fixed:** `seenMemoryIDs` is reset at the start of `Run()`. Documented that instances are single-use.
  - **Added:** `RecallEveryN` config field for recall frequency control without requiring a hook.
  - **Added:** Context cancellation handling — checked at top of loop, after API call, and in termination.
  - **Added:** Explicit `ToolRegistry.Execute` error contract — returns error for unknown tools, loop formats as tool result.
  - **Added:** `TurnState.TokensUsed`/`TokensRemaining` explicitly use fast estimation (no API round-trip).
  - **Added:** `OnToolCall` hook now receives `json.RawMessage` (consistent with `ToolHandler`).
  - **Added:** Caller Setup example showing end-to-end construction.
  - **Added:** Minimal Persist Hook example showing a simple keyword-based strategy.
  - **Added:** `MemoryRecallConfig` defaults applied in constructor when fields are zero-valued.
  - **Added:** Copy-before-inject contract for `BuildMessages()` output.
  - **Dropped:** `RecalledMemory.Tokens` field — unused since token budgeting is done on rendered text.
  - **Noted:** Package boundary tension (contextmanager doing double duty) acknowledged with future extraction path.
  - **Closed:** Open question about recall frequency — `RecallEveryN` config field added, hook available for dynamic control.
