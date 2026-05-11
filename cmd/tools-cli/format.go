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

	// Try to parse as error first
	var errData map[string]interface{}
	if err := json.Unmarshal([]byte(result), &errData); err == nil {
		if errMsg, ok := errData["error"].(string); ok {
			fmt.Fprintf(w, "Error: %s\n", errMsg)
			if recoverable, ok := errData["recoverable"].(bool); ok && recoverable {
				fmt.Fprintln(w, "(This error is recoverable - you can retry)")
			}
			return nil
		}
	}

	// Format based on tool type
	switch toolName {
	case "search-arxiv", "search-openalex", "search-web":
		return formatSearchResults(w, result)
	case "fetch-pdf":
		return formatPdfResult(w, result)
	case "search-github":
		return formatGithubResults(w, result)
	case "get-citations":
		return formatCitationResults(w, result)
	case "fetch-webpage":
		return formatWebpageResult(w, result)
	default:
		return fmt.Errorf("unknown tool: %s", toolName)
	}
}

func formatSearchResults(w io.Writer, result string) error {
	// Parse as array of results
	var results []map[string]interface{}
	if err := json.Unmarshal([]byte(result), &results); err != nil {
		return fmt.Errorf("invalid results format: %w", err)
	}

	fmt.Fprintf(w, "=== Search Results ===\n\n")

	for i, paper := range results {
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

func formatPdfResult(w io.Writer, result string) error {
	// Parse PDF result
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(result), &data); err != nil {
		return fmt.Errorf("invalid PDF result format: %w", err)
	}

	arxivID, _ := data["arxiv_id"].(string)
	pdfURL, _ := data["pdf_url"].(string)

	fmt.Fprintf(w, "=== arXiv PDF URL ===\n\n")
	fmt.Fprintf(w, "arXiv ID: %s\n", arxivID)
	fmt.Fprintf(w, "PDF URL:  %s\n", pdfURL)
	return nil
}

func formatGithubResults(w io.Writer, result string) error {
	// Parse as array of results
	var results []map[string]interface{}
	if err := json.Unmarshal([]byte(result), &results); err != nil {
		return fmt.Errorf("invalid results format: %w", err)
	}

	fmt.Fprintf(w, "=== GitHub Repository Search ===\n\n")

	for i, repo := range results {
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

func formatCitationResults(w io.Writer, result string) error {
	// Parse as array of citation results
	var results []map[string]interface{}
	if err := json.Unmarshal([]byte(result), &results); err != nil {
		return fmt.Errorf("invalid results format: %w", err)
	}

	fmt.Fprintf(w, "=== Citation Results ===\n\n")

	for i, paper := range results {
		fmt.Fprintf(w, "%d. ", i+1)

		if title, ok := paper["title"].(string); ok {
			fmt.Fprintf(w, "%s\n", title)
		}

		if id, ok := paper["id"].(string); ok {
			fmt.Fprintf(w, "   ID: %s\n", id)
		}

		if authors, ok := paper["authors"].([]interface{}); ok && len(authors) > 0 {
			authorNames := make([]string, 0, len(authors))
			for _, a := range authors {
				if name, ok := a.(string); ok {
					authorNames = append(authorNames, name)
				}
			}
			if len(authorNames) > 0 {
				fmt.Fprintf(w, "   Authors: %s\n", strings.Join(authorNames, ", "))
			}
		}

		if year, ok := paper["year"].(float64); ok {
			fmt.Fprintf(w, "   Year: %.0f\n", year)
		}

		if citedByCount, ok := paper["cited_by_count"].(float64); ok {
			fmt.Fprintf(w, "   Citations: %.0f\n", citedByCount)
		}

		if doi, ok := paper["doi"].(string); ok && doi != "" {
			fmt.Fprintf(w, "   DOI: %s\n", doi)
		}

		fmt.Fprintln(w)
	}

	fmt.Fprintf(w, "Found %d papers\n", len(results))
	return nil
}

func formatWebpageResult(w io.Writer, result string) error {
	// Parse webpage content result
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(result), &data); err != nil {
		return fmt.Errorf("invalid webpage result format: %w", err)
	}

	fmt.Fprintf(w, "=== Webpage Content ===\n\n")

	if title, ok := data["title"].(string); ok && title != "" {
		fmt.Fprintf(w, "Title: %s\n", title)
	}

	if author, ok := data["author"].(string); ok && author != "" {
		fmt.Fprintf(w, "Author: %s\n", author)
	}

	if url, ok := data["url"].(string); ok {
		fmt.Fprintf(w, "URL: %s\n", url)
	}

	if length, ok := data["length"].(float64); ok {
		fmt.Fprintf(w, "Length: %.0f characters\n", length)
	}

	if truncated, ok := data["truncated"].(bool); ok && truncated {
		fmt.Fprintf(w, "Status: TRUNCATED (content exceeded max_length)\n")
	}

	if excerpt, ok := data["excerpt"].(string); ok && excerpt != "" {
		fmt.Fprintf(w, "\nExcerpt:\n%s\n", excerpt)
	}

	if textContent, ok := data["text_content"].(string); ok && textContent != "" {
		fmt.Fprintf(w, "\n--- Content ---\n%s\n", textContent)
	}

	return nil
}
