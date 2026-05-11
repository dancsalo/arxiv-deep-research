package research

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSearchWeb_HTTPError500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal Server Error"))
	}))
	defer srv.Close()

	ts := &ResearchToolSet{
		client:           &http.Client{},
		searchWebBaseURL: srv.URL,
	}
	ts.searchWebFailures.Store(0)

	input, _ := json.Marshal(map[string]any{"query": "test"})
	result, err := ts.handleSearchWeb(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}

	var errResp map[string]any
	if err := json.Unmarshal([]byte(result), &errResp); err != nil {
		t.Fatalf("failed to parse error response: %v", err)
	}

	errorMsg := errResp["error"].(string)
	if !strings.Contains(errorMsg, "status 500") {
		t.Errorf("error should mention status code, got: %s", errorMsg)
	}

	// Verify failure counter incremented
	if failures := ts.searchWebFailures.Load(); failures != 1 {
		t.Errorf("expected failure counter to be 1 after HTTP 500, got: %d", failures)
	}
}

func TestSearchWeb_HTTPError429RateLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte("Rate limit exceeded"))
	}))
	defer srv.Close()

	ts := &ResearchToolSet{
		client:           &http.Client{},
		searchWebBaseURL: srv.URL,
	}
	ts.searchWebFailures.Store(0)

	input, _ := json.Marshal(map[string]any{"query": "test"})
	result, err := ts.handleSearchWeb(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}

	var errResp map[string]any
	if err := json.Unmarshal([]byte(result), &errResp); err != nil {
		t.Fatalf("failed to parse error response: %v", err)
	}

	errorMsg := errResp["error"].(string)
	if !strings.Contains(errorMsg, "status 429") {
		t.Errorf("error should mention rate limit status, got: %s", errorMsg)
	}

	// Verify failure counter incremented
	if failures := ts.searchWebFailures.Load(); failures != 1 {
		t.Errorf("expected failure counter to be 1 after HTTP 429, got: %d", failures)
	}
}

func TestSearchWeb_ContextTimeout(t *testing.T) {
	// Server that delays response beyond timeout
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(15 * time.Second) // Longer than 10s timeout
		w.Write([]byte("Too late"))
	}))
	defer srv.Close()

	ts := &ResearchToolSet{
		client:           &http.Client{},
		searchWebBaseURL: srv.URL,
	}
	ts.searchWebFailures.Store(0)

	input, _ := json.Marshal(map[string]any{"query": "test"})
	result, err := ts.handleSearchWeb(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}

	var errResp map[string]any
	if err := json.Unmarshal([]byte(result), &errResp); err != nil {
		t.Fatalf("failed to parse error response: %v", err)
	}

	errorMsg := errResp["error"].(string)
	if !strings.Contains(errorMsg, "request failed") {
		t.Errorf("error should mention request failure, got: %s", errorMsg)
	}

	// Failure counter should NOT increment for context timeout (not HTTP error)
	// Context errors are treated as transient network failures
	if failures := ts.searchWebFailures.Load(); failures != 0 {
		t.Errorf("expected failure counter to be 0 for context timeout, got: %d", failures)
	}
}

func TestSearchWeb_ContextCancellation(t *testing.T) {
	// Server that never responds
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
			return
		case <-time.After(30 * time.Second):
			w.Write([]byte("Finally"))
		}
	}))
	defer srv.Close()

	ts := &ResearchToolSet{
		client:           &http.Client{},
		searchWebBaseURL: srv.URL,
	}
	ts.searchWebFailures.Store(0)

	// Cancel context immediately
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	input, _ := json.Marshal(map[string]any{"query": "test"})
	result, err := ts.handleSearchWeb(ctx, input)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}

	var errResp map[string]any
	if err := json.Unmarshal([]byte(result), &errResp); err != nil {
		t.Fatalf("failed to parse error response: %v", err)
	}

	errorMsg := errResp["error"].(string)
	if !strings.Contains(errorMsg, "request failed") {
		t.Errorf("error should mention request failure, got: %s", errorMsg)
	}

	// Context cancellation should not increment failure counter
	if failures := ts.searchWebFailures.Load(); failures != 0 {
		t.Errorf("expected failure counter to be 0 for cancelled context, got: %d", failures)
	}
}
