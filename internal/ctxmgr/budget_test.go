package ctxmgr

import "testing"

func TestBudgetAvailable(t *testing.T) {
	b := ContextBudget{
		ModelContextLimit: 200000,
		MaxOutputTokens:   16000,
		SystemTokens:      1000,
		ToolDefTokens:     500,
		SafetyMargin:      8000,
	}
	got := b.Available()
	want := 174500
	if got != want {
		t.Errorf("Available: got %d, want %d", got, want)
	}
}

func TestBudgetRemaining(t *testing.T) {
	b := ContextBudget{
		ModelContextLimit: 200000,
		MaxOutputTokens:   16000,
		SystemTokens:      1000,
		ToolDefTokens:     500,
		SafetyMargin:      8000,
	}
	got := b.Remaining(50000)
	want := 124500
	if got != want {
		t.Errorf("Remaining: got %d, want %d", got, want)
	}
}

func TestBudgetRemainingNegative(t *testing.T) {
	b := ContextBudget{
		ModelContextLimit: 200000,
		MaxOutputTokens:   16000,
		SystemTokens:      1000,
		ToolDefTokens:     500,
		SafetyMargin:      8000,
	}
	got := b.Remaining(200000)
	want := -25500
	if got != want {
		t.Errorf("Remaining negative: got %d, want %d", got, want)
	}
}

func TestBudgetAvailableZeroOverhead(t *testing.T) {
	b := ContextBudget{
		ModelContextLimit: 200000,
		MaxOutputTokens:   16000,
		SystemTokens:      0,
		ToolDefTokens:     0,
		SafetyMargin:      8000,
	}
	got := b.Available()
	want := 176000
	if got != want {
		t.Errorf("Available zero overhead: got %d, want %d", got, want)
	}
}

func TestOutputTrackerDefaultReservation(t *testing.T) {
	ot := NewOutputTracker()
	got := ot.RecommendedReservation()
	if got != 16000 {
		t.Errorf("default reservation: got %d, want 16000", got)
	}
}

func TestOutputTrackerRecord(t *testing.T) {
	ot := NewOutputTracker()
	ot.Record(500)
	ot.Record(1000)
	if ot.Len() != 2 {
		t.Errorf("Len: got %d, want 2", ot.Len())
	}
}

func TestOutputTrackerReservationFloor(t *testing.T) {
	ot := NewOutputTracker()
	for i := 0; i < 10; i++ {
		ot.Record(100)
	}
	got := ot.RecommendedReservation()
	if got != 4096 {
		t.Errorf("reservation floor: got %d, want 4096", got)
	}
}

func TestOutputTrackerReservationP95(t *testing.T) {
	ot := NewOutputTracker()
	for i := 0; i < 19; i++ {
		ot.Record(5000)
	}
	ot.Record(15000)
	got := ot.RecommendedReservation()
	if got != 15000 {
		t.Errorf("reservation p95: got %d, want 15000", got)
	}
}

func TestOutputTrackerDropsOldest(t *testing.T) {
	ot := NewOutputTracker()
	for i := 1; i <= 25; i++ {
		ot.Record(i)
	}
	if ot.Len() != 20 {
		t.Errorf("Len after 25 records: got %d, want 20", ot.Len())
	}
}

func TestOutputTrackerMaxObserved(t *testing.T) {
	ot := NewOutputTracker()
	ot.Record(100)
	ot.Record(500)
	ot.Record(200)
	if ot.MaxObserved() != 500 {
		t.Errorf("MaxObserved: got %d, want 500", ot.MaxObserved())
	}
}
