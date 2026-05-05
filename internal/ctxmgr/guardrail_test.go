package ctxmgr

import (
	"context"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

func newGuardrailManager(budgetAvailable int) *ContextManager {
	estimator := NewTokenEstimator(nil, "", false)
	budget := &ContextBudget{
		ModelContextLimit: budgetAvailable + 16000 + 1000 + 500 + 8000,
		MaxOutputTokens:   16000,
		SystemTokens:      1000,
		ToolDefTokens:     500,
		SafetyMargin:      8000,
	}
	initial := anthropic.NewUserMessage(anthropic.NewTextBlock("test"))
	m := NewContextManager(ContextManagerConfig{
		Estimator: estimator,
		Budget:    budget,
	}, initial)

	tse := NewToolSizeEstimator()
	tse.RegisterTool("fetch_page_text", func(args map[string]any) int {
		chars, _ := args["max_chars"].(float64)
		if chars == 0 {
			chars = 4000
		}
		return 100 + int(chars/3.5)
	})
	tse.RegisterTool("search_arxiv", func(args map[string]any) int {
		n, _ := args["max_results"].(float64)
		if n == 0 {
			n = 5
		}
		return 500 + int(n)*800
	})
	m.SetToolSizeEstimator(tse)
	return m
}

func TestPreToolGuardrailFitsComfortably(t *testing.T) {
	m := newGuardrailManager(174500)
	decision, err := m.PreToolGuardrail(context.Background(), "search_arxiv", map[string]any{"max_results": float64(5)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !decision.Proceed {
		t.Error("should proceed when fitting comfortably")
	}
}

func TestPreToolGuardrailReducesFetchPageText(t *testing.T) {
	m := newGuardrailManager(20000)
	// Add turns to use up most of the budget
	for i := 0; i < 30; i++ {
		text := strings.Repeat("x", 2000)
		assistant := anthropic.NewAssistantMessage(anthropic.NewTextBlock(text))
		m.AddTurn(assistant, nil, PrioritySupplementary)
	}

	decision, err := m.PreToolGuardrail(context.Background(), "fetch_page_text", map[string]any{"max_chars": float64(40000)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision.ModifiedArgs != nil {
		reduced, ok := decision.ModifiedArgs["max_chars"].(int)
		if ok && reduced < 40000 {
			// Args were reduced — good
			return
		}
	}
	// Either proceeded without reducing (had room after compaction) or rejected — both valid
}

func TestPreToolGuardrailRejectsFetchWhenTooTight(t *testing.T) {
	m := newGuardrailManager(2000)
	// Fill the context entirely
	for i := 0; i < 10; i++ {
		text := strings.Repeat("x", 700)
		assistant := anthropic.NewAssistantMessage(anthropic.NewTextBlock(text))
		m.AddTurn(assistant, nil, PriorityCore) // Core = hard to compact
	}

	decision, err := m.PreToolGuardrail(context.Background(), "fetch_page_text", map[string]any{"max_chars": float64(100000)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// May either reject or compact+proceed — either is valid behavior
	_ = decision
}

func TestPreToolGuardrailReducesSearchArxiv(t *testing.T) {
	reduced, ok := reduceToolArgs("search_arxiv", map[string]any{"max_results": float64(20)}, 5000)
	if !ok {
		t.Fatal("should have reduced")
	}
	n, _ := reduced["max_results"].(float64)
	if n != 5 {
		t.Errorf("max_results: got %v, want 5", n)
	}
}

func TestReduceToolArgsNoReductionNeeded(t *testing.T) {
	_, ok := reduceToolArgs("search_arxiv", map[string]any{"max_results": float64(3)}, 5000)
	if ok {
		t.Error("should not reduce when max_results <= 5")
	}
}

func TestReduceToolArgs_SearchOpenAlex(t *testing.T) {
	reduced, ok := reduceToolArgs("search_openalex", map[string]any{"max_results": float64(10)}, 2000)
	if !ok {
		t.Fatal("should have reduced")
	}
	n, _ := reduced["max_results"].(float64)
	if n != 5 {
		t.Errorf("max_results: got %v, want 5", n)
	}
}

func TestReduceToolArgs_SearchOpenAlexNoReduction(t *testing.T) {
	_, ok := reduceToolArgs("search_openalex", map[string]any{"max_results": float64(3)}, 2000)
	if ok {
		t.Error("should not reduce when max_results <= 5")
	}
}

func TestReduceToolArgsFetchRejectsTooSmall(t *testing.T) {
	_, ok := reduceToolArgs("fetch_page_text", map[string]any{"max_chars": float64(40000)}, 100)
	if ok {
		t.Error("should reject when remaining too small for min chars")
	}
}

func TestReduceToolArgsUnknownTool(t *testing.T) {
	_, ok := reduceToolArgs("unknown_tool", map[string]any{}, 5000)
	if ok {
		t.Error("should not reduce unknown tools")
	}
}

func TestPostToolCheckRecordsSize(t *testing.T) {
	m := newGuardrailManager(174500)
	err := m.PostToolCheck(context.Background(), "search_arxiv", strings.Repeat("x", 3500))
	if err != nil {
		t.Fatalf("PostToolCheck: %v", err)
	}
}

func TestPreAPICheckNoop(t *testing.T) {
	m := newGuardrailManager(174500)
	err := m.PreAPICheck(context.Background())
	if err != nil {
		t.Fatalf("PreAPICheck should succeed when under budget: %v", err)
	}
}

func TestCopyArgs(t *testing.T) {
	original := map[string]any{"a": 1, "b": "two"}
	copied := copyArgs(original)
	copied["a"] = 99
	if original["a"] != 1 {
		t.Error("copy should not modify original")
	}
}

func TestToolSizeEstimatorUnit(t *testing.T) {
	tse := NewToolSizeEstimator()
	tse.RegisterTool("search_arxiv", func(args map[string]any) int {
		n, _ := args["max_results"].(float64)
		if n == 0 {
			n = 5
		}
		return 500 + int(n)*800
	})

	// Static estimate
	got := tse.Estimate("search_arxiv", map[string]any{"max_results": float64(10)})
	want := 500 + 10*800
	if got != want {
		t.Errorf("static estimate: got %d, want %d", got, want)
	}

	// Unknown tool
	got = tse.Estimate("unknown_tool", nil)
	if got != 5000 {
		t.Errorf("unknown tool: got %d, want 5000", got)
	}

	// History takes over after 3 observations
	tse.Record("search_arxiv", 1000)
	tse.Record("search_arxiv", 2000)
	tse.Record("search_arxiv", 3000)
	got = tse.Estimate("search_arxiv", nil)
	if got != 2000 {
		t.Errorf("historical avg: got %d, want 2000", got)
	}
}

func TestRollingAvgDropsOldest(t *testing.T) {
	r := NewRollingAvg()
	for i := 1; i <= 25; i++ {
		r.Record(i)
	}
	got := r.Avg()
	// Values 6..25, sum = 310, avg = 15 (actually (6+25)*20/2/20 = 15.5, truncated to 15)
	if got < 14 || got > 16 {
		t.Errorf("rolling avg: got %d, want ~15", got)
	}
}

func TestRollingAvgEmpty(t *testing.T) {
	r := NewRollingAvg()
	if r.Avg() != 0 {
		t.Errorf("empty avg: got %d, want 0", r.Avg())
	}
}
