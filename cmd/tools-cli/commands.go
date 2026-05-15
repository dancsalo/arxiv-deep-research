package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/dancsalo/arxiv-deep-research/internal/registry"
	"github.com/dancsalo/arxiv-deep-research/tools/research"
)

func executeCommand(ctx context.Context, toolset *research.ResearchToolSet, command string, args []string) error {
	switch command {
	case "fetch-arxiv-text":
		return executeFetchArxivText(ctx, toolset, args)
	case "fetch-webpage":
		return executeFetchWebpage(ctx, toolset, args)
	case "get-citations":
		return executeGetCitations(ctx, toolset, args)
	case "search-arxiv":
		return executeSearchArxiv(ctx, toolset, args)
	case "search-github":
		return executeSearchGithub(ctx, toolset, args)
	case "search-openalex":
		return executeSearchOpenAlex(ctx, toolset, args)
	case "search-web":
		return executeSearchWeb(ctx, toolset, args)
	default:
		return fmt.Errorf("unknown command: %s", command)
	}
}

func executeSearchArxiv(ctx context.Context, toolset *research.ResearchToolSet, args []string) error {
	// Extract query first (required positional argument before flags)
	if len(args) == 0 {
		return fmt.Errorf("query argument required\n\nUsage: tools-cli search-arxiv <query> [--max-results=N] [--search-field=FIELD] [--authors=NAMES] [--category=CAT] [--exact-phrase] [--sort-by=FIELD] [--sort-order=ORDER]\n\nExamples:\n  tools-cli search-arxiv \"attention mechanism\"\n  tools-cli search-arxiv \"Attention Is All You Need\" --exact-phrase --sort-by=relevance\n  tools-cli search-arxiv \"graph convolutional networks\" --authors=Kipf --category=cs.LG")
	}

	query := args[0]
	flagArgs := args[1:]

	fs := flag.NewFlagSet("search-arxiv", flag.ExitOnError)
	maxResults := fs.Int("max-results", 10, "maximum number of results")
	searchField := fs.String("search-field", "title", "field to search: title (default) or abstract")
	authors := fs.String("authors", "", "comma-separated author last names (e.g., 'Vaswani,Shazeer')")
	category := fs.String("category", "", "arXiv category (e.g., 'cs.LG', 'cs.CV', 'cs.CL')")
	exactPhrase := fs.Bool("exact-phrase", false, "search for exact title match")
	sortBy := fs.String("sort-by", "", "sort by: submittedDate, lastUpdatedDate, or relevance")
	sortOrder := fs.String("sort-order", "", "sort order: ascending or descending")

	if err := fs.Parse(flagArgs); err != nil {
		return err
	}

	input := map[string]interface{}{
		"query":        query,
		"max_results":  *maxResults,
		"search_field": *searchField,
		"exact_phrase": *exactPhrase,
	}

	// Add optional structured parameters
	if *authors != "" {
		// Split comma-separated authors into array
		authorList := []string{}
		for _, author := range parseCSV(*authors) {
			if author != "" {
				authorList = append(authorList, author)
			}
		}
		if len(authorList) > 0 {
			input["authors"] = authorList
		}
	}

	if *category != "" {
		input["category"] = *category
	}

	if *sortBy != "" {
		input["sort_by"] = *sortBy
	}

	if *sortOrder != "" {
		input["sort_order"] = *sortOrder
	}

	inputJSON, err := json.Marshal(input)
	if err != nil {
		return fmt.Errorf("failed to marshal input: %w", err)
	}

	result, err := callToolHandler(ctx, toolset, "search_arxiv", inputJSON)
	if err != nil {
		return err
	}

	return formatOutput(os.Stdout, "search-arxiv", result, *jsonOutput)
}

