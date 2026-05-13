package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dancsalo/arxiv-deep-research/internal/tracing"
)

func TestGenerateHTML_BasicStructure(t *testing.T) {
	trace := &tracing.Trace{
		SessionID: "test-001",
		Query:     "test query",
		Turns: []tracing.Turn{
			{Index: 0},
		},
	}

	features := SchemaFeatures{}
	outputPath := filepath.Join(t.TempDir(), "test.html")

	err := GenerateHTML(trace, features, outputPath)
	if err != nil {
		t.Fatalf("GenerateHTML failed: %v", err)
	}

	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read output: %v", err)
	}

	html := string(content)

	// Check basic structure
	if !strings.Contains(html, "<!DOCTYPE html>") {
		t.Error("missing DOCTYPE")
	}

	if !strings.Contains(html, "Trace Timeline") {
		t.Error("missing title")
	}

	if !strings.Contains(html, "test-001") {
		t.Error("missing session ID")
	}

	if !strings.Contains(html, `id="timeline"`) {
		t.Error("missing timeline container")
	}

	if !strings.Contains(html, `id="trace-data"`) {
		t.Error("missing embedded trace data")
	}
}

func TestGenerateHTML_EmbeddedData(t *testing.T) {
	trace := &tracing.Trace{
		SessionID: "test-002",
		Query:     "embedded data test",
		Turns: []tracing.Turn{
			{Index: 0, TokensRemaining: 25000},
		},
	}

	features := SchemaFeatures{}
	outputPath := filepath.Join(t.TempDir(), "test2.html")

	err := GenerateHTML(trace, features, outputPath)
	if err != nil {
		t.Fatalf("GenerateHTML failed: %v", err)
	}

	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read output: %v", err)
	}

	html := string(content)

	// Check embedded JSON
	if !strings.Contains(html, `"session_id":"test-002"`) {
		t.Error("trace data not properly embedded")
	}

	if !strings.Contains(html, `"tokens_remaining":25000`) {
		t.Error("turn data not properly embedded")
	}
}

func TestGenerateHTML_XSSProtection(t *testing.T) {
	// Test that </script> in data is properly escaped
	trace := &tracing.Trace{
		SessionID: "test-xss",
		Query:     "test</script><script>alert('XSS')</script>",
		Turns: []tracing.Turn{
			{Index: 0},
		},
	}

	features := SchemaFeatures{}
	outputPath := filepath.Join(t.TempDir(), "test-xss.html")

	err := GenerateHTML(trace, features, outputPath)
	if err != nil {
		t.Fatalf("GenerateHTML failed: %v", err)
	}

	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read output: %v", err)
	}

	html := string(content)

	// Find the trace data JSON section only
	traceDataStart := strings.Index(html, `<script type="application/json" id="trace-data">`)
	jsonData := ""
	if traceDataStart != -1 {
		jsonStart := traceDataStart + len(`<script type="application/json" id="trace-data">`)
		traceDataEnd := strings.Index(html[jsonStart:], `</script>`)
		if traceDataEnd != -1 {
			jsonData = html[jsonStart : jsonStart+traceDataEnd]
			t.Logf("JSON data: %s", jsonData[:min(200, len(jsonData))])
		}
	}

	// The JSON data should not contain literal </script>
	// which would break out of the script tag
	if strings.Contains(jsonData, `test</script>`) {
		t.Error("XSS vulnerability: </script> not escaped in JSON data")
	}

	// Go's json.Marshal escapes < and > as Unicode escapes (<, >)
	// OR our escaping converts </ to <\/
	// Either way is safe - check for the Unicode escape
	if !strings.Contains(jsonData, "\\u003c") {
		t.Error("Expected < to be Unicode escaped as \\u003c in JSON")
	}
}

