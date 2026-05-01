package ctxmgr

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
)

func newTestManager(hasDirect bool, mock TokenCounterClient) *ContextManager {
	estimator := NewTokenEstimator(mock, anthropic.ModelClaudeHaiku4_5, hasDirect)
	budget := &ContextBudget{
		ModelContextLimit: 200000,
		MaxOutputTokens:   16000,
		SystemTokens:      1000,
		ToolDefTokens:     500,
		SafetyMargin:      8000,
	}
	initial := anthropic.NewUserMessage(anthropic.NewTextBlock("test query"))
	frozen := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	return NewContextManager(ContextManagerConfig{
		Estimator: estimator,
		Budget:    budget,
		NowFunc:   func() time.Time { return frozen },
	}, initial)
}

func TestGetTokenCountCachedFresh(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	m := newTestManager(false, nil)
	m.cached = CachedCount{tokens: 5000, dirty: false, countedAt: now}
	m.nowFunc = func() time.Time { return now }

	got, err := m.GetTokenCount(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 5000 {
		t.Errorf("cached token count: got %d, want 5000", got)
	}
}

func TestGetTokenCountExactWhenDirty(t *testing.T) {
	mock := &mockCounterClient{
		result: &anthropic.MessageTokensCount{InputTokens: 6000},
	}
	m := newTestManager(true, mock)
	m.cached.dirty = true

	got, err := m.GetTokenCount(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 6000 {
		t.Errorf("exact token count: got %d, want 6000", got)
	}
	if !mock.called {
		t.Error("expected CountExact to be called")
	}
	if m.cached.dirty {
		t.Error("cache should not be dirty after exact count")
	}
}

func TestGetTokenCountFallbackOnError(t *testing.T) {
	mock := &mockCounterClient{
		err: fmt.Errorf("rate limited"),
	}
	m := newTestManager(true, mock)
	m.cached.dirty = true

	got, err := m.GetTokenCount(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == 0 {
		t.Error("fallback estimate should be non-zero")
	}
}

func TestGetTokenCountHeuristicWhenNoDirect(t *testing.T) {
	m := newTestManager(false, nil)
	m.cached.dirty = true

	got, err := m.GetTokenCount(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == 0 {
		t.Error("heuristic estimate should be non-zero")
	}
}

func TestGetTokenCountStaleCache(t *testing.T) {
	m := newTestManager(false, nil)
	stale := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	m.cached = CachedCount{tokens: 5000, dirty: false, countedAt: stale}
	m.nowFunc = func() time.Time { return stale.Add(31 * time.Second) }

	got, err := m.GetTokenCount(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == 5000 {
		t.Error("should have recalculated after 31 seconds")
	}
}

func TestAddTurnMarksCacheDirty(t *testing.T) {
	m := newTestManager(false, nil)
	m.cached.dirty = false

	assistant := anthropic.NewAssistantMessage(anthropic.NewTextBlock("response"))
	m.AddTurn(assistant, nil, PriorityResearch)

	if !m.cached.dirty {
		t.Error("AddTurn should mark cache dirty")
	}
}

func TestWillFit(t *testing.T) {
	m := newTestManager(false, nil)
	// With a small initial message, most of the budget is available
	if !m.WillFit(1000) {
		t.Error("1000 tokens should fit in a nearly empty context")
	}
}

func TestSpaceNeeded(t *testing.T) {
	m := newTestManager(false, nil)
	needed := m.SpaceNeeded(1000)
	if needed != 0 {
		t.Errorf("SpaceNeeded for 1000 in empty context: got %d, want 0", needed)
	}
}

func TestExtractFinalAnswerEmpty(t *testing.T) {
	m := newTestManager(false, nil)
	got := m.ExtractFinalAnswer()
	if got != "" {
		t.Errorf("ExtractFinalAnswer empty: got %q, want empty", got)
	}
}

func TestExtractFinalAnswer(t *testing.T) {
	m := newTestManager(false, nil)
	assistant := anthropic.NewAssistantMessage(anthropic.NewTextBlock("The answer is 42."))
	m.AddTurn(assistant, nil, PriorityCore)

	got := m.ExtractFinalAnswer()
	if got != "The answer is 42." {
		t.Errorf("ExtractFinalAnswer: got %q, want 'The answer is 42.'", got)
	}
}

func TestTurnCountAndGetTurn(t *testing.T) {
	m := newTestManager(false, nil)
	if m.TurnCount() != 0 {
		t.Errorf("initial TurnCount: got %d, want 0", m.TurnCount())
	}

	assistant := anthropic.NewAssistantMessage(anthropic.NewTextBlock("hello"))
	m.AddTurn(assistant, nil, PriorityResearch)

	if m.TurnCount() != 1 {
		t.Errorf("after AddTurn: got %d, want 1", m.TurnCount())
	}

	turn := m.GetTurn(0)
	if turn == nil {
		t.Fatal("GetTurn(0) returned nil")
	}
	if turn.EstimatedTokens == 0 {
		t.Error("estimated tokens should be non-zero")
	}
}
