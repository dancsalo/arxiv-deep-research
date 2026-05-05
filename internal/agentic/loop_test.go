package agentic

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/dancsalo/arxiv-deep-research/internal/ctxmgr"
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

func newLoopManager() *ctxmgr.ContextManager {
	estimator := ctxmgr.NewTokenEstimator(nil, "", false)
	budget := &ctxmgr.ContextBudget{
		ModelContextLimit: 200000,
		MaxOutputTokens:   16000,
		SystemTokens:      1000,
		ToolDefTokens:     500,
		SafetyMargin:      8000,
	}
	initial := anthropic.NewUserMessage(anthropic.NewTextBlock("What is quantum computing?"))
	return ctxmgr.NewContextManager(ctxmgr.ContextManagerConfig{
		Estimator: estimator,
		Budget:    budget,
	}, initial)
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
	if tool.OfTool.Name != "finish_loop" {
		t.Errorf("tool name: got %s, want 'finish_loop'", tool.OfTool.Name)
	}
}

func bgctx() context.Context {
	return context.Background()
}
