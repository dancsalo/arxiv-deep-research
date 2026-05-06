package tracing

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dancsalo/arxiv-deep-research/internal/agentic"
)

func bgctx() context.Context { return context.Background() }

func testConfig(t *testing.T) Config {
	t.Helper()
	return Config{
		Dir:       t.TempDir(),
		SessionID: "test-session",
		Query:     "test query",
		Model:     "test-model",
	}
}

func TestRecorder_TurnAccumulation(t *testing.T) {
	cfg := testConfig(t)
	hooks, rec := NewTracingHooks(cfg)

	for i := range 3 {
		hooks.OnTurnStart(bgctx(), agentic.TurnState{TurnIndex: i, TokensUsed: i * 100, TokensRemaining: 200000 - i*100})
		time.Sleep(time.Millisecond)
		hooks.OnTurnEnd(bgctx(), agentic.TurnState{TurnIndex: i, TotalCostUSD: float64(i+1) * 0.001, TokensUsed: (i + 1) * 100, TokensRemaining: 200000 - (i+1)*100})
	}

	if got := len(rec.trace.Turns); got != 3 {
		t.Fatalf("expected 3 turns, got %d", got)
	}
	for i, turn := range rec.trace.Turns {
		if turn.Index != i {
			t.Errorf("turn %d: expected index %d, got %d", i, i, turn.Index)
		}
		if turn.DurationMs < 0 {
			t.Errorf("turn %d: negative duration %d", i, turn.DurationMs)
		}
	}
}

func TestRecorder_ToolStackLIFO(t *testing.T) {
	cfg := testConfig(t)
	hooks, rec := NewTracingHooks(cfg)

	hooks.OnTurnStart(bgctx(), agentic.TurnState{TurnIndex: 0})

	input1 := json.RawMessage(`{"query":"rag"}`)
	input2 := json.RawMessage(`{"url":"http://example.com/paper.pdf"}`)

	hooks.OnToolCall(bgctx(), "search_arxiv", input1, agentic.TurnState{})
	time.Sleep(2 * time.Millisecond)
	hooks.OnToolCall(bgctx(), "fetch_pdf", input2, agentic.TurnState{})
	time.Sleep(2 * time.Millisecond)

	hooks.OnToolResult(bgctx(), "fetch_pdf", "pdf content here", agentic.TurnState{})
	hooks.OnToolResult(bgctx(), "search_arxiv", "search results", agentic.TurnState{})

	hooks.OnTurnEnd(bgctx(), agentic.TurnState{TurnIndex: 0, TotalCostUSD: 0.001})

	if got := len(rec.trace.Turns); got != 1 {
		t.Fatalf("expected 1 turn, got %d", got)
	}
	turn := rec.trace.Turns[0]
	if got := len(turn.ToolCalls); got != 2 {
		t.Fatalf("expected 2 tool calls, got %d", got)
	}

	if turn.ToolCalls[0].Name != "search_arxiv" {
		t.Errorf("expected first tool 'search_arxiv', got %q", turn.ToolCalls[0].Name)
	}
	if turn.ToolCalls[0].InputLength != len(input1) {
		t.Errorf("expected input length %d, got %d", len(input1), turn.ToolCalls[0].InputLength)
	}
	if turn.ToolCalls[1].Name != "fetch_pdf" {
		t.Errorf("expected second tool 'fetch_pdf', got %q", turn.ToolCalls[1].Name)
	}
	if turn.ToolCalls[1].ResultLength != len("pdf content here") {
		t.Errorf("expected result length %d, got %d", len("pdf content here"), turn.ToolCalls[1].ResultLength)
	}
}

func TestRecorder_CostDeltaComputation(t *testing.T) {
	cfg := testConfig(t)
	hooks, rec := NewTracingHooks(cfg)

	costs := []float64{0.001, 0.003, 0.007}
	for i, cost := range costs {
		hooks.OnTurnStart(bgctx(), agentic.TurnState{TurnIndex: i})
		hooks.OnTurnEnd(bgctx(), agentic.TurnState{TurnIndex: i, TotalCostUSD: cost})
	}

	expected := []float64{0.001, 0.002, 0.004}
	for i, turn := range rec.trace.Turns {
		if turn.CostUSD != expected[i] {
			t.Errorf("turn %d: expected cost %f, got %f", i, expected[i], turn.CostUSD)
		}
	}
}

