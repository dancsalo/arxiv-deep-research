package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dancsalo/arxiv-deep-research/memoryclient"
)

type ToolHandler func(input []byte) (string, error)

func NewMemoryToolHandlers(client *memoryclient.Client, sessionID string, currentTurn func() int) map[string]ToolHandler {
	return map[string]ToolHandler{
		"store_memory": func(input []byte) (string, error) {
			var mem memoryclient.StoreMemoryInput
			if err := json.Unmarshal(input, &mem); err != nil {
				return "", fmt.Errorf("unmarshal store_memory input: %w", err)
			}
			id, err := client.StoreMemory(context.Background(), sessionID, currentTurn(), mem)
			if err != nil {
				return "", err
			}
			result, _ := json.Marshal(map[string]any{"id": id, "stored": true})
			return string(result), nil
		},
		"search_memories": func(input []byte) (string, error) {
			var params struct {
				Query      string `json:"query"`
				Mode       string `json:"mode"`
				Limit      int    `json:"limit"`
				MemoryType string `json:"memory_type"`
			}
			if err := json.Unmarshal(input, &params); err != nil {
				return "", fmt.Errorf("unmarshal search_memories input: %w", err)
			}
			result, err := client.SearchMemories(context.Background(), params.Query, params.Mode, params.Limit, params.MemoryType)
			if err != nil {
				return "", err
			}
			b, _ := json.Marshal(result)
			return string(b), nil
		},
		"get_memory_details": func(input []byte) (string, error) {
			var params struct {
				IDs []int64 `json:"ids"`
			}
			if err := json.Unmarshal(input, &params); err != nil {
				return "", fmt.Errorf("unmarshal get_memory_details input: %w", err)
			}
			result, err := client.GetMemoryDetails(context.Background(), params.IDs)
			if err != nil {
				return "", err
			}
			b, _ := json.Marshal(result)
			return string(b), nil
		},
		"get_memory_source": func(input []byte) (string, error) {
			var params struct {
				ID int64 `json:"id"`
			}
			if err := json.Unmarshal(input, &params); err != nil {
				return "", fmt.Errorf("unmarshal get_memory_source input: %w", err)
			}
			result, err := client.GetMemorySource(context.Background(), params.ID)
			if err != nil {
				return "", err
			}
			b, _ := json.Marshal(result)
			return string(b), nil
		},
	}
}
