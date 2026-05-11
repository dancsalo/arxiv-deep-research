package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// formatOutput formats tool results for display
func formatOutput(w io.Writer, toolName string, result string, asJSON bool) error {
	if asJSON {
		// Just write the raw JSON result
		fmt.Fprintln(w, result)
		return nil
	}

	// Parse result to determine format
	var resultData map[string]interface{}
	if err := json.Unmarshal([]byte(result), &resultData); err != nil {
		return fmt.Errorf("failed to parse result: %w", err)
	}

	// Check for errors
	if errMsg, ok := resultData["error"].(string); ok {
		fmt.Fprintf(w, "Error: %s\n", errMsg)
		if recoverable, ok := resultData["recoverable"].(bool); ok && recoverable {
			fmt.Fprintln(w, "(This error is recoverable - you can retry)")
		}
		return nil
	}

	// Format based on tool type
	switch toolName {
	case "search-arxiv", "search-openalex":
		return formatSearchResults(w, resultData)
	case "fetch-pdf":
		return formatPdfResult(w, resultData)
	case "search-github":
		return formatGithubResults(w, resultData)
	default:
		return fmt.Errorf("unknown tool: %s", toolName)
	}
}

func formatSearchResults(w io.Writer, data map[string]interface{}) error {
	query, _ := data["query"].(string)
	results, ok := data["results"].([]interface{})
	if !ok {
		return fmt.Errorf("invalid results format")
	}

	fmt.Fprintf(w, "=== Search Results for %q ===\n\n", query)

	for i, item := range results {
		paper, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		fmt.Fprintf(w, "%d. %s\n", i+1, paper["title"])

		if authors, ok := paper["authors"].([]interface{}); ok && len(authors) > 0 {
			authorStrs := make([]string, 0, len(authors))
			for _, a := range authors {
				if author, ok := a.(string); ok {
					authorStrs = append(authorStrs, author)
				}
			}
			if len(authorStrs) > 0 {
				fmt.Fprintf(w, "   Authors: %s\n", strings.Join(authorStrs, ", "))
			}
		}

		if published, ok := paper["published"].(string); ok {
			fmt.Fprintf(w, "   Published: %s\n", published)
		}

		if abstract, ok := paper["abstract"].(string); ok {
			// Truncate long abstracts
			if len(abstract) > 200 {
				abstract = abstract[:197] + "..."
			}
			fmt.Fprintf(w, "   Abstract: %s\n", abstract)
		}

		if url, ok := paper["url"].(string); ok {
			fmt.Fprintf(w, "   URL: %s\n", url)
		}

		fmt.Fprintln(w)
	}

	fmt.Fprintf(w, "Found %d results (showing %d)\n", len(results), len(results))
	return nil
}

func formatPdfResult(w io.Writer, data map[string]interface{}) error {
	arxivID, _ := data["arxiv_id"].(string)
	pdfURL, _ := data["pdf_url"].(string)

	fmt.Fprintf(w, "=== arXiv PDF URL ===\n\n")
	fmt.Fprintf(w, "arXiv ID: %s\n", arxivID)
	fmt.Fprintf(w, "PDF URL:  %s\n", pdfURL)
	return nil
}

func formatGithubResults(w io.Writer, data map[string]interface{}) error {
	query, _ := data["query"].(string)
	results, ok := data["results"].([]interface{})
	if !ok {
		return fmt.Errorf("invalid results format")
	}

	fmt.Fprintf(w, "=== GitHub Repository Search for %q ===\n\n", query)

	for i, item := range results {
		repo, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		name, _ := repo["name"].(string)
		fmt.Fprintf(w, "%d. %s\n", i+1, name)

		if desc, ok := repo["description"].(string); ok && desc != "" {
			fmt.Fprintf(w, "   Description: %s\n", desc)
		}

		if stars, ok := repo["stars"].(float64); ok {
			fmt.Fprintf(w, "   Stars: %.0f\n", stars)
		}

		if lang, ok := repo["language"].(string); ok && lang != "" {
			fmt.Fprintf(w, "   Language: %s\n", lang)
		}

		if license, ok := repo["license"].(string); ok && license != "" {
			fmt.Fprintf(w, "   License: %s\n", license)
		}

		if url, ok := repo["url"].(string); ok {
			fmt.Fprintf(w, "   URL: %s\n", url)
		}

		fmt.Fprintln(w)
	}

	fmt.Fprintf(w, "Found %d repositories\n", len(results))
	return nil
}
