# HTTP Server with SSE Streaming

## Problem Statement

The `AgenticLoop` (planned in `agentic-loop.md`) is a Go struct that runs a tool-use loop with context management, memory recall, and hooks. It logs all activity via `slog.Logger`. But there is:

1. **No way to serve the loop over HTTP.** A developer building on this framework can't expose the loop as a service, watch it work from a browser, or demo it to stakeholders. The loop currently runs in-process with no network interface.

**Goal:** Add a minimal HTTP server that accepts a query via POST and streams structured events back to the client via Server-Sent Events (SSE). A simple HTML page provides the UI: a text input for the query and a live-updating event panel that renders human-readable status messages.

**Related:** The `ToolSet` interface (planned separately in `toolset.md`) standardizes how groups of tools are packaged for the `ToolRegistry`. It has zero coupling to this server plan and ships independently.

## Requirements

### Functional

1. **SSE Event Streaming:** The `AgenticLoop` already logs via `slog.Logger`. A custom `slog.Handler` bridges the gap: it writes each log record as a JSON-encoded SSE event to an `http.ResponseWriter`. The UI receives structured events in real-time as the loop runs. Known `msg` values (e.g., `turn.start`, `tool.execute`, `llm.call`) are rendered as human-readable status lines in the UI; unknown messages fall back to raw key=value display.

2. **HTTP Endpoint:** A single `POST /query` endpoint accepts a JSON body `{"query": "..."}` and returns an SSE stream. The final event contains the loop's answer. The connection stays open for the duration of the loop. Only one loop runs at a time (single-flight).

3. **Simple HTML UI:** A single-page HTML file served at `GET /`. Contains a text input, a submit button, and a scrolling event panel. JavaScript uses `fetch` with streaming body reader to consume the SSE stream. The UI formats known event types into human-readable lines (e.g., "Turn 1 — Calling search_arxiv..." instead of raw `msg=tool.execute tool=search_arxiv`). No build tools, no npm, no framework — just a single HTML file with inline CSS and JS.

4. **Graceful Client Disconnect:** When the HTTP client disconnects (browser closed, network drop), the request context is cancelled, which propagates to `AgenticLoop.Run()` and stops the loop.

### Non-Functional

1. No authentication or request persistence. This is a development/demo server.
2. The server logic must be a reusable package (`src/server/`), not locked inside `package main`.
3. Only stdlib `net/http` — no web frameworks.
4. The HTML UI is embedded in the Go binary via `go:embed` in the server package.

## Specs

### Architecture Overview

```
┌──────────────┐    POST /query     ┌──────────────────────┐
│              │  ───────────────►  │   HTTP Handler        │
│  Browser UI  │                    │                       │
│  (index.html)│  ◄─── SSE ─────  │  1. Parse query       │
│              │   (events)         │  2. Build AgenticLoop │
│              │                    │  3. Set SSE headers   │
└──────────────┘                    │  4. Create slog→SSE   │
                                    │  5. loop.Run(ctx, q)  │
                                    │  6. Send final event  │
                                    └──────────────────────┘
                                              │
                                    ┌─────────▼──────────┐
                                    │   AgenticLoop       │
                                    │   (logs via slog)   │
                                    │   (runs tools)      │
                                    │   (recalls memory)  │
                                    └────────────────────┘
```

### SSE slog Handler

The key integration piece. Implements `slog.Handler` and writes each log record as an SSE event. Lives in `src/server/` package.

