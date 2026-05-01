package tools

import (
	"context"
	"time"

	"github.com/dancsalo/arxiv-deep-research/memoryclient"
)

type SizeEstimatorFunc func(args map[string]any) int

func MemoryToolEstimators(memClient *memoryclient.Client) map[string]SizeEstimatorFunc {
	return map[string]SizeEstimatorFunc{
		"search_memories": func(args map[string]any) int {
			limit := 20.0
			if v, ok := args["limit"].(float64); ok && v > 0 {
				limit = v
			}
			return 50 + int(limit)*20
		},
		"get_memory_details": func(args map[string]any) int {
			ids, ok := args["ids"].([]any)
			if !ok || len(ids) == 0 {
				return 200
			}
			return 50 + len(ids)*200
		},
		"get_memory_source": func(args map[string]any) int {
			id, ok := args["id"].(float64)
			if !ok || id == 0 {
				return 2000
			}
			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()
			tokens, err := memClient.PeekSourceSize(ctx, int64(id))
			if err != nil {
				return 2000
			}
			return tokens + 50
		},
		"store_memory": func(args map[string]any) int {
			return 50
		},
	}
}

func ReduceToolArgs(toolName string, args map[string]any) (map[string]any, bool) {
	reduced := make(map[string]any)
	for k, v := range args {
		reduced[k] = v
	}

	switch toolName {
	case "search_memories":
		reduced["limit"] = 10
		return reduced, true
	case "get_memory_details":
		if ids, ok := args["ids"].([]any); ok && len(ids) > 3 {
			cp := make([]any, 3)
			copy(cp, ids[:3])
			reduced["ids"] = cp
			return reduced, true
		}
	}
	return reduced, false
}
