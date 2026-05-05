package research

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
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
    <summary>  We introduce BERT, a language representation model.  </summary>
    <published>2023-01-02T00:00:00Z</published>
    <author><name>Jacob Devlin</name></author>
  </entry>
</feed>`

const arxivEmptyFeed = `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
</feed>`

func TestSearchArxiv_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(arxivAtomSample))
	}))
	defer srv.Close()

	ts := newTestToolSet(srv)
	input, _ := json.Marshal(map[string]any{"query": "attention", "max_results": 2})
	result, err := ts.handleSearchArxiv(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var results []ArxivResult
	if err := json.Unmarshal([]byte(result), &results); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	if results[0].Title != "Attention Is All You Need" {
		t.Errorf("title: got %q", results[0].Title)
	}
	if len(results[0].Authors) != 2 {
		t.Errorf("authors: got %d, want 2", len(results[0].Authors))
	}
	if results[1].Abstract != "We introduce BERT, a language representation model." {
		t.Errorf("abstract not trimmed: got %q", results[1].Abstract)
	}
}

func TestSearchArxiv_EmptyResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(arxivEmptyFeed))
	}))
	defer srv.Close()

	ts := newTestToolSet(srv)
	input, _ := json.Marshal(map[string]any{"query": "xyznonexistent", "max_results": 5})
	result, err := ts.handleSearchArxiv(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var results []ArxivResult
	if err := json.Unmarshal([]byte(result), &results); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("got %d results, want 0", len(results))
	}
}

func TestSearchArxiv_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	ts := newTestToolSet(srv)
	input, _ := json.Marshal(map[string]any{"query": "test", "max_results": 5})
	result, err := ts.handleSearchArxiv(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}

	var errResp map[string]any
	if err := json.Unmarshal([]byte(result), &errResp); err != nil {
		t.Fatalf("unmarshal error response: %v", err)
	}
	if errResp["recoverable"] != true {
		t.Error("expected recoverable=true")
	}
}

func TestSearchArxiv_MalformedXML(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not xml at all <><>"))
	}))
	defer srv.Close()

	ts := newTestToolSet(srv)
	input, _ := json.Marshal(map[string]any{"query": "test"})
	result, err := ts.handleSearchArxiv(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}

	var errResp map[string]any
	if err := json.Unmarshal([]byte(result), &errResp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := errResp["error"]; !ok {
		t.Error("expected error field in response")
	}
}

func TestSearchArxiv_ContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
		w.Write([]byte(arxivAtomSample))
	}))
	defer srv.Close()

	ts := newTestToolSet(srv)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	input, _ := json.Marshal(map[string]any{"query": "test"})
	result, err := ts.handleSearchArxiv(ctx, input)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}

	var errResp map[string]any
	if err := json.Unmarshal([]byte(result), &errResp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if errResp["recoverable"] != true {
		t.Error("expected recoverable=true")
	}
}

func newTestToolSet(srv *httptest.Server) *ResearchToolSet {
	client := srv.Client()
	ts := NewResearchToolSet(client)
	// Override the base URL by using a custom transport that redirects to the test server
	ts.client.Transport = &rewriteTransport{base: srv.URL, wrapped: client.Transport}
	return ts
}

type rewriteTransport struct {
	base    string
	wrapped http.RoundTripper
}

func (t *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	parsed, _ := http.NewRequest("GET", t.base, nil)
	req.URL.Scheme = parsed.URL.Scheme
	req.URL.Host = parsed.URL.Host
	return t.wrapped.RoundTrip(req)
}
