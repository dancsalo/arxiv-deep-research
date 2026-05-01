package memoryclient

import (
	"context"
	"fmt"
	"testing"
)

type mockSearcher struct {
	result *SearchResult
	err    error
	calls  []searchCall
}

type searchCall struct {
	query   string
	mode    string
	limit   int
	memType string
}

func (m *mockSearcher) SearchMemories(_ context.Context, query, mode string, limit int, memType string) (*SearchResult, error) {
	m.calls = append(m.calls, searchCall{query, mode, limit, memType})
	return m.result, m.err
}

func TestAdapterMapsFields(t *testing.T) {
	searcher := &mockSearcher{
		result: &SearchResult{
			Matches: []MemoryIndexRow{
				{ID: 1, Type: "gotcha", Title: "pgvector needs HNSW", Score: 0.95},
				{ID: 2, Type: "decision", Title: "Use hybrid search", Score: 0.82},
			},
		},
	}

	adapter := NewRecallerAdapter(searcher)
	memories, err := adapter.RecallMemories(context.Background(), "test", "hybrid", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(memories) != 2 {
		t.Fatalf("got %d memories, want 2", len(memories))
	}

	if memories[0].ID != 1 || memories[0].Type != "gotcha" || memories[0].Title != "pgvector needs HNSW" || memories[0].Score != 0.95 {
		t.Errorf("memory[0] fields not mapped correctly: %+v", memories[0])
	}
	if memories[1].ID != 2 || memories[1].Type != "decision" || memories[1].Title != "Use hybrid search" || memories[1].Score != 0.82 {
		t.Errorf("memory[1] fields not mapped correctly: %+v", memories[1])
	}
}

func TestAdapterPassesSearchParams(t *testing.T) {
	searcher := &mockSearcher{
		result: &SearchResult{},
	}

	adapter := NewRecallerAdapter(searcher)
	_, err := adapter.RecallMemories(context.Background(), "my query", "semantic", 15)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(searcher.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(searcher.calls))
	}
	call := searcher.calls[0]
	if call.query != "my query" {
		t.Errorf("query = %q, want %q", call.query, "my query")
	}
	if call.mode != "semantic" {
		t.Errorf("mode = %q, want %q", call.mode, "semantic")
	}
	if call.limit != 15 {
		t.Errorf("limit = %d, want 15", call.limit)
	}
	if call.memType != "" {
		t.Errorf("memType = %q, want empty", call.memType)
	}
}

func TestAdapterPropagatesErrors(t *testing.T) {
	searcher := &mockSearcher{
		err: fmt.Errorf("db connection failed"),
	}

	adapter := NewRecallerAdapter(searcher)
	_, err := adapter.RecallMemories(context.Background(), "test", "hybrid", 10)
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "db connection failed" {
		t.Errorf("error = %q, want %q", err.Error(), "db connection failed")
	}
}