```go
package server

// SSEHandler is an slog.Handler that writes log records as SSE events.
// Thread-safe: all writes are protected by a shared mutex.
//
// WithAttrs and WithGroup return new SSEHandler instances that share
// the same *sync.Mutex and http.ResponseWriter (pointer sharing, not
// value copy — safe for go vet). Group names are prefixed to attribute
// keys with dot notation (e.g., group "foo" + key "bar" → "foo.bar").
type SSEHandler struct {
    w       http.ResponseWriter
    flusher http.Flusher
    mu      *sync.Mutex    // pointer — shared across WithAttrs/WithGroup copies
    attrs   []slog.Attr
    groups  []string       // group name stack
    level   slog.Level
}

func NewSSEHandler(w http.ResponseWriter, level slog.Level) *SSEHandler {
    return &SSEHandler{
        w:       w,
        flusher: w.(http.Flusher),
        mu:      &sync.Mutex{},  // heap-allocated, shared by copies
        level:   level,
    }
}

func (h *SSEHandler) Enabled(_ context.Context, level slog.Level) bool {
    return level >= h.level
}

func (h *SSEHandler) Handle(_ context.Context, r slog.Record) error {
    h.mu.Lock()
    defer h.mu.Unlock()

    event := map[string]any{
        "time":  r.Time.Format(time.RFC3339),
        "level": r.Level.String(),
        "msg":   r.Message,
    }

    // Add handler-level attrs (with group prefix)
    prefix := strings.Join(h.groups, ".")
    for _, a := range h.attrs {
        key := a.Key
        if prefix != "" {
            key = prefix + "." + key
        }
        event[key] = a.Value.Any()
    }

    // Add record-level attrs (with group prefix)
    r.Attrs(func(a slog.Attr) bool {
        key := a.Key
        if prefix != "" {
            key = prefix + "." + key
        }
        event[key] = a.Value.Any()
        return true
    })

    data, err := json.Marshal(event)
    if err != nil {
        // Marshal failure: send error indicator rather than swallowing
        data = []byte(fmt.Sprintf(`{"msg":"marshal_error","error":%q}`, err.Error()))
    }
    fmt.Fprintf(h.w, "event: log\ndata: %s\n\n", data)
    h.flusher.Flush()
    return nil
}

func (h *SSEHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
    return &SSEHandler{
        w: h.w, flusher: h.flusher, mu: h.mu,  // shared pointer
        attrs:  append(slices.Clone(h.attrs), attrs...),
        groups: slices.Clone(h.groups),
        level:  h.level,
    }
}

func (h *SSEHandler) WithGroup(name string) slog.Handler {
    if name == "" {
        return h
    }
    return &SSEHandler{
        w: h.w, flusher: h.flusher, mu: h.mu,  // shared pointer
        attrs:  slices.Clone(h.attrs),
        groups: append(slices.Clone(h.groups), name),
        level:  h.level,
    }
}

// SendDone writes the final answer as an SSE "done" event.
// Must be called from the HTTP handler, not from slog.
// Uses the same mutex to prevent interleaving with log events.
func (h *SSEHandler) SendDone(answer string) {
    h.mu.Lock()
    defer h.mu.Unlock()
    data, _ := json.Marshal(map[string]string{"answer": answer})
    fmt.Fprintf(h.w, "event: done\ndata: %s\n\n", data)
    h.flusher.Flush()
}

// SendError writes an error as an SSE "error" event.
// Uses the same mutex to prevent interleaving.
func (h *SSEHandler) SendError(err error) {
    h.mu.Lock()
    defer h.mu.Unlock()
    data, _ := json.Marshal(map[string]string{"error": err.Error()})
    fmt.Fprintf(h.w, "event: error\ndata: %s\n\n", data)
    h.flusher.Flush()
}
```

**Key fixes from critique:**
- `mu` is `*sync.Mutex` (pointer), not `sync.Mutex` (value). `WithAttrs`/`WithGroup` share the same pointer. No mutex copy.
- `groups` is a slice tracking the group name stack. Attribute keys are prefixed with dot-joined group names.
- `json.Marshal` errors produce a `marshal_error` event instead of being swallowed.
- `SendDone` and `SendError` are methods on `SSEHandler` sharing the same mutex, preventing interleaved writes.

### HTTP Server

