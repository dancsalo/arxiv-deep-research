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
	case "search-arxiv":
		return executeSearchArxiv(ctx, toolset, args)
	case "search-openalex":
		return executeSearchOpenAlex(ctx, toolset, args)
	case "fetch-pdf":
		return executeFetchPdf(ctx, toolset, args)
	case "search-github":
		return executeSearchGithub(ctx, toolset, args)
	default:
		return fmt.Errorf("unknown command: %s", command)
	}
}

func executeSearchArxiv(ctx context.Context, toolset *research.ResearchToolSet, args []string) error {
	fs := flag.NewFlagSet("search-arxiv", flag.ExitOnError)
	maxResults := fs.Int("max-results", 10, "maximum number of results")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() == 0 {
		return fmt.Errorf("query argument required\n\nUsage: tools-cli search-arxiv <query> [--max-results=N]\n\nExample: tools-cli search-arxiv \"attention mechanism\"")
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

	result, err := callToolHandler(ctx, toolset, "search_arxiv", inputJSON)
	if err != nil {
		return err
	}

	return formatOutput(os.Stdout, "search-arxiv", result, *jsonOutput)
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

func executeFetchPdf(ctx context.Context, toolset *research.ResearchToolSet, args []string) error {
	fs := flag.NewFlagSet("fetch-pdf", flag.ExitOnError)

	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() == 0 {
		return fmt.Errorf("arxiv-id argument required\n\nUsage: tools-cli fetch-pdf <arxiv-id>\n\nExpected formats:\n  - 2301.00001 (new format)\n  - arXiv:2301.00001\n  - astro-ph/9901234 (old format)\n\nExample: tools-cli fetch-pdf \"1706.03762\"")
	}

	arxivID := fs.Arg(0)

	input := map[string]interface{}{
		"arxiv_id": arxivID,
	}

	inputJSON, err := json.Marshal(input)
	if err != nil {
		return fmt.Errorf("failed to marshal input: %w", err)
	}

	result, err := callToolHandler(ctx, toolset, "fetch_arxiv_pdf", inputJSON)
	if err != nil {
		return err
	}

	return formatOutput(os.Stdout, "fetch-pdf", result, *jsonOutput)
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

// callToolHandler calls a tool handler through the registry
func callToolHandler(ctx context.Context, toolset *research.ResearchToolSet, toolName string, input json.RawMessage) (string, error) {
	// Create a registry and register all tools
	reg := registry.NewToolRegistry()
	toolset.Register(reg)

	// Execute the tool
	return reg.Execute(ctx, toolName, input)
}
