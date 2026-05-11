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
  search-arxiv <query> [--max-results=N] [--search-field=FIELD]
  search-openalex <query> [--max-results=N] [--filter=EXPR]
  fetch-pdf <arxiv-id>
  search-github <query> [--max-results=N]
  search-web <query> [--max-results=N]

Flags:
  --interactive, -i    Launch interactive mode
  --json              Output results as JSON
  --help, -h          Show this help

Examples:
  tools-cli search-arxiv "attention mechanism"
  tools-cli search-arxiv "neural networks" --search-field=abstract
  tools-cli search-openalex "neural networks" --filter "publication_year:>2023"
  tools-cli fetch-pdf "1706.03762"
  tools-cli search-github "diffusion models pytorch"
  tools-cli search-web "quantum computing tutorials"
  tools-cli --interactive
`)
}
