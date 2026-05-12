package main

import (
	"flag"
	"fmt"
	"log"
	"path/filepath"
	"strings"
)

var (
	traceFile   = flag.String("trace", "", "Path to trace JSON file (required)")
	outputFile  = flag.String("output", "", "Output HTML file (default: <basename>-timeline.html)")
	openBrowser = flag.Bool("open", false, "Open the HTML file in browser after generation")
	verbose     = flag.Bool("verbose", false, "Enable verbose logging")
)

func main() {
	flag.Parse()

	if err := run(); err != nil {
		log.Fatalf("Error: %v\n", err)
	}
}

func run() error {
	// Validate required flags
	if *traceFile == "" {
		return fmt.Errorf("--trace flag is required")
	}

	// Determine output filename
	output := *outputFile
	if output == "" {
		base := filepath.Base(*traceFile)
		base = strings.TrimSuffix(base, filepath.Ext(base))
		output = base + "-timeline.html"
	}

	if *verbose {
		log.Printf("Loading trace from: %s\n", *traceFile)
	}

	// Load trace
	trace, err := LoadTrace(*traceFile)
	if err != nil {
		return fmt.Errorf("load trace: %w", err)
	}

	if *verbose {
		log.Printf("Loaded trace: session_id=%s, turns=%d\n", trace.SessionID, len(trace.Turns))
	}

	// TODO: Generate HTML visualization
	if *verbose {
		log.Printf("Would generate HTML to: %s\n", output)
	}

	// TODO: Open in browser if requested
	if *openBrowser {
		if *verbose {
			log.Printf("Would open browser for: %s\n", output)
		}
	}

	fmt.Printf("Timeline visualization would be written to: %s\n", output)
	return nil
}
