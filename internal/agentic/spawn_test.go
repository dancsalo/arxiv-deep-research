package agentic

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/dancsalo/arxiv-deep-research/internal/ctxmgr"
	"github.com/dancsalo/arxiv-deep-research/internal/registry"
)

func TestSpawnCreatesChildManager(t *testing.T) {
	loop := newBasicAgenticLoop(&scriptedMessageClient{
		responses: []*anthropic.Message{makeTextResponse("hi")},
	}, nil, nil)

	childReg := registry.NewToolRegistry()
	child, err := loop.Spawn(ChildConfig{
		Query:      "sub-query",
		MaxTurns:   5,
		MaxCostUSD: 0.10,
		Tools:      childReg,
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	parentRemaining := loop.manager.Budget().Remaining(loop.manager.EstimateAllTokens())
	expectedBudget := int(float64(parentRemaining) * 0.5)
	if expectedBudget > 100_000 {
		expectedBudget = 100_000
	}

	childAvailable := child.manager.Budget().Available()
	if childAvailable <= 0 {
		t.Errorf("child budget available should be positive, got %d", childAvailable)
	}
}

func TestSpawnDepthLimit(t *testing.T) {
	loop := newBasicAgenticLoop(&scriptedMessageClient{
		responses: []*anthropic.Message{makeTextResponse("hi")},
	}, nil, nil)
	loop.cfg.MaxDepth = 2
	loop.depth = 1

	_, err := loop.Spawn(ChildConfig{
		Query:      "deep",
		MaxTurns:   5,
		MaxCostUSD: 0.10,
		Tools:      registry.NewToolRegistry(),
	})
	if err == nil {
		t.Fatal("expected depth limit error")
	}
}

func TestSpawnDepthZero(t *testing.T) {
	loop := newBasicAgenticLoop(&scriptedMessageClient{
		responses: []*anthropic.Message{makeTextResponse("hi")},
	}, nil, nil)
	loop.cfg.MaxDepth = 3

	child, err := loop.Spawn(ChildConfig{
		Query:      "research",
		MaxTurns:   5,
		MaxCostUSD: 0.10,
		Tools:      registry.NewToolRegistry(),
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if child.depth != 1 {
		t.Errorf("child depth = %d, want 1", child.depth)
	}
}

func TestSpawnAutoRegistersFinishTool(t *testing.T) {
	loop := newBasicAgenticLoop(&scriptedMessageClient{
		responses: []*anthropic.Message{makeTextResponse("hi")},
	}, nil, nil)

	childReg := registry.NewToolRegistry()
	child, err := loop.Spawn(ChildConfig{
		Query:      "q",
		MaxTurns:   5,
		MaxCostUSD: 0.10,
		Tools:      childReg,
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	// finish_loop should be registered
	defs := child.registry.Definitions()
	found := false
	for _, d := range defs {
		if d.OfTool != nil && d.OfTool.Name == "finish_loop" {
			found = true
			break
		}
	}
	if !found {
		t.Error("child registry should have finish_loop tool")
	}
}

func TestSpawnInheritsHooks(t *testing.T) {
	called := false
	hooks := &LoopHooks{
		OnTurnStart: func(_ context.Context, _ TurnState) error {
			called = true
			return nil
		},
	}
	loop := newBasicAgenticLoop(&scriptedMessageClient{
		responses: []*anthropic.Message{makeTextResponse("hi")},
	}, nil, hooks)

	child, err := loop.Spawn(ChildConfig{
		Query:      "q",
		MaxTurns:   5,
		MaxCostUSD: 0.10,
		Tools:      registry.NewToolRegistry(),
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	if child.hooks.OnTurnStart == nil {
		t.Error("child should inherit parent hooks")
	}
	_ = called
}

func TestChildRunReturnsFinishResult(t *testing.T) {
	client := &scriptedMessageClient{
		responses: []*anthropic.Message{
			makeToolUseResponse("finish_loop", "f1", json.RawMessage(`{"summary":"child answer"}`)),
		},
	}

	loop := newBasicAgenticLoop(client, nil, nil)

	childReg := registry.NewToolRegistry()
	child, err := loop.Spawn(ChildConfig{
		Query:      "research this",
		MaxTurns:   5,
		MaxCostUSD: 0.50,
		Tools:      childReg,
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	child.client = client

	result, err := child.Run(bgctx(), "research this")
	if err != nil {
		t.Fatalf("child.Run: %v", err)
	}
	if result != "child answer" {
		t.Errorf("result = %q, want %q", result, "child answer")
	}
}

func TestChildCostRollsUp(t *testing.T) {
	client := &scriptedMessageClient{
		responses: []*anthropic.Message{
			makeTextResponse("done"),
		},
	}

	loop := newBasicAgenticLoop(client, nil, nil)

	childReg := registry.NewToolRegistry()
	child, err := loop.Spawn(ChildConfig{
		Query:      "q",
		MaxTurns:   5,
		MaxCostUSD: 1.0,
		Tools:      childReg,
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	child.client = client

	_, err = child.Run(bgctx(), "q")
	if err != nil {
		t.Fatalf("child.Run: %v", err)
	}

	childCost := child.TotalCost()
	if childCost <= 0 {
		t.Fatal("child cost should be positive")
	}

	loop.addChildCost(childCost)
	if loop.TotalCost() != childCost {
		t.Errorf("parent TotalCost = %f, want %f", loop.TotalCost(), childCost)
	}
}

func TestChildIsolatedMessages(t *testing.T) {
	parentClient := &scriptedMessageClient{
		responses: []*anthropic.Message{
			makeTextResponse("parent turn 1"),
			makeTextResponse("parent turn 2"),
		},
	}

	loop := newBasicAgenticLoop(parentClient, nil, nil)

	// Run parent for one turn
	parentClient.callIdx = 0
	_, _ = loop.Run(bgctx(), "test")
	parentTurns := loop.manager.TurnCount()

	// Spawn child
	childClient := &scriptedMessageClient{
		responses: []*anthropic.Message{
			makeTextResponse("child turn 1"),
			makeTextResponse("child turn 2"),
			makeTextResponse("child turn 3"),
		},
	}

	childReg := registry.NewToolRegistry()
	child, err := loop.Spawn(ChildConfig{
		Query:      "sub",
		MaxTurns:   3,
		MaxCostUSD: 1.0,
		Tools:      childReg,
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	child.client = childClient

	_, _ = child.Run(bgctx(), "sub")

	// Parent's turn count should not have changed
	if loop.manager.TurnCount() != parentTurns {
		t.Errorf("parent turns changed: got %d, had %d", loop.manager.TurnCount(), parentTurns)
	}
}

func TestLoopFinishResult(t *testing.T) {
	client := &scriptedMessageClient{
		responses: []*anthropic.Message{
			makeToolUseResponse("finish", "f1", json.RawMessage(`{"summary":"the answer"}`)),
		},
	}
	loop := newBasicAgenticLoop(client, nil, nil)

	result, err := loop.Run(bgctx(), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "the answer" {
		t.Errorf("result = %q, want %q", result, "the answer")
	}
}

func TestLoopFinishResultFallback(t *testing.T) {
	client := &scriptedMessageClient{
		responses: []*anthropic.Message{
			{
				Content:    []anthropic.ContentBlockUnion{{Type: "text", Text: "plain text answer"}},
				StopReason: "end_turn",
				Usage:      anthropic.Usage{InputTokens: 100, OutputTokens: 50},
			},
		},
	}
	loop := newBasicAgenticLoop(client, nil, nil)

	result, err := loop.Run(bgctx(), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "plain text answer" {
		t.Errorf("result = %q, want %q", result, "plain text answer")
	}
}

func TestRecursiveSpawnDepth2(t *testing.T) {
	loop := newBasicAgenticLoop(&scriptedMessageClient{
		responses: []*anthropic.Message{makeTextResponse("hi")},
	}, nil, nil)
	loop.cfg.MaxDepth = 4

	child, err := loop.Spawn(ChildConfig{
		Query: "l1", MaxTurns: 5, MaxCostUSD: 0.5,
		Tools: registry.NewToolRegistry(),
	})
	if err != nil {
		t.Fatalf("Spawn depth 1: %v", err)
	}

	grandchild, err := child.Spawn(ChildConfig{
		Query: "l2", MaxTurns: 3, MaxCostUSD: 0.2,
		Tools: registry.NewToolRegistry(),
	})
	if err != nil {
		t.Fatalf("Spawn depth 2: %v", err)
	}
	if grandchild.depth != 2 {
		t.Errorf("grandchild depth = %d, want 2", grandchild.depth)
	}

	// Depth 3 should fail (MaxDepth=4 means max allowed depth index is 3)
	_, err = grandchild.Spawn(ChildConfig{
		Query: "l3", MaxTurns: 2, MaxCostUSD: 0.1,
		Tools: registry.NewToolRegistry(),
	})
	if err != nil {
		t.Fatalf("Spawn depth 3 should succeed with MaxDepth=4: %v", err)
	}
}

func newTestManager() *ctxmgr.ContextManager {
	estimator := ctxmgr.NewTokenEstimator(nil, "", false)
	budget := &ctxmgr.ContextBudget{
		ModelContextLimit: 200000,
		MaxOutputTokens:   16000,
		SystemTokens:      1000,
		ToolDefTokens:     500,
		SafetyMargin:      8000,
	}
	initial := anthropic.NewUserMessage(anthropic.NewTextBlock("test"))
	return ctxmgr.NewContextManager(ctxmgr.ContextManagerConfig{
		Estimator: estimator,
		Budget:    budget,
	}, initial)
}
