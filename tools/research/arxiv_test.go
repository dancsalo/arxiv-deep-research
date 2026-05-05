package research

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

const arxivAtomSample = `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <entry>
    <id>http://arxiv.org/abs/2301.00001v1</id>
    <title>Attention Is All You Need</title>
    <summary>We propose a new architecture based solely on attention mechanisms.</summary>
    <published>2023-01-01T00:00:00Z</published>
    <author><name>Ashish Vaswani</name></author>
    <author><name>Noam Shazeer</name></author>
  </entry>
  <entry>
    <id>http://arxiv.org/abs/2301.00002v1</id>
    <title>BERT: Pre-training of Deep Bidirectional Transformers</title>
    <summary>We introduce BERT, a language representation model.</summary>
    <published>2023-01-02T00:00:00Z</published>
    <author><name>Jacob Devlin</name></author>
  </entry>
</feed>`

func TestSearchArxiv_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(arxivAtomSample))
	}))
	defer srv.Close()

	ts := &ResearchToolSet{client: srv.Client()}
	// Override the URL by using a custom transport that redirects
	ts.client = &http.Client{Transport: &rewriteTransport{base: srv.URL, rt: http.DefaultTransport}}

	input, _ := json.Marshal(map[string]any{"query": "attention", "max_results": 2})
	result, err := ts.handleSearchArxiv(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var results []ArxivResult
	if err := json.Unmarshal([]byte(result), &results); err != nil {
		t.Fatalf("failed to parse results: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Title != "Attention Is All You Need" {
		t.Errorf("unexpected title: %s", results[0].Title)
	}
	if len(results[0].Authors) != 2 {
		t.Errorf("expected 2 authors, got %d", len(results[0].Authors))
	}
	if results[0].Authors[0] != "Ashish Vaswani" {
		t.Errorf("unexpected author: %s", results[0].Authors[0])
	}
	if results[0].Abstract != "We propose a new architecture based solely on attention mechanisms." {
		t.Errorf("unexpected abstract: %s", results[0].Abstract)
	}
}

func TestSearchArxiv_EmptyResults(t *testing.T) {
	emptyFeed := `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom"></feed>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(emptyFeed))
	}))
	defer srv.Close()

	ts := &ResearchToolSet{client: &http.Client{Transport: &rewriteTransport{base: srv.URL, rt: http.DefaultTransport}}}
	input, _ := json.Marshal(map[string]any{"query": "nonexistent"})
	result, err := ts.handleSearchArxiv(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var results []ArxivResult
	json.Unmarshal([]byte(result), &results)
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestSearchArxiv_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	ts := &ResearchToolSet{client: &http.Client{Transport: &rewriteTransport{base: srv.URL, rt: http.DefaultTransport}}}
	input, _ := json.Marshal(map[string]any{"query": "test"})
	result, err := ts.handleSearchArxiv(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}

	var errResp map[string]any
	json.Unmarshal([]byte(result), &errResp)
	if errResp["recoverable"] != true {
		t.Error("expected recoverable error")
	}
}

func TestSearchArxiv_ContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(arxivAtomSample))
	}))
	defer srv.Close()

	ts := &ResearchToolSet{client: &http.Client{Transport: &rewriteTransport{base: srv.URL, rt: http.DefaultTransport}}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	input, _ := json.Marshal(map[string]any{"query": "test"})
	result, err := ts.handleSearchArxiv(ctx, input)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}

	var errResp map[string]any
	json.Unmarshal([]byte(result), &errResp)
	if errResp["error"] == nil {
		t.Error("expected error in result")
	}
}

// rewriteTransport redirects all requests to the test server.
type rewriteTransport struct {
	base string
	rt   http.RoundTripper
}

func (t *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.URL.Scheme = "http"
	req.URL.Host = t.base[len("http://"):]
	return t.rt.RoundTrip(req)
}
