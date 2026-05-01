package memoryclient

import (
	"context"

	contextmanager "memory-store"
)

type MemorySearcher interface {
	SearchMemories(ctx context.Context, query string, mode string, limit int, memType string) (*SearchResult, error)
}

type RecallerAdapter struct {
	searcher MemorySearcher
}

func NewRecallerAdapter(client MemorySearcher) *RecallerAdapter {
	return &RecallerAdapter{searcher: client}
}

func (a *RecallerAdapter) RecallMemories(ctx context.Context, query string, mode string, limit int) ([]contextmanager.RecalledMemory, error) {
	result, err := a.searcher.SearchMemories(ctx, query, mode, limit, "")
	if err != nil {
		return nil, err
	}

	memories := make([]contextmanager.RecalledMemory, len(result.Matches))
	for i, row := range result.Matches {
		memories[i] = contextmanager.RecalledMemory{
			ID:    row.ID,
			Type:  row.Type,
			Title: row.Title,
			Score: row.Score,
		}
	}
	return memories, nil
}
