package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/dancsalo/arxiv-deep-research/tools/research"
)

func runInteractive(ctx context.Context, toolset *research.ResearchToolSet) error {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("=== Research Tools CLI ===")
	fmt.Println()

	for {
		fmt.Println("Select a tool:")
		fmt.Println("  [1] fetch-arxiv-text    - Extract text from arXiv HTML")
		fmt.Println("  [2] fetch-webpage       - Fetch webpage content")
		fmt.Println("  [3] get-citations       - Get citations and references")
		fmt.Println("  [4] search-arxiv        - Search arXiv for preprints")
		fmt.Println("  [5] search-github       - Find GitHub repositories")
		fmt.Println("  [6] search-openalex     - Search academic literature")
		fmt.Println("  [7] search-web          - Search the general web")
		fmt.Println("  [0] Exit")
		fmt.Println()

		fmt.Print("Enter selection: ")
		selection, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read input: %w", err)
		}
		selection = strings.TrimSpace(selection)

		switch selection {
		case "0":
			fmt.Println("Goodbye!")
			return nil
		case "1":
			if err := interactiveFetchArxivText(ctx, toolset, reader); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n\n", err)
			}
		case "2":
			if err := interactiveFetchWebpage(ctx, toolset, reader); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n\n", err)
			}
		case "3":
			if err := interactiveGetCitations(ctx, toolset, reader); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n\n", err)
			}
		case "4":
			if err := interactiveSearchArxiv(ctx, toolset, reader); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n\n", err)
			}
		case "5":
			if err := interactiveSearchGithub(ctx, toolset, reader); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n\n", err)
			}
		case "6":
			if err := interactiveSearchOpenAlex(ctx, toolset, reader); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n\n", err)
			}
		case "7":
			if err := interactiveSearchWeb(ctx, toolset, reader); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n\n", err)
			}
		default:
			fmt.Println("Invalid selection. Please try again.")
			fmt.Println()
			continue
		}

		fmt.Print("Run another command? [y/n]: ")
		again, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read input: %w", err)
		}
		again = strings.ToLower(strings.TrimSpace(again))
		if again != "y" && again != "yes" {
			fmt.Println("Goodbye!")
			return nil
		}
		fmt.Println()
	}
}

func interactiveSearchArxiv(ctx context.Context, toolset *research.ResearchToolSet, reader *bufio.Reader) error {
	fmt.Println("\n--- search-arxiv ---")

	fmt.Print("Query: ")
	query, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return fmt.Errorf("query cannot be empty")
	}

	fmt.Print("Max results [10]: ")
	maxResultsStr, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	maxResultsStr = strings.TrimSpace(maxResultsStr)

	maxResults := 10
	if maxResultsStr != "" {
		maxResults, err = strconv.Atoi(maxResultsStr)
		if err != nil {
			return fmt.Errorf("invalid max results: %w", err)
		}
	}

	fmt.Println("\nSearching arXiv...")

	input := map[string]interface{}{
		"query":       query,
		"max_results": maxResults,
	}
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return err
	}

	result, err := callToolHandler(ctx, toolset, "search_arxiv", inputJSON)
	if err != nil {
		return err
	}

	fmt.Println("\n=== Results ===")
	return formatOutput(os.Stdout, "search-arxiv", result, false)
}

func interactiveSearchOpenAlex(ctx context.Context, toolset *research.ResearchToolSet, reader *bufio.Reader) error {
	fmt.Println("\n--- search-openalex ---")

	fmt.Print("Query: ")
	query, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return fmt.Errorf("query cannot be empty")
	}

	fmt.Print("Max results [10]: ")
	maxResultsStr, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	maxResultsStr = strings.TrimSpace(maxResultsStr)

	maxResults := 10
	if maxResultsStr != "" {
		maxResults, err = strconv.Atoi(maxResultsStr)
		if err != nil {
			return fmt.Errorf("invalid max results: %w", err)
		}
	}

	fmt.Print("Filter (optional, e.g., publication_year:>2023): ")
	filter, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	filter = strings.TrimSpace(filter)

	fmt.Println("\nSearching OpenAlex...")

	input := map[string]interface{}{
		"query":       query,
		"max_results": maxResults,
	}
	if filter != "" {
		input["filter"] = filter
	}

	inputJSON, err := json.Marshal(input)
	if err != nil {
		return err
	}

	result, err := callToolHandler(ctx, toolset, "search_openalex", inputJSON)
	if err != nil {
		return err
	}

	fmt.Println("\n=== Results ===")
	return formatOutput(os.Stdout, "search-openalex", result, false)
}

