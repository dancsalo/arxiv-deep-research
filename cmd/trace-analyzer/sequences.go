package main

import (
	"sort"
	"strings"

	"github.com/dancsalo/arxiv-deep-research/internal/tracing"
)

// FindToolSequences detects common tool call patterns across traces
// using a sliding window approach. Returns sequences sorted by count (descending).
// minSupport filters sequences that appear in fewer than minSupport traces.
func FindToolSequences(traces []tracing.Trace, minSupport int) []ToolSequence {
	// Map of pattern -> {count, traces, latencies}
	type seqData struct {
		count     int
		traces    map[string]bool
		latencies []float64
	}

	sequences := make(map[string]*seqData)

	for _, trace := range traces {
		// Track which patterns we've seen in this trace (for support calculation)
		seenPatterns := make(map[string]bool)

		for _, turn := range trace.Turns {
			toolNames := make([]string, len(turn.ToolCalls))
			toolLatencies := make([]int64, len(turn.ToolCalls))

			for i, tc := range turn.ToolCalls {
				toolNames[i] = tc.Name
				toolLatencies[i] = tc.DurationMs
			}

			// Sliding window for patterns of length 2-5
			for windowSize := 2; windowSize <= 5 && windowSize <= len(toolNames); windowSize++ {
				for start := 0; start <= len(toolNames)-windowSize; start++ {
					pattern := toolNames[start : start+windowSize]
					key := strings.Join(pattern, "→")

					if _, ok := sequences[key]; !ok {
						sequences[key] = &seqData{
							traces:    make(map[string]bool),
							latencies: []float64{},
						}
					}

					seq := sequences[key]
					seq.count++
					seq.traces[trace.SessionID] = true

					// Sum latencies for this pattern instance
					totalLatency := int64(0)
					for i := start; i < start+windowSize; i++ {
						totalLatency += toolLatencies[i]
					}
					seq.latencies = append(seq.latencies, float64(totalLatency))

					seenPatterns[key] = true
				}
			}
		}
	}

	// Convert to result slice, filtering by minSupport
	result := []ToolSequence{}
	totalTraces := float64(len(traces))

	for key, data := range sequences {
		if len(data.traces) < minSupport {
			continue
		}

		pattern := strings.Split(key, "→")
		result = append(result, ToolSequence{
			Pattern:    pattern,
			Count:      data.count,
			Support:    float64(len(data.traces)) / totalTraces,
			AvgLatency: mean(data.latencies),
		})
	}

	// Sort by count descending
	sort.Slice(result, func(i, j int) bool {
		return result[i].Count > result[j].Count
	})

	return result
}

// BuildToolTransitions creates a transition matrix for tool->tool calls
func BuildToolTransitions(traces []tracing.Trace) map[string]map[string]int {
	transitions := make(map[string]map[string]int)

	for _, trace := range traces {
		for _, turn := range trace.Turns {
			for i := 0; i < len(turn.ToolCalls)-1; i++ {
				from := turn.ToolCalls[i].Name
				to := turn.ToolCalls[i+1].Name

				if _, ok := transitions[from]; !ok {
					transitions[from] = make(map[string]int)
				}

				transitions[from][to]++
			}
		}
	}

	return transitions
}

// mean calculates the average of a slice of float64 values
func mean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}
