package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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

	// Detect schema features
	features := DetectSchemaFeatures(trace)

	if *verbose {
		log.Printf("Generating HTML to: %s\n", output)
	}

	// Generate HTML visualization
	if err := GenerateHTML(trace, features, output); err != nil {
		return fmt.Errorf("generate HTML: %w", err)
	}

	fmt.Printf("Timeline generated: %s\n", output)

	// Open in browser if requested
	if *openBrowser {
		if *verbose {
			log.Printf("Opening in browser...\n")
		}
		if err := openBrowserFunc(output); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not open browser: %v\n", err)
		}
	}

	return nil
}

func openBrowserFunc(path string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
		args = []string{path}
	case "linux":
		cmd = "xdg-open"
		args = []string{path}
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start", path}
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}

	return exec.Command(cmd, args...).Start()
}
