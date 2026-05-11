package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
)

type Trace struct {
	SessionID         string  `json:"session_id"`
	Query             string  `json:"query"`
	Model             string  `json:"model"`
	PromptVariant     string  `json:"prompt_variant"`
	PromptHash        string  `json:"prompt_hash"`
	DurationMs        int64   `json:"duration_ms"`
	TotalCostUSD      float64 `json:"total_cost_usd"`
	TotalInputTokens  int     `json:"total_input_tokens"`
	TotalOutputTokens int     `json:"total_output_tokens"`
	Turns             []Turn  `json:"turns"`
}

type Turn struct {
	Index      int        `json:"index"`
	DurationMs int64      `json:"duration_ms"`
	ToolCalls  []ToolCall `json:"tool_calls"`
}

type ToolCall struct {
	Name string `json:"name"`
}

type TraceMetrics struct {
	Variant         string
	Turns           int
	TotalCost       float64
	DurationSec     int
	TotalToolCalls  int
	UniqueTools     int
	ToolBreakdown   map[string]int
	ParallelTurns   int
	SequentialChains int
	OutputLength    int // Words in summary (if we parse finish_loop)
}

func main() {
	diffSummaries := flag.Bool("diff-summaries", false, "show side-by-side summary comparison")
	jsonOutput := flag.Bool("json", false, "output as JSON")
	flag.Parse()

	if len(flag.Args()) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: compare-traces [--diff-summaries] [--json] <trace1.json> <trace2.json> [trace3.json...]\n")
		os.Exit(1)
	}

	traces, err := loadTraces(flag.Args())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading traces: %v\n", err)
		os.Exit(1)
	}

	metrics := computeMetrics(traces)

	if *jsonOutput {
		outputJSON(metrics)
	} else {
		outputTable(metrics, traces)
		if *diffSummaries {
			outputSummaryDiff(traces)
		}
	}
}

func loadTraces(paths []string) ([]*Trace, error) {
	traces := make([]*Trace, 0, len(paths))
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read %s: %w", path, err)
		}

		var trace Trace
		if err := json.Unmarshal(data, &trace); err != nil {
			return nil, fmt.Errorf("failed to parse %s: %w", path, err)
		}

		traces = append(traces, &trace)
	}
	return traces, nil
}

func computeMetrics(traces []*Trace) []TraceMetrics {
	metrics := make([]TraceMetrics, 0, len(traces))

	for _, trace := range traces {
		m := TraceMetrics{
			Variant:       trace.PromptVariant,
			Turns:         len(trace.Turns),
			TotalCost:     trace.TotalCostUSD,
			DurationSec:   int(trace.DurationMs / 1000),
			ToolBreakdown: make(map[string]int),
		}

		// Count tools
		uniqueTools := make(map[string]bool)
		for _, turn := range trace.Turns {
			if len(turn.ToolCalls) >= 2 {
				m.ParallelTurns++
			}
			for _, tc := range turn.ToolCalls {
				m.TotalToolCalls++
				uniqueTools[tc.Name] = true
				m.ToolBreakdown[tc.Name]++
			}
		}
		m.UniqueTools = len(uniqueTools)

		// Detect sequential chains (simplified heuristic)
		// A chain is 3+ consecutive single-tool turns
		chainLength := 0
		for _, turn := range trace.Turns {
			if len(turn.ToolCalls) == 1 {
				chainLength++
				if chainLength >= 3 {
					m.SequentialChains++
					chainLength = 0 // Reset after detecting chain
				}
			} else {
				chainLength = 0
			}
		}

		metrics = append(metrics, m)
	}

	return metrics
}

func outputTable(metrics []TraceMetrics, traces []*Trace) {
	fmt.Printf("Comparing %d traces:\n\n", len(traces))

	// Display query (assume all same query)
	if len(traces) > 0 {
		fmt.Printf("Query: %q\n\n", traces[0].Query)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', tabwriter.AlignRight|tabwriter.Debug)

	// Header
	fmt.Fprintln(w, "Variant\tTurns\tCost\tDuration\tTools\tUnique\t")

	// Rows
	for _, m := range metrics {
		fmt.Fprintf(w, "%s\t%d\t$%.2f\t%ds\t%d calls\t%d types\t\n",
			m.Variant, m.Turns, m.TotalCost, m.DurationSec, m.TotalToolCalls, m.UniqueTools)
	}
	w.Flush()

	fmt.Println()

	// Tool usage breakdown
	fmt.Println("Tool Usage Breakdown:")
	for _, m := range metrics {
		toolList := formatToolBreakdown(m.ToolBreakdown)
		fmt.Printf("  %s: %s\n", m.Variant, toolList)
	}

	fmt.Println()

	// Parallel usage
	fmt.Println("Parallel Tool Usage:")
	for _, m := range metrics {
		pct := 0
		if m.Turns > 0 {
			pct = (m.ParallelTurns * 100) / m.Turns
		}
		fmt.Printf("  %s: %d turns with 2+ tools (%d%%)\n", m.Variant, m.ParallelTurns, pct)
	}

	fmt.Println()

	// Sequential chains
	fmt.Println("Sequential Chains Detected:")
	for _, m := range metrics {
		fmt.Printf("  %s: %d chains (3+ consecutive single-tool turns)\n", m.Variant, m.SequentialChains)
	}
}

func formatToolBreakdown(breakdown map[string]int) string {
	// Sort by tool name
	tools := make([]string, 0, len(breakdown))
	for tool := range breakdown {
		tools = append(tools, tool)
	}
	sort.Strings(tools)

	parts := make([]string, 0, len(tools))
	for _, tool := range tools {
		parts = append(parts, fmt.Sprintf("%s(%d)", tool, breakdown[tool]))
	}
	return strings.Join(parts, " ")
}

func outputJSON(metrics []TraceMetrics) {
	data, err := json.MarshalIndent(metrics, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling JSON: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(data))
}

func outputSummaryDiff(traces []*Trace) {
	fmt.Println("\n=== Summary Comparison ===")
	fmt.Println("(Not implemented yet - summaries would be extracted from finish_loop tool calls and diffed)")
}
