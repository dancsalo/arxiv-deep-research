package research

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// openAlexSortMappings maps tool sort values to OpenAlex API sort parameters
var openAlexSortMappings = map[string]string{
	"cited_by_count": "cited_by_count:desc",
	// Future: "publication_date": "publication_date:desc",
}

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
	Title        string   `json:"title"`
	Authors      []string `json:"authors"`
	DOI          string   `json:"doi"`
	Abstract     string   `json:"abstract"`
	Year         int      `json:"year"`
	CitedByCount *int     `json:"cited_by_count,omitempty"`
}

type openAlexResponse struct {
	Results []openAlexWork `json:"results"`
}

type openAlexWork struct {
	Title                 string               `json:"title"`
	DOI                   string               `json:"doi"`
	PublicationYear       int                  `json:"publication_year"`
	CitedByCount          *int                 `json:"cited_by_count"`
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
		Sort       string `json:"sort"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return toolError("invalid input: "+err.Error(), false), nil
	}
	if params.MaxResults <= 0 {
		params.MaxResults = 10
	}

	// Validate sort parameter (if provided)
	var sortParam string
	if params.Sort != "" {
		var valid bool
		sortParam, valid = openAlexSortMappings[params.Sort]
		if !valid {
			return toolError(fmt.Sprintf("invalid sort value '%s': must be 'cited_by_count'", params.Sort), true), nil
		}
	}

	u := "https://api.openalex.org/works?search=" + url.QueryEscape(params.Query) +
		"&per_page=" + strconv.Itoa(params.MaxResults) +
		"&mailto=arxiv-deep-research@users.noreply.github.com"
	if params.Filter != "" {
		u += "&filter=" + url.QueryEscape(params.Filter)
	}
	if sortParam != "" {
		u += "&sort=" + sortParam
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
			Title:        work.Title,
			Authors:      authors,
			DOI:          work.DOI,
			Abstract:     reconstructAbstract(work.AbstractInvertedIndex),
			Year:         work.PublicationYear,
			CitedByCount: work.CitedByCount,
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
	if err := validateArxivPdf(ctx, r.client, pdfURL); err != nil {
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
