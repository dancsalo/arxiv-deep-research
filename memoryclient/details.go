package memoryclient

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/lib/pq"
)

type MemoryDetail struct {
	ID           int64  `json:"id"`
	SessionID    string `json:"session_id"`
	TurnIndex    *int   `json:"turn_index,omitempty"`
	Type         string `json:"type"`
	Title        string `json:"title"`
	Content      string `json:"content"`
	Tokens       int    `json:"tokens"`
	HasSource    bool   `json:"has_source"`
	SourceTokens *int   `json:"source_tokens,omitempty"`
	CreatedAt    string `json:"created_at"`
}

type DetailsResult struct {
	Memories     []MemoryDetail `json:"memories"`
	Instructions string         `json:"instructions"`
}

func (c *Client) GetMemoryDetails(ctx context.Context, ids []int64) (*DetailsResult, error) {
	query := `
		SELECT id, session_id, turn_index, memory_type, title, content,
		       token_count, source IS NOT NULL AS has_source, source_tokens, created_at
		FROM memories
		WHERE id = ANY($1)
		ORDER BY created_at DESC
	`
	dbRows, err := c.db.QueryContext(ctx, query, pq.Array(ids))
	if err != nil {
		return nil, fmt.Errorf("details query: %w", err)
	}
	defer dbRows.Close()

	var details []MemoryDetail
	for dbRows.Next() {
		var d MemoryDetail
		var createdAt time.Time
		var memoryType string
		var turnIndex sql.NullInt32
		var sourceTokens sql.NullInt32
		if err := dbRows.Scan(&d.ID, &d.SessionID, &turnIndex, &memoryType, &d.Title, &d.Content,
			&d.Tokens, &d.HasSource, &sourceTokens, &createdAt); err != nil {
			return nil, fmt.Errorf("scan detail row: %w", err)
		}
		d.Type = memoryTypeToEmoji(memoryType)
		d.CreatedAt = createdAt.Format(time.RFC3339)
		if turnIndex.Valid {
			v := int(turnIndex.Int32)
			d.TurnIndex = &v
		}
		if sourceTokens.Valid {
			v := int(sourceTokens.Int32)
			d.SourceTokens = &v
		}
		details = append(details, d)
	}

	return &DetailsResult{
		Memories:     details,
		Instructions: "Use get_memory_source with an ID to fetch the original raw content (if has_source is true).",
	}, dbRows.Err()
}

type SourceResult struct {
	ID           int64  `json:"id"`
	Title        string `json:"title"`
	Source       string `json:"source"`
	SourceTokens int    `json:"source_tokens"`
}

func (c *Client) GetMemorySource(ctx context.Context, id int64) (*SourceResult, error) {
	query := `
		SELECT id, title, source, source_tokens
		FROM memories
		WHERE id = $1 AND source IS NOT NULL
	`
	var r SourceResult
	err := c.db.QueryRowContext(ctx, query, id).Scan(&r.ID, &r.Title, &r.Source, &r.SourceTokens)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("memory %d not found or has no source", id)
	}
	if err != nil {
		return nil, fmt.Errorf("source query: %w", err)
	}
	return &r, nil
}

func (c *Client) PeekSourceSize(ctx context.Context, id int64) (int, error) {
	var tokens sql.NullInt32
	err := c.db.QueryRowContext(ctx,
		"SELECT source_tokens FROM memories WHERE id = $1", id,
	).Scan(&tokens)
	if err == sql.ErrNoRows {
		return 0, fmt.Errorf("memory %d not found", id)
	}
	if err != nil {
		return 0, err
	}
	if !tokens.Valid {
		return 0, fmt.Errorf("memory %d has no source", id)
	}
	return int(tokens.Int32), nil
}
