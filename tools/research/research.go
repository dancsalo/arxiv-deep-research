package research

import (
	"encoding/json"
	"net/http"
	"sync/atomic"

	"github.com/dancsalo/arxiv-deep-research/internal/registry"
)

type ResearchToolSet struct {
	client            *http.Client
	searchWebBaseURL  string         // For web search tool (default: DuckDuckGo HTML endpoint)
	searchWebFailures atomic.Int32   // Consecutive parse failures for circuit breaker (thread-safe)
}

func NewResearchToolSet(client *http.Client) *ResearchToolSet {
	ts := &ResearchToolSet{
		client:           client,
		searchWebBaseURL: "https://html.duckduckgo.com/html/",
	}
	ts.searchWebFailures.Store(0)
	return ts
}

func (r *ResearchToolSet) Register(reg *registry.ToolRegistry) {
	reg.Register("search_arxiv", BuildSearchArxivTool(), r.handleSearchArxiv)
	reg.Register("search_openalex", BuildSearchOpenAlexTool(), r.handleSearchOpenAlex)
	reg.Register("fetch_arxiv_pdf", BuildFetchArxivPdfTool(), r.handleFetchArxivPdf)
	reg.Register("search_web", BuildSearchWebTool(), r.handleSearchWeb)
}

func toolError(msg string, recoverable bool) string {
	b, _ := json.Marshal(map[string]any{
		"error":       msg,
		"recoverable": recoverable,
	})
	return string(b)
}
