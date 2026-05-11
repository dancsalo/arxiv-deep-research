package research

import (
	"encoding/json"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/dancsalo/arxiv-deep-research/internal/registry"
)

// httpClient interface enables test mocks
type httpClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type ResearchToolSet struct {
	client            httpClient
	baseURL           string        // for testing GitHub API, defaults to "https://api.github.com"
	searchWebBaseURL  string        // For web search tool (default: DuckDuckGo HTML endpoint)
	searchWebFailures atomic.Int32  // Consecutive parse failures for circuit breaker (thread-safe)
}

func NewResearchToolSet(client *http.Client) *ResearchToolSet {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	ts := &ResearchToolSet{
		client:           client,
		baseURL:          "https://api.github.com",
		searchWebBaseURL: "https://html.duckduckgo.com/html/",
	}
	ts.searchWebFailures.Store(0)
	return ts
}

// newResearchToolSetWithBase is for testing only
func newResearchToolSetWithBase(client httpClient, baseURL string) *ResearchToolSet {
	return &ResearchToolSet{
		client:  client,
		baseURL: baseURL,
	}
}

func (r *ResearchToolSet) Register(reg *registry.ToolRegistry) {
	reg.Register("search_arxiv", BuildSearchArxivTool(), r.handleSearchArxiv)
	reg.Register("search_openalex", BuildSearchOpenAlexTool(), r.handleSearchOpenAlex)
	reg.Register("fetch_arxiv_pdf", BuildFetchArxivPdfTool(), r.handleFetchArxivPdf)
	reg.Register("search_github_repos", BuildSearchGithubTool(), r.handleSearchGithub)
	reg.Register("search_web", BuildSearchWebTool(), r.handleSearchWeb)
}

func toolError(msg string, recoverable bool) string {
	b, _ := json.Marshal(map[string]any{
		"error":       msg,
		"recoverable": recoverable,
	})
	return string(b)
}
