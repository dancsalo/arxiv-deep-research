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
			return 50 + n*350
		},
	}
}
