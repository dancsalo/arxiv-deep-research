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
			// Base response overhead (metadata fields)
			base := 200

			// Estimate text content tokens based on max_length
			maxLength := 8000 // default
			if ml, ok := args["max_length"].(float64); ok && ml > 0 {
				maxLength = int(ml)
			}
			if maxLength > 50000 {
				maxLength = 50000
			}

			// Rough estimate: 1 token per 4 characters
			textTokens := maxLength / 4

			return base + textTokens
		},
		"fetch_arxiv_text": func(args map[string]any) int {
			maxLength := 25000
			if ml, ok := args["max_length"].(float64); ok && ml > 0 {
				maxLength = int(ml)
				if maxLength > 25000 {
					maxLength = 25000
				}
			}
			// Estimate ~4 chars per token for text content
			// Add 100 tokens for JSON structure overhead
			return (maxLength / 4) + 100
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
		"fetch_webpage_content": func(args map[string]any) int {
			// Default to 8000 chars, which is ~2000 tokens
			maxLength := 8000
			if v, ok := args["max_length"].(float64); ok && v > 0 {
				maxLength = int(v)
			}
			// Estimate: text_content + metadata (title, author, excerpt, etc)
			// Assume ~4 chars per token, plus 200 tokens overhead for metadata
			return (maxLength / 4) + 200
		},
	}
}