func TestGenerateHTML_CompactionEvents(t *testing.T) {
	// Load compaction trace fixture which contains guardrail decisions
	fixturePath := filepath.Join("testdata", "fixtures", "compaction-trace.json")
	traceData, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}

	var trace tracing.Trace
	if err := json.Unmarshal(traceData, &trace); err != nil {
		t.Fatalf("failed to unmarshal fixture: %v", err)
	}

	// Generate HTML
	features := DetectSchemaFeatures(&trace)
	outputPath := filepath.Join(t.TempDir(), "compaction-events.html")

	if err := GenerateHTML(&trace, features, outputPath); err != nil {
		t.Fatalf("GenerateHTML failed: %v", err)
	}

	// Read generated HTML
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read output: %v", err)
	}

	html := string(content)

	// Verify event-related CSS classes exist
	expectedStyles := []string{
		".event-node",
		".event-node.guardrail",
		".event-node-type",
		".event-node-label",
		".event-detail-panel",
	}

	for _, style := range expectedStyles {
		if !strings.Contains(html, style) {
			t.Errorf("missing CSS for: %s", style)
		}
	}

	// Verify JavaScript functions for events exist
	expectedFunctions := []string{
		"collectTimelineItems",
		"renderEventNode",
		"renderEventDetailPanel",
		"toggleEvent",
		"findEvent",
	}

	for _, fn := range expectedFunctions {
		if !strings.Contains(html, fn) {
			t.Errorf("missing JavaScript function: %s", fn)
		}
	}

	// Verify expandedEvents state is initialized
	if !strings.Contains(html, "expandedEvents") {
		t.Error("missing expandedEvents state management")
	}

	// Verify trace data includes guardrail_decisions
	if !strings.Contains(html, `"guardrail_decisions"`) {
		t.Error("guardrail_decisions not embedded in trace data")
	}
}

func TestGenerateHTML_Integration(t *testing.T) {
	fixtures := []struct {
		name            string
		file            string
		minSize         int
		expectedFeatures SchemaFeatures
		checkContent    []string
	}{
		{
			name:    "minimal-trace",
			file:    "minimal-trace.json",
			minSize: 9000,
			expectedFeatures: SchemaFeatures{
				HasTimestamps:        false,
				HasContextSnapshots:  false,
				HasToolRelationships: false,
				HasCompactionDiffs:   false,
				HasDisplayMetadata:   false,
				HasRichErrors:        false,
			},
			checkContent: []string{
				`"session_id":"test-session-123"`,
				`"query":"What is quantum computing?"`,
				`arxiv_search`,
			},
		},
		{
			name:    "normal-trace",
			file:    "normal-trace.json",
			minSize: 10000,
			expectedFeatures: SchemaFeatures{
				HasTimestamps:        true,
				HasContextSnapshots:  false,
				HasToolRelationships: false,
				HasCompactionDiffs:   false,
				HasDisplayMetadata:   true,
				HasRichErrors:        false,
			},
			checkContent: []string{
				`"session_id":"normal-session-456"`,
				`"query":"Find recent papers on transformer architectures`,
				`arxiv_search`,
				`openalex_search`,
				`fetch_paper`,
				`github_search`,
			},
		},
		{
			name:    "compaction-trace",
			file:    "compaction-trace.json",
			minSize: 10000,
			expectedFeatures: SchemaFeatures{
				HasTimestamps:        false,
				HasContextSnapshots:  false,
				HasToolRelationships: false,
				HasCompactionDiffs:   true,
				HasDisplayMetadata:   false,
				HasRichErrors:        false,
			},
			checkContent: []string{
				`"session_id":"compaction-session-789"`,
				`"compacted":true`,
				`"removed_content"`,
				`"compacted_turns"`,
			},
		},
		{
			name:    "error-trace",
			file:    "error-trace.json",
			minSize: 10000,
			expectedFeatures: SchemaFeatures{
				HasTimestamps:        false,
				HasContextSnapshots:  false,
				HasToolRelationships: false,
				HasCompactionDiffs:   false,
				HasDisplayMetadata:   false,
				HasRichErrors:        true,
			},
			checkContent: []string{
				`"session_id":"error-session-321"`,
				`"status":"error"`,
				`"error"`,
				`NetworkError`,
				`APIError`,
				`RateLimitError`,
			},
		},
	}

	for _, tc := range fixtures {
		t.Run(tc.name, func(t *testing.T) {
			// Load trace from fixture
			fixturePath := filepath.Join("testdata", "fixtures", tc.file)
			traceData, err := os.ReadFile(fixturePath)
			if err != nil {
				t.Fatalf("failed to read fixture %s: %v", tc.file, err)
			}

			var trace tracing.Trace
			if err := json.Unmarshal(traceData, &trace); err != nil {
				t.Fatalf("failed to unmarshal fixture %s: %v", tc.file, err)
			}

			// Detect features
			features := DetectSchemaFeatures(&trace)

			// Verify feature detection
			if features.HasTimestamps != tc.expectedFeatures.HasTimestamps {
				t.Errorf("HasTimestamps: got %v, want %v", features.HasTimestamps, tc.expectedFeatures.HasTimestamps)
			}
			if features.HasContextSnapshots != tc.expectedFeatures.HasContextSnapshots {
				t.Errorf("HasContextSnapshots: got %v, want %v", features.HasContextSnapshots, tc.expectedFeatures.HasContextSnapshots)
			}
			if features.HasToolRelationships != tc.expectedFeatures.HasToolRelationships {
				t.Errorf("HasToolRelationships: got %v, want %v", features.HasToolRelationships, tc.expectedFeatures.HasToolRelationships)
			}
			if features.HasCompactionDiffs != tc.expectedFeatures.HasCompactionDiffs {
				t.Errorf("HasCompactionDiffs: got %v, want %v", features.HasCompactionDiffs, tc.expectedFeatures.HasCompactionDiffs)
			}
			if features.HasDisplayMetadata != tc.expectedFeatures.HasDisplayMetadata {
				t.Errorf("HasDisplayMetadata: got %v, want %v", features.HasDisplayMetadata, tc.expectedFeatures.HasDisplayMetadata)
			}
			if features.HasRichErrors != tc.expectedFeatures.HasRichErrors {
				t.Errorf("HasRichErrors: got %v, want %v", features.HasRichErrors, tc.expectedFeatures.HasRichErrors)
			}

			// Generate HTML
			outputPath := filepath.Join(t.TempDir(), tc.name+".html")
			if err := GenerateHTML(&trace, features, outputPath); err != nil {
				t.Fatalf("GenerateHTML failed for %s: %v", tc.file, err)
			}

			// Verify output file exists and has reasonable size
			stat, err := os.Stat(outputPath)
			if err != nil {
				t.Fatalf("failed to stat output file: %v", err)
			}

			if stat.Size() < int64(tc.minSize) {
				t.Errorf("output file too small: got %d bytes, want at least %d", stat.Size(), tc.minSize)
			}

			// Read and verify content
			content, err := os.ReadFile(outputPath)
			if err != nil {
				t.Fatalf("failed to read output file: %v", err)
			}

			html := string(content)

			// Check basic HTML structure
			if !strings.Contains(html, "<!DOCTYPE html>") {
				t.Error("missing DOCTYPE")
			}

			if !strings.Contains(html, "Trace Timeline") {
				t.Error("missing title")
			}

			if !strings.Contains(html, `id="timeline"`) {
				t.Error("missing timeline container")
			}

			if !strings.Contains(html, `id="trace-data"`) {
				t.Error("missing embedded trace data")
			}

			// Check fixture-specific content
			for _, expected := range tc.checkContent {
				if !strings.Contains(html, expected) {
					t.Errorf("missing expected content: %q", expected)
				}
			}
		})
	}
}

