package research

import "github.com/anthropics/anthropic-sdk-go"

func BuildSearchArxivTool() anthropic.ToolUnionParam {
	t := anthropic.ToolUnionParamOfTool(
		anthropic.ToolInputSchemaParam{
			Type: "object",
			Properties: map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "Search query for arXiv papers",
				},
				"max_results": map[string]any{
					"type":        "integer",
					"description": "Maximum number of results to return (default 10)",
					"default":     10,
				},
			},
			Required: []string{"query"},
		},
		"search_arxiv",
	)
	t.OfTool.Description = anthropic.String(
		"Search arXiv for academic preprints. Returns titles, authors, abstracts, and links.",
	)
	return t
}

func BuildSearchOpenAlexTool() anthropic.ToolUnionParam {
	t := anthropic.ToolUnionParamOfTool(
		anthropic.ToolInputSchemaParam{
			Type: "object",
			Properties: map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "Search query for academic works",
				},
				"max_results": map[string]any{
					"type":        "integer",
					"description": "Maximum number of results to return (default 10)",
					"default":     10,
				},
				"filter": map[string]any{
					"type":        "string",
					"description": "Optional OpenAlex filter expression (e.g. \"publication_year:>2022\")",
				},
			},
			Required: []string{"query"},
		},
		"search_openalex",
	)
	t.OfTool.Description = anthropic.String(
		"Search OpenAlex for published academic works. Returns titles, authors, DOIs, and abstracts.",
	)
	return t
}