```go
package server

import (
    "embed"
    "io"
    "net/http"
    "sync"

    contextmanager "memory-store"
)

//go:embed static/index.html
var staticFS embed.FS

// LoopFactory creates an AgenticLoop for a given query.
// The server calls this per request, since AgenticLoop instances are single-use.
// The factory MUST set cfg.Logger = logger so the loop's output flows to SSE.
type LoopFactory func(query string, logger *slog.Logger) (*contextmanager.AgenticLoop, error)

// Server serves the agentic loop over HTTP with SSE streaming.
type Server struct {
    factory LoopFactory
    addr    string

    // single-flight: only one loop runs at a time
    mu      sync.Mutex
    running bool
}

func NewServer(factory LoopFactory, addr string) *Server {
    return &Server{factory: factory, addr: addr}
}

// ListenAndServe starts the HTTP server. Blocks until the server is stopped.
// Server-side shutdown is via process signal (Ctrl+C). No graceful drain
// mechanism — this is a dev server.
func (s *Server) ListenAndServe() error {
    return http.ListenAndServe(s.addr, s.Handler())
}

// Handler returns the http.Handler (for testing without listening).
func (s *Server) Handler() http.Handler {
    mux := http.NewServeMux()
    mux.HandleFunc("GET /", s.handleIndex)
    mux.HandleFunc("POST /query", s.handleQuery)
    return mux
}
```

**Routes:**

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/` | Serves embedded `index.html` |
| `POST` | `/query` | Accepts `{"query": "..."}`, streams SSE events |

### POST /query Handler

Reordered so factory runs **before** SSE headers are set. If the factory fails, the client gets a proper HTTP error code instead of a 200 with an SSE error event.

```go
func (s *Server) handleQuery(w http.ResponseWriter, r *http.Request) {
    // 1. Single-flight check
    s.mu.Lock()
    if s.running {
        s.mu.Unlock()
        http.Error(w, "a query is already running", http.StatusConflict)
        return
    }
    s.running = true
    s.mu.Unlock()
    defer func() {
        s.mu.Lock()
        s.running = false
        s.mu.Unlock()
    }()

    // 2. Parse and validate request body (with size limit)
    r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1MB limit
    var req struct {
        Query string `json:"query"`
    }
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, "invalid JSON body", http.StatusBadRequest)
        return
    }
    if req.Query == "" {
        http.Error(w, "query is required", http.StatusBadRequest)
        return
    }

    // 3. Check flusher support (before committing to SSE)
    flusher, ok := w.(http.Flusher)
    if !ok {
        http.Error(w, "streaming not supported", http.StatusInternalServerError)
        return
    }

    // 4. Build loop via factory BEFORE setting SSE headers.
    // If factory fails, we can still return a proper HTTP error code.
    sseHandler := NewSSEHandler(w, slog.LevelInfo)
    logger := slog.New(sseHandler)
    loop, err := s.factory(req.Query, logger)
    if err != nil {
        http.Error(w, fmt.Sprintf("loop construction failed: %v", err), http.StatusInternalServerError)
        return
    }

    // 5. Now commit to SSE streaming (headers sent, status 200 implicit)
    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("Connection", "keep-alive")
    flusher.Flush() // send headers immediately

    // 6. Run loop — r.Context() cancels when client disconnects
    answer, err := loop.Run(r.Context(), req.Query)
    if err != nil {
        if r.Context().Err() != nil {
            return // client disconnected, no one to send to
        }
        sseHandler.SendError(err)
        return
    }

    // 7. Send final answer
    sseHandler.SendDone(answer)
}
```

**Key changes from critique:**
- **Single-flight:** Mutex-based check rejects concurrent requests with 409 Conflict.
- **Body size limit:** `http.MaxBytesReader` caps at 1MB.
- **Factory before headers:** If the factory fails, client gets HTTP 500, not a 200+SSE error.
- **Done/error via SSEHandler methods:** Uses the shared mutex, no interleaving with log events.
- **Client disconnect check:** Before sending error events, checks if context is still alive.

### HTML UI

A single `index.html` embedded via `go:embed` in the `server` package. The UI renders known event types as human-readable status messages. The `//go:embed` directive is in `src/server/`, and the `static/` directory is relative to that package.

**Event rendering rules:**

