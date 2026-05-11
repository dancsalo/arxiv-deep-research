package research

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/dancsalo/arxiv-deep-research/internal/registry"
)

// httpClient interface enables test mocks
type httpClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type ResearchToolSet struct {
	client  httpClient
	baseURL string // for testing, defaults to "https://api.github.com"
}

func NewResearchToolSet(client *http.Client) *ResearchToolSet {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &ResearchToolSet{
		client:  client,
		baseURL: "https://api.github.com",
	}
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
}

func toolError(msg string, recoverable bool) string {
	b, _ := json.Marshal(map[string]any{
		"error":       msg,
		"recoverable": recoverable,
	})
	return string(b)
}
