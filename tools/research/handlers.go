package research

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type ArxivResult struct {
	Title     string   `json:"title"`
	Authors   []string `json:"authors"`
	Abstract  string   `json:"abstract"`
	Link      string   `json:"link"`
	Published string   `json:"published"`
}

type atomFeed struct {
	XMLName xml.Name    `xml:"feed"`
	Entries []atomEntry `xml:"entry"`
}

type atomEntry struct {
	Title     string       `xml:"title"`
	Summary   string       `xml:"summary"`
	ID        string       `xml:"id"`
	Authors   []atomAuthor `xml:"author"`
	Published string       `xml:"published"`
}

type atomAuthor struct {
	Name string `xml:"name"`
}

func (r *ResearchToolSet) handleSearchArxiv(ctx context.Context, input json.RawMessage) (string, error) {
	var params struct {
		Query      string `json:"query"`
		MaxResults int    `json:"max_results"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return toolError("invalid input: "+err.Error(), false), nil
	}
	if params.MaxResults <= 0 {
		params.MaxResults = 10
	}

	u := fmt.Sprintf("https://export.arxiv.org/api/query?search_query=all:%s&max_results=%d",
		url.QueryEscape(params.Query), params.MaxResults)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return toolError("request creation failed: "+err.Error(), false), nil
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return toolError("arXiv request failed: "+err.Error(), true), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return toolError(fmt.Sprintf("arXiv returned status %d", resp.StatusCode), true), nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return toolError("failed to read response: "+err.Error(), true), nil
	}

	var feed atomFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		return toolError("failed to parse arXiv XML: "+err.Error(), true), nil
	}

	results := make([]ArxivResult, 0, len(feed.Entries))
	for _, entry := range feed.Entries {
		authors := make([]string, 0, len(entry.Authors))
		for _, a := range entry.Authors {
			authors = append(authors, a.Name)
		}
		results = append(results, ArxivResult{
			Title:     strings.TrimSpace(entry.Title),
			Authors:   authors,
			Abstract:  strings.TrimSpace(entry.Summary),
			Link:      strings.TrimSpace(entry.ID),
			Published: entry.Published,
		})
	}

	b, _ := json.Marshal(results)
	return string(b), nil
}

type OpenAlexResult struct {
	Title    string   `json:"title"`
	Authors  []string `json:"authors"`
	DOI      string   `json:"doi"`
	Abstract string   `json:"abstract"`
	Year     int      `json:"year"`
}

type openAlexResponse struct {
	Results []openAlexWork `json:"results"`
}

type openAlexWork struct {
	Title                 string               `json:"title"`
	DOI                   string               `json:"doi"`
	PublicationYear       int                  `json:"publication_year"`
	Authorships           []openAlexAuthorship `json:"authorships"`
	AbstractInvertedIndex map[string][]int     `json:"abstract_inverted_index"`
}

type openAlexAuthorship struct {
	Author struct {
		DisplayName string `json:"display_name"`
	} `json:"author"`
}

func (r *ResearchToolSet) handleSearchOpenAlex(ctx context.Context, input json.RawMessage) (string, error) {
	var params struct {
		Query      string `json:"query"`
		MaxResults int    `json:"max_results"`
		Filter     string `json:"filter"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return toolError("invalid input: "+err.Error(), false), nil
	}
	if params.MaxResults <= 0 {
		params.MaxResults = 10
	}

	u := "https://api.openalex.org/works?search=" + url.QueryEscape(params.Query) +
		"&per_page=" + strconv.Itoa(params.MaxResults) +
		"&mailto=arxiv-deep-research@users.noreply.github.com"
	if params.Filter != "" {
		u += "&filter=" + url.QueryEscape(params.Filter)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return toolError("request creation failed: "+err.Error(), false), nil
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return toolError("OpenAlex request failed: "+err.Error(), true), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return toolError(fmt.Sprintf("OpenAlex returned status %d", resp.StatusCode), true), nil
	}

	var oaResp openAlexResponse
	if err := json.NewDecoder(resp.Body).Decode(&oaResp); err != nil {
		return toolError("failed to parse OpenAlex response: "+err.Error(), true), nil
	}

	results := make([]OpenAlexResult, 0, len(oaResp.Results))
	for _, work := range oaResp.Results {
		authors := make([]string, 0, len(work.Authorships))
		for _, a := range work.Authorships {
			authors = append(authors, a.Author.DisplayName)
		}
		results = append(results, OpenAlexResult{
			Title:    work.Title,
			Authors:  authors,
			DOI:      work.DOI,
			Abstract: reconstructAbstract(work.AbstractInvertedIndex),
			Year:     work.PublicationYear,
		})
	}

	b, _ := json.Marshal(results)
	return string(b), nil
}

