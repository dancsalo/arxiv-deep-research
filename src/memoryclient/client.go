package memoryclient

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"

	_ "github.com/lib/pq"
)

type Client struct {
	db         *sql.DB
	embedURL   string
	httpClient *http.Client
}

type Config struct {
	AdminDSN    string
	MemoriesDSN string
	EmbedURL    string
}

func New(cfg Config) (*Client, error) {
	db, err := EnsureDatabase(cfg.AdminDSN, cfg.MemoriesDSN)
	if err != nil {
		return nil, fmt.Errorf("ensure database: %w", err)
	}

	if err := RunMigrations(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrations: %w", err)
	}

	return &Client{
		db:         db,
		embedURL:   cfg.EmbedURL,
		httpClient: &http.Client{},
	}, nil
}

func (c *Client) Close() error {
	return c.db.Close()
}

type embedRequest struct {
	Texts []string `json:"texts"`
}

type embedResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
	Dimensions int         `json:"dimensions"`
}

func (c *Client) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	body, err := json.Marshal(embedRequest{Texts: texts})
	if err != nil {
		return nil, fmt.Errorf("marshal embed request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, "POST", c.embedURL+"/embed", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embed request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("embed returned %d", resp.StatusCode)
	}

	var out embedResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode embed response: %w", err)
	}
	return out.Embeddings, nil
}
