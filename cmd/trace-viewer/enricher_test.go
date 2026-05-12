package main

import (
	"testing"
	"time"

	"github.com/dancsalo/arxiv-deep-research/internal/tracing"
)

func TestDetectSchemaFeatures_MinimalSchema(t *testing.T) {
	trace := &tracing.Trace{
		Turns: []tracing.Turn{
			{
				LLMCall: &tracing.LLMCall{},
				ToolCalls: []tracing.ToolCall{
					{Name: "test_tool"},
				},
			},
		},
	}

	features := DetectSchemaFeatures(trace)

	if features.HasTimestamps {
		t.Error("minimal schema should not have timestamps")
	}
	if features.HasContextSnapshots {
		t.Error("minimal schema should not have context snapshots")
	}
	if features.HasToolRelationships {
		t.Error("minimal schema should not have tool relationships")
	}
	if features.HasCompactionDiffs {
		t.Error("minimal schema should not have compaction diffs")
	}
	if features.HasDisplayMetadata {
		t.Error("minimal schema should not have display metadata")
	}
	if features.HasRichErrors {
		t.Error("minimal schema should not have rich errors")
	}
}

func TestDetectSchemaFeatures_FullSchema(t *testing.T) {
	now := time.Now()

	trace := &tracing.Trace{
		Turns: []tracing.Turn{
			{
				LLMCall: &tracing.LLMCall{
					StartedAt: &now,
					InputSummary: &tracing.InputSummary{
						SystemTokens: 100,
					},
				},
				ToolCalls: []tracing.ToolCall{
					{
						Name:            "test_tool",
						StartedAt:       &now,
						ParentToolIndex: intPtr(0),
						Error: &tracing.ToolError{
							Type: "TestError",
						},
					},
				},
				Display: &tracing.TurnDisplay{
					Label: "Test Turn",
				},
			},
		},
		GuardrailDecisions: []tracing.GuardrailDecision{
			{
				Compacted: true,
				RemovedContent: &tracing.RemovedContent{
					ToolResultsCount: 1,
				},
			},
		},
	}

	features := DetectSchemaFeatures(trace)

	if !features.HasTimestamps {
		t.Error("full schema should have timestamps")
	}
	if !features.HasContextSnapshots {
		t.Error("full schema should have context snapshots")
	}
	if !features.HasToolRelationships {
		t.Error("full schema should have tool relationships")
	}
	if !features.HasCompactionDiffs {
		t.Error("full schema should have compaction diffs")
	}
	if !features.HasDisplayMetadata {
		t.Error("full schema should have display metadata")
	}
	if !features.HasRichErrors {
		t.Error("full schema should have rich errors")
	}
}

func intPtr(i int) *int {
	return &i
}
