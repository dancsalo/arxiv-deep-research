package research

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSearchOpenAlex_CitationSort(t *testing.T) {
	// Mock response with papers sorted by citation count
	citation1000 := 1000
	citation500 := 500
	citation100 := 100
	mockResponse := `{
		"results": [
			{
				"title": "Most Cited Paper",
				"doi": "https://doi.org/10.1234/paper1",
				"publication_year": 2020,
				"cited_by_count": 1000,
				"authorships": [{"author": {"display_name": "Author A"}}],
				"abstract_inverted_index": {"First": [0], "paper": [1]}
			},
			{
				"title": "Medium Cited Paper",
				"doi": "https://doi.org/10.1234/paper2",
				"publication_year": 2021,
				"cited_by_count": 500,
				"authorships": [{"author": {"display_name": "Author B"}}],
				"abstract_inverted_index": {"Second": [0], "paper": [1]}
			},
			{
				"title": "Least Cited Paper",
				"doi": "https://doi.org/10.1234/paper3",
				"publication_year": 2022,
				"cited_by_count": 100,
				"authorships": [{"author": {"display_name": "Author C"}}],
				"abstract_inverted_index": {"Third": [0], "paper": [1]}
			}
		]
	}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify sort parameter is present
		sortParam := r.URL.Query().Get("sort")
		if sortParam != "cited_by_count:desc" {
			t.Errorf("expected sort=cited_by_count:desc in URL, got: %s", sortParam)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(mockResponse))
	}))
	defer srv.Close()

	ts := &ResearchToolSet{client: &http.Client{Transport: &rewriteTransport{base: srv.URL, rt: http.DefaultTransport}}}
	input, _ := json.Marshal(map[string]any{"query": "transformers", "sort": "cited_by_count", "max_results": 3})
	result, err := ts.handleSearchOpenAlex(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var results []OpenAlexResult
	if err := json.Unmarshal([]byte(result), &results); err != nil {
		t.Fatalf("failed to parse results: %v", err)
	}

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// Verify citation counts are populated and in descending order
	if results[0].CitedByCount == nil || *results[0].CitedByCount != citation1000 {
		t.Errorf("expected first result to have 1000 citations, got: %v", results[0].CitedByCount)
	}
	if results[1].CitedByCount == nil || *results[1].CitedByCount != citation500 {
		t.Errorf("expected second result to have 500 citations, got: %v", results[1].CitedByCount)
	}
	if results[2].CitedByCount == nil || *results[2].CitedByCount != citation100 {
		t.Errorf("expected third result to have 100 citations, got: %v", results[2].CitedByCount)
	}

	// Verify titles match expected order
	if results[0].Title != "Most Cited Paper" {
		t.Errorf("unexpected first title: %s", results[0].Title)
	}
}

func TestSearchOpenAlex_NoSortParameter(t *testing.T) {
	mockResponse := `{"results": [{"title": "Test", "doi": "test", "publication_year": 2020, "cited_by_count": 42, "authorships": [], "abstract_inverted_index": null}]}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify sort parameter is NOT present (backward compatibility)
		sortParam := r.URL.Query().Get("sort")
		if sortParam != "" {
			t.Errorf("expected no sort parameter when omitted, got: %s", sortParam)
		}
		w.Write([]byte(mockResponse))
	}))
	defer srv.Close()

	ts := &ResearchToolSet{client: &http.Client{Transport: &rewriteTransport{base: srv.URL, rt: http.DefaultTransport}}}
	input, _ := json.Marshal(map[string]any{"query": "test", "max_results": 5})
	result, err := ts.handleSearchOpenAlex(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var results []OpenAlexResult
	if err := json.Unmarshal([]byte(result), &results); err != nil {
		t.Fatalf("failed to parse results: %v", err)
	}

	// Results should still work, just without explicit sorting
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
}