| `msg` value | UI rendering |
|-------------|-------------|
| `turn.start` | `"▶ Turn {turn} started ({tokens_remaining} tokens remaining)"` |
| `memory.recall` | `"🧠 Recalled {injected} memories (searched: {query})"` |
| `llm.call` | `"💬 LLM response: {output_tokens} tokens, ${cost_usd}"` |
| `tool.execute` | `"🔧 {tool} completed ({latency_ms}ms)"` |
| `hook.error` | `"⚠️ Hook error ({hook}): {err}"` |
| `memory.recall.failed` | `"⚠️ Memory recall failed: {err}"` |
| `loop.cancelled` | `"⛔ Loop cancelled"` |
| (other) | Raw: `"{msg} key=value key=value..."` |

**Key HTML/JS fixes from critique:**
- Uses `insertAdjacentHTML('beforeend', ...)` instead of `innerHTML +=` for DOM performance.
- SSE event type reset at the top of each line-processing iteration (not after data line).
- Known `msg` types are rendered as formatted human-readable lines.

```html
<!DOCTYPE html>
<html>
<head>
    <title>Agentic Loop</title>
    <style>
        * { box-sizing: border-box; margin: 0; padding: 0; }
        body { font-family: monospace; background: #1a1a2e; color: #e0e0e0; padding: 2rem; }
        h1 { color: #7D56F4; margin-bottom: 1rem; }
        .input-row { display: flex; gap: 0.5rem; margin-bottom: 1rem; }
        input[type="text"] {
            flex: 1; padding: 0.5rem; background: #16213e; border: 1px solid #7D56F4;
            color: #e0e0e0; font-family: monospace; font-size: 1rem; border-radius: 4px;
        }
        button {
            padding: 0.5rem 1.5rem; background: #7D56F4; color: white; border: none;
            font-family: monospace; font-size: 1rem; cursor: pointer; border-radius: 4px;
        }
        button:disabled { opacity: 0.5; cursor: not-allowed; }
        #log {
            background: #0f0f23; border: 1px solid #333; border-radius: 4px;
            padding: 1rem; height: 70vh; overflow-y: auto; font-size: 0.85rem;
            line-height: 1.6;
        }
        .log-entry { margin-bottom: 0.25rem; }
        .log-time { color: #666; }
        .log-info { color: #04B575; }
        .log-warn { color: #FFD700; }
        .log-error { color: #FF6B6B; }
        .log-debug { color: #666; }
        .log-attrs { color: #888; }
        .answer { background: #1a3a1a; padding: 1rem; border-radius: 4px;
                  margin-top: 0.5rem; border: 1px solid #04B575; white-space: pre-wrap; }
        .error-box { background: #3a1a1a; padding: 1rem; border-radius: 4px;
                     margin-top: 0.5rem; border: 1px solid #FF6B6B; }
        .status { color: #888; font-style: italic; margin-bottom: 0.5rem; }
    </style>
</head>
<body>
    <h1>Agentic Loop</h1>
    <div class="input-row">
        <input type="text" id="query" placeholder="Enter your query..." autofocus />
        <button id="submit" onclick="run()">Run</button>
    </div>
    <div id="status" class="status"></div>
    <div id="log"></div>
    <script>
        const MSG_FORMATS = {
            'turn.start': d => `▶ Turn ${d.turn} started (${d.tokens_remaining} tokens remaining)`,
            'memory.recall': d => `🧠 Recalled ${d.injected} memories (searched: "${d.query}")`,
            'memory.recall.skip': d => `🧠 Memory recall skipped: ${d.reason}`,
            'memory.recall.failed': d => `⚠️ Memory recall failed: ${d.err}`,
            'llm.call': d => `💬 LLM response: ${d.output_tokens} tokens, $${Number(d.cost_usd).toFixed(4)}`,
            'tool.execute': d => `🔧 ${d.tool} completed (${d.latency_ms}ms)`,
            'tool.unknown': d => `⚠️ Unknown tool: ${d.tool}`,
            'hook.error': d => `⚠️ Hook error (${d.hook}): ${d.err}`,
            'loop.cancelled': d => `⛔ Loop cancelled`,
        };

        async function run() {
            const query = document.getElementById('query').value.trim();
            if (!query) return;
            const btn = document.getElementById('submit');
            const log = document.getElementById('log');
            const status = document.getElementById('status');
            btn.disabled = true;
            log.innerHTML = '';
            status.textContent = 'Running...';

            try {
                const resp = await fetch('/query', {
                    method: 'POST',
                    headers: {'Content-Type': 'application/json'},
                    body: JSON.stringify({query}),
                });

                if (!resp.ok) {
                    const text = await resp.text();
                    log.insertAdjacentHTML('beforeend',
                        `<div class="error-box">HTTP ${resp.status}: ${esc(text)}</div>`);
                    return;
                }

                const reader = resp.body.getReader();
                const decoder = new TextDecoder();
                let buffer = '';

                while (true) {
                    const {done, value} = await reader.read();
                    if (done) break;
                    buffer += decoder.decode(value, {stream: true});

                    const lines = buffer.split('\n');
                    buffer = lines.pop();

                    let eventType = 'log';
                    for (const line of lines) {
                        if (line.startsWith('event: ')) {
                            eventType = line.slice(7).trim();
                        } else if (line.startsWith('data: ')) {
                            try {
                                const data = JSON.parse(line.slice(6));
                                handleEvent(eventType, data);
                            } catch (e) { /* skip malformed */ }
                            eventType = 'log';
                        } else if (line === '') {
                            eventType = 'log';
                        }
                    }
                }
            } catch (e) {
                log.insertAdjacentHTML('beforeend',
                    `<div class="error-box">Connection error: ${esc(e.message)}</div>`);
            } finally {
                btn.disabled = false;
                status.textContent = '';
            }
        }

        function handleEvent(type, data) {
            const log = document.getElementById('log');
            if (type === 'done') {
                document.getElementById('status').textContent = 'Complete';
                log.insertAdjacentHTML('beforeend',
                    `<div class="answer"><strong>Answer:</strong>\n${esc(data.answer)}</div>`);
            } else if (type === 'error') {
                document.getElementById('status').textContent = 'Error';
                log.insertAdjacentHTML('beforeend',
                    `<div class="error-box">${esc(data.error)}</div>`);
            } else {
                const time = data.time ? data.time.split('T')[1]?.split('.')[0] || '' : '';
                const level = data.level || 'INFO';
                const msg = data.msg || '';
                const fmt = MSG_FORMATS[msg];
                let text;
                if (fmt) {
                    text = fmt(data);
                } else {
                    const attrs = Object.entries(data)
                        .filter(([k]) => !['time','level','msg'].includes(k))
                        .map(([k,v]) => `${k}=${JSON.stringify(v)}`)
                        .join(' ');
                    text = `${msg} ${attrs}`.trim();
                }
                const levelClass = level === 'WARN' ? 'log-warn' :
                                   level === 'ERROR' ? 'log-error' :
                                   level === 'DEBUG' ? 'log-debug' : 'log-info';
                log.insertAdjacentHTML('beforeend',
                    `<div class="log-entry">` +
                    `<span class="log-time">${time}</span> ` +
                    `<span class="${levelClass}">${esc(text)}</span>` +
                    `</div>`);
            }
            log.scrollTop = log.scrollHeight;
        }

        function esc(s) {
            const d = document.createElement('div');
            d.textContent = String(s);
            return d.innerHTML;
        }

        document.getElementById('query').addEventListener('keydown', (e) => {
            if (e.key === 'Enter') run();
        });
    </script>
</body>
</html>
```

