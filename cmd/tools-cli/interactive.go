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
		fmt.Println("  [1] search-arxiv        - Search arXiv for preprints")
		fmt.Println("  [2] search-openalex     - Search academic literature")
		fmt.Println("  [3] fetch-pdf           - Get arXiv PDF download URL")
		fmt.Println("  [4] search-github       - Find GitHub repositories")
		fmt.Println("  [5] search-web          - Search the general web")
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
			if err := interactiveSearchArxiv(ctx, toolset, reader); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n\n", err)
			}
		case "2":
			if err := interactiveSearchOpenAlex(ctx, toolset, reader); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n\n", err)
			}
		case "3":
			if err := interactiveFetchPdf(ctx, toolset, reader); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n\n", err)
			}
		case "4":
			if err := interactiveSearchGithub(ctx, toolset, reader); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n\n", err)
			}
		case "5":
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

func interactiveFetchPdf(ctx context.Context, toolset *research.ResearchToolSet, reader *bufio.Reader) error {
	fmt.Println("\n--- fetch-pdf ---")

	fmt.Print("arXiv ID (e.g., 1706.03762): ")
	arxivID, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	arxivID = strings.TrimSpace(arxivID)
	if arxivID == "" {
		return fmt.Errorf("arXiv ID cannot be empty")
	}

	fmt.Println("\nFetching PDF URL...")

	input := map[string]interface{}{
		"arxiv_id": arxivID,
	}
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return err
	}

	result, err := callToolHandler(ctx, toolset, "fetch_arxiv_pdf", inputJSON)
	if err != nil {
		return err
	}

	fmt.Println("\n=== Result ===")
	return formatOutput(os.Stdout, "fetch-pdf", result, false)
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
