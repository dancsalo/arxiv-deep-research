package memoryclient

import (
	"context"
	"fmt"
	"time"
)

type MemoryIndexRow struct {
	ID     int64   `json:"id"`
	Type   string  `json:"type"`
	Title  string  `json:"title"`
	Date   string  `json:"date"`
	Tokens int     `json:"tokens"`
	Score  float64 `json:"score"`
}

type SearchResult struct {
	Matches      []MemoryIndexRow `json:"matches"`
	SearchMode   string           `json:"search_mode"`
	Degraded     bool             `json:"degraded,omitempty"`
	Instructions string           `json:"instructions"`
}

func (c *Client) SearchMemories(ctx context.Context, query string, mode string, limit int, memType string) (*SearchResult, error) {
	if mode == "" {
		mode = "hybrid"
	}
	if limit == 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	var rows []MemoryIndexRow
	var err error
	var degraded bool

	switch mode {
	case "text":
		rows, err = c.searchFTS(ctx, query, limit, memType)
	case "semantic":
		rows, err = c.searchSemantic(ctx, query, limit, memType)
	case "hybrid":
		rows, degraded, err = c.searchHybrid(ctx, query, limit, memType)
	default:
		return nil, fmt.Errorf("unknown search mode: %s", mode)
	}
	if err != nil {
		return nil, err
	}

	return &SearchResult{
		Matches:    rows,
		SearchMode: mode,
		Degraded:   degraded,
		Instructions: "Use get_memory_details with IDs from this index to fetch full content. " +
			"Types: 🎯=session-goal 🔴=gotcha 🟡=problem-fix 🔵=how-it-works 🟢=what-changed 🟣=discovery 🟠=why-it-exists 🟤=decision ⚖️=trade-off",
	}, nil
}

func (c *Client) searchFTS(ctx context.Context, query string, limit int, memType string) ([]MemoryIndexRow, error) {
	var args []any
	args = append(args, query) // $1

	typeClause := ""
	if memType != "" {
		typeClause = " AND memory_type = $2"
		args = append(args, memType)
	}

	limitPlaceholder := fmt.Sprintf("$%d", len(args)+1)
	args = append(args, limit)

	q := fmt.Sprintf(`
		SELECT id, memory_type, title, created_at, token_count,
		       ts_rank_cd(search_vector, websearch_to_tsquery('english', $1)) AS rank
		FROM memories
		WHERE search_vector @@ websearch_to_tsquery('english', $1)
		%s
		ORDER BY rank DESC
		LIMIT %s
	`, typeClause, limitPlaceholder)

	dbRows, err := c.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("fts query: %w", err)
	}
	defer dbRows.Close()

	var results []MemoryIndexRow
	for dbRows.Next() {
		var r MemoryIndexRow
		var createdAt time.Time
		var memoryType string
		if err := dbRows.Scan(&r.ID, &memoryType, &r.Title, &createdAt, &r.Tokens, &r.Score); err != nil {
			return nil, fmt.Errorf("scan fts row: %w", err)
		}
		r.Type = memoryTypeToEmoji(memoryType)
		r.Date = createdAt.Format("2006-01-02")
		results = append(results, r)
	}
	return results, dbRows.Err()
}

func (c *Client) searchSemantic(ctx context.Context, query string, limit int, memType string) ([]MemoryIndexRow, error) {
	embeddings, err := c.Embed(ctx, []string{query})
	if err != nil {
		return nil, fmt.Errorf("embedding failed (required for semantic mode): %w", err)
	}

	vecStr := pgVectorLiteral(embeddings[0])

	var args []any
	args = append(args, vecStr) // $1

	typeClause := ""
	if memType != "" {
		typeClause = " AND memory_type = $2"
		args = append(args, memType)
	}

	limitPlaceholder := fmt.Sprintf("$%d", len(args)+1)
	args = append(args, limit)

	q := fmt.Sprintf(`
		SELECT id, memory_type, title, created_at, token_count,
		       1 - (embedding <=> $1::vector) AS score
		FROM memories
		WHERE embedding IS NOT NULL
		%s
		ORDER BY embedding <=> $1::vector
		LIMIT %s
	`, typeClause, limitPlaceholder)

	dbRows, err := c.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("semantic query: %w", err)
	}
	defer dbRows.Close()

	var results []MemoryIndexRow
	for dbRows.Next() {
		var r MemoryIndexRow
		var createdAt time.Time
		var memoryType string
		if err := dbRows.Scan(&r.ID, &memoryType, &r.Title, &createdAt, &r.Tokens, &r.Score); err != nil {
			return nil, fmt.Errorf("scan semantic row: %w", err)
		}
		r.Type = memoryTypeToEmoji(memoryType)
		r.Date = createdAt.Format("2006-01-02")
		results = append(results, r)
	}
	return results, dbRows.Err()
}

func (c *Client) searchHybrid(ctx context.Context, query string, limit int, memType string) ([]MemoryIndexRow, bool, error) {
	embeddings, err := c.Embed(ctx, []string{query})
	if err != nil {
		rows, ftsErr := c.searchFTS(ctx, query, limit, memType)
		return rows, true, ftsErr
	}

	vecStr := pgVectorLiteral(embeddings[0])

	var args []any
	args = append(args, query)  // $1
	args = append(args, vecStr) // $2

	typeClause := ""
	if memType != "" {
		typeClause = " AND memory_type = $3"
		args = append(args, memType)
	}

	limitPlaceholder := fmt.Sprintf("$%d", len(args)+1)
	args = append(args, limit)

	q := fmt.Sprintf(`
		WITH fts AS (
			SELECT id, ROW_NUMBER() OVER (
				ORDER BY ts_rank_cd(search_vector, websearch_to_tsquery('english', $1)) DESC
			) AS rank
			FROM memories
			WHERE search_vector @@ websearch_to_tsquery('english', $1)
			%s
			LIMIT 100
		),
		sem AS (
			SELECT id, ROW_NUMBER() OVER (
				ORDER BY embedding <=> $2::vector
			) AS rank
			FROM memories
			WHERE embedding IS NOT NULL
			%s
			LIMIT 100
		),
		combined AS (
			SELECT COALESCE(f.id, s.id) AS id,
			       COALESCE(1.0 / (60 + f.rank), 0) + COALESCE(1.0 / (60 + s.rank), 0) AS rrf_score
			FROM fts f
			FULL OUTER JOIN sem s ON f.id = s.id
		)
		SELECT m.id, m.memory_type, m.title, m.created_at, m.token_count, c.rrf_score
		FROM combined c
		JOIN memories m ON m.id = c.id
		ORDER BY c.rrf_score DESC
		LIMIT %s
	`, typeClause, typeClause, limitPlaceholder)

	dbRows, err := c.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, false, fmt.Errorf("hybrid query: %w", err)
	}
	defer dbRows.Close()

	var results []MemoryIndexRow
	for dbRows.Next() {
		var r MemoryIndexRow
		var createdAt time.Time
		var memoryType string
		if err := dbRows.Scan(&r.ID, &memoryType, &r.Title, &createdAt, &r.Tokens, &r.Score); err != nil {
			return nil, false, fmt.Errorf("scan hybrid row: %w", err)
		}
		r.Type = memoryTypeToEmoji(memoryType)
		r.Date = createdAt.Format("2006-01-02")
		results = append(results, r)
	}
	return results, false, dbRows.Err()
}