func interactiveFetchArxivText(ctx context.Context, toolset *research.ResearchToolSet, reader *bufio.Reader) error {
	fmt.Println("\n--- fetch-arxiv-text ---")

	fmt.Print("arXiv ID (e.g., 1706.03762): ")
	arxivID, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	arxivID = strings.TrimSpace(arxivID)
	if arxivID == "" {
		return fmt.Errorf("arXiv ID cannot be empty")
	}

	fmt.Print("Max length [25000]: ")
	maxLengthStr, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	maxLengthStr = strings.TrimSpace(maxLengthStr)

	maxLength := 25000
	if maxLengthStr != "" {
		maxLength, err = strconv.Atoi(maxLengthStr)
		if err != nil {
			return fmt.Errorf("invalid max length: %w", err)
		}
	}

	fmt.Println("\nFetching arXiv HTML text content...")

	input := map[string]interface{}{
		"arxiv_id":   arxivID,
		"max_length": maxLength,
	}
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return err
	}

	result, err := callToolHandler(ctx, toolset, "fetch_arxiv_text", inputJSON)
	if err != nil {
		return err
	}

	fmt.Println("\n=== Result ===")
	return formatOutput(os.Stdout, "fetch-arxiv-text", result, false)
}

func interactiveSearchGithub(ctx context.Context, toolset *research.ResearchToolSet, reader *bufio.Reader) error {
	fmt.Println("\n--- search-github ---")

	fmt.Print("Query: ")
	query, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return fmt.Errorf("query cannot be empty")
	}

	fmt.Print("Max results [5]: ")
	maxResultsStr, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	maxResultsStr = strings.TrimSpace(maxResultsStr)

	maxResults := 5
	if maxResultsStr != "" {
		maxResults, err = strconv.Atoi(maxResultsStr)
		if err != nil {
			return fmt.Errorf("invalid max results: %w", err)
		}
	}

	fmt.Println("\nSearching GitHub...")

	input := map[string]interface{}{
		"query":       query,
		"max_results": maxResults,
	}
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return err
	}

	result, err := callToolHandler(ctx, toolset, "search_github_repos", inputJSON)
	if err != nil {
		return err
	}

	fmt.Println("\n=== Results ===")
	return formatOutput(os.Stdout, "search-github", result, false)
}

func interactiveSearchWeb(ctx context.Context, toolset *research.ResearchToolSet, reader *bufio.Reader) error {
	fmt.Println("\n--- search-web ---")

	fmt.Print("Query: ")
	query, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return fmt.Errorf("query cannot be empty")
	}

	fmt.Print("Max results [10]: ")
	maxResultsStr, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	maxResultsStr = strings.TrimSpace(maxResultsStr)

	maxResults := 10
	if maxResultsStr != "" {
		maxResults, err = strconv.Atoi(maxResultsStr)
		if err != nil {
			return fmt.Errorf("invalid max results: %w", err)
		}
	}

	fmt.Println("\nSearching web...")

	input := map[string]interface{}{
		"query":       query,
		"max_results": maxResults,
	}
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return err
	}

	result, err := callToolHandler(ctx, toolset, "search_web", inputJSON)
	if err != nil {
		return err
	}

	fmt.Println("\n=== Results ===")
	return formatOutput(os.Stdout, "search-web", result, false)
}

func interactiveGetCitations(ctx context.Context, toolset *research.ResearchToolSet, reader *bufio.Reader) error {
	fmt.Println("\n--- get-citations ---")

	fmt.Print("Work ID (e.g., W2741809807): ")
	workID, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	workID = strings.TrimSpace(workID)
	if workID == "" {
		return fmt.Errorf("work ID cannot be empty")
	}

	fmt.Print("Direction (references or cited_by): ")
	direction, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	direction = strings.TrimSpace(direction)
	if direction == "" {
		return fmt.Errorf("direction cannot be empty")
	}

	fmt.Print("Max results [10]: ")
	maxResultsStr, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	maxResultsStr = strings.TrimSpace(maxResultsStr)

	maxResults := 10
	if maxResultsStr != "" {
		maxResults, err = strconv.Atoi(maxResultsStr)
		if err != nil {
			return fmt.Errorf("invalid max results: %w", err)
		}
	}

	fmt.Println("\nFetching citations...")

	input := map[string]interface{}{
		"work_id":     workID,
		"direction":   direction,
		"max_results": maxResults,
	}
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return err
	}

	result, err := callToolHandler(ctx, toolset, "get_citations_and_references", inputJSON)
	if err != nil {
		return err
	}

	fmt.Println("\n=== Results ===")
	return formatOutput(os.Stdout, "get-citations", result, false)
}

func interactiveFetchWebpage(ctx context.Context, toolset *research.ResearchToolSet, reader *bufio.Reader) error {
	fmt.Println("\n--- fetch-webpage ---")

	fmt.Print("URL: ")
	url, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	url = strings.TrimSpace(url)
	if url == "" {
		return fmt.Errorf("URL cannot be empty")
	}

	fmt.Print("Max length [8000]: ")
	maxLengthStr, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	maxLengthStr = strings.TrimSpace(maxLengthStr)

	maxLength := 8000
	if maxLengthStr != "" {
		maxLength, err = strconv.Atoi(maxLengthStr)
		if err != nil {
			return fmt.Errorf("invalid max length: %w", err)
		}
	}

	fmt.Println("\nFetching webpage...")

	input := map[string]interface{}{
		"url":        url,
		"max_length": maxLength,
	}
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return err
	}

	result, err := callToolHandler(ctx, toolset, "fetch_webpage_content", inputJSON)
	if err != nil {
		return err
	}

	fmt.Println("\n=== Result ===")
	return formatOutput(os.Stdout, "fetch-webpage", result, false)
}
