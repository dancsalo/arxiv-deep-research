package main

import (
	"github.com/dancsalo/arxiv-deep-research/internal/tracing"
)

// SchemaFeatures tracks which optional schema fields are present
type SchemaFeatures struct {
	HasTimestamps        bool
	HasContextSnapshots  bool
	HasToolRelationships bool
	HasCompactionDiffs   bool
	HasDisplayMetadata   bool
	HasRichErrors        bool
}

// DetectSchemaFeatures checks which optional fields are present in the trace.
// Note: This function samples only the first turn for most features (timestamps,
// context snapshots, tool relationships, display metadata, and rich errors).
// Only compaction diffs are detected by scanning all guardrail decisions across
// the entire trace. This sampling approach is sufficient because these features
// are typically either consistently present or absent throughout a trace.
func DetectSchemaFeatures(trace *tracing.Trace) SchemaFeatures {
	features := SchemaFeatures{}

	if len(trace.Turns) == 0 {
		return features
	}

	firstTurn := trace.Turns[0]

	// Check for timestamps
	if firstTurn.LLMCall != nil && firstTurn.LLMCall.StartedAt != nil {
		features.HasTimestamps = true
	}

	// Check for context snapshots
	if firstTurn.LLMCall != nil && firstTurn.LLMCall.InputSummary != nil {
		features.HasContextSnapshots = true
	}

	// Check for tool relationships
	if len(firstTurn.ToolCalls) > 0 && firstTurn.ToolCalls[0].ParentToolIndex != nil {
		features.HasToolRelationships = true
	}

	// Check for display metadata
	if firstTurn.Display != nil {
		features.HasDisplayMetadata = true
	}

	// Check for rich error context
	if len(firstTurn.ToolCalls) > 0 && firstTurn.ToolCalls[0].Error != nil {
		features.HasRichErrors = true
	}

	// Check for compaction diffs (check trace-level and turn-level guardrail decisions)
	for _, gd := range trace.GuardrailDecisions {
		if gd.Compacted && gd.RemovedContent != nil {
			features.HasCompactionDiffs = true
			break
		}
	}

	// Also check turn-level guardrail decisions
	if !features.HasCompactionDiffs {
		for _, turn := range trace.Turns {
			for _, gd := range turn.GuardrailDecisions {
				if gd.Compacted && gd.RemovedContent != nil {
					features.HasCompactionDiffs = true
					break
				}
			}
			if features.HasCompactionDiffs {
				break
			}
		}
	}

	return features
}