### Example Module Setup

The example lives in `examples/03-server/` as its own Go module. It imports the `memory-store` module (from `src/`) via a `replace` directive pointing to the local path.

**`examples/03-server/go.mod`:**

```
module agentic-server

go 1.24.2

require memory-store v0.0.0

replace memory-store => ../../src
```

The example's `main.go` wires up a concrete `LoopFactory` and starts the server:

```go
package main

import (
    "context"
    "flag"
    "fmt"
    "log/slog"
    "os"
    "time"

    "github.com/anthropics/anthropic-sdk-go"
    "github.com/anthropics/anthropic-sdk-go/bedrock"
    "github.com/anthropics/anthropic-sdk-go/option"

    contextmanager "memory-store"
    "memory-store/server"
)

func main() {
    addr := flag.String("addr", ":8080", "listen address")
    useBedrock := flag.Bool("bedrock", true, "use AWS Bedrock")
    model := flag.String("model", "", "model ID override")
    flag.Parse()

    // Create Anthropic client (shared across requests)
    ctx := context.Background()
    var opts []option.RequestOption
    if *useBedrock {
        opts = append(opts, bedrock.WithLoadDefaultConfig(ctx))
    }
    apiClient := anthropic.NewClient(opts...)

    var modelID anthropic.Model
    if *model != "" {
        modelID = anthropic.Model(*model)
    } else if *useBedrock {
        modelID = "us.anthropic.claude-3-5-haiku-20241022-v1:0"
    } else {
        modelID = anthropic.ModelClaudeHaiku4_5
    }

    // Factory: builds a fresh AgenticLoop per request
    factory := func(query string, logger *slog.Logger) (*contextmanager.AgenticLoop, error) {
        systemBlocks := []anthropic.TextBlockParam{
            {Text: "You are a helpful research assistant.", Type: "text"},
        }

        initialMsg := anthropic.NewUserMessage(anthropic.NewTextBlock(query))
        manager := contextmanager.NewContextManager(contextmanager.ContextManagerConfig{
            Estimator: contextmanager.NewTokenEstimator(nil),
            Budget: &contextmanager.ContextBudget{
                ModelContextLimit: 200000,
                MaxOutputTokens:   8192,
                SafetyMargin:      2000,
            },
            System:  systemBlocks,
            NowFunc: time.Now,
        }, initialMsg)

        registry := contextmanager.NewToolRegistry()
        // Register tool sets here:
        // contextmanager.RegisterToolSets(registry, mytools.NewSearchToolSet(...))

        // Register finish tool
        registry.Register("finish", contextmanager.BuildFinishTool(),
            func(ctx context.Context, input json.RawMessage) (string, error) {
                return string(input), nil
            })

        loop := contextmanager.NewAgenticLoop(
            &apiClient,
            manager,
            registry,
            nil, // no memory recaller for this example
            contextmanager.AgenticLoopConfig{
                MaxTurns:        20,
                MaxCostUSD:      0.50,
                Model:           modelID,
                SessionID:       fmt.Sprintf("web-%d", time.Now().UnixMilli()),
                FinishTool:      "finish",
                DefaultPriority: contextmanager.PriorityCore,
                Logger:          logger, // ← routes slog to SSE
            },
            systemBlocks,
        )
        return loop, nil
    }

    srv := server.NewServer(factory, *addr)
    fmt.Printf("Listening on %s\n", *addr)
    if err := srv.ListenAndServe(); err != nil {
        fmt.Fprintf(os.Stderr, "server error: %v\n", err)
        os.Exit(1)
    }
}
```

