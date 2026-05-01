package ctxmgr

import "sort"

type ContextBudget struct {
	ModelContextLimit int
	MaxOutputTokens   int
	SystemTokens      int
	ToolDefTokens     int
	SafetyMargin      int
}

func (b *ContextBudget) Available() int {
	return b.ModelContextLimit - b.MaxOutputTokens - b.SystemTokens - b.ToolDefTokens - b.SafetyMargin
}

func (b *ContextBudget) Remaining(currentTokens int) int {
	return b.Available() - currentTokens
}

type OutputTracker struct {
	observations []int
	maxObserved  int
}

func NewOutputTracker() *OutputTracker {
	return &OutputTracker{}
}

func (t *OutputTracker) Record(outputTokens int) {
	t.observations = append(t.observations, outputTokens)
	if outputTokens > t.maxObserved {
		t.maxObserved = outputTokens
	}
	if len(t.observations) > 20 {
		t.observations = t.observations[1:]
	}
}

func (t *OutputTracker) RecommendedReservation() int {
	if len(t.observations) == 0 {
		return 16000
	}
	sorted := make([]int, len(t.observations))
	copy(sorted, t.observations)
	sort.Ints(sorted)
	p95idx := int(float64(len(sorted)) * 0.95)
	if p95idx >= len(sorted) {
		p95idx = len(sorted) - 1
	}
	p95 := sorted[p95idx]
	if p95 < 4096 {
		return 4096
	}
	return p95
}

func (t *OutputTracker) MaxObserved() int {
	return t.maxObserved
}

func (t *OutputTracker) Len() int {
	return len(t.observations)
}
