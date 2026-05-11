package research

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGetReferences_Success(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")

		// First call: fetch work with referenced_works
		if strings.Contains(r.URL.Path, "/W2741809807") {
			response := `{
				"id": "https://openalex.org/W2741809807",
				"referenced_works": [
					"https://openalex.org/W123",
					"https://openalex.org/W456",
					"https://openalex.org/W789"
				]
			}`
			w.Write([]byte(response))
			return
		}

		// Second call: batch fetch metadata
		if strings.Contains(r.URL.RawQuery, "filter=openalex") {
			response := `{
				"results": [
					{
						"id": "https://openalex.org/W123",
						"title": "First Reference",
						"doi": "https://doi.org/10.1234/ref1",
						"publication_year": 2019,
						"cited_by_count": 100,
						"authorships": [
							{"author": {"display_name": "Alice Smith"}},
							{"author": {"display_name": "Bob Jones"}}
						],
						"abstract_inverted_index": null
					},
					{
						"id": "https://openalex.org/W456",
						"title": "Second Reference",
						"doi": "https://doi.org/10.1234/ref2",
						"publication_year": 2018,
						"cited_by_count": 50,
						"authorships": [
							{"author": {"display_name": "Charlie Brown"}}
						],
						"abstract_inverted_index": null
					},
					{
						"id": "https://openalex.org/W789",
						"title": "Third Reference",
						"doi": "https://doi.org/10.1234/ref3",
						"publication_year": 2020,
						"cited_by_count": null,
						"authorships": [],
						"abstract_inverted_index": null
					}
				]
			}`
			w.Write([]byte(response))
			return
		}
	}))
	defer srv.Close()

	ts := newTestResearchToolSet(&http.Client{Transport: &rewriteTransport{base: srv.URL, rt: http.DefaultTransport}})
	input, _ := json.Marshal(map[string]any{"work_id": "W2741809807", "direction": "references", "max_results": 5})
	result, err := ts.handleGetCitationsAndReferences(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify 2 API calls were made
	if callCount != 2 {
		t.Errorf("expected 2 API calls, got %d", callCount)
	}

	var results []CitationResult
	if err := json.Unmarshal([]byte(result), &results); err != nil {
		t.Fatalf("failed to parse results: %v", err)
	}

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// Verify first result
	if results[0].ID != "W123" {
		t.Errorf("expected ID W123, got %s", results[0].ID)
	}
	if results[0].Title != "First Reference" {
		t.Errorf("unexpected title: %s", results[0].Title)
	}
	if len(results[0].Authors) != 2 {
		t.Errorf("expected 2 authors, got %d", len(results[0].Authors))
	}
	if results[0].Year != 2019 {
		t.Errorf("expected year 2019, got %d", results[0].Year)
	}
	if results[0].CitedByCount == nil || *results[0].CitedByCount != 100 {
		t.Errorf("expected citation count 100, got %v", results[0].CitedByCount)
	}

	// Verify third result has nil citation count
	if results[2].CitedByCount != nil {
		t.Errorf("expected nil citation count for third result, got %v", results[2].CitedByCount)
	}
}