### Request Lifecycle

```
Client                          Server
  │                               │
  │  POST /query {"query":"..."}  │
  │ ─────────────────────────────►│
  │                               │  Parse body, validate
  │                               │  Single-flight check (409 if busy)
  │                               │  factory(query, logger) → loop
  │                               │  (if factory fails → HTTP 500)
  │                               │  Set SSE headers (200 committed)
  │                               │  loop.Run(r.Context(), query)
  │                               │    │
  │  event: log                   │    │  logger.Info("turn.start", ...)
  │  data: {"msg":"turn.start"}   │◄───┘
  │◄──────────────────────────────│
  │                               │    ... (more log events)
  │                               │
  │                               │  loop.Run returns answer
  │  event: done                  │
  │  data: {"answer":"..."}       │
  │◄──────────────────────────────│
  │                               │  Handler returns
```

**Client disconnect:** When the browser tab closes, `r.Context()` is cancelled. `loop.Run()` detects this and returns `context.Canceled`. The handler checks `r.Context().Err()` and returns without sending events (no one is listening).

**Same-origin note:** The UI and API are served from the same origin (`/` and `/query`), so CORS is not needed. If a caller needs cross-origin access (e.g., from a notebook), they would need to add CORS middleware — this is out of scope for v1.

## Contracts

