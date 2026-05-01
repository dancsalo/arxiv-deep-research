package tools

import "testing"

func TestSearchMemoriesEstimator(t *testing.T) {
	estimators := MemoryToolEstimators(nil)
	est := estimators["search_memories"]

	tests := []struct {
		name string
		args map[string]any
		want int
	}{
		{"default limit", map[string]any{}, 50 + 20*20},
		{"limit 10", map[string]any{"limit": float64(10)}, 50 + 10*20},
		{"limit 5", map[string]any{"limit": float64(5)}, 50 + 5*20},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := est(tt.args)
			if got != tt.want {
				t.Errorf("got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestGetMemoryDetailsEstimator(t *testing.T) {
	estimators := MemoryToolEstimators(nil)
	est := estimators["get_memory_details"]

	tests := []struct {
		name string
		args map[string]any
		want int
	}{
		{"3 ids", map[string]any{"ids": []any{1.0, 2.0, 3.0}}, 50 + 3*200},
		{"empty ids", map[string]any{"ids": []any{}}, 200},
		{"no ids key", map[string]any{}, 200},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := est(tt.args)
			if got != tt.want {
				t.Errorf("got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestGetMemorySourceEstimator_Fallback(t *testing.T) {
	estimators := MemoryToolEstimators(nil)
	est := estimators["get_memory_source"]

	got := est(map[string]any{"id": float64(0)})
	if got != 2000 {
		t.Errorf("expected fallback 2000, got %d", got)
	}

	got = est(map[string]any{})
	if got != 2000 {
		t.Errorf("expected fallback 2000 for missing id, got %d", got)
	}
}

func TestStoreMemoryEstimator(t *testing.T) {
	estimators := MemoryToolEstimators(nil)
	est := estimators["store_memory"]
	got := est(map[string]any{})
	if got != 50 {
		t.Errorf("expected 50, got %d", got)
	}
}

func TestReduceToolArgs_SearchMemories(t *testing.T) {
	args := map[string]any{"query": "test", "limit": float64(20)}
	reduced, ok := ReduceToolArgs("search_memories", args)
	if !ok {
		t.Error("expected reduction")
	}
	if reduced["limit"] != 10 {
		t.Errorf("expected limit=10, got %v", reduced["limit"])
	}
	if reduced["query"] != "test" {
		t.Error("query should be preserved")
	}
}

func TestReduceToolArgs_SearchMemories_NoLimit(t *testing.T) {
	args := map[string]any{"query": "test"}
	reduced, ok := ReduceToolArgs("search_memories", args)
	if !ok {
		t.Error("expected reduction")
	}
	if reduced["limit"] != 10 {
		t.Errorf("expected limit=10, got %v", reduced["limit"])
	}
}

func TestReduceToolArgs_GetMemoryDetails_Truncate(t *testing.T) {
	args := map[string]any{"ids": []any{1.0, 2.0, 3.0, 4.0, 5.0}}
	reduced, ok := ReduceToolArgs("get_memory_details", args)
	if !ok {
		t.Error("expected reduction")
	}
	ids := reduced["ids"].([]any)
	if len(ids) != 3 {
		t.Errorf("expected 3 ids, got %d", len(ids))
	}
}

func TestReduceToolArgs_GetMemoryDetails_NoReduction(t *testing.T) {
	args := map[string]any{"ids": []any{1.0, 2.0}}
	_, ok := ReduceToolArgs("get_memory_details", args)
	if ok {
		t.Error("expected no reduction for <= 3 ids")
	}
}

func TestReduceToolArgs_UnknownTool(t *testing.T) {
	args := map[string]any{"foo": "bar"}
	_, ok := ReduceToolArgs("unknown_tool", args)
	if ok {
		t.Error("expected no reduction for unknown tool")
	}
}
