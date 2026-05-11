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
	}
}
