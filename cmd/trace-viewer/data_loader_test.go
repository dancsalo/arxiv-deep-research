package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadTrace(t *testing.T) {
	path := filepath.Join("testdata", "fixtures", "minimal-trace.json")
	trace, err := LoadTrace(path)

	if err != nil {
		t.Fatalf("LoadTrace() unexpected error: %v", err)
	}

	if trace == nil {
		t.Fatal("LoadTrace() returned nil trace")
	}

	// Validate minimal trace structure
	if trace.SessionID != "test-session-123" {
		t.Errorf("SessionID = %q, want %q", trace.SessionID, "test-session-123")
	}
	if trace.Query != "What is quantum computing?" {
		t.Errorf("Query = %q, want %q", trace.Query, "What is quantum computing?")
	}
	if len(trace.Turns) != 1 {
		t.Errorf("len(Turns) = %d, want 1", len(trace.Turns))
	}
}

func TestLoadTrace_InvalidPath(t *testing.T) {
	path := filepath.Join("testdata", "fixtures", "does-not-exist.json")
	_, err := LoadTrace(path)

	if err == nil {
		t.Fatal("LoadTrace() expected error for nonexistent file, got nil")
	}
	if !strings.Contains(err.Error(), "no such file") {
		t.Errorf("LoadTrace() error = %v, want substring %q", err, "no such file")
	}
}

func TestLoadTrace_InvalidJSON(t *testing.T) {
	// Create a temp file with invalid JSON
	tmpDir := t.TempDir()
	badFile := filepath.Join(tmpDir, "bad.json")
	if err := os.WriteFile(badFile, []byte("{invalid json}"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadTrace(badFile)
	if err == nil {
		t.Fatal("LoadTrace() expected error for invalid JSON, got nil")
	}
	if !strings.Contains(err.Error(), "decode") && !strings.Contains(err.Error(), "unmarshal") {
		t.Errorf("LoadTrace() error = %v, want decode/unmarshal error", err)
	}
}