func TestSearchOpenAlex_EmptyStringSortParameter(t *testing.T) {
	mockResponse := `{"results": [{"title": "Test", "doi": "test", "publication_year": 2020, "cited_by_count": 42, "authorships": [], "abstract_inverted_index": null}]}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify sort parameter is NOT present when explicitly set to empty string
		sortParam := r.URL.Query().Get("sort")
		if sortParam != "" {
			t.Errorf("expected no sort parameter when set to empty string, got: %s", sortParam)
		}
		w.Write([]byte(mockResponse))
	}))
	defer srv.Close()

	ts := &ResearchToolSet{client: &http.Client{Transport: &rewriteTransport{base: srv.URL, rt: http.DefaultTransport}}}
	// Explicitly pass empty string for sort
	input, _ := json.Marshal(map[string]any{"query": "test", "sort": ""})
	result, err := ts.handleSearchOpenAlex(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var results []OpenAlexResult
	if err := json.Unmarshal([]byte(result), &results); err != nil {
		t.Fatalf("failed to parse results: %v", err)
	}

	// Empty string treated as no sort (backward compat)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
}

func TestSearchOpenAlex_InvalidSortValue(t *testing.T) {
	ts := &ResearchToolSet{client: &http.Client{}}
	input, _ := json.Marshal(map[string]any{"query": "test", "sort": "invalid_value"})
	result, err := ts.handleSearchOpenAlex(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}

	var errResp map[string]any
	if err := json.Unmarshal([]byte(result), &errResp); err != nil {
		t.Fatalf("failed to parse error response: %v", err)
	}

	// Should return tool error
	if errResp["error"] == nil {
		t.Error("expected error field in response")
	}
	errorMsg := errResp["error"].(string)
	if !strings.Contains(errorMsg, "invalid sort value") {
		t.Errorf("error message should mention 'invalid sort value', got: %s", errorMsg)
	}
	if !strings.Contains(errorMsg, "cited_by_count") {
		t.Errorf("error message should list valid values, got: %s", errorMsg)
	}

	// Should be recoverable
	if errResp["recoverable"] != true {
		t.Error("invalid sort error should be recoverable")
	}
}

func TestSearchOpenAlex_NullCitationCount(t *testing.T) {
	mockResponse := `{
		"results": [
			{
				"title": "Very New Paper",
				"doi": "https://doi.org/10.1234/new",
				"publication_year": 2023,
				"cited_by_count": null,
				"authorships": [],
				"abstract_inverted_index": null
			}
		]
	}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(mockResponse))
	}))
	defer srv.Close()

	ts := &ResearchToolSet{client: &http.Client{Transport: &rewriteTransport{base: srv.URL, rt: http.DefaultTransport}}}
	input, _ := json.Marshal(map[string]any{"query": "test"})
	result, err := ts.handleSearchOpenAlex(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var results []OpenAlexResult
	if err := json.Unmarshal([]byte(result), &results); err != nil {
		t.Fatalf("failed to parse results: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	// CitedByCount should be nil (not 0)
	if results[0].CitedByCount != nil {
		t.Errorf("expected nil citation count for null value, got: %v", *results[0].CitedByCount)
	}

	// Verify JSON output omits field with omitempty tag
	jsonBytes, _ := json.Marshal(results[0])
	if strings.Contains(string(jsonBytes), "cited_by_count") {
		t.Error("JSON output should omit cited_by_count field when nil (omitempty tag)")
	}
}

func TestSearchOpenAlex_ZeroCitationCount(t *testing.T) {
	mockResponse := `{
		"results": [
			{
				"title": "Unpopular Paper",
				"doi": "https://doi.org/10.1234/unpopular",
				"publication_year": 2023,
				"cited_by_count": 0,
				"authorships": [],
				"abstract_inverted_index": null
			}
		]
	}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(mockResponse))
	}))
	defer srv.Close()

	ts := &ResearchToolSet{client: &http.Client{Transport: &rewriteTransport{base: srv.URL, rt: http.DefaultTransport}}}
	input, _ := json.Marshal(map[string]any{"query": "test"})
	result, err := ts.handleSearchOpenAlex(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var results []OpenAlexResult
	if err := json.Unmarshal([]byte(result), &results); err != nil {
		t.Fatalf("failed to parse results: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	// CitedByCount should be pointer to 0 (not nil)
	if results[0].CitedByCount == nil {
		t.Error("expected non-nil citation count for zero value")
	}
	if *results[0].CitedByCount != 0 {
		t.Errorf("expected citation count of 0, got: %d", *results[0].CitedByCount)
	}

	// Verify JSON output includes field with value 0
	jsonBytes, _ := json.Marshal(results[0])
	if !strings.Contains(string(jsonBytes), `"cited_by_count":0`) {
		t.Error("JSON output should include cited_by_count:0 for zero value")
	}
}