func reconstructAbstract(index map[string][]int) string {
	if len(index) == 0 {
		return ""
	}

	maxPos := 0
	for _, positions := range index {
		for _, pos := range positions {
			if pos > maxPos {
				maxPos = pos
			}
		}
	}

	words := make([]string, maxPos+1)
	type wordPos struct {
		word string
		pos  int
	}
	var pairs []wordPos
	for word, positions := range index {
		for _, pos := range positions {
			pairs = append(pairs, wordPos{word, pos})
		}
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].pos < pairs[j].pos })
	for _, p := range pairs {
		words[p.pos] = p.word
	}

	return strings.Join(words, " ")
}

type ArxivPdfResult struct {
	ArxivID string `json:"arxiv_id"`
	PdfURL  string `json:"pdf_url"`
	Version string `json:"version,omitempty"`
}

// GitHub search result types
type GitHubRepoResult struct {
	Name        string   `json:"name"`
	FullName    string   `json:"full_name"`
	Description string   `json:"description"`
	Stars       int      `json:"stars"`
	URL         string   `json:"url"`
	Language    string   `json:"language"`
	License     string   `json:"license"`
	Topics      []string `json:"topics"`
	UpdatedAt   string   `json:"updated_at"`
	IsArchived  bool     `json:"is_archived"`
}

type githubSearchResponse struct {
	TotalCount        int              `json:"total_count"`
	IncompleteResults bool             `json:"incomplete_results"`
	Items             []githubRepoItem `json:"items"`
}

type githubRepoItem struct {
	Name        string             `json:"name"`
	FullName    string             `json:"full_name"`
	Description string             `json:"description"`
	HTMLURL     string             `json:"html_url"`
	Stars       int                `json:"stargazers_count"`
	Language    string             `json:"language"`
	License     *githubLicenseInfo `json:"license"`
	Topics      []string           `json:"topics"`
	UpdatedAt   string             `json:"updated_at"`
	Archived    bool               `json:"archived"`
}

type githubLicenseInfo struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

