package research

import (
	"bytes"
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

	readability "github.com/go-shiori/go-readability"
	pdf "github.com/ledongthuc/pdf"
	"golang.org/x/net/html"
)

// openAlexSortMappings maps tool sort values to OpenAlex API sort parameters.
// Currently only supports citation sorting. Add more sort options here when needed.
var openAlexSortMappings = map[string]string{
	"cited_by_count": "cited_by_count:desc",
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
		Query       string `json:"query"`
		MaxResults  int    `json:"max_results"`
		SearchField string `json:"search_field"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return toolError("invalid input: "+err.Error(), false), nil
	}
	if params.MaxResults <= 0 {
		params.MaxResults = 10
	}

	// Map search_field to arXiv API prefix
	searchPrefix := "ti" // Default to title
	switch params.SearchField {
	case "abstract":
		searchPrefix = "abs"
	case "title", "": // Empty string defaults to title
		searchPrefix = "ti"
	default:
		return toolError(fmt.Sprintf("invalid search_field '%s': must be 'title' or 'abstract'", params.SearchField), false), nil
	}

	// Build search query with AND logic for multi-word queries
	// This ensures all terms must match rather than any term (OR logic)
	queryTerms := strings.Fields(params.Query)
	var searchQuery string
	if len(queryTerms) > 1 {
		// Multi-word: join with AND
		parts := make([]string, len(queryTerms))
		for i, term := range queryTerms {
			parts[i] = fmt.Sprintf("%s:%s", searchPrefix, url.QueryEscape(term))
		}
		searchQuery = strings.Join(parts, "+AND+")
	} else {
		// Single word: simple format
		searchQuery = fmt.Sprintf("%s:%s", searchPrefix, url.QueryEscape(params.Query))
	}

	u := fmt.Sprintf("https://export.arxiv.org/api/query?search_query=%s&max_results=%d",
		searchQuery, params.MaxResults)

	// Rate limit: max 3 requests per second per arXiv TOS
	r.arxivRateLimiter.Wait()

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

// OpenAlexResult represents a single result from OpenAlex API.
// Optional fields use pointer types with omitempty to minimize token usage:
// - nil pointer → field omitted from JSON (no data available)
// - non-nil pointer → field included with value (data available, may be 0)
type OpenAlexResult struct {
	Title        string   `json:"title"`
	Authors      []string `json:"authors"`
	DOI          string   `json:"doi"`
	Abstract     string   `json:"abstract"`
	Year         int      `json:"year"`
	CitedByCount *int     `json:"cited_by_count,omitempty"` // nil if OpenAlex has no citation data
}

// CitationResult represents a lightweight result for citations and references.
// Excludes abstract to save tokens (~180 tokens per result vs ~360 for full results).
type CitationResult struct {
	ID           string   `json:"id"`                       // OpenAlex work ID (W...)
	Title        string   `json:"title"`
	Authors      []string `json:"authors"`                  // First 3 authors only
	Year         int      `json:"year"`
	CitedByCount *int     `json:"cited_by_count,omitempty"` // Pointer for null handling
	DOI          string   `json:"doi"`
}

type openAlexResponse struct {
	Results []openAlexWork `json:"results"`
}

type openAlexWork struct {
	ID                    string               `json:"id"` // Full OpenAlex URL (https://openalex.org/W123)
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
	// Empty string is treated as no sort parameter (backward compat)
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
	ArxivID           string `json:"arxiv_id"`
	TextContent       string `json:"text_content"`
	PageCount         int    `json:"page_count"`
	CharCount         int    `json:"char_count"`
	ExtractionQuality string `json:"extraction_quality"`
	Truncated         bool   `json:"truncated"`
	Version           string `json:"version,omitempty"`
	Error             string `json:"error,omitempty"`
}

func (r *ResearchToolSet) handleFetchArxivPdf(ctx context.Context, input json.RawMessage) (string, error) {
	// Parse and validate input
	var params struct {
		ArxivID   string `json:"arxiv_id"`
		MaxLength int    `json:"max_length"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return toolError("invalid input: "+err.Error(), false), nil
	}
	if params.ArxivID == "" {
		return toolError("arxiv_id is required", false), nil
	}
	if params.MaxLength == 0 {
		params.MaxLength = 8000 // Default
	}
	if params.MaxLength > 50000 {
		params.MaxLength = 50000 // Max limit
	}

	// Normalize and validate arXiv ID
	normalized, version, err := normalizeArxivID(params.ArxivID)
	if err != nil {
		return toolError("invalid arXiv ID format: "+err.Error(), false), nil
	}

	// Construct PDF URL
	pdfURL := fmt.Sprintf("https://export.arxiv.org/pdf/%s.pdf", normalized)

	// Rate limit: max 3 requests per second per arXiv TOS
	r.arxivRateLimiter.Wait()

	// Download PDF
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pdfURL, nil)
	if err != nil {
		result := ArxivPdfResult{
			ArxivID:           normalized,
			TextContent:       "",
			PageCount:         0,
			CharCount:         0,
			ExtractionQuality: "failed",
			Truncated:         false,
			Version:           version,
			Error:             fmt.Sprintf("failed to create request: %v", err),
		}
		b, _ := json.Marshal(result)
		return string(b), nil
	}

	resp, err := r.client.Do(req)
	if err != nil {
		result := ArxivPdfResult{
			ArxivID:           normalized,
			TextContent:       "",
			PageCount:         0,
			CharCount:         0,
			ExtractionQuality: "failed",
			Truncated:         false,
			Version:           version,
			Error:             fmt.Sprintf("PDF download failed: %v", err),
		}
		b, _ := json.Marshal(result)
		return string(b), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		recoverable := resp.StatusCode >= 500 // 5xx errors are recoverable
		result := ArxivPdfResult{
			ArxivID:           normalized,
			TextContent:       "",
			PageCount:         0,
			CharCount:         0,
			ExtractionQuality: "failed",
			Truncated:         false,
			Version:           version,
			Error:             fmt.Sprintf("PDF download failed: HTTP %d", resp.StatusCode),
		}
		b, _ := json.Marshal(result)
		if recoverable {
			return toolError(result.Error, true), nil
		}
		return string(b), nil
	}

	// Read PDF bytes
	pdfBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		result := ArxivPdfResult{
			ArxivID:           normalized,
			TextContent:       "",
			PageCount:         0,
			CharCount:         0,
			ExtractionQuality: "failed",
			Truncated:         false,
			Version:           version,
			Error:             fmt.Sprintf("failed to read PDF: %v", err),
		}
		b, _ := json.Marshal(result)
		return string(b), nil
	}

	// Extract text
	textContent, pageCount, extractErr := extractPdfText(pdfBytes, params.MaxLength)

	// Calculate metrics
	charCount := len(textContent)
	truncated := false
	if extractErr == nil && charCount >= params.MaxLength {
		truncated = true
	}

	// Assess quality
	errorMsg := ""
	if extractErr != nil {
		errorMsg = extractErr.Error()
	}
	quality := assessExtractionQuality(charCount, pageCount, errorMsg)

	// Return result
	result := ArxivPdfResult{
		ArxivID:           normalized,
		TextContent:       textContent,
		PageCount:         pageCount,
		CharCount:         charCount,
		ExtractionQuality: quality,
		Truncated:         truncated,
		Version:           version,
		Error:             errorMsg,
	}
	b, _ := json.Marshal(result)
	return string(b), nil
}