func TestEndToEnd_FullWorkflow(t *testing.T) {
	// Test complete workflow: load → detect → generate → verify

	tracePath := filepath.Join("testdata", "fixtures", "normal-trace.json")
	outputPath := filepath.Join(t.TempDir(), "e2e-timeline.html")

	// Load trace
	trace, err := LoadTrace(tracePath)
	if err != nil {
		t.Fatalf("LoadTrace failed: %v", err)
	}

	// Detect features
	features := DetectSchemaFeatures(trace)

	// Generate HTML
	err = GenerateHTML(trace, features, outputPath)
	if err != nil {
		t.Fatalf("GenerateHTML failed: %v", err)
	}

	// Verify output
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read output: %v", err)
	}

	html := string(content)

	// Check structure
	requiredElements := []string{
		"<!DOCTYPE html>",
		"Trace Timeline",
		"normal-session-456",
		"Find recent papers on transformer architectures",
		`id="timeline"`,
		`id="trace-data"`,
		"renderTimeline",
		"toggleTurn",
		"renderEventNode",
	}

	for _, elem := range requiredElements {
		if !strings.Contains(html, elem) {
			t.Errorf("missing required element: %s", elem)
		}
	}

	// Verify embedded trace data is valid JSON
	start := strings.Index(html, `<script type="application/json" id="trace-data">`)
	if start == -1 {
		t.Fatal("could not find embedded trace data start tag")
	}

	jsonStart := start + len(`<script type="application/json" id="trace-data">`)
	end := strings.Index(html[jsonStart:], `</script>`)
	if end == -1 {
		t.Fatal("could not find embedded trace data end tag")
	}

	embeddedJSON := html[jsonStart : jsonStart+end]

	var embeddedTrace tracing.Trace
	if err := json.Unmarshal([]byte(embeddedJSON), &embeddedTrace); err != nil {
		t.Fatalf("embedded trace data is not valid JSON: %v", err)
	}

	// Verify trace data integrity
	if embeddedTrace.SessionID != trace.SessionID {
		t.Error("session ID mismatch in embedded data")
	}

	if len(embeddedTrace.Turns) != len(trace.Turns) {
		t.Error("turn count mismatch in embedded data")
	}
}