func TestGetCitingPapers_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.RawQuery, "filter=cites:") {
			t.Error("expected filter=cites: in URL")
		}

		response := `{
			"results": [
				{
					"id": "https://openalex.org/W999",
					"title": "Paper Citing Us",
					"doi": "https://doi.org/10.1234/citing1",
					"publication_year": 2021,
					"cited_by_count": 5,
					"authorships": [
						{"author": {"display_name": "David Lee"}}
					],
					"abstract_inverted_index": null
				}
			]
		}`
		w.Write([]byte(response))
	}))
	defer srv.Close()

	ts := newTestResearchToolSet(&http.Client{Transport: &rewriteTransport{base: srv.URL, rt: http.DefaultTransport}})
	input, _ := json.Marshal(map[string]any{"work_id": "W2741809807", "direction": "cited_by", "max_results": 10})
	result, err := ts.handleGetCitationsAndReferences(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var results []CitationResult
	if err := json.Unmarshal([]byte(result), &results); err != nil {
		t.Fatalf("failed to parse results: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if results[0].ID != "W999" {
		t.Errorf("expected ID W999, got %s", results[0].ID)
	}
	if results[0].Title != "Paper Citing Us" {
		t.Errorf("unexpected title: %s", results[0].Title)
	}
}

func TestGetCitations_InvalidWorkIDFormat(t *testing.T) {
	ts := newTestResearchToolSet(&http.Client{})
	input, _ := json.Marshal(map[string]any{"work_id": "invalid-id", "direction": "references"})
	result, err := ts.handleGetCitationsAndReferences(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var errorResp map[string]any
	if err := json.Unmarshal([]byte(result), &errorResp); err != nil {
		t.Fatalf("failed to parse error response: %v", err)
	}

	if errorResp["error"] == nil {
		t.Error("expected error field in response")
	}
	if recoverable, ok := errorResp["recoverable"].(bool); !ok || recoverable {
		t.Error("expected recoverable=false for invalid work_id")
	}
	if !strings.Contains(errorResp["error"].(string), "invalid work_id format") {
		t.Errorf("unexpected error message: %s", errorResp["error"])
	}
}

func TestGetCitations_InvalidDirection(t *testing.T) {
	ts := newTestResearchToolSet(&http.Client{})
	input, _ := json.Marshal(map[string]any{"work_id": "W2741809807", "direction": "invalid"})
	result, err := ts.handleGetCitationsAndReferences(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var errorResp map[string]any
	if err := json.Unmarshal([]byte(result), &errorResp); err != nil {
		t.Fatalf("failed to parse error response: %v", err)
	}

	if errorResp["error"] == nil {
		t.Error("expected error field in response")
	}
	if recoverable, ok := errorResp["recoverable"].(bool); !ok || recoverable {
		t.Error("expected recoverable=false for invalid direction")
	}
	if !strings.Contains(errorResp["error"].(string), "invalid direction") {
		t.Errorf("unexpected error message: %s", errorResp["error"])
	}
}

func TestGetCitations_WorkNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error": "not found"}`))
	}))
	defer srv.Close()

	ts := newTestResearchToolSet(&http.Client{Transport: &rewriteTransport{base: srv.URL, rt: http.DefaultTransport}})
	input, _ := json.Marshal(map[string]any{"work_id": "W999999999", "direction": "references"})
	result, err := ts.handleGetCitationsAndReferences(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var errorResp map[string]any
	if err := json.Unmarshal([]byte(result), &errorResp); err != nil {
		t.Fatalf("failed to parse error response: %v", err)
	}

	if errorResp["error"] == nil {
		t.Error("expected error field in response")
	}
	if recoverable, ok := errorResp["recoverable"].(bool); !ok || recoverable {
		t.Error("expected recoverable=false for work not found")
	}
	if !strings.Contains(errorResp["error"].(string), "work not found") {
		t.Errorf("unexpected error message: %s", errorResp["error"])
	}
}

func TestGetReferences_NoReferences(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := `{
			"id": "https://openalex.org/W123",
			"referenced_works": []
		}`
		w.Write([]byte(response))
	}))
	defer srv.Close()

	ts := newTestResearchToolSet(&http.Client{Transport: &rewriteTransport{base: srv.URL, rt: http.DefaultTransport}})
	input, _ := json.Marshal(map[string]any{"work_id": "W123", "direction": "references"})
	result, err := ts.handleGetCitationsAndReferences(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != "[]" {
		t.Errorf("expected empty array [], got %s", result)
	}
}

