package contextmanager

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

type scriptedMessageClient struct {
	responses []*anthropic.Message
	callIdx   int
}

func (s *scriptedMessageClient) CreateMessage(_ context.Context, _ anthropic.MessageNewParams) (*anthropic.Message, error) {
	if s.callIdx >= len(s.responses) {
		return nil, fmt.Errorf("no more scripted responses")
	}
	resp := s.responses[s.callIdx]
	s.callIdx++
	return resp, nil
}

func makeTextResponse(text string) *anthropic.Message {
	return &anthropic.Message{
		Content: []anthropic.ContentBlockUnion{
			{Type: "text", Text: text},
		},
		StopReason: "end_turn",
		Usage:      anthropic.Usage{InputTokens: 100, OutputTokens: 50},
	}
}

func newLoopManager() *ContextManager {
	estimator := NewTokenEstimator(nil, "", false)
	budget := &ContextBudget{
		ModelContextLimit: 200000,
		MaxOutputTokens:   16000,
		SystemTokens:      1000,
		ToolDefTokens:     500,
		SafetyMargin:      8000,
	}
	initial := anthropic.NewUserMessage(anthropic.NewTextBlock("What is quantum computing?"))
	return NewContextManager(ContextManagerConfig{
		Estimator: estimator,
		Budget:    budget,
	}, initial)
}

func TestResearchLoopMaxTurns(t *testing.T) {
	client := &scriptedMessageClient{
		responses: []*anthropic.Message{
			makeTextResponse("Thinking about it..."),
			makeTextResponse("Still thinking..."),
			makeTextResponse("Here's the answer: quantum computing is cool."),
		},
	}
	manager := newLoopManager()

	cfg := LoopConfig{
		MaxTurns:   3,
		MaxCostUSD: 1.0,
		Model:      anthropic.ModelClaudeHaiku4_5,
	}
	executor := func(name string, args map[string]any) (string, error) {
		return `{}`, nil
	}

	result, err := ResearchLoop(bgctx(), client, "test", cfg, nil, nil, executor, manager)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == "" {
		t.Error("expected non-empty result")
	}
}

func TestResearchLoopCostLimit(t *testing.T) {
	resp := makeTextResponse("response")
	resp.Usage = anthropic.Usage{InputTokens: 1_000_000, OutputTokens: 1_000_000}

	client := &scriptedMessageClient{
		responses: []*anthropic.Message{resp},
	}
	manager := newLoopManager()

	cfg := LoopConfig{
		MaxTurns:   10,
		MaxCostUSD: 0.0001,
		Model:      anthropic.ModelClaudeHaiku4_5,
	}
	executor := func(name string, args map[string]any) (string, error) {
		return `{}`, nil
	}

	_, err := ResearchLoop(bgctx(), client, "test", cfg, nil, nil, executor, manager)
	if err == nil {
		t.Fatal("expected cost limit error")
	}
}

func TestResearchLoopEndTurn(t *testing.T) {
	client := &scriptedMessageClient{
		responses: []*anthropic.Message{
			makeTextResponse("The answer is 42."),
		},
	}
	manager := newLoopManager()

	cfg := LoopConfig{
		MaxTurns:   10,
		MaxCostUSD: 1.0,
		Model:      anthropic.ModelClaudeHaiku4_5,
	}
	executor := func(name string, args map[string]any) (string, error) {
		return `{}`, nil
	}

	result, err := ResearchLoop(bgctx(), client, "test", cfg, nil, nil, executor, manager)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "The answer is 42." {
		t.Errorf("result: got %q, want 'The answer is 42.'", result)
	}
}

func TestResearchLoopAPIError(t *testing.T) {
	client := &scriptedMessageClient{
		responses: nil, // no responses → error
	}
	manager := newLoopManager()

	cfg := LoopConfig{
		MaxTurns:   1,
		MaxCostUSD: 1.0,
		Model:      anthropic.ModelClaudeHaiku4_5,
	}
	executor := func(name string, args map[string]any) (string, error) {
		return `{}`, nil
	}

	_, err := ResearchLoop(bgctx(), client, "test", cfg, nil, nil, executor, manager)
	if err == nil {
		t.Fatal("expected API error")
	}
}

func TestEstimateCost(t *testing.T) {
	usage := anthropic.Usage{InputTokens: 1000, OutputTokens: 500}
	cost := estimateCost(usage, anthropic.ModelClaudeHaiku4_5)
	if cost <= 0 {
		t.Error("cost should be positive")
	}
	expected := 0.0028
	if cost < expected*0.9 || cost > expected*1.1 {
		t.Errorf("cost: got %f, want ~%f", cost, expected)
	}
}

func TestEstimateCostDefaultModel(t *testing.T) {
	usage := anthropic.Usage{InputTokens: 1000, OutputTokens: 500}
	cost := estimateCost(usage, "unknown-model")
	if cost <= 0 {
		t.Error("cost should be positive for unknown model (uses default)")
	}
}

func TestJsonToMap(t *testing.T) {
	data := json.RawMessage(`{"key": "value", "num": 42}`)
	m := jsonToMap(data)
	if m["key"] != "value" {
		t.Errorf("key: got %v, want 'value'", m["key"])
	}
}

func TestJsonToMapInvalid(t *testing.T) {
	data := json.RawMessage(`not json`)
	m := jsonToMap(data)
	if len(m) != 0 {
		t.Errorf("invalid json should return empty map, got %v", m)
	}
}

func TestBuildFinishTool(t *testing.T) {
	tool := BuildFinishTool()
	if tool.OfTool == nil {
		t.Fatal("expected OfTool to be set")
	}
	if tool.OfTool.Name != "finish_research" {
		t.Errorf("tool name: got %s, want 'finish_research'", tool.OfTool.Name)
	}
}

func bgctx() context.Context {
	return context.Background()
}
