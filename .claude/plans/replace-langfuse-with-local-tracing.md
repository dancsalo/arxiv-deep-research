# Plan: Replace Langfuse with Zero-Infrastructure Local Tracing

## Problem Statement

The current OTel → Langfuse tracing stack requires 5 Docker containers (Postgres, ClickHouse, MinIO, Redis, Langfuse server) and a separate worker process. The ingestion pipeline is unreliable in self-hosted mode (queue jobs sit unprocessed). We need tracing that works immediately, with zero infrastructure, and is queryable from the command line.

## Requirements

1. Remove all Langfuse and OpenTelemetry dependencies
2. Replace with structured JSON trace files written to disk (one file per run)
3. Preserve the same information captured today: turns, tool calls, LLM calls (model, tokens, cost), timing
4. Add error tracking (not present in old OTel hooks)
5. Zero external dependencies — no Docker, no databases, no network services
6. Integrate with the existing `LoopHooks` interface (no changes to core agentic loop)
7. Provide basic `make` targets for querying traces

## Design: JSON Trace Files

### Trace Format

Each run produces a single `.json` file in `.traces/`:

```
.traces/
  demo-1714934400000.json
  web-1714934402000123456.json
```

Filename: `{session_id}.json`

### Trace Schema

```json
{
  "session_id": "demo-1714934400000",
  "query": "retrieval augmented generation",
  "model": "us.anthropic.claude-3-5-haiku-20241022-v1:0",
  "started_at": "2026-05-05T19:00:00Z",
  "ended_at": "2026-05-05T19:00:12Z",
  "duration_ms": 12000,
  "status": "ok",
  "error": "",
  "total_input_tokens": 4500,
  "total_output_tokens": 1200,
  "total_cost_usd": 0.0032,
  "turns": [
    {
      "index": 0,
      "started_at": "...",
      "ended_at": "...",
      "duration_ms": 3000,
      "tokens_used": 2000,
      "tokens_remaining": 198000,
      "cost_usd": 0.0015,
      "llm_call": {
        "model": "...",
        "input_tokens": 1500,
        "output_tokens": 500,
        "duration_ms": 2800,
        "stop_reason": "tool_use",
        "error": ""
      },
      "tool_calls": [
        {
          "name": "search_arxiv",
          "input_length": 45,
          "result_length": 3200,
          "duration_ms": 1200
        }
      ]
    }
  ]
}
```

### Key Design Decisions

**Per-turn cost:** Computed as `TurnState.TotalCostUSD - previousTurnCostUSD` (the hooks receive cumulative cost; the recorder stores the previous value and diffs).

**LLM call → turn association:** The `Recorder` maintains a `pendingLLMCall *LLMCall` field. `TracedClient.CreateMessage` sets it. The `OnTurnEnd` hook reads it, attaches it to the current turn, and clears it. This works because within a single loop, the call sequence is always: `OnTurnStart → TracedClient.CreateMessage → OnToolCall* → OnToolResult* → OnTurnEnd`.

**Tool timing uses a stack:** The recorder maintains `toolStartStack []time.Time` (matching the existing `toolSpanStack` pattern in the old OTel hooks). `OnToolCall` pushes a start time; `OnToolResult` pops it. Tools are sequential within a turn so this is LIFO.

**`input_length` is byte length of the raw JSON input payload** (same as what the old OTel hooks recorded as `gen_ai.tool.input_length`).

**Child loops (Spawn) — out of scope for v1:** The `Spawn` method shares the parent's hooks and client unconditionally (see `types.go:229`). Adding a `Hooks` field to `ChildConfig` would change the agentic loop core. Instead, for v1 child loops will record their turns into the parent's trace (interleaved). This is acceptable because: (a) the research-demo doesn't use Spawn, (b) interleaved turns are still useful data, and (c) a future v2 can add `ChildConfig.Hooks` if needed. The `pendingLLMCall` pattern is safe for interleaved turns because `CreateMessage` and `OnTurnEnd` are always called in sequence within each loop's goroutine, and `Spawn` children run on the calling goroutine (not a separate one) — confirmed by reading `agentic.go` where `childLoop.Run(ctx, query)` is a blocking call inside a tool handler.

**Server per-request lifecycle:** The factory function creates a new `Recorder` + hooks + `TracedClient` per request. This fixes a concurrency bug in the existing code where a single `TracingHooks` instance is shared across all requests. Sketch:

```go
factory := func(query string, logger *slog.Logger) (*agentic.Loop, func(), error) {
    cfg := tracing.Config{
        Dir:       ".traces",
        SessionID: fmt.Sprintf("web-%d", time.Now().UnixNano()),
        Query:     query,
        Model:     string(modelID),
        Logger:    logger,
    }
    hooks, recorder := tracing.NewTracingHooks(cfg)
    var client agentic.MessageClient = &sdkAdapter{...}
    client = &tracing.TracedClient{Inner: client, Recorder: recorder}
    loop := agentic.NewLoop(client, manager, reg, nil, agentic.LoopConfig{Hooks: hooks, ...}, system)
    cleanup := func() {
        if err := recorder.Flush(); err != nil {
            logger.Error("failed to flush trace", "err", err)
        }
    }
    return loop, cleanup, nil
}
```

The handler calls `defer cleanup()` immediately after receiving the loop, ensuring traces are written even if the request panics.

**Disk retention — out of scope.** Operator is responsible for cleanup (`rm`, `logrotate`, or a future `make trace-clean` target). Documented in README.

**When tracing is disabled (`cfg.Dir == ""`):** `NewTracingHooks` returns nil hooks (loop ignores them) and the caller should not wrap with `TracedClient` at all. The demo/server check `cfg.Enabled()` before wrapping.

## Specs

### Package: `internal/tracing/`

**Delete** existing files (provider.go, hooks.go, client.go, attributes.go).

**New files:**

#### `trace.go` — Types

```go
package tracing

import "time"

type Trace struct {
    SessionID         string  `json:"session_id"`
    Query             string  `json:"query"`
    Model             string  `json:"model"`
    StartedAt         time.Time `json:"started_at"`
    EndedAt           time.Time `json:"ended_at"`
    DurationMs        int64   `json:"duration_ms"`
    Status            string  `json:"status"`
    Error             string  `json:"error,omitempty"`
    TotalInputTokens  int     `json:"total_input_tokens"`
    TotalOutputTokens int     `json:"total_output_tokens"`
    TotalCostUSD      float64 `json:"total_cost_usd"`
    Turns             []Turn  `json:"turns"`
}

type Turn struct {
    Index           int        `json:"index"`
    StartedAt       time.Time  `json:"started_at"`
    EndedAt         time.Time  `json:"ended_at"`
    DurationMs      int64      `json:"duration_ms"`
    TokensUsed      int        `json:"tokens_used"`
    TokensRemaining int        `json:"tokens_remaining"`
    CostUSD         float64    `json:"cost_usd"`
    LLMCall         *LLMCall   `json:"llm_call,omitempty"`
    ToolCalls       []ToolCall `json:"tool_calls"`
}

type LLMCall struct {
    Model        string `json:"model"`
    InputTokens  int    `json:"input_tokens"`
    OutputTokens int    `json:"output_tokens"`
    DurationMs   int64  `json:"duration_ms"`
    StopReason   string `json:"stop_reason"`
    Error        string `json:"error,omitempty"`
}

type ToolCall struct {
    Name         string `json:"name"`
    InputLength  int    `json:"input_length"`
    ResultLength int    `json:"result_length"`
    DurationMs   int64  `json:"duration_ms"`
}

type Config struct {
    Dir       string      // directory for trace files; "" disables tracing
    SessionID string
    Query     string
    Model     string
    Logger    *slog.Logger
}

func (cfg Config) Enabled() bool {
    return cfg.Dir != ""
}
```

#### `recorder.go` — Recorder (state accumulator + flush)

```go
package tracing

import (
    "encoding/json"
    "log/slog"
    "os"
    "path/filepath"
    "sync"
    "time"
)

type Recorder struct {
    cfg            Config
    trace          Trace
    currentTurn    *Turn
    pendingLLMCall *LLMCall
    toolStartStack []time.Time
    prevCostUSD    float64
    mu             sync.Mutex
}

func NewRecorder(cfg Config) *Recorder {
    return &Recorder{
        cfg: cfg,
        trace: Trace{
            SessionID: cfg.SessionID,
            Query:     cfg.Query,
            Model:     cfg.Model,
            StartedAt: time.Now(),
            Status:    "ok",
        },
    }
}

func (r *Recorder) SetError(err error) {
    if err == nil { return }
    r.mu.Lock()
    defer r.mu.Unlock()
    r.trace.Status = "error"
    r.trace.Error = err.Error()
}

func (r *Recorder) Flush() error {
    if !r.cfg.Enabled() { return nil }
    r.mu.Lock()
    defer r.mu.Unlock()
    r.trace.EndedAt = time.Now()
    r.trace.DurationMs = r.trace.EndedAt.Sub(r.trace.StartedAt).Milliseconds()
    if err := os.MkdirAll(r.cfg.Dir, 0o755); err != nil { return err }
    data, err := json.MarshalIndent(r.trace, "", "  ")
    if err != nil { return err }
    return os.WriteFile(filepath.Join(r.cfg.Dir, r.cfg.SessionID+".json"), data, 0o644)
}
```

