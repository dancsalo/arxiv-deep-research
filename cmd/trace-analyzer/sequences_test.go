package main

import (
	"testing"

	"github.com/dancsalo/arxiv-deep-research/internal/tracing"
)

func TestFindToolSequences(t *testing.T) {
	traces := []tracing.Trace{
		{
			SessionID: "trace-1",
			Turns: []tracing.Turn{
				{
					ToolCalls: []tracing.ToolCall{
						{Name: "search_arxiv", DurationMs: 1000},
						{Name: "fetch_arxiv_pdf", DurationMs: 1500},
						{Name: "search_github_repos", DurationMs: 800},
					},
				},
			},
		},
		{
			SessionID: "trace-2",
			Turns: []tracing.Turn{
				{
					ToolCalls: []tracing.ToolCall{
						{Name: "search_arxiv", DurationMs: 1200},
						{Name: "fetch_arxiv_pdf", DurationMs: 1400},
					},
				},
			},
		},
		{
			SessionID: "trace-3",
			Turns: []tracing.Turn{
				{
					ToolCalls: []tracing.ToolCall{
						{Name: "search_arxiv", DurationMs: 1100},
						{Name: "fetch_arxiv_pdf", DurationMs: 1600},
					},
				},
			},
		},
	}

	sequences := FindToolSequences(traces, 2)

	// Should find ["search_arxiv", "fetch_arxiv_pdf"] in all 3 traces
	found := false
	for _, seq := range sequences {
		if len(seq.Pattern) == 2 &&
		   seq.Pattern[0] == "search_arxiv" &&
		   seq.Pattern[1] == "fetch_arxiv_pdf" {
			found = true

			if seq.Count != 3 {
				t.Errorf("expected count=3 for [search_arxiv, fetch_arxiv_pdf], got %d", seq.Count)
			}

			expectedSupport := 1.0
			if seq.Support != expectedSupport {
				t.Errorf("expected support=%.1f, got %.1f", expectedSupport, seq.Support)
			}
		}
	}

	if !found {
		t.Error("expected to find [search_arxiv, fetch_arxiv_pdf] sequence")
	}

	// Should find ["fetch_arxiv_pdf", "search_github_repos"] in 1 trace
	found = false
	for _, seq := range sequences {
		if len(seq.Pattern) == 2 &&
		   seq.Pattern[0] == "fetch_arxiv_pdf" &&
		   seq.Pattern[1] == "search_github_repos" {
			found = true

			if seq.Count != 1 {
				t.Errorf("expected count=1 for [fetch_arxiv_pdf, search_github_repos], got %d", seq.Count)
			}
		}
	}

	// With minSupport=2, should NOT find single-occurrence sequence
	if found {
		t.Error("should not find sequence with count < minSupport")
	}
}

func TestFindToolSequences_LongerPatterns(t *testing.T) {
	traces := []tracing.Trace{
		{
			SessionID: "trace-1",
			Turns: []tracing.Turn{
				{
					ToolCalls: []tracing.ToolCall{
						{Name: "A"},
						{Name: "B"},
						{Name: "C"},
						{Name: "D"},
					},
				},
			},
		},
		{
			SessionID: "trace-2",
			Turns: []tracing.Turn{
				{
					ToolCalls: []tracing.ToolCall{
						{Name: "A"},
						{Name: "B"},
						{Name: "C"},
					},
				},
			},
		},
	}

	sequences := FindToolSequences(traces, 2)

	// Should find [A,B,C] in both traces
	found := false
	for _, seq := range sequences {
		if len(seq.Pattern) == 3 &&
		   seq.Pattern[0] == "A" &&
		   seq.Pattern[1] == "B" &&
		   seq.Pattern[2] == "C" {
			found = true
			if seq.Count != 2 {
				t.Errorf("expected count=2 for [A,B,C], got %d", seq.Count)
			}
		}
	}

	if !found {
		t.Error("expected to find [A,B,C] sequence")
	}
}

func TestBuildToolTransitions(t *testing.T) {
	traces := []tracing.Trace{
		{
			Turns: []tracing.Turn{
				{
					ToolCalls: []tracing.ToolCall{
						{Name: "A"},
						{Name: "B"},
						{Name: "C"},
					},
				},
			},
		},
		{
			Turns: []tracing.Turn{
				{
					ToolCalls: []tracing.ToolCall{
						{Name: "A"},
						{Name: "B"},
					},
				},
			},
		},
	}

	transitions := BuildToolTransitions(traces)

	if transitions["A"]["B"] != 2 {
		t.Errorf("expected A->B transition count=2, got %d", transitions["A"]["B"])
	}

	if transitions["B"]["C"] != 1 {
		t.Errorf("expected B->C transition count=1, got %d", transitions["B"]["C"])
	}

	if transitions["A"]["C"] != 0 {
		t.Errorf("expected A->C transition count=0, got %d", transitions["A"]["C"])
	}
}
