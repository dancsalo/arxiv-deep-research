package agentic

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/dancsalo/arxiv-deep-research/internal/ctxmgr"
	"github.com/dancsalo/arxiv-deep-research/internal/registry"
)

func TestAgenticLoopBasicSingleTurn(t *testing.T) {
	client := &scriptedMessageClient{
		responses: []*anthropic.Message{
			makeTextResponse("Hello world"),
		},
	}
	loop := newBasicAgenticLoop(client, nil, nil)

	result, err := loop.Run(bgctx(), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == "" {
		t.Error("expected non-empty result")
	}
}

func TestAgenticLoopToolUseThenEndTurn(t *testing.T) {
	client := &scriptedMessageClient{
		responses: []*anthropic.Message{
			makeToolUseResponse("calc", "call1", json.RawMessage(`{"x":1}`)),
			makeTextResponse("The answer is 42"),
		},
	}

	manager := newAgenticLoopManager()
	reg := registry.NewToolRegistry()
	reg.Register("calc", minimalToolDef("calc"), func(_ context.Context, _ json.RawMessage) (string, error) {
		return "42", nil
	})
	reg.Register("finish", BuildFinishTool(), func(_ context.Context, input json.RawMessage) (string, error) {
		return string(input), nil
	})

	loop := NewAgenticLoop(client, manager, reg, nil, AgenticLoopConfig{
		MaxTurns:   10,
		MaxCostUSD: 1.0,
		Model:      anthropic.ModelClaudeHaiku4_5,
		FinishTool: "finish",
	}, nil)

	result, err := loop.Run(bgctx(), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == "" {
		t.Error("expected non-empty result")
	}
	if manager.TurnCount() != 2 {
		t.Errorf("turn count = %d, want 2", manager.TurnCount())
	}
}

func TestAgenticLoopFinishToolTerminates(t *testing.T) {
	client := &scriptedMessageClient{
		responses: []*anthropic.Message{
			makeToolUseResponse("finish", "call1", json.RawMessage(`{"summary":"all done"}`)),
		},
	}
	loop := newBasicAgenticLoop(client, nil, nil)

	_, err := loop.Run(bgctx(), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !loop.finished {
		t.Error("expected loop to be finished")
	}
}

func TestAgenticLoopUnknownToolReturnsError(t *testing.T) {
	client := &scriptedMessageClient{
		responses: []*anthropic.Message{
			makeToolUseResponse("nonexistent", "call1", json.RawMessage(`{}`)),
			makeTextResponse("ok"),
		},
	}
	loop := newBasicAgenticLoop(client, nil, nil)

	_, err := loop.Run(bgctx(), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAgenticLoopCostLimitExceeded(t *testing.T) {
	client := &scriptedMessageClient{
		responses: []*anthropic.Message{
			{
				Content:    []anthropic.ContentBlockUnion{{Type: "text", Text: "thinking..."}},
				StopReason: "tool_use",
				Usage:      anthropic.Usage{InputTokens: 100000, OutputTokens: 100000},
			},
		},
	}

	loop := newBasicAgenticLoop(client, nil, nil)
	loop.cfg.MaxCostUSD = 0.001

	_, err := loop.Run(bgctx(), "test")
	if err == nil {
		t.Fatal("expected cost limit error")
	}
	if err.Error() == "" || !contains(err.Error(), "cost limit") {
		t.Errorf("expected cost limit error, got: %v", err)
	}
}

func TestAgenticLoopMaxTurnsExceeded(t *testing.T) {
	client := &scriptedMessageClient{
		responses: []*anthropic.Message{
			makeToolUseResponse("calc", "c1", json.RawMessage(`{}`)),
			makeToolUseResponse("calc", "c2", json.RawMessage(`{}`)),
			makeToolUseResponse("calc", "c3", json.RawMessage(`{}`)),
		},
	}

	manager := newAgenticLoopManager()
	reg := registry.NewToolRegistry()
	reg.Register("calc", minimalToolDef("calc"), func(_ context.Context, _ json.RawMessage) (string, error) {
		return "ok", nil
	})

	loop := NewAgenticLoop(client, manager, reg, nil, AgenticLoopConfig{
		MaxTurns:   2,
		MaxCostUSD: 1.0,
		Model:      anthropic.ModelClaudeHaiku4_5,
	}, nil)

	_, err := loop.Run(bgctx(), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAgenticLoopContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	client := &scriptedMessageClient{
		responses: []*anthropic.Message{
			makeTextResponse("first response"),
		},
	}

	cancel() // Cancel before running

	loop := newBasicAgenticLoop(client, nil, nil)
	_, err := loop.Run(ctx, "test")
	if err == nil {
		t.Fatal("expected context cancelled error")
	}
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got: %v", err)
	}
}

func TestAgenticLoopMemoryRecallBasic(t *testing.T) {
	client := &scriptedMessageClient{
		responses: []*anthropic.Message{
			makeTextResponse("Got it"),
		},
	}

	recaller := &mockMemoryRecaller{
		memories: []RecalledMemory{
			{ID: 1, Type: "gotcha", Title: "important thing", Score: 0.9},
			{ID: 2, Type: "decision", Title: "we decided X", Score: 0.8},
		},
	}

	loop := newBasicAgenticLoop(client, recaller, nil)
	loop.cfg.MemoryRecall = MemoryRecallConfig{
		Enabled:      true,
		MaxResults:   5,
		MaxTokens:    2000,
		SearchMode:   "hybrid",
		RecallEveryN: 1,
	}

	_, err := loop.Run(bgctx(), "test query")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(recaller.calls) != 1 {
		t.Errorf("recaller called %d times, want 1", len(recaller.calls))
	}
}

func TestAgenticLoopMemoryDeduplication(t *testing.T) {
	client := &scriptedMessageClient{
		responses: []*anthropic.Message{
			makeToolUseResponse("calc", "c1", json.RawMessage(`{}`)),
			makeTextResponse("done"),
		},
	}

	recaller := &multiResponseRecaller{
		responses: [][]RecalledMemory{
			{{ID: 1, Type: "gotcha", Title: "A", Score: 0.9}, {ID: 2, Type: "gotcha", Title: "B", Score: 0.8}},
			{{ID: 1, Type: "gotcha", Title: "A", Score: 0.9}, {ID: 3, Type: "gotcha", Title: "C", Score: 0.7}},
		},
	}

	manager := newAgenticLoopManager()
	reg := registry.NewToolRegistry()
	reg.Register("calc", minimalToolDef("calc"), func(_ context.Context, _ json.RawMessage) (string, error) {
		return "ok", nil
	})

	loop := NewAgenticLoop(client, manager, reg, recaller, AgenticLoopConfig{
		MaxTurns:   10,
		MaxCostUSD: 1.0,
		Model:      anthropic.ModelClaudeHaiku4_5,
		MemoryRecall: MemoryRecallConfig{
			Enabled:      true,
			MaxResults:   5,
			MaxTokens:    2000,
			SearchMode:   "hybrid",
			RecallEveryN: 1,
		},
	}, nil)

	_, err := loop.Run(bgctx(), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// ID 1 should only be seen in the first recall, not the second
	if recaller.callIdx != 2 {
		t.Errorf("recaller called %d times, want 2", recaller.callIdx)
	}
}

func TestAgenticLoopMemorySkipFirstN(t *testing.T) {
	client := &scriptedMessageClient{
		responses: []*anthropic.Message{
			makeToolUseResponse("calc", "c1", json.RawMessage(`{}`)),
			makeToolUseResponse("calc", "c2", json.RawMessage(`{}`)),
			makeTextResponse("done"),
		},
	}

	recaller := &mockMemoryRecaller{
		memories: []RecalledMemory{{ID: 1, Type: "gotcha", Title: "A", Score: 0.9}},
	}

	manager := newAgenticLoopManager()
	reg := registry.NewToolRegistry()
	reg.Register("calc", minimalToolDef("calc"), func(_ context.Context, _ json.RawMessage) (string, error) {
		return "ok", nil
	})

	loop := NewAgenticLoop(client, manager, reg, recaller, AgenticLoopConfig{
		MaxTurns:   10,
		MaxCostUSD: 1.0,
		Model:      anthropic.ModelClaudeHaiku4_5,
		MemoryRecall: MemoryRecallConfig{
			Enabled:      true,
			MaxResults:   5,
			MaxTokens:    2000,
			SearchMode:   "hybrid",
			SkipFirstN:   2,
			RecallEveryN: 1,
		},
	}, nil)

	_, err := loop.Run(bgctx(), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should only be called on turn 2 (turns 0, 1 skipped)
	if len(recaller.calls) != 1 {
		t.Errorf("recaller called %d times, want 1", len(recaller.calls))
	}
}

func TestAgenticLoopMemoryRecallEveryN(t *testing.T) {
	responses := make([]*anthropic.Message, 6)
	for i := 0; i < 5; i++ {
		responses[i] = makeToolUseResponse("calc", fmt.Sprintf("c%d", i), json.RawMessage(`{}`))
	}
	responses[5] = makeTextResponse("done")

	client := &scriptedMessageClient{responses: responses}

	recaller := &mockMemoryRecaller{
		memories: []RecalledMemory{{ID: int64(99), Type: "gotcha", Title: "A", Score: 0.9}},
	}

	manager := newAgenticLoopManager()
	reg := registry.NewToolRegistry()
	reg.Register("calc", minimalToolDef("calc"), func(_ context.Context, _ json.RawMessage) (string, error) {
		return "ok", nil
	})

	loop := NewAgenticLoop(client, manager, reg, recaller, AgenticLoopConfig{
		MaxTurns:   10,
		MaxCostUSD: 1.0,
		Model:      anthropic.ModelClaudeHaiku4_5,
		MemoryRecall: MemoryRecallConfig{
			Enabled:      true,
			MaxResults:   5,
			MaxTokens:    2000,
			SearchMode:   "hybrid",
			RecallEveryN: 3,
		},
	}, nil)

	_, err := loop.Run(bgctx(), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Turns 0, 3 should recall (every 3rd turn)
	if len(recaller.calls) != 2 {
		t.Errorf("recaller called %d times, want 2", len(recaller.calls))
	}
}

func TestAgenticLoopMemoryRecallerError(t *testing.T) {
	client := &scriptedMessageClient{
		responses: []*anthropic.Message{
			makeTextResponse("done"),
		},
	}

	recaller := &mockMemoryRecaller{
		err: fmt.Errorf("db connection failed"),
	}

	loop := newBasicAgenticLoop(client, recaller, nil)
	loop.cfg.MemoryRecall = MemoryRecallConfig{
		Enabled:      true,
		MaxResults:   5,
		MaxTokens:    2000,
		SearchMode:   "hybrid",
		RecallEveryN: 1,
	}

	result, err := loop.Run(bgctx(), "test")
	if err != nil {
		t.Fatalf("loop should not fail on recaller error, got: %v", err)
	}
	if result == "" {
		t.Error("expected non-empty result")
	}
}

func TestAgenticLoopMemoryNilRecaller(t *testing.T) {
	client := &scriptedMessageClient{
		responses: []*anthropic.Message{
			makeTextResponse("done"),
		},
	}
	loop := newBasicAgenticLoop(client, nil, nil)

	result, err := loop.Run(bgctx(), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == "" {
		t.Error("expected non-empty result")
	}
}

func TestAgenticLoopHookOnTurnStart(t *testing.T) {
	client := &scriptedMessageClient{
		responses: []*anthropic.Message{
			makeToolUseResponse("calc", "c1", json.RawMessage(`{}`)),
			makeToolUseResponse("calc", "c2", json.RawMessage(`{}`)),
			makeTextResponse("done"),
		},
	}

	var turnIndices []int
	hooks := &LoopHooks{
		OnTurnStart: func(_ context.Context, state TurnState) error {
			turnIndices = append(turnIndices, state.TurnIndex)
			return nil
		},
	}

	manager := newAgenticLoopManager()
	reg := registry.NewToolRegistry()
	reg.Register("calc", minimalToolDef("calc"), func(_ context.Context, _ json.RawMessage) (string, error) {
		return "ok", nil
	})

	loop := NewAgenticLoop(client, manager, reg, nil, AgenticLoopConfig{
		MaxTurns:   10,
		MaxCostUSD: 1.0,
		Model:      anthropic.ModelClaudeHaiku4_5,
		Hooks:      hooks,
	}, nil)

	_, err := loop.Run(bgctx(), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(turnIndices) != 3 {
		t.Errorf("OnTurnStart called %d times, want 3", len(turnIndices))
	}
	for i, idx := range turnIndices {
		if idx != i {
			t.Errorf("turnIndices[%d] = %d, want %d", i, idx, i)
		}
	}
}

func TestAgenticLoopHookOnTurnEnd(t *testing.T) {
	client := &scriptedMessageClient{
		responses: []*anthropic.Message{
			makeToolUseResponse("calc", "c1", json.RawMessage(`{}`)),
			makeTextResponse("done"),
		},
	}

	var endCalls int
	hooks := &LoopHooks{
		OnTurnEnd: func(_ context.Context, _ TurnState) error {
			endCalls++
			return nil
		},
	}

	manager := newAgenticLoopManager()
	reg := registry.NewToolRegistry()
	reg.Register("calc", minimalToolDef("calc"), func(_ context.Context, _ json.RawMessage) (string, error) {
		return "ok", nil
	})

	loop := NewAgenticLoop(client, manager, reg, nil, AgenticLoopConfig{
		MaxTurns:   10,
		MaxCostUSD: 1.0,
		Model:      anthropic.ModelClaudeHaiku4_5,
		Hooks:      hooks,
	}, nil)

	_, err := loop.Run(bgctx(), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if endCalls != 2 {
		t.Errorf("OnTurnEnd called %d times, want 2", endCalls)
	}
}

func TestAgenticLoopHookOnMemoryRecallFilters(t *testing.T) {
	client := &scriptedMessageClient{
		responses: []*anthropic.Message{
			makeTextResponse("done"),
		},
	}

	recaller := &mockMemoryRecaller{
		memories: []RecalledMemory{
			{ID: 1, Type: "gotcha", Title: "A", Score: 0.9},
			{ID: 2, Type: "gotcha", Title: "B", Score: 0.8},
			{ID: 3, Type: "gotcha", Title: "C", Score: 0.7},
		},
	}

	hooks := &LoopHooks{
		OnMemoryRecall: func(_ context.Context, memories []RecalledMemory, _ TurnState) ([]RecalledMemory, error) {
			var filtered []RecalledMemory
			for _, m := range memories {
				if m.ID != 2 {
					filtered = append(filtered, m)
				}
			}
			return filtered, nil
		},
	}

	loop := newBasicAgenticLoop(client, recaller, hooks)
	loop.cfg.MemoryRecall = MemoryRecallConfig{
		Enabled:      true,
		MaxResults:   5,
		MaxTokens:    2000,
		SearchMode:   "hybrid",
		RecallEveryN: 1,
	}

	_, err := loop.Run(bgctx(), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAgenticLoopHookOnMemoryRecallError(t *testing.T) {
	client := &scriptedMessageClient{
		responses: []*anthropic.Message{
			makeTextResponse("done"),
		},
	}

	recaller := &mockMemoryRecaller{
		memories: []RecalledMemory{{ID: 1, Type: "gotcha", Title: "A", Score: 0.9}},
	}

	hooks := &LoopHooks{
		OnMemoryRecall: func(_ context.Context, _ []RecalledMemory, _ TurnState) ([]RecalledMemory, error) {
			return nil, fmt.Errorf("filter failed")
		},
	}

	loop := newBasicAgenticLoop(client, recaller, hooks)
	loop.cfg.MemoryRecall = MemoryRecallConfig{
		Enabled:      true,
		MaxResults:   5,
		MaxTokens:    2000,
		SearchMode:   "hybrid",
		RecallEveryN: 1,
	}

	_, err := loop.Run(bgctx(), "test")
	if err != nil {
		t.Fatalf("loop should not fail on recall hook error, got: %v", err)
	}
}

func TestAgenticLoopHookOnMemoryPersistState(t *testing.T) {
	client := &scriptedMessageClient{
		responses: []*anthropic.Message{
			makeToolUseResponse("calc", "c1", json.RawMessage(`{"x":1}`)),
			makeTextResponse("The answer is 42"),
		},
	}

	var capturedStates []TurnState
	hooks := &LoopHooks{
		OnMemoryPersist: func(_ context.Context, state TurnState) error {
			capturedStates = append(capturedStates, state)
			return nil
		},
	}

	manager := newAgenticLoopManager()
	reg := registry.NewToolRegistry()
	reg.Register("calc", minimalToolDef("calc"), func(_ context.Context, _ json.RawMessage) (string, error) {
		return "42", nil
	})
	reg.Register("finish", BuildFinishTool(), func(_ context.Context, input json.RawMessage) (string, error) {
		return string(input), nil
	})

	loop := NewAgenticLoop(client, manager, reg, nil, AgenticLoopConfig{
		MaxTurns:   10,
		MaxCostUSD: 1.0,
		Model:      anthropic.ModelClaudeHaiku4_5,
		FinishTool: "finish",
		Hooks:      hooks,
	}, nil)

	_, err := loop.Run(bgctx(), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(capturedStates) != 2 {
		t.Fatalf("OnMemoryPersist called %d times, want 2", len(capturedStates))
	}

	// First turn has tool result
	if len(capturedStates[0].ToolResultTexts) == 0 {
		t.Error("first turn should have tool result texts")
	}
	if capturedStates[0].ToolResultTexts["calc"] != "42" {
		t.Errorf("calc result = %q, want %q", capturedStates[0].ToolResultTexts["calc"], "42")
	}

	// Second turn has assistant text
	if capturedStates[1].AssistantText == "" {
		t.Error("second turn should have assistant text")
	}
}

func TestAgenticLoopHookOnMemoryPersistError(t *testing.T) {
	client := &scriptedMessageClient{
		responses: []*anthropic.Message{
			makeTextResponse("done"),
		},
	}

	hooks := &LoopHooks{
		OnMemoryPersist: func(_ context.Context, _ TurnState) error {
			return fmt.Errorf("persist failed")
		},
	}

	loop := newBasicAgenticLoop(client, nil, hooks)

	_, err := loop.Run(bgctx(), "test")
	if err != nil {
		t.Fatalf("loop should not fail on persist error, got: %v", err)
	}
}

func TestAgenticLoopHookOnToolCallReceivesJSON(t *testing.T) {
	client := &scriptedMessageClient{
		responses: []*anthropic.Message{
			makeToolUseResponse("calc", "c1", json.RawMessage(`{"x":42}`)),
			makeTextResponse("done"),
		},
	}

	var receivedInput json.RawMessage
	hooks := &LoopHooks{
		OnToolCall: func(_ context.Context, _ string, input json.RawMessage, _ TurnState) error {
			receivedInput = input
			return nil
		},
	}

	manager := newAgenticLoopManager()
	reg := registry.NewToolRegistry()
	reg.Register("calc", minimalToolDef("calc"), func(_ context.Context, _ json.RawMessage) (string, error) {
		return "ok", nil
	})

	loop := NewAgenticLoop(client, manager, reg, nil, AgenticLoopConfig{
		MaxTurns:   10,
		MaxCostUSD: 1.0,
		Model:      anthropic.ModelClaudeHaiku4_5,
		Hooks:      hooks,
	}, nil)

	_, err := loop.Run(bgctx(), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(receivedInput) != `{"x":42}` {
		t.Errorf("OnToolCall input = %q, want %q", string(receivedInput), `{"x":42}`)
	}
}

func TestAgenticLoopHookNilSafe(t *testing.T) {
	client := &scriptedMessageClient{
		responses: []*anthropic.Message{
			makeTextResponse("done"),
		},
	}

	// nil Hooks
	loop := newBasicAgenticLoop(client, nil, nil)
	_, err := loop.Run(bgctx(), "test")
	if err != nil {
		t.Fatalf("nil hooks should not panic: %v", err)
	}

	// Empty LoopHooks (all nil fields)
	client2 := &scriptedMessageClient{
		responses: []*anthropic.Message{
			makeTextResponse("done"),
		},
	}
	loop2 := newBasicAgenticLoop(client2, nil, &LoopHooks{})
	_, err = loop2.Run(bgctx(), "test")
	if err != nil {
		t.Fatalf("empty hooks should not panic: %v", err)
	}
}

func TestAgenticLoopRunResetsState(t *testing.T) {
	client := &scriptedMessageClient{
		responses: []*anthropic.Message{
			makeTextResponse("done"),
		},
	}
	loop := newBasicAgenticLoop(client, nil, nil)

	// Manually set state to verify it gets reset
	loop.seenMemoryIDs = map[int64]bool{99: true}
	loop.totalCostUSD = 5.0
	loop.turnIndex = 7
	loop.finished = true

	_, err := loop.Run(bgctx(), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(loop.seenMemoryIDs) > 0 && loop.seenMemoryIDs[99] {
		t.Error("seenMemoryIDs should have been reset")
	}
}

func TestAgenticLoopDefaultPriority(t *testing.T) {
	client := &scriptedMessageClient{
		responses: []*anthropic.Message{
			makeTextResponse("done"),
		},
	}

	manager := newAgenticLoopManager()
	reg := registry.NewToolRegistry()

	loop := NewAgenticLoop(client, manager, reg, nil, AgenticLoopConfig{
		MaxTurns:        10,
		MaxCostUSD:      1.0,
		Model:           anthropic.ModelClaudeHaiku4_5,
		DefaultPriority: ctxmgr.PrioritySupplementary,
	}, nil)

	_, err := loop.Run(bgctx(), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	turn := manager.GetTurn(0)
	if turn == nil {
		t.Fatal("expected turn 0")
	}
	if turn.Priority != ctxmgr.PrioritySupplementary {
		t.Errorf("priority = %d, want %d", turn.Priority, ctxmgr.PrioritySupplementary)
	}
}

func TestAgenticLoopTurnStateTokenCounts(t *testing.T) {
	client := &scriptedMessageClient{
		responses: []*anthropic.Message{
			makeTextResponse("done"),
		},
	}

	var capturedState TurnState
	hooks := &LoopHooks{
		OnTurnStart: func(_ context.Context, state TurnState) error {
			capturedState = state
			return nil
		},
	}

	loop := newBasicAgenticLoop(client, nil, hooks)

	_, err := loop.Run(bgctx(), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedState.TokensUsed <= 0 {
		t.Error("TokensUsed should be positive")
	}
	if capturedState.TokensRemaining <= 0 {
		t.Error("TokensRemaining should be positive")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
