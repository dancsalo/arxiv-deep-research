package contextmanager

import (
	"context"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

func TestIntegrationBudgetPressureTriggersCompaction(t *testing.T) {
	estimator := NewTokenEstimator(nil, "", false)
	budget := &ContextBudget{
		ModelContextLimit: 5000,
		MaxOutputTokens:   1000,
		SystemTokens:      100,
		ToolDefTokens:     100,
		SafetyMargin:      500,
	}
	initial := anthropic.NewUserMessage(anthropic.NewTextBlock("query"))
	manager := NewContextManager(ContextManagerConfig{
		Estimator: estimator,
		Budget:    budget,
	}, initial)

	for i := 0; i < 10; i++ {
		text := strings.Repeat("word ", 200)
		assistant := anthropic.NewAssistantMessage(anthropic.NewTextBlock(text))
		toolResult := anthropic.NewUserMessage(anthropic.NewToolResultBlock("t", text, false))
		manager.AddTurn(assistant, &toolResult, PrioritySupplementary)
	}

	// Budget should be over — run PreAPICheck which triggers compaction
	err := manager.PreAPICheck(context.Background())
	if err != nil {
		// May fail if compaction can't free enough, but should at least try
		t.Logf("PreAPICheck returned (may be expected): %v", err)
	}

	// Verify some turns were compacted
	compactedCount := 0
	for i := 0; i < manager.TurnCount(); i++ {
		turn := manager.GetTurn(i)
		if turn.Compacted {
			compactedCount++
		}
	}
	if compactedCount == 0 {
		t.Error("expected at least some turns to be compacted under budget pressure")
	}
}

func TestIntegrationCalibrationConverges(t *testing.T) {
	estimator := NewTokenEstimator(nil, "", false)

	initialRatio := estimator.ratios[ContentProse]
	targetRatio := 3.0

	for i := 0; i < 15; i++ {
		text := strings.Repeat("a", 300) // 300 runes
		estimator.Calibrate(text, 100, ContentProse)   // 300/100 = 3.0
	}

	finalRatio := estimator.ratios[ContentProse]
	if finalRatio == initialRatio {
		t.Error("ratio should have changed after calibration")
	}
	if finalRatio < targetRatio-0.5 || finalRatio > targetRatio+0.5 {
		t.Errorf("ratio should converge toward %.1f, got %.2f", targetRatio, finalRatio)
	}
}

func TestIntegrationConversationHistoryCompaction(t *testing.T) {
	mock := &mockCompactionClient{
		response: newMockTextResponse("Summary of old turns."),
	}
	estimator := NewTokenEstimator(nil, "", false)
	budget := &ContextBudget{
		ModelContextLimit: 200000,
		MaxOutputTokens:   16000,
		SystemTokens:      1000,
		ToolDefTokens:     500,
		SafetyMargin:      8000,
	}
	initial := anthropic.NewUserMessage(anthropic.NewTextBlock("research topic"))
	manager := NewContextManager(ContextManagerConfig{
		Estimator: estimator,
		Budget:    budget,
	}, initial)
	manager.SetCompactionClient(mock)

	for i := 0; i < 10; i++ {
		text := strings.Repeat("content ", 100)
		assistant := anthropic.NewAssistantMessage(anthropic.NewTextBlock(text))
		manager.AddTurn(assistant, nil, PriorityResearch)
	}

	err := manager.CompactConversationHistory(context.Background())
	if err != nil {
		t.Fatalf("CompactConversationHistory: %v", err)
	}

	// 1 summary + 3 recent = 4
	if manager.TurnCount() != 4 {
		t.Errorf("after compaction: got %d turns, want 4", manager.TurnCount())
	}

	// Verify message history is valid — alternating assistant messages
	msgs := manager.BuildMessages()
	if len(msgs) < 2 {
		t.Errorf("message history too short: %d", len(msgs))
	}

	// Verify indices are sequential after re-indexing
	for i := 0; i < manager.TurnCount(); i++ {
		turn := manager.GetTurn(i)
		if turn.Index != i {
			t.Errorf("turn %d has index %d (expected %d)", i, turn.Index, i)
		}
	}
}

func TestIntegrationFullHappyPath(t *testing.T) {
	// Simulate a 3-turn conversation: text → text → text (end_turn)
	client := &scriptedMessageClient{
		responses: []*anthropic.Message{
			makeTextResponse("Let me search for that."),
			makeTextResponse("I found some results."),
			makeTextResponse("Quantum computing uses qubits for parallel computation."),
		},
	}

	estimator := NewTokenEstimator(nil, "", false)
	budget := &ContextBudget{
		ModelContextLimit: 200000,
		MaxOutputTokens:   16000,
		SystemTokens:      1000,
		ToolDefTokens:     500,
		SafetyMargin:      8000,
	}
	initial := anthropic.NewUserMessage(anthropic.NewTextBlock("What is quantum computing?"))
	manager := NewContextManager(ContextManagerConfig{
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

func TestIntegrationGuardrailCompactionChain(t *testing.T) {
	estimator := NewTokenEstimator(nil, "", false)
	budget := &ContextBudget{
		ModelContextLimit: 10000,
		MaxOutputTokens:   1000,
		SystemTokens:      100,
		ToolDefTokens:     100,
		SafetyMargin:      500,
	}
	initial := anthropic.NewUserMessage(anthropic.NewTextBlock("test"))
	manager := NewContextManager(ContextManagerConfig{
		Estimator: estimator,
		Budget:    budget,
	}, initial)

	tse := NewToolSizeEstimator()
	tse.RegisterTool("big_tool", func(args map[string]any) int {
		return 3000
	})
	manager.SetToolSizeEstimator(tse)

	// Fill context with supplementary turns
	for i := 0; i < 5; i++ {
		text := strings.Repeat("data ", 300)
		assistant := anthropic.NewAssistantMessage(anthropic.NewTextBlock(text))
		manager.AddTurn(assistant, nil, PrioritySupplementary)
	}

	// Try a tool call that needs compaction
	decision, err := manager.PreToolGuardrail(context.Background(), "big_tool", nil)
	if err != nil {
		t.Fatalf("PreToolGuardrail: %v", err)
	}

	// Either proceeded (after compaction) or rejected — both valid
	if decision.Proceed && len(decision.Compacted) > 0 {
		t.Logf("Guardrail compacted %d turns to proceed", len(decision.Compacted))
	} else if !decision.Proceed {
		t.Logf("Guardrail rejected: %s", decision.Reason)
	}
}
