package main

import (
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