func (r *ResearchToolSet) handleFetchArxivPdf(ctx context.Context, input json.RawMessage) (string, error) {
	// Parse and validate input
	var params struct {
		ArxivID string `json:"arxiv_id"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return toolError("invalid input: "+err.Error(), false), nil
	}
	if params.ArxivID == "" {
		return toolError("arxiv_id is required", false), nil
	}

	// Normalize and validate arXiv ID
	normalized, version, err := normalizeArxivID(params.ArxivID)
	if err != nil {
		return toolError("invalid arXiv ID format: "+err.Error(), false), nil
	}

	// Construct PDF URL
	pdfURL := fmt.Sprintf("https://export.arxiv.org/pdf/%s.pdf", normalized)

	// Validate URL (HEAD request to check existence)
	// Convert httpClient interface to *http.Client for validateArxivPdf
	// validateArxivPdf needs *http.Client to access Transport field
	var concreteClient *http.Client
	if c, ok := r.client.(*http.Client); ok {
		concreteClient = c
	} else {
		// For test mocks without Transport, create a default client
		concreteClient = &http.Client{Timeout: 10 * time.Second}
	}
	if err := validateArxivPdf(ctx, concreteClient, pdfURL); err != nil {
		return toolError("PDF not found: "+err.Error(), true), nil
	}

	// Return result
	result := ArxivPdfResult{
		ArxivID: normalized,
		PdfURL:  pdfURL,
		Version: version,
	}
	b, _ := json.Marshal(result)
	return string(b), nil
}

func normalizeArxivID(id string) (normalized string, version string, err error) {
	// Strip common prefixes and whitespace
	id = strings.TrimSpace(id)
	id = strings.TrimPrefix(id, "arXiv:")
	id = strings.TrimPrefix(id, "http://arxiv.org/abs/")
	id = strings.TrimPrefix(id, "https://arxiv.org/abs/")

	// Extract version suffix (v1, v2, etc.) - must be at end and followed by digits
	version = ""
	versionRegex := regexp.MustCompile(`v\d+$`)
	if match := versionRegex.FindString(id); match != "" {
		version = match
		id = strings.TrimSuffix(id, match)
	}

	// Validate format
	// New format: YYMM.NNNNN (4 digits + period + 4-5 digits)
	newFormatRegex := regexp.MustCompile(`^\d{4}\.\d{4,5}$`)
	// Old format: category/YYMMNNN (e.g., astro-ph/9901234)
	oldFormatRegex := regexp.MustCompile(`^[a-z\-]+/\d{7}$`)

	if !newFormatRegex.MatchString(id) && !oldFormatRegex.MatchString(id) {
		return "", "", fmt.Errorf("invalid arXiv ID format: %s", id)
	}

	return id, version, nil
}

func validateArxivPdf(ctx context.Context, client *http.Client, pdfURL string) error {
	// Configure client with timeout and redirect checking
	// Use the provided client's transport but add our own CheckRedirect
	clientWithRedirectCheck := &http.Client{
		Timeout:   10 * time.Second,
		Transport: client.Transport, // Use provided transport (important for tests)
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Validate redirect target is still arxiv.org domain
			if !strings.Contains(req.URL.Host, "arxiv.org") {
				return fmt.Errorf("suspicious redirect to: %s", req.URL.String())
			}
			// Allow up to 3 redirects
			if len(via) >= 3 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}

	// Send HEAD request with timeout
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodHead, pdfURL, nil)
	req.Header.Set("User-Agent", "arxiv-deep-research/1.0")

	resp, err := clientWithRedirectCheck.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Check final status after following redirects
	if resp.StatusCode != 200 && resp.StatusCode != 301 && resp.StatusCode != 302 {
		if resp.StatusCode == 404 {
			return fmt.Errorf("paper not found")
		}
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	return nil
}

func (r *ResearchToolSet) handleSearchGithub(ctx context.Context, input json.RawMessage) (string, error) {
	var params struct {
		Query      string `json:"query"`
		MaxResults int    `json:"max_results"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return toolError("invalid input: "+err.Error(), false), nil
	}
	if params.Query == "" {
		return toolError("query is required", false), nil
	}
	if params.MaxResults <= 0 {
		params.MaxResults = 5
	}
	if params.MaxResults > 5 {
		params.MaxResults = 5
	}

	// Add quality filters to query: min 100 stars, updated in last 2 years
	enhancedQuery := fmt.Sprintf("%s stars:>100 pushed:>2024-01-01", params.Query)

	// Build GitHub API search URL
	apiURL := fmt.Sprintf("%s/search/repositories?q=%s&sort=stars&order=desc&per_page=%d",
		r.baseURL, url.QueryEscape(enhancedQuery), params.MaxResults)

	return r.executeGithubSearch(ctx, apiURL, params.Query)
}

