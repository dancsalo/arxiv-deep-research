# Context Window Management for Agentic Loops

## Problem Statement

The current research pipeline is a fixed sequence where context accumulates unchecked. There is no mechanism to:

1. Know how much context space remains before making a tool call
2. Decide whether a tool call's result is worth the context it consumes
3. Shrink or restructure existing context to make room for new information

This plan designs a `ContextManager` in Go using `anthropic-sdk-go` that sits between the agent loop and the Claude API.

---

## Architecture Overview

```
┌──────────────────────────────────────────────────────┐
│                    Agent Loop                         │
│                                                      │
│  ┌────────────┐     ┌───────────────────┐            │
│  │  Claude API │◀───│  ContextManager   │            │
│  │  Messages   │───▶│                   │            │
│  └────────────┘     │  - budget         │            │
│       │             │  - messageLog     │            │
│       ▼             │  - estimator      │            │
│  ┌────────────┐     │  - guardrail      │            │
│  │  Tools      │───▶│                   │            │
│  └────────────┘     └───────────────────┘            │
└──────────────────────────────────────────────────────┘
```

**Control model: Manager-first.** The ContextManager owns the message history and enforces budget constraints. The agent (Claude) does not see budget information or make context decisions. The manager intercepts tool results, checks budget, and applies compaction or rejection before the next API call. Claude focuses on the research task; the manager handles the bookkeeping.

