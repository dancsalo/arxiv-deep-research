package memoryclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEmbed_ServiceDown(t *testing.T) {
	c := &Client{
		embedURL:   "http://localhost:1",
		httpClient: &http.Client{},
	}
	_, err := c.Embed(context.Background(), []string{"hello"})
	if err == nil {
		t.Fatal("expected error when service is down")
	}
}

func TestEmbed_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()

	c := &Client{
		embedURL:   srv.URL,
		httpClient: &http.Client{},
	}
	_, err := c.Embed(context.Background(), []string{"hello"})
	if err == nil {
		t.Fatal("expected error on 500")
	}
	if got := err.Error(); got != "embed returned 500" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestEmbed_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		vec := make([]float32, 384)
		for i := range vec {
			vec[i] = float32(i) * 0.001
		}
		resp := embedResponse{
			Embeddings: [][]float32{vec},
			Dimensions: 384,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := &Client{
		embedURL:   srv.URL,
		httpClient: &http.Client{},
	}
	result, err := c.Embed(context.Background(), []string{"hello"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 embedding, got %d", len(result))
	}
	if len(result[0]) != 384 {
		t.Fatalf("expected 384 dims, got %d", len(result[0]))
	}
}

func TestEmbed_ContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	c := &Client{
		embedURL:   srv.URL,
		httpClient: &http.Client{},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := c.Embed(ctx, []string{"hello"})
	if err == nil {
		t.Fatal("expected error on cancelled context")
	}
}
