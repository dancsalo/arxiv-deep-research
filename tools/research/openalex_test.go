package research

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dancsalo/arxiv-deep-research/internal/registry"
)

const openAlexJSONSample = `{
  "results": [
    {
      "title": "Retrieval-Augmented Generation for Knowledge-Intensive NLP Tasks",
      "doi": "https://doi.org/10.1234/rag",
      "publication_year": 2020,
      "authorships": [
        {"author": {"display_name": "Patrick Lewis"}},
        {"author": {"display_name": "Ethan Perez"}}
      ],
      "abstract_inverted_index": {"the": [0, 3], "cat": [1], "sat": [2], "mat": [4]}
    },
    {
      "title": "Dense Passage Retrieval",
      "doi": "https://doi.org/10.1234/dpr",
      "publication_year": 2020,
      "authorships": [
        {"author": {"display_name": "Vladimir Karpukhin"}}
      ],
      "abstract_inverted_index": {"hello": [0], "world": [1]}
    },
    {
      "title": "Paper With No Abstract",
      "doi": "https://doi.org/10.1234/none",
      "publication_year": 2021,
      "authorships": [],
      "abstract_inverted_index": null
    }
  ]
}`

func TestSearchOpenAlex_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(openAlexJSONSample))
	}))
	defer srv.Close()

	ts := newTestToolSet(srv)
	input, _ := json.Marshal(map[string]any{"query": "RAG", "max_results": 3})
	result, err := ts.handleSearchOpenAlex(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var results []OpenAlexResult
	if err := json.Unmarshal([]byte(result), &results); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("got %d results, want 3", len(results))
	}
	if results[0].Title != "Retrieval-Augmented Generation for Knowledge-Intensive NLP Tasks" {
		t.Errorf("title: got %q", results[0].Title)
	}
	if len(results[0].Authors) != 2 {
		t.Errorf("authors: got %d, want 2", len(results[0].Authors))
	}
	if results[0].DOI != "https://doi.org/10.1234/rag" {
		t.Errorf("doi: got %q", results[0].DOI)
	}
	if results[0].Year != 2020 {
		t.Errorf("year: got %d, want 2020", results[0].Year)
	}
	if results[0].Abstract != "the cat sat the mat" {
		t.Errorf("abstract: got %q, want %q", results[0].Abstract, "the cat sat the mat")
	}
}

func TestSearchOpenAlex_NullAbstract(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(openAlexJSONSample))
	}))
	defer srv.Close()

	ts := newTestToolSet(srv)
	input, _ := json.Marshal(map[string]any{"query": "test", "max_results": 3})
	result, err := ts.handleSearchOpenAlex(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var results []OpenAlexResult
	json.Unmarshal([]byte(result), &results)
	if results[2].Abstract != "" {
		t.Errorf("null abstract should be empty string, got %q", results[2].Abstract)
	}
}

func TestSearchOpenAlex_WithFilter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.RawQuery, "filter=") {
			t.Error("expected filter in query string")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Write([]byte(`{"results": []}`))
	}))
	defer srv.Close()

	ts := newTestToolSet(srv)
	input, _ := json.Marshal(map[string]any{
		"query":  "test",
		"filter": "publication_year:>2022",
	})
	result, err := ts.handleSearchOpenAlex(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var results []OpenAlexResult
	json.Unmarshal([]byte(result), &results)
	if len(results) != 0 {
		t.Errorf("got %d results, want 0", len(results))
	}
}

func TestSearchOpenAlex_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	ts := newTestToolSet(srv)
	input, _ := json.Marshal(map[string]any{"query": "test"})
	result, err := ts.handleSearchOpenAlex(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}

	var errResp map[string]any
	json.Unmarshal([]byte(result), &errResp)
	if errResp["recoverable"] != true {
		t.Error("expected recoverable=true")
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
	if got != "the cat sat the mat" {
		t.Errorf("got %q, want %q", got, "the cat sat the mat")
	}
}

func TestReconstructAbstract_Nil(t *testing.T) {
	got := reconstructAbstract(nil)
	if got != "" {
		t.Errorf("got %q, want empty string", got)
	}
}

func TestToolError_Format(t *testing.T) {
	result := toolError("api down", true)
	var parsed map[string]any
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if parsed["error"] != "api down" {
		t.Errorf("error: got %v", parsed["error"])
	}
	if parsed["recoverable"] != true {
		t.Errorf("recoverable: got %v", parsed["recoverable"])
	}
}

func TestResearchToolEstimators(t *testing.T) {
	estimators := ResearchToolEstimators()

	got := estimators["search_arxiv"](map[string]any{"max_results": float64(5)})
	want := 50 + 5*400
	if got != want {
		t.Errorf("arxiv(5): got %d, want %d", got, want)
	}

	got = estimators["search_openalex"](map[string]any{"max_results": float64(3)})
	want = 50 + 3*350
	if got != want {
		t.Errorf("openalex(3): got %d, want %d", got, want)
	}

	// Defaults to n=10
	got = estimators["search_arxiv"](map[string]any{})
	want = 50 + 10*400
	if got != want {
		t.Errorf("arxiv(default): got %d, want %d", got, want)
	}
}

func TestToolSetRegister(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(arxivEmptyFeed))
	}))
	defer srv.Close()

	client := srv.Client()
	client.Transport = &rewriteTransport{base: srv.URL, wrapped: client.Transport}

	ts := NewResearchToolSet(client)
	reg := registryForTest()
	ts.Register(reg)

	input, _ := json.Marshal(map[string]any{"query": "test"})
	_, err := reg.Execute(context.Background(), "search_arxiv", input)
	if err != nil {
		t.Errorf("search_arxiv not registered: %v", err)
	}

	_, err = reg.Execute(context.Background(), "search_openalex", input)
	if err != nil {
		t.Errorf("search_openalex not registered: %v", err)
	}
}

func registryForTest() *registry.ToolRegistry {
	return registry.NewToolRegistry()
}
