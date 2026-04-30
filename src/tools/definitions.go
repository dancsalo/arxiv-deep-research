package tools

import "github.com/anthropics/anthropic-sdk-go"

func BuildSearchMemoriesTool() anthropic.ToolUnionParam {
	t := anthropic.ToolUnionParamOfTool(
		anthropic.ToolInputSchemaParam{
			Type: "object",
			Properties: map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "Search query for memories",
				},
				"mode": map[string]any{
					"type":        "string",
					"enum":        []string{"text", "semantic", "hybrid"},
					"description": "Search mode: 'text' (full-text), 'semantic' (vector similarity), 'hybrid' (both, default)",
					"default":     "hybrid",
				},
				"limit": map[string]any{
					"type":        "integer",
					"description": "Max results to return (default 20)",
					"default":     20,
				},
				"memory_type": map[string]any{
					"type":        "string",
					"description": "Filter by memory type (e.g. 'gotcha', 'decision'). Omit for all types.",
				},
			},
			Required: []string{"query"},
		},
		"search_memories",
	)
	t.OfTool.Description = anthropic.String(
		"Search past research memories. Returns a compact index with IDs, types, titles, dates, and token costs. " +
			"Use get_memory_details to fetch full content of specific memories.",
	)
	return t
}

func BuildGetMemoryDetailsTool() anthropic.ToolUnionParam {
	t := anthropic.ToolUnionParamOfTool(
		anthropic.ToolInputSchemaParam{
			Type: "object",
			Properties: map[string]any{
				"ids": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "integer"},
					"description": "Memory IDs to fetch (from search_memories results)",
				},
			},
			Required: []string{"ids"},
		},
		"get_memory_details",
	)
	t.OfTool.Description = anthropic.String(
		"Fetch full content of specific memories by ID. Use after search_memories to drill into relevant results. " +
			"Returns content, metadata, and whether raw source is available for deeper inspection via get_memory_source.",
	)
	return t
}

func BuildGetMemorySourceTool() anthropic.ToolUnionParam {
	t := anthropic.ToolUnionParamOfTool(
		anthropic.ToolInputSchemaParam{
			Type: "object",
			Properties: map[string]any{
				"id": map[string]any{
					"type":        "integer",
					"description": "Memory ID to fetch raw source for",
				},
			},
			Required: []string{"id"},
		},
		"get_memory_source",
	)
	t.OfTool.Description = anthropic.String(
		"Fetch the original uncompacted raw content that produced a memory. " +
			"This is the largest retrieval — only use when you need the full original context.",
	)
	return t
}

func BuildStoreMemoryTool() anthropic.ToolUnionParam {
	t := anthropic.ToolUnionParamOfTool(
		anthropic.ToolInputSchemaParam{
			Type: "object",
			Properties: map[string]any{
				"memory_type": map[string]any{
					"type":        "string",
					"enum":        []string{"session-goal", "gotcha", "problem-fix", "how-it-works", "what-changed", "discovery", "why-it-exists", "decision", "trade-off"},
					"description": "Category of this memory",
				},
				"title": map[string]any{
					"type":        "string",
					"description": "Compressed title (3-15 words). Must name the specific entity and finding. Bad: 'Observation about search'. Good: 'Hook timeout: 60s too short for npm install'.",
				},
				"content": map[string]any{
					"type":        "string",
					"description": "Full observation content",
				},
				"source": map[string]any{
					"type":        "string",
					"description": "Raw uncompacted original content (optional, for Layer 3 retrieval)",
				},
			},
			Required: []string{"memory_type", "title", "content"},
		},
		"store_memory",
	)
	t.OfTool.Description = anthropic.String(
		"Store an observation, decision, or finding as a persistent memory for future sessions. " +
			"Choose the most specific memory_type. Write a title that would let a future agent decide whether to fetch the full content. " +
			"Title MUST be 3-15 words and include the specific entity (function, tool, config) and the key finding.",
	)
	return t
}

func BuildAllTools() []anthropic.ToolUnionParam {
	return []anthropic.ToolUnionParam{
		BuildSearchMemoriesTool(),
		BuildGetMemoryDetailsTool(),
		BuildGetMemorySourceTool(),
		BuildStoreMemoryTool(),
	}
}