#### `hooks.go` — LoopHooks factory

```go
package tracing

import "github.com/dancsalo/arxiv-deep-research/internal/agentic"

func NewTracingHooks(cfg Config) (*agentic.LoopHooks, *Recorder) {
    rec := NewRecorder(cfg)
    if !cfg.Enabled() {
        return nil, rec
    }
    return &agentic.LoopHooks{
        OnTurnStart:  rec.onTurnStart,
        OnTurnEnd:    rec.onTurnEnd,
        OnToolCall:   rec.onToolCall,
        OnToolResult: rec.onToolResult,
    }, rec
}
```

Hook implementations on Recorder:
- `onTurnStart`: lock, create `currentTurn` with index and start time, unlock
- `onTurnEnd`: lock, compute duration, compute per-turn cost delta, attach pendingLLMCall, sum tokens, append turn to trace.Turns, clear state, unlock
- `onToolCall`: lock, push `time.Now()` to `toolStartStack`, append partial ToolCall to currentTurn, unlock
- `onToolResult`: lock, pop from toolStartStack, compute duration, set result_length on last tool call, unlock

#### `client.go` — TracedClient

```go
package tracing

import (
    "context"
    "time"

    "github.com/anthropics/anthropic-sdk-go"
    "github.com/dancsalo/arxiv-deep-research/internal/agentic"
)

type TracedClient struct {
    Inner    agentic.MessageClient
    Recorder *Recorder
}

func (tc *TracedClient) CreateMessage(ctx context.Context, params anthropic.MessageNewParams) (*anthropic.Message, error) {
    start := time.Now()
    resp, err := tc.Inner.CreateMessage(ctx, params)
    elapsed := time.Since(start)

    call := &LLMCall{
        Model:      string(params.Model),
        DurationMs: elapsed.Milliseconds(),
    }
    if err != nil {
        call.Error = err.Error()
    } else {
        call.InputTokens = int(resp.Usage.InputTokens)
        call.OutputTokens = int(resp.Usage.OutputTokens)
        call.StopReason = string(resp.StopReason)
    }
    tc.Recorder.mu.Lock()
    tc.Recorder.pendingLLMCall = call
    tc.Recorder.mu.Unlock()

    return resp, err
}
```

### Makefile targets

```makefile
TRACE_DIR ?= .traces

.PHONY: trace-list trace-show trace-errors

trace-list: ## List recent traces
	@find $(TRACE_DIR) -name '*.json' -print0 2>/dev/null | xargs -0 ls -t 2>/dev/null | head -20 | xargs -I{} jq -r '[.session_id, .status, .query[0:40], "\(.total_cost_usd)"] | join("\t")' {} || echo "No traces in $(TRACE_DIR)/"

trace-show: ## Show a trace: make trace-show RUN=<session_id>
	@jq . $(TRACE_DIR)/$(RUN).json

trace-errors: ## Show failed traces
	@find $(TRACE_DIR) -name '*.json' -print0 2>/dev/null | xargs -0 jq -r 'select(.status=="error") | [.session_id, .error[0:60]] | join("\t")' 2>/dev/null || echo "No errors"
```

## Contracts

### `NewTracingHooks(cfg Config) (*agentic.LoopHooks, *Recorder)`
- If `!cfg.Enabled()`, returns nil hooks and a no-op recorder (Flush returns nil)
- One recorder per loop instance — create fresh for each request/run
- Hooks populate the recorder's internal state during loop execution

### `Recorder.Flush() error`
- Writes the trace JSON to `cfg.Dir/<session_id>.json`
- Creates the directory if it doesn't exist (os.MkdirAll)
- Sets `EndedAt` and computes `DurationMs` at flush time
- Returns error only on I/O failure (caller should log it)
- No-op if `!cfg.Enabled()`

### `Recorder.SetError(err error)`
- Sets `trace.Status = "error"` and `trace.Error = err.Error()`
- No-op if err is nil

### `TracedClient.CreateMessage(ctx, params) (*Message, error)`
- Records timing, model, input/output tokens, stop reason, error
- Stores as `pendingLLMCall` on the recorder (consumed by OnTurnEnd)
- Passes through the response/error unchanged
- Only wraps client when tracing is enabled (caller checks `cfg.Enabled()`)

## Decisions & Tradeoffs