### Server ↔ LoopFactory

- Called once per request with `(query string, logger *slog.Logger)`.
- **Must** set `cfg.Logger = logger` so the loop's slog output flows to SSE.
- Returns a fully-constructed `*AgenticLoop` ready to `Run()`.
- If construction fails (DB down, missing config), return an error — the handler returns HTTP 500 with the error message (before SSE headers are sent).

### Server ↔ SSEHandler

- `SSEHandler` implements `slog.Handler` from Go stdlib.
- `ResponseWriter` must implement `http.Flusher`. Checked before factory call.
- Thread-safe: `mu` is `*sync.Mutex` (heap-allocated pointer), shared across `WithAttrs`/`WithGroup` copies. No value copy of mutex.
- `SendDone` and `SendError` use the same mutex as `Handle` — no interleaved writes.
- Group support: group names are prefixed to attribute keys with dot notation.

### Server ↔ HTML UI

- UI uses `fetch` with streaming body reader (not `EventSource`) because the request is POST.
- Three SSE event types: `log` (intermediate), `done` (final answer), `error` (failure).
- Known `msg` values are rendered as human-readable lines via `MSG_FORMATS` lookup.
- Non-200 HTTP responses (factory errors, 409 conflict) are handled before SSE starts — the UI reads the body as text and displays the error.

## Decisions & Tradeoffs

1. **SSE via slog.Handler vs. custom event emitter:** The loop logs via `slog`, and the handler routes it to SSE. The loop doesn't know about HTTP. Tradeoff: limited to `slog.Record` contents, but this covers all events defined in the agentic loop plan.

2. **fetch + streaming reader vs. EventSource:** `EventSource` only supports GET. We use POST + `ReadableStream` with manual SSE parsing. Tradeoff: more JS code, but supports POST body and works in all modern browsers.

3. **Human-readable rendering vs. raw log forwarding:** The UI renders known `msg` types as formatted status lines (e.g., "Turn 1 started") via a client-side lookup table `MSG_FORMATS`. Unknown messages fall back to raw key=value. Tradeoff: the UI needs to track the set of known message types, but the user experience is dramatically better than a wall of JSON.

4. **Factory before SSE headers:** The handler calls the factory before setting `Content-Type: text/event-stream`. If the factory fails, the client gets a real HTTP error code (500). Tradeoff: a brief delay before streaming starts while the factory runs, but proper error semantics.

5. **Single-flight request limiting:** Only one loop runs at a time. Concurrent requests get 409 Conflict. Tradeoff: can't run parallel queries, but prevents accidental API budget exhaustion (the most common misuse for a dev server).

6. **Server package in `src/server/` vs. `examples/`:** The `Server` struct, `SSEHandler`, and embedded HTML live in `src/server/` — a sub-package of the framework module. This makes them importable by any consumer, not just the example. The example `main.go` in `examples/03-server/` just wires up the factory and starts the server. Tradeoff: adds to `src/` module size, but the server is a genuine framework component, not demo-only code.

7. **`replace` directive for local module import:** The example's `go.mod` uses `replace memory-store => ../../src` to import the local framework module. This is the standard Go pattern for multi-module repos without a module proxy. Tradeoff: the `replace` directive is local-only and won't work from an external consumer, but examples are by definition local.

8. **No server-side graceful shutdown:** `ListenAndServe()` blocks until killed. No `http.Server.Shutdown()`, no signal handling. This is a dev server — `Ctrl+C` is sufficient. Client-side disconnect propagation (via request context) is the important graceful behavior and is fully handled.

