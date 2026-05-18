package research

import "github.com/anthropics/anthropic-sdk-go"

func BuildSearchArxivTool() anthropic.ToolUnionParam {
	t := anthropic.ToolUnionParamOfTool(
		anthropic.ToolInputSchemaParam{
			Type: "object",
			Properties: map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "Search terms or paper title",
				},
				"authors": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Author last names (optional). Example: [\"Vaswani\", \"Shazeer\"] for multi-author search. Use this for author-specific searches rather than including names in query.",
				},
				"category": map[string]any{
					"type":        "string",
					"description": "arXiv category to filter results (optional). Common categories: 'cs.LG' (Machine Learning), 'cs.CV' (Computer Vision), 'cs.CL' (NLP), 'cs.AI' (Artificial Intelligence), 'cs.RO' (Robotics), 'stat.ML' (Statistics/ML). Use this to avoid wrong-domain contamination.",
				},
				"exact_phrase": map[string]any{
					"type":        "boolean",
					"description": "If true, searches for exact title match (optional, default: false). Use this when searching for a specific known paper by its full title. Automatically uses relevance sorting.",
					"default":     false,
				},
				"max_results": map[string]any{
					"type":        "integer",
					"description": "Maximum number of results to return (default 10)",
					"default":     10,
				},
				"search_field": map[string]any{
					"type":        "string",
					"description": "Field to search: 'title' (default) or 'abstract' (legacy parameter, usually not needed with structured params)",
					"default":     "title",
				},
				"sort_by": map[string]any{
					"type":        "string",
					"description": "Sort by: 'relevance' (best for specific papers) or 'submittedDate' (best for recent papers). Default: 'relevance' for exact_phrase searches, 'submittedDate' otherwise.",
					"enum":        []string{"submittedDate", "lastUpdatedDate", "relevance"},
				},
				"sort_order": map[string]any{
					"type":        "string",
					"description": "Sort order: 'descending' (default) or 'ascending'",
					"enum":        []string{"descending", "ascending"},
					"default":     "descending",
				},
			},
			Required: []string{"query"},
		},
		"search_arxiv",
	)
	t.OfTool.Description = anthropic.String(
		"Search arXiv for academic preprints. STRUCTURED SEARCH: Provide explicit authors/category/exact_phrase parameters for best results. Examples: (1) Specific paper: query=\"Attention Is All You Need\", exact_phrase=true, sort_by=\"relevance\". (2) Author search: query=\"graph convolutional networks\", authors=[\"Kipf\"], category=\"cs.LG\". (3) Topic search: query=\"diffusion models\", category=\"cs.CV\". Available categories: cs.LG (ML), cs.CV (Vision), cs.CL (NLP), cs.AI (AI), cs.RO (Robotics).",
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
					"description": "Search query for academic works. Be SPECIFIC: use full paper titles, author names, and domain context. Generic queries (e.g., 'DARTS', 'transformers') often return wrong-domain papers. Good: 'DARTS Differentiable Architecture Search neural networks Liu'. Bad: 'DARTS' (returns biology papers).",
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
		"Search the general web. Returns titles and URLs only (no snippets). Use as fallback when arXiv/OpenAlex lack coverage. NOT reliable for 'most cited papers' - use search_openalex with citation sorting instead. WARNING: May be unreliable due to bot detection.",
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

func BuildFetchWebpageContentTool() anthropic.ToolUnionParam {
	t := anthropic.ToolUnionParamOfTool(
		anthropic.ToolInputSchemaParam{
			Type: "object",
			Properties: map[string]any{
				"url": map[string]any{
					"type":        "string",
					"description": "URL of webpage to fetch and extract main content from",
				},
				"max_length": map[string]any{
					"type":        "integer",
					"description": "Maximum content length in characters (default 8000, max 15000)",
					"default":     8000,
				},
			},
			Required: []string{"url"},
		},
		"fetch_webpage_content",
	)
	t.OfTool.Description = anthropic.String(
		"Fetch and extract main article content from a webpage. Returns cleaned text with HTML/ads/navigation removed using readability extraction. Use after search_web to get full article text when snippets are empty or you need more detail. Rate limit: 1 request per 2 seconds to be respectful. Works best on article/blog pages; may fail on dynamic JavaScript sites or paywalled content.",
	)
	return t
}

type ArxivTextResult struct {
	ArxivID     string `json:"arxiv_id"`
	TextContent string `json:"text_content"`
	Truncated   bool   `json:"truncated"`
	Error       string `json:"error,omitempty"`
}

func BuildFetchArxivTextTool() anthropic.ToolUnionParam {
	t := anthropic.ToolUnionParamOfTool(
		anthropic.ToolInputSchemaParam{
			Type: "object",
			Properties: map[string]any{
				"arxiv_id": map[string]any{
					"type":        "string",
					"description": "arXiv identifier. Formats: 2301.00001, arXiv:2301.00001, 2301.00001v2 (new), or astro-ph/9901234 (old)",
				},
				"max_length": map[string]any{
					"type":        "integer",
					"description": "Maximum text content length in characters (default 25000, max 25000)",
					"default":     25000,
				},
			},
			Required: []string{"arxiv_id"},
		},
		"fetch_arxiv_text",
	)
	t.OfTool.Description = anthropic.String(
		"Fetches and extracts text content from an arXiv preprint HTML page. Returns cleaned article text extracted from arXiv's HTML rendering. Text is truncated to max_length (default/max 25000 characters). Not all papers have HTML versions - older papers or recent submissions may return an error. Rate limit: 1 request per 3 seconds per arXiv TOS.",
	)
	return t
}