Why manager-first over agent-first: Claude will ignore budget hints in practice (critique #7). Giving the agent context management tools burns tokens on meta-reasoning that could go toward research. The guardrail is the real enforcement — make it the only mechanism.

---

## 1. Token Estimation

### Available APIs

Anthropic exposes a **free, exact token counting endpoint**:

- **Endpoint:** `POST https://api.anthropic.com/v1/messages/count_tokens`
- **Go SDK:** `client.Messages.CountTokens()` (verify exists in `anthropic-sdk-go` v1.38.0)
- **Cost:** Free (rate-limited: 100–8,000 RPM depending on tier)
- **Bedrock caveat:** Bedrock does **not** proxy this endpoint. If using Bedrock, exact counting is unavailable.

### Approach: Heuristic-primary, exact-optional

Because the project uses Bedrock, the heuristic estimator is the **primary** method. Exact counting is available only when using the direct Anthropic API and is used opportunistically (once per turn, not per tool call).

```go
type TokenEstimator struct {
    client       *anthropic.Client // nil if Bedrock
    model        anthropic.Model
    charsPerTok  float64           // calibrated over time, starts at 3.5
    hasDirect    bool              // true if direct API (not Bedrock)

    // Per-content-type ratios for better heuristic accuracy
    ratios       map[ContentType]float64
}

type ContentType int

const (
    ContentProse ContentType = iota
    ContentJSON
    ContentCode
    ContentMixed
)

var defaultRatios = map[ContentType]float64{
    ContentProse: 4.0,
    ContentJSON:  3.0,
    ContentCode:  3.5,
    ContentMixed: 3.5,
}

func (e *TokenEstimator) EstimateFast(text string, ct ContentType) int {
    ratio, ok := e.ratios[ct]
    if !ok {
        ratio = 3.5
    }
    return int(float64(len(text)) / ratio)
}

func (e *TokenEstimator) CountExact(
    ctx context.Context,
    messages []anthropic.MessageParam,
    system []anthropic.TextBlockParam,
    tools []anthropic.ToolUnionParam,
) (int, error) {
    if !e.hasDirect {
        return 0, fmt.Errorf("exact counting unavailable on Bedrock")
    }
    resp, err := e.client.Messages.CountTokens(ctx, anthropic.MessageCountTokensParams{
        Model:    e.model,
        Messages: messages,
        System:   system,
        Tools:    tools,
    })
    if err != nil {
        return 0, fmt.Errorf("count_tokens failed: %w", err)
    }
    return int(resp.InputTokens), nil
}
```

### When to use which

| Situation | Method | Why |
|-----------|--------|-----|
| Every budget check (default) | `EstimateFast()` | No network call, works on Bedrock |
| Once per turn, pre-API-call (direct API only) | `CountExact()` | Calibrate the heuristic, catch drift |
| After a surprisingly large tool result | `CountExact()` | Verify we haven't blown the budget |

### Caching exact counts

To avoid rate limit exhaustion, `CountExact` results are cached with a dirty flag:

```go
type CachedCount struct {
    tokens    int
    dirty     bool      // set true when messages change
    countedAt time.Time
}

func (m *ContextManager) GetTokenCount(ctx context.Context) (int, error) {
    if !m.cached.dirty && time.Since(m.cached.countedAt) < 30*time.Second {
        return m.cached.tokens, nil
    }
    if m.estimator.hasDirect {
        exact, err := m.estimator.CountExact(ctx, m.messages, m.system, m.tools)
        if err != nil {
            // Fall back to heuristic on error (rate limit, network, etc.)
            return m.estimateAll(), nil
        }
        m.cached = CachedCount{tokens: exact, dirty: false, countedAt: time.Now()}
        m.estimator.Calibrate(m.messagesText(), exact)
        return exact, nil
    }
    return m.estimateAll(), nil
}
```

Max 2 `CountExact` calls per turn (pre-API and post-oversize-result). At 20 turns that's 40 RPM — well within limits.

### Calibration

Per-content-type calibration instead of a single global ratio:

```go
func (e *TokenEstimator) Calibrate(text string, actualTokens int) {
    if actualTokens == 0 {
        return
    }
    ct := classifyContent(text) // heuristic: check for {}, code patterns, etc.
    observed := float64(len(text)) / float64(actualTokens)
    old := e.ratios[ct]
    e.ratios[ct] = 0.8*old + 0.2*observed
}
```

---

## 2. Context Budget Model

### Constants per model

| Model | Context Window | Safety Margin | Max Output | Available for Input |
|-------|---------------|---------------|------------|-------------------|
| Sonnet 4.6 | 200,000 | 8,000 | 16,000 | 176,000 |
| Haiku 4.5 | 200,000 | 8,000 | 16,000 | 176,000 |
| Opus 4.7 | 200,000 | 8,000 | 16,000 | 176,000 |

Safety margin is 8,000 (not 2,000) because:
- Primary estimation is heuristic, not exact
- Tool results can be 2-3x larger than predicted
- Compaction output size is unpredictable

```go
type ContextBudget struct {
    ModelContextLimit int
    MaxOutputTokens   int
    SystemTokens      int // measured once at init
    ToolDefTokens     int // measured once at init
    SafetyMargin      int
}

func (b *ContextBudget) Available() int {
    return b.ModelContextLimit - b.MaxOutputTokens - b.SystemTokens - b.ToolDefTokens - b.SafetyMargin
}

func (b *ContextBudget) Remaining(currentTokens int) int {
    return b.Available() - currentTokens
}
```

### Adaptive output reservation

Instead of a fixed 16,000 output reservation, track actual output usage and adapt:

```go
type OutputTracker struct {
    observations []int // recent output token counts
    maxObserved  int
}

func (t *OutputTracker) Record(outputTokens int) {
    t.observations = append(t.observations, outputTokens)
    if outputTokens > t.maxObserved {
        t.maxObserved = outputTokens
    }
    if len(t.observations) > 20 {
        t.observations = t.observations[1:]
    }
}

// Reserve max(p95 of observed, 4096) — enough for most turns, not wasteful
func (t *OutputTracker) RecommendedReservation() int {
    if len(t.observations) == 0 {
        return 16000 // conservative default
    }
    sorted := sortInts(t.observations)
    p95idx := int(float64(len(sorted)) * 0.95)
    p95 := sorted[p95idx]
    if p95 < 4096 {
        return 4096
    }
    return p95
}
```

---

## 3. Message Log (Replaces "Sections")

The previous plan used "sections" as a flat abstraction that didn't map to the Claude API's message format. This was the biggest architectural gap.

**The message log IS the message history.** The unit of management is a `Turn` — an assistant response + its corresponding tool results. This respects the `tool_use` ↔ `tool_result` ID binding that the API requires.

```go
type Turn struct {
    Index          int
    Assistant       anthropic.MessageParam   // the assistant's response (may contain tool_use blocks)
    ToolResults     *anthropic.MessageParam   // the user message with tool_result blocks (nil if no tools)
    EstimatedTokens int                       // combined estimate for both messages
    Compacted       bool                      // has this turn been summarized?
    CompactionLevel int                       // 0=full, 1=reduced, 2=summary, 3=tombstone
    Priority        TurnPriority
}

type TurnPriority int

const (
    PriorityCore          TurnPriority = 1 // user query, final synthesis
    PriorityResearch      TurnPriority = 2 // critique, analysis
    PrioritySupplementary TurnPriority = 3 // web fetches, extra searches
)
```

### Why turns, not sections?

- A `tool_use` block and its `tool_result` must be in adjacent messages. Evicting one without the other causes API validation errors.
- Turns can be compacted as a unit: replace the entire assistant+tool_result pair with a single user message containing a text summary of what happened.
- Turns have a natural ordering — the message history is sequential.
- Priority is assigned per-turn, not per-content-block, which avoids the problem of mixed-priority content within a single message.

### Compacting a turn

To compact turn N, replace the assistant message + tool result message with a **summary turn**: a single user message with a text block summarizing what the agent did and learned.

```go
func (m *ContextManager) CompactTurn(ctx context.Context, turnIdx int) error {
    turn := m.turns[turnIdx]
    if turn.CompactionLevel >= 3 {
        return nil // already a tombstone
    }

    content := m.extractTurnText(turn)
    summary, err := m.summarize(ctx, content, turn.CompactionLevel+1)
    if err != nil {
        return fmt.Errorf("compaction failed for turn %d: %w", turnIdx, err)
    }

    // Replace with summary message
    m.turns[turnIdx] = Turn{
        Index:           turn.Index,
        Assistant:        anthropic.NewAssistantMessage(anthropic.NewTextBlock(summary)),
        ToolResults:      nil,
        EstimatedTokens:  m.estimator.EstimateFast(summary, ContentProse),
        Compacted:        true,
        CompactionLevel:  turn.CompactionLevel + 1,
        Priority:         turn.Priority,
    }
    m.cached.dirty = true
    return nil
}
```

### Building messages from turns

```go
func (m *ContextManager) BuildMessages() []anthropic.MessageParam {
    // Start with the initial user message (the research query)
    msgs := []anthropic.MessageParam{m.initialUserMessage}

    for _, turn := range m.turns {
        msgs = append(msgs, turn.Assistant)
        if turn.ToolResults != nil {
            msgs = append(msgs, *turn.ToolResults)
        }
    }
    return msgs
}
```

No mapping ambiguity — turns ARE messages.

---

## 4. Context Operations

### 4.1 Turn Management

| Operation | Description |
|-----------|-------------|
| `AddTurn(assistant, toolResults)` | Record a new turn. Estimates tokens, assigns priority. |
| `GetTurn(index)` | Retrieve a turn by index. |
| `CompactTurn(ctx, index)` | Summarize a turn to the next compaction level. |
| `EvictTurn(index)` | Replace a turn with a one-line tombstone. |
| `BuildMessages()` | Assemble `[]anthropic.MessageParam` from all turns. |

### 4.2 Space-Making Operations

| Operation | Description | Token Impact |
|-----------|-------------|-------------|
| `CompactTurn(ctx, idx)` | Summarize one turn (level 0→1→2→3). | Reduces by ~50-80% per level |
| `CompactOldest(ctx, target)` | Compact turns oldest-first until under target. | Variable |
| `CompactByPriority(ctx, target)` | Compact lowest-priority turns first. | Variable |
| `EvictTurn(idx)` | Replace with tombstone: "Turn N: searched arXiv, found 10 papers." | Reclaims ~95% |

### 4.3 Budget Queries

| Operation | Description |
|-----------|-------------|
| `BudgetStatus(ctx)` | Total budget, used tokens, remaining, per-turn breakdown. |
| `WillFit(estimatedTokens)` | Can N tokens be added without compaction? |
| `SpaceNeeded(estimatedTokens)` | How many tokens must be freed to fit N new tokens? Returns 0 if it fits. |
| `EstimateToolResult(toolName, args)` | Predict tokens a tool will return. |

### 4.4 Conversation History Compaction

When the conversation itself gets long (many agent turns), the manager compacts older turns into a **conversation summary** — a system-level text block prepended to the message history.

```go
func (m *ContextManager) CompactConversationHistory(ctx context.Context) error {
    // Find turns older than the last 3 that aren't already tombstones
    compactable := m.turnsOlderThan(len(m.turns) - 3)
    if len(compactable) == 0 {
        return nil
    }

    // Summarize the old turns into a single narrative
    oldContent := m.extractMultiTurnText(compactable)
    summary, err := m.summarize(ctx, oldContent, 2)
    if err != nil {
        return fmt.Errorf("conversation compaction failed: %w", err)
    }

    // Replace old turns with a single summary turn
    summaryTurn := Turn{
        Index:           0,
        Assistant:        anthropic.NewAssistantMessage(anthropic.NewTextBlock(summary)),
        ToolResults:      nil,
        EstimatedTokens:  m.estimator.EstimateFast(summary, ContentProse),
        Compacted:        true,
        CompactionLevel:  2,
        Priority:         PriorityCore,
    }

    // Keep the summary + recent turns
    recent := m.turns[len(m.turns)-3:]
    m.turns = append([]Turn{summaryTurn}, recent...)
    m.cached.dirty = true
    return nil
}
```

---

## 5. Guardrail-Based Tool Control

The manager enforces budget constraints. Claude doesn't see costs or make context decisions.

### Pre-tool-call guardrail

Before executing any tool, the manager checks if the result will likely fit. If not, it compacts or rejects.

```go
type GuardrailDecision struct {
    Proceed     bool
    ModifiedArgs map[string]any // e.g., reduced max_chars
    Reason       string
    Compacted    []int          // turn indices that were compacted to make room
}

func (m *ContextManager) PreToolGuardrail(
    ctx context.Context,
    toolName string,
    args map[string]any,
) (GuardrailDecision, error) {
    estimated := m.EstimateToolResult(toolName, args)
    currentTokens, err := m.GetTokenCount(ctx)
    if err != nil {
        return GuardrailDecision{}, err
    }
    remaining := m.budget.Remaining(currentTokens)

    // Case 1: fits comfortably
    if estimated < remaining-m.budget.SafetyMargin {
        return GuardrailDecision{Proceed: true}, nil
    }

    // Case 2: tight — try reducing args first
    if reduced, ok := m.reduceToolArgs(toolName, args, remaining); ok {
        newEst := m.EstimateToolResult(toolName, reduced)
        if newEst < remaining-m.budget.SafetyMargin {
            return GuardrailDecision{
                Proceed:      true,
                ModifiedArgs: reduced,
                Reason:       fmt.Sprintf("reduced args to fit: %d tokens estimated", newEst),
            }, nil
        }
    }

    // Case 3: need to compact
    needed := m.SpaceNeeded(estimated)
    compactedIdxs, err := m.autoCompact(ctx, needed)
    if err != nil {
        return GuardrailDecision{
            Proceed: false,
            Reason:  fmt.Sprintf("compaction failed: %v", err),
        }, nil
    }

    // Re-check after compaction
    currentTokens, _ = m.GetTokenCount(ctx)
    remaining = m.budget.Remaining(currentTokens)
    if estimated < remaining {
        return GuardrailDecision{
            Proceed:   true,
            Compacted: compactedIdxs,
            Reason:    fmt.Sprintf("compacted %d turns to free space", len(compactedIdxs)),
        }, nil
    }

    // Case 4: even after compaction, won't fit
    return GuardrailDecision{
        Proceed: false,
        Reason: fmt.Sprintf(
            "need ~%d tokens, only %d available after compaction",
            estimated, remaining,
        ),
    }, nil
}

func (m *ContextManager) reduceToolArgs(
    toolName string,
    args map[string]any,
    remainingTokens int,
) (map[string]any, bool) {
    reduced := copyArgs(args)
    switch toolName {
    case "fetch_page_text":
        maxChars := int(float64(remainingTokens) * 0.5 * 3.5)
        if maxChars < 1000 {
            return nil, false
        }
        reduced["max_chars"] = maxChars
        return reduced, true
    case "search_arxiv":
        if v, ok := args["max_results"]; ok {
            if n, ok := v.(int); ok && n > 5 {
                reduced["max_results"] = 5
                return reduced, true
            }
        }
    }
    return nil, false
}
```

### Post-tool-call check

After a tool executes, verify the actual result size. If it blew past the estimate, compact immediately.

```go
func (m *ContextManager) PostToolCheck(
    ctx context.Context,
    toolName string,
    resultText string,
) error {
    actualTokens := m.estimator.EstimateFast(resultText, ContentJSON)

    // Update tool size history for future estimates
    m.toolHistory.Record(toolName, actualTokens)

    // If the result was way bigger than expected, try to rebalance
    currentTokens, err := m.GetTokenCount(ctx)
    if err != nil {
        return err
    }
    if m.budget.Remaining(currentTokens) < m.budget.SafetyMargin {
        _, err := m.autoCompact(ctx, m.budget.SafetyMargin*2)
        if err != nil {
            return fmt.Errorf("post-tool compaction failed: %w", err)
        }
    }
    return nil
}
```

### Pre-API-call check

Last chance before sending to Claude. Emergency compaction if over budget.

```go
func (m *ContextManager) PreAPICheck(ctx context.Context) error {
    currentTokens, err := m.GetTokenCount(ctx)
    if err != nil {
        return err
    }
    if currentTokens > m.budget.Available() {
        target := int(float64(m.budget.Available()) * 0.8)
        _, err := m.autoCompact(ctx, currentTokens-target)
        if err != nil {
            return fmt.Errorf("pre-API compaction failed, context too large (%d tokens): %w",
                currentTokens, err)
        }
    }
    return nil
}
```

---

## 6. Compaction Strategies

### 6.1 Summarization (via Claude)

Uses a **separate** Claude call with Haiku. The compaction call has its own budget concern: if the content to summarize is huge, chunk it.

```go
const maxCompactionInputTokens = 50_000 // Haiku can handle this comfortably

func (m *ContextManager) summarize(ctx context.Context, content string, level int) (string, error) {
    contentTokens := m.estimator.EstimateFast(content, ContentMixed)

    // If content is too large for a single compaction call, chunk it
    if contentTokens > maxCompactionInputTokens {
        return m.summarizeChunked(ctx, content, level)
    }

    targetChars := len(content) / (2 * (level + 1)) // progressively shorter per level
    prompt := fmt.Sprintf(
        "Summarize in under %d characters, preserving key facts, numbers, and conclusions:\n\n%s",
        targetChars, content,
    )

    resp, err := m.compactionClient.Messages.New(ctx, anthropic.MessageNewParams{
        Model:     anthropic.ModelClaudeHaiku4_5,
        MaxTokens: int64(targetChars / 3),
        Messages: []anthropic.MessageParam{
            anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
        },
    })
    if err != nil {
        return "", fmt.Errorf("summarization API call failed: %w", err)
    }

    for _, block := range resp.Content {
        if tb, ok := block.AsAny().(anthropic.TextBlock); ok {
            return tb.Text, nil
        }
    }
    return "", fmt.Errorf("summarization returned no text")
}

func (m *ContextManager) summarizeChunked(ctx context.Context, content string, level int) (string, error) {
    chunkSize := int(float64(maxCompactionInputTokens) * 3.5) // chars
    var summaries []string
    for i := 0; i < len(content); i += chunkSize {
        end := i + chunkSize
        if end > len(content) {
            end = len(content)
        }
        s, err := m.summarize(ctx, content[i:end], level)
        if err != nil {
            return "", err
        }
        summaries = append(summaries, s)
    }
    // If multiple chunks, summarize the summaries
    if len(summaries) > 1 {
        combined := strings.Join(summaries, "\n\n")
        return m.summarize(ctx, combined, level)
    }
    return summaries[0], nil
}
```

### 6.2 Extractive compression (no API call)

For structured content, extract key fields without calling Claude:

```go
func (m *ContextManager) extractiveTruncate(content string, maxTokens int) string {
    maxChars := int(float64(maxTokens) * 3.5)
    if len(content) <= maxChars {
        return content
    }
    return content[:maxChars] + "\n[...truncated]"
}
```

### 6.3 Compaction levels

| Level | Name | Strategy | Cost |
|-------|------|----------|------|
| 0 | Full | Original content | None |
| 1 | Reduced | Extractive truncation to ~50% | No API call |
| 2 | Summary | Claude Haiku summarization to ~20% | 1 API call |
| 3 | Tombstone | One line: "Turn N: [action] → [key finding]" | No API call |

Auto-compaction escalates through levels. Level 1 is tried first (free), then 2 (cheap), then 3 (lossy but maximal).

### 6.4 Auto-compaction policy

```go
func (m *ContextManager) autoCompact(ctx context.Context, tokensNeeded int) ([]int, error) {
    freed := 0
    var compactedIdxs []int

    // Sort turns by priority (lowest first), then by age (oldest first)
    candidates := m.compactionCandidates()

    for _, idx := range candidates {
        if freed >= tokensNeeded {
            break
        }
        turn := m.turns[idx]
        beforeTokens := turn.EstimatedTokens

        nextLevel := turn.CompactionLevel + 1
        if nextLevel > 3 {
            continue
        }

        var err error
        if nextLevel == 1 {
            // Extractive — no API call
            content := m.extractTurnText(turn)
            truncated := m.extractiveTruncate(content, beforeTokens/2)
            m.turns[idx].Assistant = anthropic.NewAssistantMessage(anthropic.NewTextBlock(truncated))
            m.turns[idx].ToolResults = nil
            m.turns[idx].EstimatedTokens = m.estimator.EstimateFast(truncated, ContentProse)
            m.turns[idx].CompactionLevel = 1
            m.turns[idx].Compacted = true
        } else {
            err = m.CompactTurn(ctx, idx)
        }

        if err != nil {
            // Log but continue — try other turns
            continue
        }

        freed += beforeTokens - m.turns[idx].EstimatedTokens
        compactedIdxs = append(compactedIdxs, idx)
    }

    if freed < tokensNeeded {
        return compactedIdxs, fmt.Errorf(
            "could only free %d of %d tokens needed (%d turns compacted)",
            freed, tokensNeeded, len(compactedIdxs),
        )
    }

    m.cached.dirty = true
    return compactedIdxs, nil
}
```

---

## 7. Tool Result Size Estimation

Tool size estimates are derived from tool definitions at registration time, not from a separate hardcoded table.

```go
type ToolSizeEstimator struct {
    // Static estimates derived from tool schema at registration
    staticEstimates map[string]func(args map[string]any) int

    // Observed sizes from actual tool calls (rolling window)
    history map[string]*RollingAvg
}

type RollingAvg struct {
    values []int
    sum    int
}

func (r *RollingAvg) Record(v int) {
    r.values = append(r.values, v)
    r.sum += v
    if len(r.values) > 20 {
        r.sum -= r.values[0]
        r.values = r.values[1:]
    }
}

func (r *RollingAvg) Avg() int {
    if len(r.values) == 0 {
        return 0
    }
    return r.sum / len(r.values)
}

func (e *ToolSizeEstimator) Estimate(toolName string, args map[string]any) int {
    // Prefer historical average if we have observations
    if h, ok := e.history[toolName]; ok && len(h.values) >= 3 {
        return h.Avg()
    }
    // Fall back to static estimate
    if fn, ok := e.staticEstimates[toolName]; ok {
        return fn(args)
    }
    return 5000 // default for unknown tools
}

// RegisterTool sets up static estimates based on known tool behavior.
// Called once during init, alongside tool definition.
func (e *ToolSizeEstimator) RegisterTool(name string, estimateFn func(args map[string]any) int) {
    e.staticEstimates[name] = estimateFn
}
```

Registration happens alongside tool definition — single source of truth:

```go
tools := buildTools()
sizeEstimator.RegisterTool("search_arxiv", func(args map[string]any) int {
    n, _ := args["max_results"].(float64)
    if n == 0 { n = 5 }
    return 500 + int(n)*800
})
sizeEstimator.RegisterTool("fetch_page_text", func(args map[string]any) int {
    chars, _ := args["max_chars"].(float64)
    if chars == 0 { chars = 4000 }
    return 100 + int(chars/3.5)
})
```

---

## 8. The Agentic Loop

### Termination conditions

The loop stops when any of these is true:
1. The agent calls `finish_research` tool (normal completion)
2. `maxTurns` is reached (default: 20)
3. Budget is exhausted AND compaction cannot free enough space
4. Total API cost exceeds a dollar limit

```go
type LoopConfig struct {
    MaxTurns     int
    MaxCostUSD   float64
    Model        anthropic.Model
}

type FinishResult struct {
    Summary string `json:"summary"`
}

func ResearchLoop(ctx context.Context, client *anthropic.Client, query string, cfg LoopConfig) (string, error) {
    manager := NewContextManager(client, cfg.Model)
    manager.SetInitialMessage(anthropic.NewUserMessage(anthropic.NewTextBlock(query)))

    tools := buildResearchTools()
    var totalCostUSD float64
    finished := false

    for turn := 0; turn < cfg.MaxTurns && !finished; turn++ {
        // Pre-API check: compact if needed
        if err := manager.PreAPICheck(ctx); err != nil {
            return "", fmt.Errorf("turn %d pre-API check failed: %w", turn, err)
        }

        messages := manager.BuildMessages()

        resp, err := client.Messages.New(ctx, anthropic.MessageNewParams{
            Model:     cfg.Model,
            MaxTokens: 8192,
            Tools:     tools,
            System:    manager.SystemBlocks(),
            Messages:  messages,
        })
        if err != nil {
            return "", fmt.Errorf("turn %d API error: %w", turn, err)
        }

        // Track cost
        totalCostUSD += estimateCost(resp.Usage, cfg.Model)
        if totalCostUSD > cfg.MaxCostUSD {
            return "", fmt.Errorf("cost limit exceeded: $%.4f > $%.4f", totalCostUSD, cfg.MaxCostUSD)
        }

        // Update output tracker
        manager.outputTracker.Record(int(resp.Usage.OutputTokens))

        // Process response
        var toolResults []anthropic.ContentBlockParamUnion

        for _, block := range resp.Content {
            switch b := block.AsAny().(type) {
            case anthropic.TextBlock:
                // Text output — could be final answer

            case anthropic.ToolUseBlock:
                if b.Name == "finish_research" {
                    finished = true
                    var result FinishResult
                    json.Unmarshal(b.Input, &result)
                    // Still need to return tool result for valid message history
                    toolResults = append(toolResults,
                        anthropic.NewToolResultBlock(b.ID, result.Summary, false),
                    )
                    continue
                }

                // Guardrail check
                args := jsonToMap(b.Input)
                decision, err := manager.PreToolGuardrail(ctx, b.Name, args)
                if err != nil {
                    toolResults = append(toolResults,
                        anthropic.NewToolResultBlock(b.ID, fmt.Sprintf("error: %v", err), true),
                    )
                    continue
                }
                if !decision.Proceed {
                    toolResults = append(toolResults,
                        anthropic.NewToolResultBlock(b.ID,
                            fmt.Sprintf("REJECTED: %s", decision.Reason), true),
                    )
                    continue
                }

                // Use modified args if guardrail adjusted them
                execArgs := args
                if decision.ModifiedArgs != nil {
                    execArgs = decision.ModifiedArgs
                }

                // Execute tool
                resultJSON, err := executeToolByName(b.Name, execArgs)
                if err != nil {
                    toolResults = append(toolResults,
                        anthropic.NewToolResultBlock(b.ID, fmt.Sprintf("error: %v", err), true),
                    )
                    continue
                }

                // Post-tool check
                if err := manager.PostToolCheck(ctx, b.Name, resultJSON); err != nil {
                    // Log but don't fail — the result is already computed
                }

                toolResults = append(toolResults,
                    anthropic.NewToolResultBlock(b.ID, resultJSON, false),
                )
            }
        }

        // Record the turn
        assistantMsg := resp.ToParam()
        var toolResultMsg *anthropic.MessageParam
        if len(toolResults) > 0 {
            msg := anthropic.NewUserMessage(toolResults...)
            toolResultMsg = &msg
        }
        manager.AddTurn(assistantMsg, toolResultMsg, PriorityResearch)

        if resp.StopReason == "end_turn" || finished {
            break
        }
    }

    return manager.ExtractFinalAnswer(), nil
}
```

### The `finish_research` tool

```go
func buildFinishTool() anthropic.ToolUnionParam {
    t := anthropic.ToolUnionParamOfTool(
        anthropic.ToolInputSchemaParam{
            Type: "object",
            Properties: map[string]any{
                "summary": map[string]any{
                    "type":        "string",
                    "description": "The final research summary",
                },
            },
            Required: []string{"summary"},
        },
        "finish_research",
    )
    t.OfTool.Description = anthropic.String(
        "Call this when research is complete. Provide the final synthesized summary.",
    )
    return t
}
```

---

## 9. Error Handling

Every fallible operation has an explicit error path. The general strategy:

| Error | Response |
|-------|----------|
| `CountExact` fails (rate limit, network) | Fall back to `EstimateFast` |
| Compaction API call fails | Skip that turn, try the next candidate |
| All compaction candidates exhausted | Return error to loop, which terminates gracefully |
| Tool execution fails | Return error as `tool_result` with `is_error: true` |
| Pre-API check fails (context too large, compaction insufficient) | Return error to loop |
| Tool result way larger than estimated | Trigger immediate compaction of oldest turns |
| Cost limit exceeded | Return error to loop |

The loop never panics. Every error either degrades gracefully (fallback) or terminates with a clear message.

---

## 10. Complete Operation Catalog

| # | Operation | Category | When to Use |
|---|-----------|----------|-------------|
| 1 | `AddTurn` | Turn mgmt | After each API response |
| 2 | `GetTurn` | Turn mgmt | Inspecting a specific turn |
| 3 | `CompactTurn` | Space-making | Summarize a turn to next level |
| 4 | `EvictTurn` | Space-making | Replace with tombstone |
| 5 | `CompactOldest` | Space-making | Free space, oldest first |
| 6 | `CompactByPriority` | Space-making | Free space, lowest priority first |
| 7 | `CompactConversationHistory` | Space-making | Summarize old turns into single narrative |
| 8 | `BudgetStatus` | Query | Check remaining tokens |
| 9 | `WillFit` | Query | Can N tokens be added? |
| 10 | `SpaceNeeded` | Query | How many tokens to free for N new tokens? |
| 11 | `EstimateToolResult` | Query | Predict tool output size |
| 12 | `GetTokenCount` | Query | Get token count (cached exact or heuristic) |
| 13 | `BuildMessages` | Assembly | Compose `[]anthropic.MessageParam` from turns |
| 14 | `PreToolGuardrail` | Guardrail | Check/compact before tool execution |
| 15 | `PostToolCheck` | Guardrail | Verify budget after tool result |
| 16 | `PreAPICheck` | Guardrail | Emergency compaction before API call |
| 17 | `autoCompact` | Policy | Free N tokens using best strategy |
| 18 | `Calibrate` | Maintenance | Update heuristic ratios from exact counts |
| 19 | `Record` (tool history) | Maintenance | Track actual tool result sizes |
| 20 | `RecommendedReservation` | Query | Adaptive output token reservation |

---

## 11. Implementation Order

1. **`TokenEstimator`** — `EstimateFast` with per-content-type ratios + optional `CountExact` for direct API. Write tests.
2. **`ContextBudget`** + `OutputTracker` — data structs, arithmetic. Write tests.
3. **`Turn`** struct + `BuildMessages()` — the core message-log abstraction.
4. **`ContextManager`** — CRUD operations, `GetTokenCount` with caching.
5. **Compaction** — levels 1 (extractive), 2 (summarization), 3 (tombstone). Including chunked summarization for large content.
6. **`ToolSizeEstimator`** — registration alongside tool definitions, rolling history.
7. **Guardrails** — `PreToolGuardrail`, `PostToolCheck`, `PreAPICheck`.
8. **`ResearchLoop`** — the agentic loop with termination conditions, cost tracking.
9. **`finish_research` tool** + other research tools ported to Go.
10. **Integration tests** — run the loop with mocked tool responses, verify compaction triggers at expected thresholds.

---

## 12. Decisions Made (from critique)

| Critique # | Issue | Resolution |
|------------|-------|------------|
| 1 | Python code, Go project | All code samples rewritten in Go |
| 2 | `count_tokens` may not work on Bedrock | Heuristic-primary, exact-optional. Explicit Bedrock fallback. |
| 3 | `count_exact` rate limit exhaustion | Cached with dirty flag, max 2 calls/turn |
| 4 | Sections vs. messages mismatch | Replaced sections with `Turn` — maps directly to message pairs |
| 5 | Compaction consumes context recursively | Chunked summarization, max input size, extractive level first |
| 6 | Menu injection costs tokens + breaks cache | Removed. Manager-first model — no budget injection. |
| 7 | Agent won't follow the menu | Removed agent-facing budget. Guardrail-only enforcement. |
| 8 | No error handling | Every operation returns `error`, fallback strategy per error type |
| 9 | No conversation history compaction | `CompactConversationHistory` collapses old turns into summary |
| 10 | No termination condition | `maxTurns`, `maxCostUSD`, `finish_research` tool, exhausted budget |
| 11 | Fixed output reservation wastes tokens | `OutputTracker` with adaptive p95-based reservation |
| 12 | `TOOL_ARG_PRESETS` second source of truth | Removed. Size estimators registered alongside tool definitions. |
| 13 | Single EMA broken for mixed content | Per-content-type ratios |
| 14 | Agent vs. manager — who's in charge? | Manager-first. Agent has no context tools. |
| 15 | Safety margin too small for heuristic | Raised to 8,000 tokens |
