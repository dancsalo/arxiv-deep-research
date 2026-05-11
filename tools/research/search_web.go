package research

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxSearchWebFailures = 3 // Circuit breaker threshold

func (r *ResearchToolSet) handleSearchWeb(ctx context.Context, input json.RawMessage) (string, error) {
	// Circuit breaker: if we've seen too many consecutive parse failures, return error
	failures := r.searchWebFailures.Load()
	if failures >= maxSearchWebFailures {
		return toolError("web search temporarily disabled due to repeated failures (likely bot detection)", true), nil
	}

	var params struct {
		Query      string `json:"query"`
		MaxResults int    `json:"max_results"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return toolError("invalid input: "+err.Error(), false), nil
	}
	if params.MaxResults <= 0 || params.MaxResults > 10 {
		params.MaxResults = 10
	}

	// Build DuckDuckGo HTML search URL
	searchURL := r.searchWebBaseURL + "?q=" + url.QueryEscape(params.Query)

	// Add explicit timeout (DuckDuckGo may be slow or hanging)
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
	if err != nil {
		return toolError("request creation failed: "+err.Error(), false), nil
	}

	// Set User-Agent to mimic a browser (may not help with CAPTCHA)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")

	resp, err := r.client.Do(req)
	if err != nil {
		return toolError("web search request failed: "+err.Error(), true), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// HTTP errors (rate limiting, server errors) count toward circuit breaker
		r.searchWebFailures.Add(1)
		failures := r.searchWebFailures.Load()
		return toolError(fmt.Sprintf("web search returned status %d (failure %d/%d)", resp.StatusCode, failures, maxSearchWebFailures), true), nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return toolError("failed to read response: "+err.Error(), true), nil
	}

	htmlBody := string(body)

	// Detect CAPTCHA (DuckDuckGo returns "Please complete the following challenge")
	if strings.Contains(htmlBody, "Please complete the following challenge") {
		failures := r.searchWebFailures.Add(1)
		return toolError(fmt.Sprintf("CAPTCHA detected (failure %d/%d)", failures, maxSearchWebFailures), true), nil
	}

	// Parse HTML
	results, err := parseDuckDuckGoHTML(htmlBody)
	if err != nil {
		failures := r.searchWebFailures.Add(1)
		return toolError(fmt.Sprintf("web search parse failed: %v (failure %d/%d)", err, failures, maxSearchWebFailures), true), nil
	}

	// Success: reset failure counter
	r.searchWebFailures.Store(0)

	// Limit results to max_results
	if len(results) > params.MaxResults {
		results = results[:params.MaxResults]
	}

	b, _ := json.Marshal(results)
	return string(b), nil
}
