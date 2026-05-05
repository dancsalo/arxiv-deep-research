package research

import (
	"encoding/json"
	"net/http"

	"github.com/dancsalo/arxiv-deep-research/internal/registry"
)

type ResearchToolSet struct {
	client *http.Client
}

func NewResearchToolSet(client *http.Client) *ResearchToolSet {
	return &ResearchToolSet{client: client}
}

func (r *ResearchToolSet) Register(reg *registry.ToolRegistry) {
	reg.Register("search_arxiv", BuildSearchArxivTool(), r.handleSearchArxiv)
	reg.Register("search_openalex", BuildSearchOpenAlexTool(), r.handleSearchOpenAlex)
}

func toolError(msg string, recoverable bool) string {
	b, _ := json.Marshal(map[string]any{
		"error":       msg,
		"recoverable": recoverable,
	})
	return string(b)
}
