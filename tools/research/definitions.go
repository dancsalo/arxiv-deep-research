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
				"sort": map[string]any{
					"type":        "string",
					"description": "Sort order: 'cited_by_count' (most cited first). Omit for default relevance ranking.",
				},
			},
			Required: []string{"query"},
		},
		"search_openalex",
	)
	t.OfTool.Description = anthropic.String(
		"Search OpenAlex for published academic works. Returns titles, authors, DOIs, abstracts, and citation counts (when available). Use sort='cited_by_count' for descending citation order; default is relevance.",
	)
	return t
}

func BuildFetchArxivPdfTool() anthropic.ToolUnionParam {
	t := anthropic.ToolUnionParamOfTool(
		anthropic.ToolInputSchemaParam{
			Type: "object",
			Properties: map[string]any{
				"arxiv_id": map[string]any{
					"type":        "string",
					"description": "arXiv identifier. Formats: 2301.00001, arXiv:2301.00001, 2301.00001v2 (new), or astro-ph/9901234 (old)",
				},
			},
			Required: []string{"arxiv_id"},
		},
		"fetch_arxiv_pdf",
	)
	t.OfTool.Description = anthropic.String(
		"Fetches the direct PDF download URL for an arXiv preprint. Returns URL only, does not download content. Always returns latest version. arXiv papers only. Rate limit: 1 request per 3 seconds per arXiv TOS.",
	)
	return t
}

func BuildSearchWebTool() anthropic.ToolUnionParam {
	t := anthropic.ToolUnionParamOfTool(
		anthropic.ToolInputSchemaParam{
			Type: "object",
			Properties: map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "Search query for web search",
				},
				"max_results": map[string]any{
					"type":        "integer",
					"description": "Maximum number of results to return (default 10, max 10)",
					"default":     10,
				},
			},
			Required: []string{"query"},
		},
		"search_web",
	)
	t.OfTool.Description = anthropic.String(
		"Search the general web. Returns titles, snippets, and URLs. Use as fallback when arXiv/OpenAlex lack coverage. NOT reliable for 'most cited papers' - use search_openalex with citation sorting instead. WARNING: May be unreliable due to bot detection.",
	)
	return t
}