| Decision | Rationale |
|----------|-----------|
| JSON files over SQLite | Zero dependencies, greppable, jq-queryable, git-ignorable |
| One file per run | Simple to reason about, easy to delete old traces |
| jq-based CLI over custom binary | No build step, composable with unix tools |
| Keep LoopHooks interface unchanged | No changes to agentic loop core — drop-in replacement |
| `.traces/` default dir | Conventional, easy to .gitignore |
| No OTel dependency at all | Eliminates 4+ transitive deps, simplifies go.mod |
| Drop OnMemoryRecall/OnMemoryPersist tracing | Low debugging value; can add back if needed |
| pendingLLMCall pattern | Simple single-field handoff; safe because calls are sequential within a loop |
| toolStartStack (not single field) | Handles multiple sequential tool calls per turn correctly |
| Child-loop tracing — v1 shares parent recorder | Avoids changing ChildConfig; interleaved turns are still useful data. Spawn runs child synchronously on same goroutine so no race. v2 can add ChildConfig.Hooks |
| UnixNano in server session IDs | Avoids millisecond collision for concurrent requests |
| Per-request recorder in server | Fixes existing concurrency bug where shared TracingHooks mutated state across requests |
| Recorder accepts Logger | Caller logs flush errors; no silent failures |
| Skip TracedClient wrapper when disabled | Zero overhead when tracing is off |
| Disk retention out of scope | Operator responsibility; future `make trace-clean` if needed |

## Implementation Order

Write new code first, then swap callers, then delete old code:

1. Write `internal/tracing/trace.go` (types + Config)
2. Write `internal/tracing/recorder.go` (Recorder, NewRecorder, SetError, Flush)
3. Write `internal/tracing/hooks.go` (NewTracingHooks + hook methods)
4. Write `internal/tracing/client.go` (TracedClient)
5. Write `internal/tracing/recorder_test.go` (unit tests: turn accumulation, tool stack, cost delta, flush output, error paths, disabled no-op)
6. Update `cmd/research-demo/main.go` — swap to new tracing Config, wrap client conditionally, call Flush after Run
7. Update `cmd/server/main.go` — per-request recorder in factory, return cleanup func, defer in handler
8. Delete old tracing files (provider.go, attributes.go, old hooks.go, old client.go)
9. `go mod tidy` to remove OTel deps
10. Remove Langfuse services from `docker-compose.langfuse.yml`, keep embedding-api
11. Add Makefile trace targets
12. Add `.traces/` to `.gitignore`
13. `make check`

## Critical Files

- `internal/tracing/` — all new files (trace.go, recorder.go, hooks.go, client.go, recorder_test.go)
- `cmd/research-demo/main.go` — swap tracing init
- `cmd/server/main.go` — swap tracing init, change factory signature
- `server/` package — if factory signature changes, the server handler needs updating
- `go.mod` / `go.sum` — remove OTel deps
- `docker-compose.langfuse.yml` — remove Langfuse services
- `Makefile` — add trace-* targets
- `.gitignore` — add `.traces/`

## Verification

1. `make check` passes (build + lint + tests)
2. Run research-demo without TRACE_DIR set — no tracing, no crash
3. Run research-demo with `TRACE_DIR=.traces`:
   ```
   go run ./cmd/research-demo -query "transformers" -max-turns 2
   ```
   Verify `.traces/demo-*.json` exists with correct schema
4. `make trace-list` shows the trace
5. `make trace-show RUN=demo-*` pretty-prints it
6. Simulate error: run with invalid model, verify `status: "error"` in trace

## Revision Log

- v1: Initial plan
- v2: Addressed round 1 critiques — added error tracking, per-request lifecycle, cost delta logic, LLM→turn association, correct implementation order
- v3: Addressed round 2 critiques:
  - Fixed: tool timing uses a stack (`toolStartStack`) not single field (#4)
  - Resolved: child-loop tracing declared out of scope for v1; Spawn runs synchronously so shared recorder is safe (#1, #2)
  - Fixed: cleanup uses `defer` and logs flush errors (#5, #9)
  - Fixed: skip TracedClient entirely when disabled (#7)
  - Fixed: jq string slicing to `[0:40]` syntax (#5-makefile)
  - Fixed: use `find -print0 | xargs -0` for safe file handling (#5-makefile)
  - Clarified: `input_length` is byte length of raw JSON payload (#3)
  - Clarified: disk retention is out of scope, documented (#3-PM)
  - Removed docker-compose restructuring contradiction (#10)
  - Dropped OnMemoryRecall and OnMemoryPersist explicitly (#8-PM)
