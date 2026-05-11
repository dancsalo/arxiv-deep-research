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

func BuildSearchGithubTool() anthropic.ToolUnionParam {
	t := anthropic.ToolUnionParamOfTool(
		anthropic.ToolInputSchemaParam{
			Type: "object",
			Properties: map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "Search query for GitHub repositories. Supports GitHub search syntax: 'transformer language:python stars:>100 pushed:>2023-01-01'. Examples: 'attention mechanism implementation', 'diffusion model pytorch'.",
				},
				"max_results": map[string]any{
					"type":        "integer",
					"description": "Maximum number of results to return (default 5, max 5)",
				},
			},
			Required: []string{"query"},
		},
		"search_github_repos",
	)
	t.OfTool.Description = anthropic.String(
		"Search GitHub repositories for code implementations, sorted by stars. IMPORTANT: Results are automatically filtered to show only popular, actively-maintained repos (>100 stars, updated within 2 years, not archived). Returns repository name, description, star count, language, license, topics, last updated, and URL. Use when user asks for established, production-ready implementations or popular GitHub repos. NOT suitable for finding experimental, niche, or small projects.",
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

func BuildGetCitationsAndReferencesTool() anthropic.ToolUnionParam {
	t := anthropic.ToolUnionParamOfTool(
		anthropic.ToolInputSchemaParam{
			Type: "object",
			Properties: map[string]any{
				"work_id": map[string]any{
					"type":        "string",
					"description": "OpenAlex work ID (format: W2741809807). Use search_openalex to find work IDs.",
				},
				"direction": map[string]any{
					"type":        "string",
					"description": "Direction: 'references' (papers cited BY this work) or 'cited_by' (papers that cite this work)",
				},
				"max_results": map[string]any{
					"type":        "integer",
					"description": "Maximum number of results to return (default 10, max 50)",
					"default":     10,
				},
			},
			Required: []string{"work_id", "direction"},
		},
		"get_citations_and_references",
	)
	t.OfTool.Description = anthropic.String(
		"Get citations and references for an academic work from OpenAlex. Use direction='references' to get the bibliography (papers cited BY this work), or direction='cited_by' to get forward citations (papers that cite this work). Returns lightweight metadata: title, authors, year, citation count, DOI. For abstracts, use search_openalex with the DOI.",
	)
	return t
}
