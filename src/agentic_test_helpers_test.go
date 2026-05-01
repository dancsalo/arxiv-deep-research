package contextmanager

import (
	"context"
	"encoding/json"

	"github.com/anthropics/anthropic-sdk-go"
)

type mockMemoryRecaller struct {
	memories []RecalledMemory
	err      error
	calls    []recallCall
}

type recallCall struct {
	query string
	mode  string
	limit int
}

func (m *mockMemoryRecaller) RecallMemories(_ context.Context, query, mode string, limit int) ([]RecalledMemory, error) {
	m.calls = append(m.calls, recallCall{query, mode, limit})
	return m.memories, m.err
}

type multiResponseRecaller struct {
	responses [][]RecalledMemory
	callIdx   int
}

func (m *multiResponseRecaller) RecallMemories(_ context.Context, _, _ string, _ int) ([]RecalledMemory, error) {
	if m.callIdx >= len(m.responses) {
		return nil, nil
	}
	resp := m.responses[m.callIdx]
	m.callIdx++
	return resp, nil
}

func makeToolUseResponse(toolName, id string, input json.RawMessage) *anthropic.Message {
	return &anthropic.Message{
		Content: []anthropic.ContentBlockUnion{
			{
				Type:  "tool_use",
				ID:    id,
				Name:  toolName,
				Input: input,
			},
		},
		StopReason: "tool_use",
		Usage:      anthropic.Usage{InputTokens: 100, OutputTokens: 50},
	}
}

func newAgenticLoopManager() *ContextManager {
	estimator := NewTokenEstimator(nil, "", false)
	budget := &ContextBudget{
		ModelContextLimit: 200000,
		MaxOutputTokens:   16000,
		SystemTokens:      1000,
		ToolDefTokens:     500,
		SafetyMargin:      8000,
	}
	initial := anthropic.NewUserMessage(anthropic.NewTextBlock("test query"))
	return NewContextManager(ContextManagerConfig{
		Estimator: estimator,
		Budget:    budget,
	}, initial)
}

func newBasicAgenticLoop(client MessageClient, recaller MemoryRecaller, hooks *LoopHooks) *AgenticLoop {
	manager := newAgenticLoopManager()
	registry := NewToolRegistry()
	registry.Register("finish", BuildFinishTool(), func(_ context.Context, input json.RawMessage) (string, error) {
		return string(input), nil
	})

	return NewAgenticLoop(client, manager, registry, recaller, AgenticLoopConfig{
		MaxTurns:   10,
		MaxCostUSD: 1.0,
		Model:      anthropic.ModelClaudeHaiku4_5,
		FinishTool: "finish",
		Hooks:      hooks,
	}, nil)
}
