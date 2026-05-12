package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/dancsalo/arxiv-deep-research/internal/tracing"
)

// LoadTrace reads and parses a trace JSON file
func LoadTrace(path string) (*tracing.Trace, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read trace file: %w", err)
	}

	var trace tracing.Trace
	if err := json.Unmarshal(data, &trace); err != nil {
		return nil, fmt.Errorf("decode trace JSON: %w", err)
	}

	return &trace, nil
}
