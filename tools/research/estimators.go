package research

func ResearchToolEstimators() map[string]func(args map[string]any) int {
	return map[string]func(args map[string]any) int{
		"search_arxiv": func(args map[string]any) int {
			n := 10
			if v, ok := args["max_results"].(float64); ok && v > 0 {
				n = int(v)
			}
			return 50 + n*400
		},
		"search_openalex": func(args map[string]any) int {
			n := 10
			if v, ok := args["max_results"].(float64); ok && v > 0 {
				n = int(v)
			}
			// Increased from 350 to 360 per result to account for citation count field (~10 tokens per result)
			return 50 + n*360
		},
		"fetch_arxiv_pdf": func(args map[string]any) int {
			return 100 // Fixed cost: just returns URL (small JSON response)
		},
		"search_github_repos": func(args map[string]any) int {
			// Estimate: base overhead + query + results
			// Each result: ~200 tokens (name, desc, metadata, URL)
			n := 5 // default
			if v, ok := args["max_results"].(float64); ok && v > 0 {
				n = int(v)
				if n > 5 {
					n = 5 // cap at 5
				}
			}

			// Query length estimation
			queryTokens := 0
			if query, ok := args["query"].(string); ok {
				queryTokens = len(query) / 4 // rough estimate: 4 chars per token
			}

			baseOverhead := 50
			resultTokens := n * 200

			return baseOverhead + queryTokens + resultTokens
		},
		"search_web": func(args map[string]any) int {
			// Web search returns up to 10 results with title, snippet, URL
			// Estimated ~200 tokens per result
			return 50 + 10*200 // 2050 tokens
		},
		"get_citations_and_references": func(args map[string]any) int {
			n := 10
			if v, ok := args["max_results"].(float64); ok && v > 0 {
				n = int(v)
			}
			// Base overhead (50) + per-result (180 tokens)
			// 180 = title(30) + authors(40) + year(5) + doi(30) + id(20) + cited_by_count(10) + JSON(45)
			return 50 + n*180
		},
	}
}