// parseCSV splits a comma-separated string into trimmed parts
func parseCSV(s string) []string {
	if s == "" {
		return nil
	}
	var result []string
	for _, part := range splitOn(s, ',') {
		trimmed := trimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// splitOn splits a string on a delimiter (simple implementation)
func splitOn(s string, delim rune) []string {
	var result []string
	var current []rune
	for _, r := range s {
		if r == delim {
			result = append(result, string(current))
			current = nil
		} else {
			current = append(current, r)
		}
	}
	if len(current) > 0 || len(result) == 0 {
		result = append(result, string(current))
	}
	return result
}

// trimSpace removes leading/trailing whitespace
func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}

func executeSearchOpenAlex(ctx context.Context, toolset *research.ResearchToolSet, args []string) error {
	fs := flag.NewFlagSet("search-openalex", flag.ExitOnError)
	maxResults := fs.Int("max-results", 10, "maximum number of results")
	filter := fs.String("filter", "", "OpenAlex filter expression")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() == 0 {
		return fmt.Errorf("query argument required\n\nUsage: tools-cli search-openalex <query> [--max-results=N] [--filter=EXPR]\n\nExample: tools-cli search-openalex \"neural networks\" --filter \"publication_year:>2023\"")
	}

	query := fs.Arg(0)

	input := map[string]interface{}{
		"query":       query,
		"max_results": *maxResults,
	}
	if *filter != "" {
		input["filter"] = *filter
	}

	inputJSON, err := json.Marshal(input)
	if err != nil {
		return fmt.Errorf("failed to marshal input: %w", err)
	}

	result, err := callToolHandler(ctx, toolset, "search_openalex", inputJSON)
	if err != nil {
		return err
	}

	return formatOutput(os.Stdout, "search-openalex", result, *jsonOutput)
}

func executeFetchArxivText(ctx context.Context, toolset *research.ResearchToolSet, args []string) error {
	fs := flag.NewFlagSet("fetch-arxiv-text", flag.ExitOnError)
	maxLength := fs.Int("max-length", 25000, "maximum text content length in characters")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() == 0 {
		return fmt.Errorf("arxiv-id argument required\n\nUsage: tools-cli fetch-arxiv-text <arxiv-id> [--max-length=N]\n\nExpected formats:\n  - 2301.00001 (new format)\n  - arXiv:2301.00001\n  - astro-ph/9901234 (old format)\n\nExamples:\n  tools-cli fetch-arxiv-text \"1706.03762\"\n  tools-cli fetch-arxiv-text \"2301.00001\" --max-length 25000")
	}

	arxivID := fs.Arg(0)

	input := map[string]interface{}{
		"arxiv_id":   arxivID,
		"max_length": *maxLength,
	}

	inputJSON, err := json.Marshal(input)
	if err != nil {
		return fmt.Errorf("failed to marshal input: %w", err)
	}

	result, err := callToolHandler(ctx, toolset, "fetch_arxiv_text", inputJSON)
	if err != nil {
		return err
	}

	return formatOutput(os.Stdout, "fetch-arxiv-text", result, *jsonOutput)
}

func executeSearchGithub(ctx context.Context, toolset *research.ResearchToolSet, args []string) error {
	fs := flag.NewFlagSet("search-github", flag.ExitOnError)
	maxResults := fs.Int("max-results", 5, "maximum number of results (max 5)")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() == 0 {
		return fmt.Errorf("query argument required\n\nUsage: tools-cli search-github <query> [--max-results=N]\n\nExample: tools-cli search-github \"diffusion models pytorch\"")
	}

	query := fs.Arg(0)

	input := map[string]interface{}{
		"query":       query,
		"max_results": *maxResults,
	}

	inputJSON, err := json.Marshal(input)
	if err != nil {
		return fmt.Errorf("failed to marshal input: %w", err)
	}

	result, err := callToolHandler(ctx, toolset, "search_github_repos", inputJSON)
	if err != nil {
		return err
	}

	return formatOutput(os.Stdout, "search-github", result, *jsonOutput)
}

func executeSearchWeb(ctx context.Context, toolset *research.ResearchToolSet, args []string) error {
	fs := flag.NewFlagSet("search-web", flag.ExitOnError)
	maxResults := fs.Int("max-results", 10, "maximum number of results")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() == 0 {
		return fmt.Errorf("query argument required\n\nUsage: tools-cli search-web <query> [--max-results=N]\n\nExample: tools-cli search-web \"quantum computing tutorials\"")
	}

	query := fs.Arg(0)

	input := map[string]interface{}{
		"query":       query,
		"max_results": *maxResults,
	}

	inputJSON, err := json.Marshal(input)
	if err != nil {
		return fmt.Errorf("failed to marshal input: %w", err)
	}

	result, err := callToolHandler(ctx, toolset, "search_web", inputJSON)
	if err != nil {
		return err
	}

	return formatOutput(os.Stdout, "search-web", result, *jsonOutput)
}

func executeGetCitations(ctx context.Context, toolset *research.ResearchToolSet, args []string) error {
	fs := flag.NewFlagSet("get-citations", flag.ExitOnError)
	maxResults := fs.Int("max-results", 10, "maximum number of results")
	direction := fs.String("direction", "", "direction: 'references' (bibliography) or 'cited_by' (forward citations)")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() == 0 {
		return fmt.Errorf("work_id argument required\n\nUsage: tools-cli get-citations <work_id> --direction=<references|cited_by> [--max-results=N]\n\nExample: tools-cli get-citations W2741809807 --direction references --max-results 10")
	}

	if *direction == "" {
		return fmt.Errorf("--direction flag required (references or cited_by)\n\nUsage: tools-cli get-citations <work_id> --direction=<references|cited_by> [--max-results=N]\n\nExample: tools-cli get-citations W2741809807 --direction references --max-results 10")
	}

	workID := fs.Arg(0)

	input := map[string]interface{}{
		"work_id":     workID,
		"direction":   *direction,
		"max_results": *maxResults,
	}

	inputJSON, err := json.Marshal(input)
	if err != nil {
		return fmt.Errorf("failed to marshal input: %w", err)
	}

	result, err := callToolHandler(ctx, toolset, "get_citations_and_references", inputJSON)
	if err != nil {
		return err
	}

	return formatOutput(os.Stdout, "get-citations", result, *jsonOutput)
}

func executeFetchWebpage(ctx context.Context, toolset *research.ResearchToolSet, args []string) error {
	fs := flag.NewFlagSet("fetch-webpage", flag.ExitOnError)
	maxLength := fs.Int("max-length", 8000, "maximum content length in characters")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() == 0 {
		return fmt.Errorf("url argument required\n\nUsage: tools-cli fetch-webpage <url> [--max-length=N]\n\nExamples:\n  tools-cli fetch-webpage \"https://example.com/article\"\n  tools-cli fetch-webpage \"https://blog.example.com\" --max-length 10000")
	}

	url := fs.Arg(0)

	input := map[string]interface{}{
		"url":        url,
		"max_length": *maxLength,
	}

	inputJSON, err := json.Marshal(input)
	if err != nil {
		return fmt.Errorf("failed to marshal input: %w", err)
	}

	result, err := callToolHandler(ctx, toolset, "fetch_webpage_content", inputJSON)
	if err != nil {
		return err
	}

	return formatOutput(os.Stdout, "fetch-webpage", result, *jsonOutput)
}

// callToolHandler calls a tool handler through the registry
func callToolHandler(ctx context.Context, toolset *research.ResearchToolSet, toolName string, input json.RawMessage) (string, error) {
	// Create a registry and register all tools
	reg := registry.NewToolRegistry()
	toolset.Register(reg)

	// Execute the tool
	return reg.Execute(ctx, toolName, input)
}
