package ctxmgr

import "testing"

func TestToolSizeEstimatorStaticEstimate(t *testing.T) {
	tse := NewToolSizeEstimator()
	tse.RegisterTool("search_arxiv", func(args map[string]any) int {
		n, _ := args["max_results"].(float64)
		if n == 0 {
			n = 5
		}
		return 500 + int(n)*800
	})
	got := tse.Estimate("search_arxiv", map[string]any{"max_results": float64(10)})
	want := 500 + 10*800
	if got != want {
		t.Errorf("static estimate: got %d, want %d", got, want)
	}
}

func TestToolSizeEstimatorUnknownTool(t *testing.T) {
	tse := NewToolSizeEstimator()
	got := tse.Estimate("unknown", nil)
	if got != 5000 {
		t.Errorf("unknown tool: got %d, want 5000", got)
	}
}

func TestToolSizeEstimatorHistoryOverStatic(t *testing.T) {
	tse := NewToolSizeEstimator()
	tse.RegisterTool("my_tool", func(args map[string]any) int {
		return 100
	})
	tse.Record("my_tool", 500)
	tse.Record("my_tool", 500)
	tse.Record("my_tool", 500)

	got := tse.Estimate("my_tool", nil)
	if got != 500 {
		t.Errorf("history should override static: got %d, want 500", got)
	}
}

func TestToolSizeEstimatorHistoryBelowThreshold(t *testing.T) {
	tse := NewToolSizeEstimator()
	tse.RegisterTool("my_tool", func(args map[string]any) int {
		return 100
	})
	tse.Record("my_tool", 500)
	tse.Record("my_tool", 500)
	// Only 2 observations — should use static

	got := tse.Estimate("my_tool", nil)
	if got != 100 {
		t.Errorf("below threshold should use static: got %d, want 100", got)
	}
}

func TestToolSizeEstimatorRegister(t *testing.T) {
	tse := NewToolSizeEstimator()
	tse.RegisterTool("custom", func(args map[string]any) int { return 42 })
	got := tse.Estimate("custom", nil)
	if got != 42 {
		t.Errorf("registered tool: got %d, want 42", got)
	}
}

func TestRollingAvgBasic(t *testing.T) {
	r := NewRollingAvg()
	r.Record(10)
	r.Record(20)
	r.Record(30)
	if r.Avg() != 20 {
		t.Errorf("avg: got %d, want 20", r.Avg())
	}
	if r.Len() != 3 {
		t.Errorf("len: got %d, want 3", r.Len())
	}
}

func TestRollingAvgWindowEviction(t *testing.T) {
	r := NewRollingAvg()
	for i := 1; i <= 25; i++ {
		r.Record(i)
	}
	if r.Len() != 20 {
		t.Errorf("len after 25 records: got %d, want 20", r.Len())
	}
	// Values 6..25, sum = (6+25)*20/2 = 310, avg = 15
	got := r.Avg()
	if got < 14 || got > 16 {
		t.Errorf("avg after eviction: got %d, want ~15", got)
	}
}

func TestRollingAvgEmptyAvg(t *testing.T) {
	r := NewRollingAvg()
	if r.Avg() != 0 {
		t.Errorf("empty avg: got %d, want 0", r.Avg())
	}
}