func (r *ResearchToolSet) executeGithubSearch(ctx context.Context, apiURL, originalQuery string) (string, error) {
	// Create request
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return toolError(fmt.Sprintf("request creation failed for query '%s': %s", originalQuery, err.Error()), false), nil
	}

	// Use current stable API version
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "arxiv-deep-research (github.com/dancsalo/arxiv-deep-research)")

	// Execute request with retry logic
	var resp *http.Response
	maxRetries := 2
	for attempt := 0; attempt <= maxRetries; attempt++ {
		resp, err = r.client.Do(req)
		if err != nil {
			if attempt < maxRetries {
				time.Sleep(time.Duration(attempt+1) * time.Second)
				continue
			}
			return toolError(fmt.Sprintf("GitHub API request failed for query '%s': %s", originalQuery, err.Error()), true), nil
		}

		// Retry on 5xx errors
		if resp.StatusCode >= 500 && resp.StatusCode < 600 && attempt < maxRetries {
			resp.Body.Close()
			time.Sleep(time.Duration(attempt+1) * time.Second)
			continue
		}
		break
	}
	defer resp.Body.Close()

	// Check rate limit headers proactively for monitoring
	rateLimitRemaining := resp.Header.Get("X-RateLimit-Remaining")
	rateLimitReset := resp.Header.Get("X-RateLimit-Reset")

	// Log rate limit info for operational visibility
	log.Printf("[GitHub API] rate_limit_remaining=%s rate_limit_reset_unix=%s query=%q",
		rateLimitRemaining, rateLimitReset, originalQuery)

	// Handle rate limiting
	if resp.StatusCode == 403 || resp.StatusCode == 429 {
		if rateLimitRemaining == "0" {
			// Parse Unix timestamp and format for human readability
			resetUnix, parseErr := strconv.ParseInt(rateLimitReset, 10, 64)
			if parseErr != nil {
				return toolError(fmt.Sprintf("GitHub API rate limit exceeded. Reset time unparseable: %s", rateLimitReset), true), nil
			}
			resetTime := time.Unix(resetUnix, 0)
			minutesUntilReset := int(time.Until(resetTime).Minutes())
			return toolError(fmt.Sprintf("GitHub API rate limit exceeded (60/hr unauthenticated). Resets at %s (in ~%d minutes).",
				resetTime.Format(time.RFC3339), minutesUntilReset), true), nil
		}
		// Check for Retry-After header (secondary rate limit)
		if retryAfter := resp.Header.Get("Retry-After"); retryAfter != "" {
			return toolError(fmt.Sprintf("GitHub API secondary rate limit hit. Retry after %s seconds.", retryAfter), true), nil
		}
	}

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return toolError(fmt.Sprintf("GitHub API returned status %d for query '%s': %s", resp.StatusCode, originalQuery, string(bodyBytes)), true), nil
	}

	// Parse response
	var ghResp githubSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&ghResp); err != nil {
		return toolError(fmt.Sprintf("failed to parse GitHub response for query '%s': %s", originalQuery, err.Error()), true), nil
	}

	// Convert to our result format, filtering archived repos
	results := make([]GitHubRepoResult, 0, len(ghResp.Items))
	for _, item := range ghResp.Items {
		if item.Archived {
			continue
		}

		licenseName := "Unknown"
		if item.License != nil && item.License.Name != "" {
			licenseName = item.License.Name
		}

		desc := item.Description
		if desc == "" {
			desc = "(No description provided)"
		}

		lang := item.Language
		if lang == "" {
			lang = "Unknown"
		}

		results = append(results, GitHubRepoResult{
			Name:        item.Name,
			FullName:    item.FullName,
			Description: desc,
			Stars:       item.Stars,
			URL:         item.HTMLURL,
			Language:    lang,
			License:     licenseName,
			Topics:      item.Topics,
			UpdatedAt:   item.UpdatedAt,
			IsArchived:  false, // already filtered
		})
	}

	b, err := json.Marshal(results)
	if err != nil {
		// Marshaling should never fail for our controlled structs, but handle defensively
		log.Printf("[ERROR] Failed to marshal GitHub results for query %q: %v", originalQuery, err)
		return toolError(fmt.Sprintf("internal error: failed to serialize results for query '%s'", originalQuery), false), nil
	}
	return string(b), nil
}
