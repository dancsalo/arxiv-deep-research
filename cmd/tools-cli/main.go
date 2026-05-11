package main

import (
	"flag"
	"fmt"
	"os"
)

var (
	interactive bool
	jsonOutput  bool
	helpFlag    bool
)

func init() {
	flag.BoolVar(&interactive, "i", false, "Run tools CLI in interactive mode")
	flag.BoolVar(&jsonOutput, "j", false, "Output results in JSON format")
	flag.BoolVar(&helpFlag, "help", false, "Show help information")
}

func printUsage() {
	fmt.Println("Tools CLI - Interactive Research Tool Testing")
	fmt.Println("\nUsage:")
	fmt.Println("  tools-cli [options] [command]")
	fmt.Println("\nOptions:")
	flag.PrintDefaults()
	fmt.Println("\nCommands:")
	fmt.Println("  Run tools interactively or execute specific commands")
	fmt.Println("\nExamples:")
	fmt.Println("  tools-cli -i              # Start interactive mode")
	fmt.Println("  tools-cli -j github       # Execute GitHub search tool with JSON output")
}

func main() {
	flag.Usage = printUsage
	flag.Parse()

	// Check for help flag first
	if helpFlag {
		printUsage()
		os.Exit(0)
	}

	// Determine execution mode
	if interactive {
		// TODO: Implement runInteractive() in a future task
		runInteractive()
	} else if flag.NArg() > 0 {
		// TODO: Implement executeCommand() in a future task
		executeCommand(flag.Args())
	} else {
		fmt.Println("Error: No command specified. Use --help for usage information.")
		os.Exit(1)
	}
}

// Placeholder functions for future implementation
func runInteractive() {
	fmt.Println("Interactive mode not yet implemented.")
	os.Exit(1)
}

func executeCommand(args []string) {
	fmt.Println("Command execution not yet implemented.")
	os.Exit(1)
}