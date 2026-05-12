package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/template"

	"github.com/dancsalo/arxiv-deep-research/internal/tracing"
)

// GenerateHTML creates a self-contained HTML file for the trace timeline
func GenerateHTML(trace *tracing.Trace, features SchemaFeatures, outputPath string) error {
	// Serialize trace to JSON
	traceJSON, err := json.Marshal(trace)
	if err != nil {
		return fmt.Errorf("marshaling trace: %w", err)
	}

	// Escape JSON for safe embedding in HTML script tag
	// Replace "</" with "<\/" to prevent breaking out of script tags
	safeJSON := strings.ReplaceAll(string(traceJSON), "</", `<\/`)

	// Parse template
	tmpl, err := template.New("timeline").Parse(htmlTemplate)
	if err != nil {
		return fmt.Errorf("parsing template: %w", err)
	}

	// Prepare template data
	data := struct {
		SessionID  string
		Query      string
		DurationMs int64
		Status     string
		TraceJSON  string
	}{
		SessionID:  trace.SessionID,
		Query:      trace.Query,
		DurationMs: trace.DurationMs,
		Status:     trace.Status,
		TraceJSON:  safeJSON,
	}

	// Render template
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("executing template: %w", err)
	}

	// Write to file
	if err := os.WriteFile(outputPath, buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("writing output file: %w", err)
	}

	return nil
}
