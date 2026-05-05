package research

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

const openAlexJSONSample = `{
  "results": [
    {
      "title": "Retrieval-Augmented Generation for Knowledge-Intensive NLP Tasks",
      "doi": "https://doi.org/10.1234/rag2020",
      "publication_year": 2020,
      "authorships": [
        {"author": {"display_name": "Patrick Lewis"}},
        {"author": {"display_name": "Ethan Perez"}}
      ],
      "abstract_inverted_index": {"We": [0], "explore": [1], "RAG": [2], "methods": [3]}
    },
    {
      "title": "Dense Passage Retrieval",
      "doi": "https://doi.org/10.1234/dpr2020",
      "publication_year": 2020,
      "authorships": [
        {"author": {"display_name": "Vladimir Karpukhin"}}
      ],
      "abstract_inverted_index": null
    },
    {
      "title": "REALM: Retrieval-Augmented Language Model Pre-Training",
      "doi": "https://doi.org/10.1234/realm2020",
      "publication_year": 2020,
      "authorships": [],
      "abstract_inverted_index": {"Pre-training": [0], "with": [1], "retrieval": [2]}
    }
  ]
}`

func TestSearchOpenAlex_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(openAlexJSONSample))
	}))
	defer srv.Close()

	ts := &ResearchToolSet{client: &http.Client{Transport: &rewriteTransport{base: srv.URL, rt: http.DefaultTransport}}}
	input, _ := json.Marshal(map[string]any{"query": "RAG", "max_results": 3})
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
	if results[0].Title != "Retrieval-Augmented Generation for Knowledge-Intensive NLP Tasks" {
		t.Errorf("unexpected title: %s", results[0].Title)
	}
	if len(results[0].Authors) != 2 {
		t.Errorf("expected 2 authors, got %d", len(results[0].Authors))
	}
	if results[0].Abstract != "We explore RAG methods" {
		t.Errorf("unexpected abstract: %q", results[0].Abstract)
	}
	if results[0].DOI != "https://doi.org/10.1234/rag2020" {
		t.Errorf("unexpected DOI: %s", results[0].DOI)
	}
}

func TestSearchOpenAlex_NullAbstract(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(openAlexJSONSample))
	}))
	defer srv.Close()

	ts := &ResearchToolSet{client: &http.Client{Transport: &rewriteTransport{base: srv.URL, rt: http.DefaultTransport}}}
	input, _ := json.Marshal(map[string]any{"query": "test"})
	result, _ := ts.handleSearchOpenAlex(context.Background(), input)

	var results []OpenAlexResult
	json.Unmarshal([]byte(result), &results)
	if results[1].Abstract != "" {
		t.Errorf("expected empty abstract for null inverted index, got: %q", results[1].Abstract)
	}
}

func TestSearchOpenAlex_WithFilter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("filter") == "" {
			t.Error("expected filter parameter in URL")
		}
		w.Write([]byte(`{"results": []}`))
	}))
	defer srv.Close()

	ts := &ResearchToolSet{client: &http.Client{Transport: &rewriteTransport{base: srv.URL, rt: http.DefaultTransport}}}
	input, _ := json.Marshal(map[string]any{"query": "test", "filter": "publication_year:>2022"})
	_, err := ts.handleSearchOpenAlex(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSearchOpenAlex_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	ts := &ResearchToolSet{client: &http.Client{Transport: &rewriteTransport{base: srv.URL, rt: http.DefaultTransport}}}
	input, _ := json.Marshal(map[string]any{"query": "test"})
	result, err := ts.handleSearchOpenAlex(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}

	var errResp map[string]any
	json.Unmarshal([]byte(result), &errResp)
	if errResp["recoverable"] != true {
		t.Error("expected recoverable error")
	}
}

func TestReconstructAbstract(t *testing.T) {
	index := map[string][]int{
		"the": {0, 3},
		"cat": {1},
		"sat": {2},
		"mat": {4},
	}
	got := reconstructAbstract(index)
	want := "the cat sat the mat"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestReconstructAbstract_Nil(t *testing.T) {
	got := reconstructAbstract(nil)
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestToolError_Format(t *testing.T) {
	result := toolError("api down", true)
	var parsed map[string]any
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("toolError produced invalid JSON: %v", err)
	}
	if parsed["error"] != "api down" {
		t.Errorf("unexpected error field: %v", parsed["error"])
	}
	if parsed["recoverable"] != true {
		t.Errorf("unexpected recoverable field: %v", parsed["recoverable"])
	}
}

func TestResearchToolEstimators(t *testing.T) {
	estimators := ResearchToolEstimators()

	got := estimators["search_arxiv"](map[string]any{"max_results": float64(5)})
	if got != 2050 {
		t.Errorf("search_arxiv(5): got %d, want 2050", got)
	}

	got = estimators["search_openalex"](map[string]any{"max_results": float64(3)})
	if got != 1100 {
		t.Errorf("search_openalex(3): got %d, want 1100", got)
	}

	got = estimators["search_arxiv"](map[string]any{})
	if got != 4050 {
		t.Errorf("search_arxiv(default): got %d, want 4050", got)
	}
}
