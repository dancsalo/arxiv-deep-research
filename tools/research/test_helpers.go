package research

// newTestResearchToolSet creates a ResearchToolSet configured for testing.
// This helper ensures all required fields are properly initialized.
func newTestResearchToolSet(client httpClient) *ResearchToolSet {
	return &ResearchToolSet{
		client:           client,
		baseURL:          "https://api.github.com",
		searchWebBaseURL: "https://html.duckduckgo.com/html/",
		arxivRateLimiter: newRateLimiter(0), // No rate limiting in tests
	}
}

// newTestResearchToolSetWithBase creates a ResearchToolSet with custom baseURL for testing.
func newTestResearchToolSetWithBase(client httpClient, baseURL string) *ResearchToolSet {
	return &ResearchToolSet{
		client:           client,
		baseURL:          baseURL,
		searchWebBaseURL: "https://html.duckduckgo.com/html/",
		arxivRateLimiter: newRateLimiter(0), // No rate limiting in tests
	}
}