func TestGetReferences_MaxResultsCapping(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// First call: work with 100 references
		if strings.Contains(r.URL.Path, "/W123") {
			refs := make([]string, 100)
			for i := 0; i < 100; i++ {
				refs[i] = "https://openalex.org/W" + string(rune(1000+i))
			}
			response := map[string]any{
				"id":               "https://openalex.org/W123",
				"referenced_works": refs,
			}
			json.NewEncoder(w).Encode(response)
			return
		}

		// Second call: verify only 10 IDs in filter
		filterParam := r.URL.Query().Get("filter")
		if filterParam == "" {
			t.Error("expected filter parameter")
			return
		}
		// Count pipe separators to determine number of IDs
		idCount := strings.Count(filterParam, "|") + 1
		if idCount > 10 {
			t.Errorf("expected max 10 IDs in filter, got %d", idCount)
		}

		w.Write([]byte(`{"results": []}`))
	}))
	defer srv.Close()

	ts := newTestResearchToolSet(&http.Client{Transport: &rewriteTransport{base: srv.URL, rt: http.DefaultTransport}})
	input, _ := json.Marshal(map[string]any{"work_id": "W123", "direction": "references", "max_results": 10})
	_, err := ts.handleGetCitationsAndReferences(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetCitations_AuthorLimiting(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := `{
			"results": [
				{
					"id": "https://openalex.org/W999",
					"title": "Many Authors Paper",
					"doi": "https://doi.org/10.1234/many",
					"publication_year": 2021,
					"cited_by_count": 10,
					"authorships": [
						{"author": {"display_name": "Author 1"}},
						{"author": {"display_name": "Author 2"}},
						{"author": {"display_name": "Author 3"}},
						{"author": {"display_name": "Author 4"}},
						{"author": {"display_name": "Author 5"}},
						{"author": {"display_name": "Author 6"}},
						{"author": {"display_name": "Author 7"}},
						{"author": {"display_name": "Author 8"}},
						{"author": {"display_name": "Author 9"}},
						{"author": {"display_name": "Author 10"}}
					],
					"abstract_inverted_index": null
				}
			]
		}`
		w.Write([]byte(response))
	}))
	defer srv.Close()

	ts := newTestResearchToolSet(&http.Client{Transport: &rewriteTransport{base: srv.URL, rt: http.DefaultTransport}})
	input, _ := json.Marshal(map[string]any{"work_id": "W123", "direction": "cited_by", "max_results": 10})
	result, err := ts.handleGetCitationsAndReferences(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var results []CitationResult
	if err := json.Unmarshal([]byte(result), &results); err != nil {
		t.Fatalf("failed to parse results: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if len(results[0].Authors) != 3 {
		t.Errorf("expected 3 authors (limited), got %d", len(results[0].Authors))
	}

	if results[0].Authors[0] != "Author 1" || results[0].Authors[2] != "Author 3" {
		t.Errorf("unexpected author names: %v", results[0].Authors)
	}
}

func TestGetCitations_NullCitationCount(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := `{
			"results": [
				{
					"id": "https://openalex.org/W999",
					"title": "Paper With Null Citations",
					"doi": "https://doi.org/10.1234/null",
					"publication_year": 2021,
					"cited_by_count": null,
					"authorships": [],
					"abstract_inverted_index": null
				}
			]
		}`
		w.Write([]byte(response))
	}))
	defer srv.Close()

	ts := newTestResearchToolSet(&http.Client{Transport: &rewriteTransport{base: srv.URL, rt: http.DefaultTransport}})
	input, _ := json.Marshal(map[string]any{"work_id": "W123", "direction": "cited_by"})
	result, err := ts.handleGetCitationsAndReferences(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var results []CitationResult
	if err := json.Unmarshal([]byte(result), &results); err != nil {
		t.Fatalf("failed to parse results: %v", err)
	}

	if results[0].CitedByCount != nil {
		t.Errorf("expected nil citation count, got %v", results[0].CitedByCount)
	}

	// Verify field is omitted from JSON output
	jsonBytes := []byte(result)
	if strings.Contains(string(jsonBytes), "cited_by_count") {
		t.Error("JSON output should omit cited_by_count field when nil (omitempty tag)")
	}
}

func TestGetCitations_ZeroCitationCount(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := `{
			"results": [
				{
					"id": "https://openalex.org/W999",
					"title": "Paper With Zero Citations",
					"doi": "https://doi.org/10.1234/zero",
					"publication_year": 2023,
					"cited_by_count": 0,
					"authorships": [],
					"abstract_inverted_index": null
				}
			]
		}`
		w.Write([]byte(response))
	}))
	defer srv.Close()

	ts := newTestResearchToolSet(&http.Client{Transport: &rewriteTransport{base: srv.URL, rt: http.DefaultTransport}})
	input, _ := json.Marshal(map[string]any{"work_id": "W123", "direction": "cited_by"})
	result, err := ts.handleGetCitationsAndReferences(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var results []CitationResult
	if err := json.Unmarshal([]byte(result), &results); err != nil {
		t.Fatalf("failed to parse results: %v", err)
	}

	if results[0].CitedByCount == nil || *results[0].CitedByCount != 0 {
		t.Errorf("expected citation count 0, got %v", results[0].CitedByCount)
	}

	// Verify field is included in JSON output
	jsonBytes := []byte(result)
	if !strings.Contains(string(jsonBytes), `"cited_by_count":0`) {
		t.Error("JSON output should include cited_by_count:0 for zero citations")
	}
}
