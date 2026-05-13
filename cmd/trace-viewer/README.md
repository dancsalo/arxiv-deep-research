# trace-viewer

Interactive timeline visualization for single research agent traces.

## Usage

```bash
# Basic usage
./trace-viewer --trace=.traces/session-001.json

# Open in browser automatically
./trace-viewer --trace=.traces/session-001.json --open

# Specify output path
./trace-viewer --trace=.traces/session-001.json --output=my-timeline.html

# Verbose output
./trace-viewer --trace=.traces/session-001.json --verbose
```

## Via Makefile

```bash
# Build the binary
make build-viewer

# Visualize most recent trace
make trace-view
```

## Features

- **Timeline View**: Horizontal timeline with turn nodes and event nodes
- **Expandable Details**: Click any node to expand inline detail panel
- **Multiple Expansions**: Compare multiple turns by expanding them simultaneously
- **Event Nodes**: Compaction and guardrail events shown inline
- **Progressive Enhancement**: Works with current trace schema, enhances with optional fields
- **Self-Contained**: Generated HTML works offline, no server needed

## Schema Support

The tool supports progressive enhancement with these optional fields:

- **Per-event timestamps**: Enables temporal visualization
- **Context snapshots**: Shows what's in the context at each turn
- **Tool relationships**: Visualizes dependency graphs
- **Compaction diffs**: Before/after comparison for context management
- **Display metadata**: Rich labels and status indicators
- **Rich error context**: Structured error information

Missing fields are gracefully handled with fallback behavior.

## Development

```bash
# Run tests
cd cmd/trace-viewer
go test -v

# Build
go build -o trace-viewer .

# Test with fixtures
./trace-viewer --trace=testdata/fixtures/normal-trace.json --open
```

## Architecture

- **Static HTML Generator**: Reads trace JSON, generates self-contained HTML
- **No Dependencies**: Pure Go stdlib backend, vanilla JavaScript frontend
- **Follows trace-analyzer Pattern**: Consistent with existing tooling
