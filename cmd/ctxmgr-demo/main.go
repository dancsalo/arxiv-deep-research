package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/dancsalo/arxiv-deep-research/internal/ctxmgr"
)

type mockCompactionClient struct {
	calls int
}

func (m *mockCompactionClient) CreateMessage(_ context.Context, _ anthropic.MessageNewParams) (*anthropic.Message, error) {
	m.calls++
	return &anthropic.Message{
		Content: []anthropic.ContentBlockUnion{
			{Type: "text", Text: fmt.Sprintf("Summary #%d: key findings from earlier research.", m.calls)},
		},
	}, nil
}

func main() {
	ctx := context.Background()

	// =========================================================
	// 1. SETUP
	// =========================================================
	fmt.Println("=== 1. Setup ===")

	budget := &ctxmgr.ContextBudget{
		ModelContextLimit: 5000,
		MaxOutputTokens:   500,
		SystemTokens:      200,
		ToolDefTokens:     100,
		SafetyMargin:      300,
	}
	fmt.Printf("Budget: %d limit, %d available for conversation\n",
		budget.ModelContextLimit, budget.Available())

	estimator := ctxmgr.NewTokenEstimator(nil, "", false)
	fmt.Println("Estimator: fast-only mode (no API counting)")

	initial := anthropic.NewUserMessage(anthropic.NewTextBlock("Find recent papers on transformer attention mechanisms"))
	manager := ctxmgr.NewContextManager(ctxmgr.ContextManagerConfig{
		Estimator: estimator,
		Budget:    budget,
	}, initial)

	mock := &mockCompactionClient{}
	manager.SetCompactionClient(mock)

	tse := ctxmgr.NewToolSizeEstimator()
	tse.RegisterTool("search_arxiv", func(args map[string]any) int {
		n := 10
		if v, ok := args["max_results"]; ok {
			if f, ok := v.(float64); ok {
				n = int(f)
			}
		}
		return n * 400
	})
	tse.RegisterTool("fetch_page_text", func(_ map[string]any) int {
		return 3000
	})
	manager.SetToolSizeEstimator(tse)
	fmt.Println("Tool size estimator: search_arxiv (400/result), fetch_page_text (3000)")
	fmt.Println()

	// =========================================================
	// 2. AGENTIC LOOP (correct lifecycle per turn)
	//    PreAPICheck → call Claude → PreToolGuardrail → execute tool → PostToolCheck → AddTurn
	// =========================================================
	fmt.Println("=== 2. Agentic Loop (with guardrails) ===")

	turns := []struct {
		text     string
		toolName string
		priority ctxmgr.TurnPriority
		label    string
	}{
		{strings.Repeat("Searching arxiv for papers. ", 20), "search_arxiv", ctxmgr.PriorityResearch, "search call"},
		{strings.Repeat("Found 5 relevant papers. ", 15), "", ctxmgr.PriorityResearch, "search results"},
		{strings.Repeat("Fetching paper abstract. ", 25), "fetch_page_text", ctxmgr.PrioritySupplementary, "fetch abstract"},
		{strings.Repeat("The paper discusses multi-head attention. ", 20), "", ctxmgr.PriorityResearch, "reading paper"},
		{strings.Repeat("Comparing methodologies across papers. ", 15), "", ctxmgr.PriorityCore, "analysis"},
		{strings.Repeat("Secondary reference lookup. ", 30), "fetch_page_text", ctxmgr.PrioritySupplementary, "secondary ref"},
		{strings.Repeat("Cross-referencing citations. ", 20), "search_arxiv", ctxmgr.PrioritySupplementary, "citations"},
		{strings.Repeat("Final synthesis of findings. ", 10), "", ctxmgr.PriorityCore, "synthesis"},
	}

	for i, t := range turns {
		fmt.Printf("\n  --- Turn %d [%s] ---\n", i+1, t.label)

		// Step A: PreAPICheck before calling Claude
		if err := manager.PreAPICheck(ctx); err != nil {
			fmt.Printf("  PreAPICheck: %v\n", err)
		}

		// Step B: Claude responds (simulated) — assistant text
		assistant := anthropic.NewAssistantMessage(anthropic.NewTextBlock(t.text))

		var toolResult *anthropic.MessageParam
		if t.toolName != "" {
			// Step C: PreToolGuardrail — can we afford this tool result?
			args := map[string]any{"max_results": float64(10)}
			if t.toolName == "fetch_page_text" {
				args = map[string]any{"url": "https://arxiv.org/abs/2301.00001"}
			}

			decision, err := manager.PreToolGuardrail(ctx, t.toolName, args)
			if err != nil {
				fmt.Printf("  Guardrail error: %v\n", err)
			} else if !decision.Proceed {
				fmt.Printf("  Guardrail BLOCKED: %s\n", decision.Reason)
				// Still add the assistant message without tool results
				manager.AddTurn(assistant, nil, t.priority)
				total := manager.EstimateAllTokens()
				fmt.Printf("  Added (no tool): total≈%d, remaining≈%d\n", total, budget.Remaining(total))
				continue
			} else {
				if len(decision.Compacted) > 0 {
					fmt.Printf("  Guardrail: compacted %d turns to fit\n", len(decision.Compacted))
				}
				if decision.ModifiedArgs != nil {
					fmt.Printf("  Guardrail: reduced args → %v\n", decision.ModifiedArgs)
					args = decision.ModifiedArgs
				}
				fmt.Printf("  Guardrail: proceed ✓\n")
			}

			// Step D: Execute the tool (simulated result)
			resultText := strings.Repeat("result data ", 30)
			tr := anthropic.NewUserMessage(anthropic.NewToolResultBlock(
				fmt.Sprintf("tool-%d", i), resultText, false,
			))
			toolResult = &tr

			// Step E: PostToolCheck — record actual size, emergency compact if needed
			if err := manager.PostToolCheck(ctx, t.toolName, resultText); err != nil {
				fmt.Printf("  PostToolCheck: %v\n", err)
			}
		}

		// Step F: AddTurn — store in message log
		manager.AddTurn(assistant, toolResult, t.priority)

		total := manager.EstimateAllTokens()
		remaining := budget.Remaining(total)
		fmt.Printf("  Added: priority=%d | total≈%d tokens, remaining≈%d\n",
			t.priority, total, remaining)
	}
	fmt.Println()

	// =========================================================
	// 3. POST-LOOP STATE
	// =========================================================
	fmt.Println("=== 3. Post-Loop State ===")

	totalAfter := manager.EstimateAllTokens()
	fmt.Printf("  Total tokens: %d, remaining=%d\n",
		totalAfter, budget.Remaining(totalAfter))
	fmt.Printf("  Compaction client called %d time(s)\n", mock.calls)

	compactedCount := 0
	for i := 0; i < manager.TurnCount(); i++ {
		turn := manager.GetTurn(i)
		if turn.Compacted {
			compactedCount++
		}
	}
	fmt.Printf("  Turns compacted: %d / %d\n", compactedCount, manager.TurnCount())
	fmt.Println()

	// =========================================================
	// 4. ESTIMATOR CALIBRATION
	// =========================================================
	fmt.Println("=== 4. Estimator Calibration ===")

	sampleText := strings.Repeat("The transformer architecture uses self-attention. ", 50)
	before := manager.EstimateText(sampleText, ctxmgr.ContentProse)
	fmt.Printf("  Before calibration: \"%s...\" ≈ %d tokens\n", sampleText[:40], before)

	for i := 0; i < 10; i++ {
		estimator.Calibrate(sampleText, len([]rune(sampleText))/3, ctxmgr.ContentProse)
	}

	after := manager.EstimateText(sampleText, ctxmgr.ContentProse)
	fmt.Printf("  After 10 calibrations (ratio→3.0): same text ≈ %d tokens\n", after)
	fmt.Printf("  Drift: %+d tokens\n", after-before)
	fmt.Println()

	// =========================================================
	// 5. OUTPUT TRACKER
	// =========================================================
	fmt.Println("=== 5. Output Tracker ===")

	tracker := manager.OutputTracker()
	outputs := []int{2100, 3400, 1800, 5200, 2900, 4100, 3300}
	for _, o := range outputs {
		tracker.Record(o)
		fmt.Printf("  Recorded output: %d tokens\n", o)
	}
	fmt.Printf("  Max observed: %d\n", tracker.MaxObserved())
	fmt.Printf("  Recommended reservation (p95): %d\n", tracker.RecommendedReservation())
	fmt.Println()

	// =========================================================
	// 6. FINAL STATE
	// =========================================================
	fmt.Println("=== 6. Final State ===")
	fmt.Printf("  Turn count: %d\n", manager.TurnCount())

	for i := 0; i < manager.TurnCount(); i++ {
		turn := manager.GetTurn(i)
		status := "intact"
		if turn.Compacted {
			status = fmt.Sprintf("compacted (level %d)", turn.CompactionLevel)
		}
		fmt.Printf("    Turn %d: priority=%d, ~%d tokens, %s\n",
			i, turn.Priority, turn.EstimatedTokens, status)
	}

	finalTotal := manager.EstimateAllTokens()
	fmt.Printf("  Total tokens: %d / %d available\n", finalTotal, budget.Available())
	fmt.Printf("  Final answer preview: %q\n", truncate(manager.ExtractFinalAnswer(), 60))
}

func truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "..."
}