func TestRecorder_FlushOutputFormat(t *testing.T) {
	cfg := testConfig(t)
	hooks, rec := NewTracingHooks(cfg)

	hooks.OnTurnStart(bgctx(), agentic.TurnState{TurnIndex: 0})
	hooks.OnTurnEnd(bgctx(), agentic.TurnState{TurnIndex: 0, TotalCostUSD: 0.001, TokensUsed: 500, TokensRemaining: 199500})

	if err := rec.Flush(); err != nil {
		t.Fatalf("flush failed: %v", err)
	}

	path := filepath.Join(cfg.Dir, cfg.SessionID+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}

	var trace Trace
	if err := json.Unmarshal(data, &trace); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if trace.SessionID != "test-session" {
		t.Errorf("expected session 'test-session', got %q", trace.SessionID)
	}
	if trace.Status != "ok" {
		t.Errorf("expected status 'ok', got %q", trace.Status)
	}
	if len(trace.Turns) != 1 {
		t.Errorf("expected 1 turn, got %d", len(trace.Turns))
	}
	if trace.DurationMs < 0 {
		t.Errorf("expected non-negative duration, got %d", trace.DurationMs)
	}
}

func TestRecorder_FlushCreatesDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sub", "dir")
	cfg := Config{Dir: dir, SessionID: "test", Query: "q", Model: "m"}
	_, rec := NewTracingHooks(cfg)

	if err := rec.Flush(); err != nil {
		t.Fatalf("flush failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "test.json")); err != nil {
		t.Fatalf("file not created: %v", err)
	}
}

func TestRecorder_FlushError_BadPath(t *testing.T) {
	cfg := Config{Dir: "/dev/null/impossible", SessionID: "test", Query: "q", Model: "m"}
	_, rec := NewTracingHooks(cfg)

	if err := rec.Flush(); err == nil {
		t.Fatal("expected error for bad path, got nil")
	}
}

func TestRecorder_SetError(t *testing.T) {
	cfg := testConfig(t)
	_, rec := NewTracingHooks(cfg)

	rec.SetError(fmt.Errorf("model not found"))

	if rec.trace.Status != "error" {
		t.Errorf("expected status 'error', got %q", rec.trace.Status)
	}
	if rec.trace.Error != "model not found" {
		t.Errorf("expected error 'model not found', got %q", rec.trace.Error)
	}
}

func TestRecorder_SetErrorNilIsNoOp(t *testing.T) {
	cfg := testConfig(t)
	_, rec := NewTracingHooks(cfg)

	rec.SetError(nil)

	if rec.trace.Status != "ok" {
		t.Errorf("expected status 'ok', got %q", rec.trace.Status)
	}
}

func TestRecorder_DisabledNoOp(t *testing.T) {
	cfg := Config{Dir: "", SessionID: "test", Query: "q", Model: "m"}
	hooks, rec := NewTracingHooks(cfg)

	if hooks != nil {
		t.Error("expected nil hooks when disabled")
	}
	if err := rec.Flush(); err != nil {
		t.Errorf("expected nil error from disabled flush, got %v", err)
	}
}

func TestRecorder_PendingLLMCallAttachedToTurn(t *testing.T) {
	cfg := testConfig(t)
	hooks, rec := NewTracingHooks(cfg)

	hooks.OnTurnStart(bgctx(), agentic.TurnState{TurnIndex: 0})

	rec.mu.Lock()
	rec.pendingLLMCall = &LLMCall{
		Model:        "claude-3-5-haiku",
		InputTokens:  500,
		OutputTokens: 200,
		DurationMs:   1500,
		StopReason:   "end_turn",
	}
	rec.mu.Unlock()

	hooks.OnTurnEnd(bgctx(), agentic.TurnState{TurnIndex: 0, TotalCostUSD: 0.002})

	turn := rec.trace.Turns[0]
	if turn.LLMCall == nil {
		t.Fatal("expected LLMCall to be attached")
	}
	if turn.LLMCall.Model != "claude-3-5-haiku" {
		t.Errorf("expected model 'claude-3-5-haiku', got %q", turn.LLMCall.Model)
	}
	if turn.LLMCall.InputTokens != 500 {
		t.Errorf("expected 500 input tokens, got %d", turn.LLMCall.InputTokens)
	}

	rec.mu.Lock()
	if rec.pendingLLMCall != nil {
		t.Error("expected pendingLLMCall to be cleared")
	}
	rec.mu.Unlock()
}

func TestRecorder_TotalTokensAccumulated(t *testing.T) {
	cfg := testConfig(t)
	hooks, rec := NewTracingHooks(cfg)

	for i := range 2 {
		hooks.OnTurnStart(bgctx(), agentic.TurnState{TurnIndex: i})
		rec.mu.Lock()
		rec.pendingLLMCall = &LLMCall{
			InputTokens:  (i + 1) * 100,
			OutputTokens: (i + 1) * 50,
		}
		rec.mu.Unlock()
		hooks.OnTurnEnd(bgctx(), agentic.TurnState{TurnIndex: i, TotalCostUSD: float64(i+1) * 0.001})
	}

	if rec.trace.TotalInputTokens != 300 {
		t.Errorf("expected 300 total input tokens, got %d", rec.trace.TotalInputTokens)
	}
	if rec.trace.TotalOutputTokens != 150 {
		t.Errorf("expected 150 total output tokens, got %d", rec.trace.TotalOutputTokens)
	}
}