// extractPdfText extracts text from PDF bytes with panic recovery.
// Returns text (truncated to maxLength), page count, and error.
// maxLength must be > 0.
func extractPdfText(pdfBytes []byte, maxLength int) (text string, pageCount int, err error) {
	// Panic recovery for PDF library issues
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("PDF extraction panic: %v", r)
			text = ""
			pageCount = 0
		}
	}()

	// Create PDF reader
	reader, err := pdf.NewReader(bytes.NewReader(pdfBytes), int64(len(pdfBytes)))
	if err != nil {
		return "", 0, fmt.Errorf("failed to parse PDF: %w", err)
	}

	pageCount = reader.NumPage()
	if pageCount == 0 {
		return "", 0, fmt.Errorf("PDF has zero pages")
	}

	// Extract text from all pages using row-based extraction for better spacing
	var textBuilder strings.Builder
	for i := 1; i <= pageCount; i++ {
		page := reader.Page(i)
		if page.V.IsNull() {
			continue
		}

		// Use GetTextByRow for better word spacing
		rows, err := page.GetTextByRow()
		if err != nil {
			// Fall back to GetPlainText if row extraction fails
			fonts := make(map[string]*pdf.Font)
			pageText, err2 := page.GetPlainText(fonts)
			if err2 != nil {
				// Log but continue - partial extraction is acceptable
				continue
			}
			// Add spaces between words for GetPlainText fallback
			pageText = addSpacesBetweenWords(pageText)

			// Check if we're approaching the limit
			if textBuilder.Len()+len(pageText) > maxLength {
				remaining := maxLength - textBuilder.Len()
				if remaining > 0 {
					textBuilder.WriteString(pageText[:remaining])
				}
				break
			}
			textBuilder.WriteString(pageText)
		} else {
			// Process rows to extract text with proper spacing
			for _, row := range rows {
				for _, word := range row.Content {
					wordText := word.S
					if wordText == "" {
						continue
					}

					// Check if we're approaching the limit
					if textBuilder.Len()+len(wordText)+1 > maxLength {
						remaining := maxLength - textBuilder.Len()
						if remaining > 0 {
							if remaining <= len(wordText) {
								textBuilder.WriteString(wordText[:remaining])
							} else {
								textBuilder.WriteString(wordText)
							}
						}
						return textBuilder.String(), pageCount, nil
					}

					// Add word with space
					if textBuilder.Len() > 0 {
						textBuilder.WriteString(" ")
					}
					textBuilder.WriteString(wordText)
				}
				// Add newline after each row
				if textBuilder.Len() > 0 && textBuilder.String()[textBuilder.Len()-1] != '\n' {
					textBuilder.WriteString("\n")
				}
			}
		}

		// Add page separator
		if i < pageCount {
			textBuilder.WriteString("\n")
		}
	}

	return textBuilder.String(), pageCount, nil
}

