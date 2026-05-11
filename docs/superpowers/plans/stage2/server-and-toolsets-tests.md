# HTTP Server with SSE Streaming: Test Plan

Companion to `server-and-toolsets.md`. Tests use Go stdlib `testing` + `net/http/httptest`.

## Test Files

- `src/server/sse_test.go` — SSEHandler tests
- `src/server/server_test.go` — Server and HTTP handler tests

---

## SSEHandler Tests (`sse_test.go`)

### Test: Handle writes SSE event format

- Create `SSEHandler` with `httptest.NewRecorder()`
- Call `Handle()` with a `slog.Record` containing msg="test" and attrs key1=val1
- Verify output matches `event: log\ndata: {...}\n\n`
- Verify JSON contains `time`, `level`, `msg`, `key1`

### Test: Handle respects level filtering

- Create handler at `slog.LevelWarn`
- `Enabled(ctx, slog.LevelInfo)` returns false
- `Enabled(ctx, slog.LevelWarn)` returns true

### Test: WithAttrs returns new handler with shared mutex

- Call `h2 := handler.WithAttrs([]slog.Attr{slog.String("k", "v")})`
- Write via `h2.Handle()` → output includes `k=v`
- Write via original `handler.Handle()` → output does NOT include `k=v`
- Both write to the same `ResponseWriter` (verify output contains both events)

### Test: WithGroup prefixes attribute keys

- `h2 := handler.WithGroup("grp")`
- Write via `h2` with attr `key=val`
- Verify JSON contains `"grp.key"` not `"key"`

### Test: Nested groups

- `h2 := handler.WithGroup("a").WithGroup("b")`
- Write with attr `key=val`
- Verify JSON contains `"a.b.key"`

### Test: Handle survives json.Marshal error

- Create a `slog.Record` and add an attr whose Value wraps an unmarshallable type (e.g., a func)
- Call `Handle()` → does not panic
- Output contains `marshal_error` in the data

### Test: SendDone writes done event

- Call `sseHandler.SendDone("the answer")`
- Verify output: `event: done\ndata: {"answer":"the answer"}\n\n`

### Test: SendError writes error event

- Call `sseHandler.SendError(fmt.Errorf("boom"))`
- Verify output: `event: error\ndata: {"error":"boom"}\n\n`

### Test: Concurrent Handle and SendDone don't interleave

- Spawn 10 goroutines calling `Handle()` and 1 calling `SendDone()`
- Verify each SSE event in the output is well-formed (no partial writes)
- Uses `sync.WaitGroup` and checks output with line-by-line parsing

---

## Server Tests (`server_test.go`)

### Mock LoopFactory

```go
func mockFactory(answer string, err error) server.LoopFactory {
    return func(query string, logger *slog.Logger) (*contextmanager.AgenticLoop, error) {
        // Return a pre-configured loop or error
    }
}
```

Note: Since `AgenticLoop` depends on `MessageClient`, tests will need either a mock loop or a thin wrapper. If mocking is too complex, tests can focus on the HTTP layer (request parsing, SSE headers, error codes) and use a factory that returns an error for non-happy-path tests.

### Test: GET / serves HTML

- `httptest.NewServer(srv.Handler())`
- GET `/` → status 200, `Content-Type: text/html`, body contains `<title>Agentic Loop</title>`

### Test: POST /query with missing body

- POST `/query` with empty body → 400 "invalid JSON body"

### Test: POST /query with empty query

- POST `/query` with `{"query": ""}` → 400 "query is required"

### Test: POST /query with oversized body

- POST `/query` with body > 1MB → 400 (MaxBytesReader triggers)

### Test: POST /query factory error returns HTTP 500

- Factory returns error
- POST `/query` with `{"query": "test"}` → 500, body contains error message
- Response is NOT SSE (no `Content-Type: text/event-stream`)

### Test: POST /query success returns SSE stream

- Factory returns a mock loop that logs one event then returns an answer
- POST `/query` → status 200, `Content-Type: text/event-stream`
- Body contains at least one `event: log` and one `event: done`

### Test: Single-flight rejects concurrent request

- Factory that blocks (e.g., on a channel)
- Send first POST → starts running (blocks in factory/loop)
- Send second POST → 409 "a query is already running"
- Unblock first request → completes normally
- Send third POST → 200 (slot freed)

### Test: Client disconnect cancels loop context

- Factory returns a loop whose `Run` blocks until context is cancelled
- Start POST request
- Close the client connection (cancel the request context)
- Verify `Run` returns with `context.Canceled`
- Verify single-flight slot is freed (next request succeeds)

### Test: Handler routes are correct

- GET `/query` → 405 (method not allowed) or 404
- POST `/` → 405 or 404
- GET `/nonexistent` → 404

---

## Integration Test

### Test: Full request lifecycle

End-to-end test using `httptest.NewServer`:

1. Start server with a factory that builds a real `AgenticLoop` with a `scriptedMessageClient` (from existing test patterns)
2. POST `/query` with `{"query": "test"}`
3. Read SSE stream
4. Verify:
   - At least one `event: log` with `msg=turn.start`
   - Final `event: done` with non-empty answer
   - All events are valid JSON
   - Events arrive in order (turn numbers increase)
