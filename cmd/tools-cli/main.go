package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/dancsalo/arxiv-deep-research/tools/research"
)

var (
	interactive = flag.Bool("interactive", false, "launch interactive mode")
	interactiveShort = flag.Bool("i", false, "launch interactive mode (shorthand)")
	jsonOutput = flag.Bool("json", false, "output results as JSON")
	helpFlag = flag.Bool("help", false, "show help")
	helpShort = flag.Bool("h", false, "show help (shorthand)")
)

func main() {
	flag.Usage = printUsage
	flag.Parse()

	if *helpFlag || *helpShort {
		printUsage()
		os.Exit(0)
	}

	httpClient := &http.Client{Timeout: 30 * time.Second}
	toolset := research.NewResearchToolSet(httpClient)
	ctx := context.Background()

	if *interactive || *interactiveShort {
		if err := runInteractive(ctx, toolset); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	args := flag.Args()
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Error: no command specified")
		printUsage()
		os.Exit(1)
	}

	command := args[0]
	commandArgs := args[1:]

	if err := executeCommand(ctx, toolset, command, commandArgs); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `tools-cli - Research Tools Command Line Interface

Usage:
  tools-cli [flags] <command> [args]

Commands:
  fetch-arxiv-text <arxiv-id> [--max-length=N]
  fetch-webpage <url> [--max-length=N]
  get-citations <work-id> --direction=<references|cited_by> [--max-results=N]
  search-arxiv <query> [--max-results=N] [--search-field=FIELD]
  search-github <query> [--max-results=N]
  search-openalex <query> [--max-results=N] [--filter=EXPR]
  search-web <query> [--max-results=N]

Flags:
  --interactive, -i    Launch interactive mode
  --json              Output results as JSON
  --help, -h          Show this help

Examples:
  tools-cli fetch-arxiv-text "1706.03762"
  tools-cli fetch-arxiv-text "2301.00001" --max-length=25000
  tools-cli fetch-webpage "https://jalammar.github.io/illustrated-transformer/"
  tools-cli get-citations W2741809807 --direction references --max-results 10
  tools-cli search-arxiv "attention mechanism"
  tools-cli search-arxiv "neural networks" --search-field=abstract
  tools-cli search-github "diffusion models pytorch"
  tools-cli search-openalex "neural networks" --filter "publication_year:>2023"
  tools-cli search-web "quantum computing tutorials"
  tools-cli --interactive
`)
}