// addSpacesBetweenWords attempts to add spaces between concatenated words
// by detecting case transitions and common patterns
func addSpacesBetweenWords(text string) string {
	if text == "" {
		return text
	}

	var result strings.Builder
	result.Grow(len(text) * 2) // Pre-allocate more space for added spaces

	runes := []rune(text)
	for i := 0; i < len(runes); i++ {
		result.WriteRune(runes[i])

		// Add space between lowercase and uppercase (camelCase detection)
		if i < len(runes)-1 {
			current := runes[i]
			next := runes[i+1]

			// Add space if: lowercase followed by uppercase
			if (current >= 'a' && current <= 'z') && (next >= 'A' && next <= 'Z') {
				result.WriteRune(' ')
			}
		}
	}

	return result.String()
}

// assessExtractionQuality returns "good", "poor", or "failed" based on
// extraction results. charCount is before truncation, errorMsg is empty on success.
func assessExtractionQuality(charCount, pageCount int, errorMsg string) string {
	if errorMsg != "" || pageCount == 0 {
		return "failed"
	}

	// Calculate text density (chars per page)
	density := 0
	if pageCount > 0 {
		density = charCount / pageCount
	}

	// Good: high density and sufficient text
	if density > 100 && charCount > 1000 {
		return "good"
	}

	return "poor"
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
	// Cap at 5 results to limit token usage and response size per product decision
	// (see plan: phase 1 scoped to 5 results for MVP validation)
	if params.MaxResults > 5 {
		params.MaxResults = 5
	}

	// Add quality filters to query: min 100 stars, updated in last 2 years
	// Calculate 2 years ago dynamically to maintain recency invariant
	twoYearsAgo := time.Now().AddDate(-2, 0, 0).Format("2006-01-02")
	enhancedQuery := fmt.Sprintf("%s stars:>100 pushed:>%s", params.Query, twoYearsAgo)

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
		// Check for Retry-After header first (secondary rate limit or explicit retry)
		if retryAfter := resp.Header.Get("Retry-After"); retryAfter != "" {
			// Retry-After can be seconds (integer) or HTTP date
			if seconds, err := strconv.Atoi(retryAfter); err == nil {
				retryTime := time.Now().Add(time.Duration(seconds) * time.Second)
				return toolError(fmt.Sprintf("GitHub API rate limit hit. Retry after %d seconds (at %s).",
					seconds, retryTime.Format(time.RFC3339)), true), nil
			}
			// Fall back to displaying raw value if not an integer
			return toolError(fmt.Sprintf("GitHub API rate limit hit. Retry after: %s", retryAfter), true), nil
		}

		// Check primary rate limit
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

		// Rate limit response but no clear reason
		return toolError(fmt.Sprintf("GitHub API rate limit response (status %d) but limit headers unclear", resp.StatusCode), true), nil
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

		// Handle nil Topics slice
		topics := item.Topics
		if topics == nil {
			topics = []string{}
		}

		results = append(results, GitHubRepoResult{
			Name:        item.Name,
			FullName:    item.FullName,
			Description: desc,
			Stars:       item.Stars,
			URL:         item.HTMLURL,
			Language:    lang,
			License:     licenseName,
			Topics:      topics,
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

type WebSearchResult struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

// cleanSearchURL removes DuckDuckGo tracking parameters from search result URLs
func cleanSearchURL(rawURL string) string {
	// DuckDuckGo wraps result URLs in a redirect: /l/?uddg=<encoded>&rut=<encoded>
	// Extract the 'uddg' parameter which contains the actual URL
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}

	if strings.HasPrefix(u.Path, "/l/") {
		if uddg := u.Query().Get("uddg"); uddg != "" {
			decoded, err := url.QueryUnescape(uddg)
			if err == nil {
				return decoded
			}
		}
	}

	return rawURL
}

// parseDuckDuckGoHTML extracts search results from DuckDuckGo HTML response.
// Returns error if parsing fails or structure doesn't match expected format.
func parseDuckDuckGoHTML(htmlBody string) ([]WebSearchResult, error) {
	doc, err := html.Parse(strings.NewReader(htmlBody))
	if err != nil {
		return nil, fmt.Errorf("html parse failed: %w", err)
	}

	var results []WebSearchResult
	var visit func(*html.Node)
	visit = func(n *html.Node) {
		// DuckDuckGo result divs have class="result results_links results_links_deep web-result"
		if n.Type == html.ElementNode && n.Data == "div" {
			if hasClass(n, "result") && hasClass(n, "results_links") {
				if result := extractResult(n); result != nil {
					results = append(results, *result)
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
		}
	}
	visit(doc)

	if len(results) == 0 {
		return nil, fmt.Errorf("no results found in HTML (possible CAPTCHA or format change)")
	}

	return results, nil
}

func hasClass(n *html.Node, class string) bool {
	for _, attr := range n.Attr {
		if attr.Key == "class" && strings.Contains(attr.Val, class) {
			return true
		}
	}
	return false
}

// extractResult extracts title and URL from a single result div
func extractResult(div *html.Node) *WebSearchResult {
	var title, rawURL string

	var visit func(*html.Node)
	visit = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "a":
				// Title link has class="result__a"
				if hasClass(n, "result__a") {
					title = getTextContent(n)
					for _, attr := range n.Attr {
						if attr.Key == "href" {
							rawURL = attr.Val
							break
						}
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
		}
	}
	visit(div)

	if title == "" || rawURL == "" {
		return nil
	}

	return &WebSearchResult{
		Title: strings.TrimSpace(title),
		URL:   cleanSearchURL(rawURL),
	}
}

func getTextContent(n *html.Node) string {
	var buf strings.Builder
	var visit func(*html.Node)
	visit = func(node *html.Node) {
		if node.Type == html.TextNode {
			buf.WriteString(node.Data)
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
		}
	}
	visit(n)
	return buf.String()
}

// handleGetCitationsAndReferences fetches citations or references for a given OpenAlex work.
func (r *ResearchToolSet) handleGetCitationsAndReferences(ctx context.Context, input json.RawMessage) (string, error) {
	var params struct {
		WorkID     string `json:"work_id"`
		Direction  string `json:"direction"`
		MaxResults int    `json:"max_results"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return toolError("invalid input: "+err.Error(), false), nil
	}

	// Validate work_id format
	if !regexp.MustCompile(`^W\d+$`).MatchString(params.WorkID) {
		return toolError("invalid work_id format: must be OpenAlex ID like W2741809807", false), nil
	}

	// Validate direction
	if params.Direction != "references" && params.Direction != "cited_by" {
		return toolError("invalid direction: must be 'references' or 'cited_by'", false), nil
	}

	// Apply max_results constraints
	if params.MaxResults <= 0 {
		params.MaxResults = 10
	}
	if params.MaxResults > 50 {
		params.MaxResults = 50
	}

	// Route to appropriate handler
	if params.Direction == "references" {
		return r.fetchReferences(ctx, params.WorkID, params.MaxResults)
	}
	return r.fetchCitingPapers(ctx, params.WorkID, params.MaxResults)
}

// fetchReferences fetches papers cited BY the given work (bibliography).
// Uses 2 API calls: fetch work to get referenced_works, then batch fetch metadata.
func (r *ResearchToolSet) fetchReferences(ctx context.Context, workID string, maxResults int) (string, error) {
	// Fetch work to get referenced_works array
	workURL := fmt.Sprintf("https://api.openalex.org/works/%s?mailto=arxiv-deep-research@users.noreply.github.com", workID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, workURL, nil)
	if err != nil {
		return toolError("request creation failed: "+err.Error(), false), nil
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return toolError("OpenAlex request failed: "+err.Error(), true), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return toolError(fmt.Sprintf("work not found: %s", workID), false), nil
	}
	if resp.StatusCode != http.StatusOK {
		return toolError(fmt.Sprintf("OpenAlex returned status %d", resp.StatusCode), true), nil
	}

	var work struct {
		ReferencedWorks []string `json:"referenced_works"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&work); err != nil {
		return toolError("failed to parse work response: "+err.Error(), true), nil
	}

	if len(work.ReferencedWorks) == 0 {
		return "[]", nil
	}

	// Limit to max_results
	refIDs := work.ReferencedWorks
	if len(refIDs) > maxResults {
		refIDs = refIDs[:maxResults]
	}

	// Batch fetch metadata
	filterValue := strings.Join(refIDs, "|")
	batchURL := fmt.Sprintf("https://api.openalex.org/works?filter=openalex:%s&per_page=%d&mailto=arxiv-deep-research@users.noreply.github.com",
		url.QueryEscape(filterValue), len(refIDs))

	return r.fetchAndFormatCitations(ctx, batchURL)
}

// fetchCitingPapers fetches papers that cite the given work (forward citations).
// Uses 1 API call with OpenAlex filter.
func (r *ResearchToolSet) fetchCitingPapers(ctx context.Context, workID string, maxResults int) (string, error) {
	u := fmt.Sprintf("https://api.openalex.org/works?filter=cites:%s&per_page=%d&mailto=arxiv-deep-research@users.noreply.github.com",
		workID, maxResults)
	return r.fetchAndFormatCitations(ctx, u)
}

// fetchAndFormatCitations is a shared helper that fetches works from OpenAlex and formats them as CitationResult.
func (r *ResearchToolSet) fetchAndFormatCitations(ctx context.Context, apiURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
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
		return toolError("failed to parse response: "+err.Error(), true), nil
	}

	results := make([]CitationResult, 0, len(oaResp.Results))
	for _, work := range oaResp.Results {
		// Extract first 3 authors only
		authors := make([]string, 0, min(3, len(work.Authorships)))
		for i := 0; i < min(3, len(work.Authorships)); i++ {
			authors = append(authors, work.Authorships[i].Author.DisplayName)
		}

		// Extract work ID from full URL (https://openalex.org/W123 → W123)
		workID := work.ID
		if idx := strings.LastIndex(work.ID, "/"); idx != -1 {
			workID = work.ID[idx+1:]
		}

		results = append(results, CitationResult{
			ID:           workID,
			Title:        work.Title,
			Authors:      authors,
			Year:         work.PublicationYear,
			CitedByCount: work.CitedByCount,
			DOI:          work.DOI,
		})
	}

	b, _ := json.Marshal(results)
	return string(b), nil
}


// WebpageContent represents the extracted content from a webpage.
type WebpageContent struct {
	URL         string `json:"url"`
	Title       string `json:"title"`
	Author      string `json:"author,omitempty"`
	Length      int    `json:"length"`
	Excerpt     string `json:"excerpt"`
	TextContent string `json:"text_content"`
	Truncated   bool   `json:"truncated"`
}

// handleFetchWebpageContent fetches a webpage and extracts its main content using readability.
func (r *ResearchToolSet) handleFetchWebpageContent(ctx context.Context, input json.RawMessage) (string, error) {
	var params struct {
		URL       string `json:"url"`
		MaxLength int    `json:"max_length"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return toolError("invalid input: "+err.Error(), false), nil
	}

	// Validate URL
	if params.URL == "" {
		return toolError("url is required", false), nil
	}
	parsedURL, err := url.Parse(params.URL)
	if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		return toolError("invalid URL: must be http or https", false), nil
	}

	// Apply max_length constraints
	if params.MaxLength <= 0 {
		params.MaxLength = 8000
	}
	if params.MaxLength > 15000 {
		params.MaxLength = 15000
	}

	// Rate limiting: enforce 2-second delay between requests
	time.Sleep(2 * time.Second)

	// Fetch webpage with timeout
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, params.URL, nil)
	if err != nil {
		return toolError("request creation failed: "+err.Error(), false), nil
	}

	// Set User-Agent to identify ourselves politely
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; ArxivDeepResearch/1.0; +https://github.com/dancsalo/arxiv-deep-research)")

	resp, err := r.client.Do(req)
	if err != nil {
		return toolError("request failed: "+err.Error(), true), nil
	}
	defer resp.Body.Close()

	// Handle common error status codes
	switch resp.StatusCode {
	case http.StatusOK:
		// Continue
	case http.StatusForbidden, http.StatusUnauthorized:
		return toolError(fmt.Sprintf("access denied (status %d): site may block scrapers or require authentication", resp.StatusCode), false), nil
	case http.StatusNotFound:
		return toolError("page not found (404)", false), nil
	case http.StatusTooManyRequests:
		return toolError("rate limited by target site (429)", true), nil
	default:
		return toolError(fmt.Sprintf("request returned status %d", resp.StatusCode), true), nil
	}

	// Check content type (only process HTML)
	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(strings.ToLower(contentType), "text/html") {
		return toolError(fmt.Sprintf("unsupported content type: %s (expected text/html)", contentType), false), nil
	}

	// Parse with readability
	article, err := readability.FromReader(resp.Body, parsedURL)
	if err != nil {
		return toolError("failed to extract content: "+err.Error(), true), nil
	}

	// Prepare result
	textContent := article.TextContent
	truncated := false
	if len(textContent) > params.MaxLength {
		textContent = textContent[:params.MaxLength]
		truncated = true
	}

	result := WebpageContent{
		URL:         params.URL,
		Title:       article.Title,
		Author:      article.Byline,
		Length:      len(article.TextContent),
		Excerpt:     article.Excerpt,
		TextContent: textContent,
		Truncated:   truncated,
	}

	b, _ := json.Marshal(result)
	return string(b), nil
}