## Implementation Order

### Section 1: SSEHandler
- Implement `slog.Handler` with `Handle`, `WithAttrs`, `WithGroup`
- Implement `SendDone`, `SendError` methods (mutex-safe)
- File: `src/server/sse.go`

### Section 2: Server Struct & HTTP Handler
- `Server` struct with `NewServer`, `Handler()`, `ListenAndServe()`
- `POST /query` handler with single-flight, body limit, factory-before-headers
- `GET /` handler serving embedded HTML
- File: `src/server/server.go`

### Section 3: HTML UI
- Single `index.html` with `MSG_FORMATS` rendering, `insertAdjacentHTML`, SSE parsing
- File: `src/server/static/index.html`
- Embedded via `//go:embed static/index.html` in `src/server/server.go`

### Section 4: Example main.go & Module
- `examples/03-server/go.mod` with `replace` directive
- `examples/03-server/main.go` with concrete `LoopFactory`, flag parsing, server start
- Makefile: already picks up `examples/*/main.go` via wildcard — no changes needed

## Open Questions

1. **Multi-turn conversation:** Single-turn only for v1. Multi-turn would need session management. Out of scope.

2. **SSE reconnection:** No replay mechanism on disconnect. The UI shows "Connection error" and re-enables the button. Acceptable for a dev tool.

3. **DOM performance at scale:** `insertAdjacentHTML` is faster than `innerHTML +=` but still unbounded. For very long sessions (100+ events), the DOM may slow. A max-events cap or virtual scroll could help — deferred to v2.

## Revision Log

- v1 (initial): Plan created.
- v2 (revision 1): Addressed staff engineer and PM critiques.
  - **Fixed:** `SSEHandler.mu` is `*sync.Mutex` (pointer), not value. `WithAttrs`/`WithGroup` share the pointer. No mutex copy bug.
  - **Fixed:** `SSEHandler.Handle` implements group prefixing for attribute keys. `WithGroup` support is now spec-compliant.
  - **Fixed:** `json.Marshal` errors produce a `marshal_error` event instead of being swallowed.
  - **Fixed:** `SendDone`/`SendError` are methods on `SSEHandler` sharing its mutex. No interleaved writes with log events.
  - **Fixed:** Handler calls factory BEFORE setting SSE headers. Factory failures return HTTP 500, not 200+error-event.
  - **Fixed:** HTML uses `insertAdjacentHTML('beforeend', ...)` instead of `innerHTML +=` for DOM performance.
  - **Fixed:** JS SSE parser resets `eventType` at empty-line boundaries, not just after data lines.
  - **Added:** Single-flight request limiting (mutex + `running` flag, 409 on conflict).
  - **Added:** `http.MaxBytesReader` for 1MB request body limit.
  - **Added:** Full `examples/03-server/go.mod` with `replace memory-store => ../../src`.
  - **Added:** Full `examples/03-server/main.go` skeleton showing factory wiring.
  - **Added:** Human-readable event rendering via `MSG_FORMATS` lookup table in the UI.
  - **Added:** Status indicator in UI ("Running...", "Complete", "Error").
  - **Added:** Non-200 HTTP response handling in UI (for factory errors, 409 conflict).
  - **Changed:** Server package moved from `examples/03-server/` (package main) to `src/server/` (importable package). Example main.go is just a thin wiring layer.
  - **Changed:** `//go:embed` directive lives in `src/server/server.go`, embedding `static/index.html` relative to that package.
  - **Noted:** Same-origin serves both UI and API — CORS not needed. Documented for clarity.
  - **Noted:** Server-side shutdown is via process signal only (dev server). No `http.Server.Shutdown()`.
  - **Noted:** `ToolSet` can ship independently of the server — zero coupling between the two features.
- v3 (revision 2): Extracted ToolSet into its own plan (`toolset.md`). Renumbered implementation sections. Removed ToolSet specs, contracts, and decision from this plan.
