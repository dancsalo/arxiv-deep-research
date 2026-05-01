package agentic

import (
	"context"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/dancsalo/arxiv-deep-research/internal/ctxmgr"
)

func TestIntegrationFullHappyPath(t *testing.T) {
	// Simulate a 3-turn conversation: text → text → text (end_turn)
	client := &scriptedMessageClient{
		responses: []*anthropic.Message{
			makeTextResponse("Let me search for that."),
			makeTextResponse("I found some results."),
			makeTextResponse("Quantum computing uses qubits for parallel computation."),
		},
	}

	estimator := ctxmgr.NewTokenEstimator(nil, "", false)
	budget := &ctxmgr.ContextBudget{
		ModelContextLimit: 200000,
		MaxOutputTokens:   16000,
		SystemTokens:      1000,
		ToolDefTokens:     500,
		SafetyMargin:      8000,
	}
	initial := anthropic.NewUserMessage(anthropic.NewTextBlock("What is quantum computing?"))
	manager := ctxmgr.NewContextManager(ctxmgr.ContextManagerConfig{
		Estimator: estimator,
		Budget:    budget,
	}, initial)

	cfg := LoopConfig{
		MaxTurns:   10,
		MaxCostUSD: 1.0,
		Model:      anthropic.ModelClaudeHaiku4_5,
	}
	executor := func(name string, args map[string]any) (string, error) {
		return `{"result": "mock data"}`, nil
	}

	result, err := ResearchLoop(context.Background(), client, "What is quantum computing?", cfg, nil, nil, executor, manager)
	if err != nil {
		t.Fatalf("ResearchLoop: %v", err)
	}
	if result == "" {
		t.Error("expected non-empty result")
	}
	if manager.TurnCount() == 0 {
		t.Error("expected turns to be recorded")
	}
}
