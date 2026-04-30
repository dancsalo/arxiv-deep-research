package memoryclient

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

type StoreMemoryInput struct {
	MemoryType string `json:"memory_type"`
	Title      string `json:"title"`
	Content    string `json:"content"`
	Source     string `json:"source,omitempty"`
}

func validateTitle(title string) error {
	words := strings.Fields(title)
	if len(words) < 3 {
		return fmt.Errorf("title too short (%d words, minimum 3): %q", len(words), title)
	}
	if len(words) > 15 {
		return fmt.Errorf("title too long (%d words, maximum 15): %q", len(words), title)
	}
	return nil
}

func estimateTokensFast(text string) int {
	if len(text) == 0 {
		return 0
	}
	n := len(text) / 4
	if n == 0 {
		return 1
	}
	return n
}

var validMemoryTypes = map[string]bool{
	"session-goal": true, "gotcha": true, "problem-fix": true,
	"how-it-works": true, "what-changed": true, "discovery": true,
	"why-it-exists": true, "decision": true, "trade-off": true,
}

func pgVectorLiteral(v []float32) string {
	parts := make([]string, len(v))
	for i, f := range v {
		parts[i] = fmt.Sprintf("%f", f)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func nullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

func memoryTypeToEmoji(t string) string {
	switch t {
	case "session-goal":
		return "🎯"
	case "gotcha":
		return "🔴"
	case "problem-fix":
		return "🟡"
	case "how-it-works":
		return "🔵"
	case "what-changed":
		return "🟢"
	case "discovery":
		return "🟣"
	case "why-it-exists":
		return "🟠"
	case "decision":
		return "🟤"
	case "trade-off":
		return "⚖️"
	default:
		return t
	}
}

func (c *Client) StoreMemory(ctx context.Context, sessionID string, turnIndex int, mem StoreMemoryInput) (int64, error) {
	if !validMemoryTypes[mem.MemoryType] {
		return 0, fmt.Errorf("invalid memory type: %q", mem.MemoryType)
	}
	if err := validateTitle(mem.Title); err != nil {
		return 0, err
	}

	embedText := mem.Title + ". " + mem.Content
	embeddings, err := c.Embed(ctx, []string{embedText})
	if err != nil {
		return c.insertWithoutEmbedding(ctx, sessionID, turnIndex, mem)
	}

	tokenCount := estimateTokensFast(mem.Content)
	var sourceTokens *int
	if mem.Source != "" {
		st := estimateTokensFast(mem.Source)
		sourceTokens = &st
	}

	vecStr := pgVectorLiteral(embeddings[0])

	query := `
		INSERT INTO memories (session_id, turn_index, memory_type, title, content, source, token_count, source_tokens, embedding)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::vector)
		RETURNING id
	`
	var id int64
	err = c.db.QueryRowContext(ctx, query,
		sessionID, turnIndex, mem.MemoryType, mem.Title, mem.Content, nullString(mem.Source),
		tokenCount, sourceTokens, vecStr,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insert memory: %w", err)
	}

	return id, nil
}

func (c *Client) insertWithoutEmbedding(ctx context.Context, sessionID string, turnIndex int, mem StoreMemoryInput) (int64, error) {
	tokenCount := estimateTokensFast(mem.Content)
	var sourceTokens *int
	if mem.Source != "" {
		st := estimateTokensFast(mem.Source)
		sourceTokens = &st
	}

	query := `
		INSERT INTO memories (session_id, turn_index, memory_type, title, content, source, token_count, source_tokens)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id
	`
	var id int64
	err := c.db.QueryRowContext(ctx, query,
		sessionID, turnIndex, mem.MemoryType, mem.Title, mem.Content, nullString(mem.Source),
		tokenCount, sourceTokens,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insert memory (no embedding): %w", err)
	}
	return id, nil
}
