package research

import (
	"encoding/json"
	"net/http"
	"sync"
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
	arxivRateLimiter  *rateLimiter  // Rate limiter for arXiv API (max 3 req/sec)
}

// rateLimiter enforces minimum time between requests
type rateLimiter struct {
	mu           sync.Mutex
	lastRequest  time.Time
	minInterval  time.Duration
}

// newRateLimiter creates a rate limiter with the specified minimum interval between requests
func newRateLimiter(minInterval time.Duration) *rateLimiter {
	return &rateLimiter{
		minInterval: minInterval,
	}
}

// Wait blocks until enough time has passed since the last request
func (rl *rateLimiter) Wait() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	if !rl.lastRequest.IsZero() {
		elapsed := time.Since(rl.lastRequest)
		if remaining := rl.minInterval - elapsed; remaining > 0 {
			time.Sleep(remaining)
		}
	}
	rl.lastRequest = time.Now()
}

func NewResearchToolSet(client *http.Client) *ResearchToolSet {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	ts := &ResearchToolSet{
		client:           client,
		baseURL:          "https://api.github.com",
		searchWebBaseURL: "https://html.duckduckgo.com/html/",
		arxivRateLimiter: newRateLimiter(334 * time.Millisecond), // 3 req/sec = 333.33ms minimum, rounded to 334ms
	}
	ts.searchWebFailures.Store(0)
	return ts
}

// newResearchToolSetWithBase is for testing only
func newResearchToolSetWithBase(client httpClient, baseURL string) *ResearchToolSet {
	return &ResearchToolSet{
		client:           client,
		baseURL:          baseURL,
		arxivRateLimiter: newRateLimiter(334 * time.Millisecond),
	}
}

func (r *ResearchToolSet) Register(reg *registry.ToolRegistry) {
	reg.Register("search_arxiv", BuildSearchArxivTool(), r.handleSearchArxiv)
	reg.Register("search_openalex", BuildSearchOpenAlexTool(), r.handleSearchOpenAlex)
	reg.Register("fetch_arxiv_text", BuildFetchArxivTextTool(), r.handleFetchArxivText)
	reg.Register("search_github_repos", BuildSearchGithubTool(), r.handleSearchGithub)
	reg.Register("search_web", BuildSearchWebTool(), r.handleSearchWeb)
	reg.Register("get_citations_and_references", BuildGetCitationsAndReferencesTool(), r.handleGetCitationsAndReferences)
	reg.Register("fetch_webpage_content", BuildFetchWebpageContentTool(), r.handleFetchWebpageContent)
}

func toolError(msg string, recoverable bool) string {
	b, _ := json.Marshal(map[string]any{
		"error":       msg,
		"recoverable": recoverable,
	})
	return string(b)
}
