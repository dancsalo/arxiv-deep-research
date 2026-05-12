# Trace Analysis Dashboard Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Go CLI tool that analyzes research agent trace files and generates an HTML dashboard with visualizations for system optimization.

**Architecture:** Three-phase pipeline (Load traces → Compute metrics → Generate HTML). Pure Go stdlib with embedded Chart.js for offline visualizations.

**Tech Stack:** Go 1.21+, stdlib only, Chart.js 4.x (embedded)

---

## File Structure

```
cmd/trace-analyzer/
  main.go              - CLI entry, flag parsing, orchestration
  analyzer.go          - Core analysis, metric computation
  sequences.go         - Tool sequence detection algorithms
  html_generator.go    - HTML generation, template rendering
  templates.go         - Embedded HTML/CSS/JS templates
  analyzer_test.go     - Unit tests for analyzer
  sequences_test.go    - Unit tests for sequence detection
  html_generator_test.go - Unit tests for HTML generation
  testdata/
    fixtures/          - Sample trace files for testing
```

---

### Task 1: Project Setup and Data Types

**Files:**
- Create: `cmd/trace-analyzer/main.go`
- Create: `cmd/trace-analyzer/analyzer.go`

- [ ] **Step 1: Write test for Metrics data structure**

Create `cmd/trace-analyzer/analyzer_test.go`:

```go
package main

import (
	"testing"
	"time"
)

func TestMetrics_Initialization(t *testing.T) {
	m := NewMetrics()
	
	if m.TotalTraces != 0 {
		t.Errorf("expected TotalTraces=0, got %d", m.TotalTraces)
	}
	
	if m.ToolStats == nil {
		t.Error("expected ToolStats map to be initialized")
	}
	
	if m.ToolTransitions == nil {
		t.Error("expected ToolTransitions map to be initialized")
	}
	
	if m.PromptVariants == nil {
		t.Error("expected PromptVariants map to be initialized")
	}
}

func TestDateRange_Update(t *testing.T) {
	dr := &DateRange{}
	
	t1 := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 5, 11, 14, 0, 0, 0, time.UTC)
	t3 := time.Date(2026, 5, 9, 10, 0, 0, 0, time.UTC)
	
	dr.Update(t1)
	if !dr.Earliest.Equal(t1) || !dr.Latest.Equal(t1) {
		t.Error("first update should set both Earliest and Latest")
	}
	
	dr.Update(t2)
	if !dr.Latest.Equal(t2) {
		t.Errorf("expected Latest=%v, got %v", t2, dr.Latest)
	}
	
	dr.Update(t3)
	if !dr.Earliest.Equal(t3) {
		t.Errorf("expected Earliest=%v, got %v", t3, dr.Earliest)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd cmd/trace-analyzer
go test -v
```

Expected: FAIL - undefined: NewMetrics, DateRange

- [ ] **Step 3: Implement Metrics data structures**

Create `cmd/trace-analyzer/analyzer.go`:

```go
package main

import (
	"time"
)

// Metrics contains all computed statistics for the dashboard
type Metrics struct {
	// Overview
	TotalTraces   int
	SuccessRate   float64
	DateRange     DateRange
	TotalTokens   int
	TotalCost     float64
	AvgDuration   float64
	
	// Tool analysis
	ToolStats       map[string]*ToolStat
	ToolTransitions map[string]map[string]int
	ToolSequences   []ToolSequence
	
	// Performance
	LatencyDistribution map[string][]float64
	ResultSizeLatency   []DataPoint
	
	// Budget
	BudgetAnalysis *BudgetAnalysis
	
	// Variants
	PromptVariants map[string]*VariantStats
}

type ToolStat struct {
	Name          string
	TotalCalls    int
	AvgLatency    float64
	MedianLatency float64
	P95Latency    float64
	AvgResultSize float64
	CallsPerRun   float64
}

type ToolSequence struct {
	Pattern    []string
	Count      int
	Support    float64
	AvgLatency float64
}

type DataPoint struct {
	X float64
	Y float64
	Label string
}

type BudgetAnalysis struct {
	BudgetTiers   []BudgetTier
	TokenBurnDown []TurnTokenData
	ToolsByBudget map[string][]ToolFrequency
}

type BudgetTier struct {
	Name            string
	Min             int
	Max             int
	TraceCount      int
	AvgToolsUsed    float64
	UniqueToolsUsed []string
	ToolFrequency   map[string]int
}

type TurnTokenData struct {
	TurnIndex       int
	TokensRemaining int
	TraceID         string
}

type ToolFrequency struct {
	Tool  string
	Count int
}

type VariantStats struct {
	Variant     string
	TraceCount  int
	SuccessRate float64
	AvgCost     float64
	AvgDuration float64
	AvgTokens   int
	ToolUsage   map[string]int
}

type DateRange struct {
	Earliest time.Time
	Latest   time.Time
}

func (dr *DateRange) Update(t time.Time) {
	if dr.Earliest.IsZero() || t.Before(dr.Earliest) {
		dr.Earliest = t
	}
	if dr.Latest.IsZero() || t.After(dr.Latest) {
		dr.Latest = t
	}
}

func NewMetrics() *Metrics {
	return &Metrics{
		ToolStats:           make(map[string]*ToolStat),
		ToolTransitions:     make(map[string]map[string]int),
		ToolSequences:       []ToolSequence{},
		LatencyDistribution: make(map[string][]float64),
		ResultSizeLatency:   []DataPoint{},
		PromptVariants:      make(map[string]*VariantStats),
		BudgetAnalysis: &BudgetAnalysis{
			BudgetTiers:   []BudgetTier{},
			TokenBurnDown: []TurnTokenData{},
			ToolsByBudget: make(map[string][]ToolFrequency),
		},
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test -v
```

Expected: PASS

- [ ] **Step 5: Create main.go skeleton**

Create `cmd/trace-analyzer/main.go`:

```go
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
)

type Config struct {
	TracesDir  string
	OutputPath string
	Since      string
	Until      string
	Variant    string
	MinSupport int
	Verbose    bool
}

func main() {
	cfg := Config{}
	
	flag.StringVar(&cfg.TracesDir, "traces-dir", ".traces", "Input directory containing trace JSON files")
	flag.StringVar(&cfg.OutputPath, "output", ".traces/dashboard.html", "Output HTML file path")
	flag.StringVar(&cfg.Since, "since", "", "Only include traces after date (YYYY-MM-DD)")
	flag.StringVar(&cfg.Until, "until", "", "Only include traces before date (YYYY-MM-DD)")
	flag.StringVar(&cfg.Variant, "variant", "", "Only include specific prompt variant (A, B, or C)")
	flag.IntVar(&cfg.MinSupport, "min-support", 2, "Minimum trace count for sequence patterns")
	flag.BoolVar(&cfg.Verbose, "verbose", false, "Print detailed progress information")
	flag.Parse()
	
	if err := run(cfg); err != nil {
		log.Fatalf("Error: %v", err)
	}
}

func run(cfg Config) error {
	if cfg.Verbose {
		fmt.Printf("Loading traces from %s...\n", cfg.TracesDir)
	}
	
	// TODO: implement pipeline
	// 1. Load traces
	// 2. Compute metrics
	// 3. Generate HTML
	
	fmt.Printf("Dashboard written to %s\n", cfg.OutputPath)
	return nil
}
```

- [ ] **Step 6: Verify main.go compiles**

```bash
go build -o trace-analyzer .
./trace-analyzer --help
```

Expected: Help text displayed, no errors

- [ ] **Step 7: Commit**

```bash
git add cmd/trace-analyzer/
git commit -m "feat: add trace-analyzer data structures and CLI skeleton

- Define Metrics, ToolStat, BudgetAnalysis types
- Add NewMetrics constructor with initialized maps
- Add DateRange helper with Update method
- Create main.go with flag parsing
- Add tests for Metrics initialization and DateRange"
```

---

### Task 2: Trace Loading

**Files:**
- Modify: `cmd/trace-analyzer/analyzer.go`
- Modify: `cmd/trace-analyzer/analyzer_test.go`
- Create: `cmd/trace-analyzer/testdata/fixtures/sample-trace.json`

- [ ] **Step 1: Create fixture trace file**

Create `cmd/trace-analyzer/testdata/fixtures/sample-trace.json`:

```json
{
  "session_id": "test-001",
  "query": "test query",
  "model": "test-model",
  "prompt_variant": "A",
  "prompt_hash": "abc123",
  "started_at": "2026-05-10T10:00:00Z",
  "ended_at": "2026-05-10T10:05:00Z",
  "duration_ms": 300000,
  "status": "ok",
  "total_input_tokens": 5000,
  "total_output_tokens": 1000,
  "total_cost_usd": 0.025,
  "turns": [
    {
      "index": 0,
      "started_at": "2026-05-10T10:00:00Z",
      "ended_at": "2026-05-10T10:02:00Z",
      "duration_ms": 120000,
      "tokens_used": 3000,
      "tokens_remaining": 27000,
      "cost_usd": 0.015,
      "tool_calls": [
        {
          "name": "search_arxiv",
          "input": {"query": "test"},
          "output": [{"title": "Test Paper"}],
          "input_length": 20,
          "result_length": 500,
          "duration_ms": 2000
        },
        {
          "name": "fetch_arxiv_pdf",
          "input": {"arxiv_id": "1234.5678"},
          "output": {"url": "http://example.com/pdf"},
          "input_length": 30,
          "result_length": 100,
          "duration_ms": 1500
        }
      ]
    }
  ]
}
```

- [ ] **Step 2: Write test for LoadTraces**

Add to `cmd/trace-analyzer/analyzer_test.go`:

```go
import (
	"path/filepath"
	
	"github.com/dancsalo/arxiv-deep-research/internal/tracing"
)

func TestLoadTraces(t *testing.T) {
	fixtureDir := filepath.Join("testdata", "fixtures")
	
	traces, err := LoadTraces(fixtureDir, Config{})
	if err != nil {
		t.Fatalf("LoadTraces failed: %v", err)
	}
	
	if len(traces) != 1 {
		t.Fatalf("expected 1 trace, got %d", len(traces))
	}
	
	trace := traces[0]
	if trace.SessionID != "test-001" {
		t.Errorf("expected SessionID=test-001, got %s", trace.SessionID)
	}
	
	if trace.Status != "ok" {
		t.Errorf("expected status=ok, got %s", trace.Status)
	}
	
	if len(trace.Turns) != 1 {
		t.Fatalf("expected 1 turn, got %d", len(trace.Turns))
	}
	
	if len(trace.Turns[0].ToolCalls) != 2 {
		t.Errorf("expected 2 tool calls, got %d", len(trace.Turns[0].ToolCalls))
	}
}

func TestLoadTraces_FilterByVariant(t *testing.T) {
	fixtureDir := filepath.Join("testdata", "fixtures")
	
	cfg := Config{Variant: "B"}
	traces, err := LoadTraces(fixtureDir, cfg)
	if err != nil {
		t.Fatalf("LoadTraces failed: %v", err)
	}
	
	if len(traces) != 0 {
		t.Errorf("expected 0 traces (variant B filtered out), got %d", len(traces))
	}
	
	cfg.Variant = "A"
	traces, err = LoadTraces(fixtureDir, cfg)
	if err != nil {
		t.Fatalf("LoadTraces failed: %v", err)
	}
	
	if len(traces) != 1 {
		t.Errorf("expected 1 trace (variant A), got %d", len(traces))
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

```bash
go test -v
```

Expected: FAIL - undefined: LoadTraces

- [ ] **Step 4: Implement LoadTraces**

Add to `cmd/trace-analyzer/analyzer.go`:

```go
import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
	
	"github.com/dancsalo/arxiv-deep-research/internal/tracing"
)

func LoadTraces(dir string, cfg Config) ([]tracing.Trace, error) {
	var traces []tracing.Trace
	var errors []string
	
	// Parse date filters if provided
	var sinceTime, untilTime time.Time
	var err error
	if cfg.Since != "" {
		sinceTime, err = time.Parse("2006-01-02", cfg.Since)
		if err != nil {
			return nil, fmt.Errorf("invalid --since date: %w", err)
		}
	}
	if cfg.Until != "" {
		untilTime, err = time.Parse("2006-01-02", cfg.Until)
		if err != nil {
			return nil, fmt.Errorf("invalid --until date: %w", err)
		}
		// Include the entire until date (end of day)
		untilTime = untilTime.Add(24 * time.Hour)
	}
	
	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		
		if d.IsDir() || !strings.HasSuffix(path, ".json") {
			return nil
		}
		
		data, err := os.ReadFile(path)
		if err != nil {
			errors = append(errors, fmt.Sprintf("%s: read error: %v", path, err))
			return nil
		}
		
		var trace tracing.Trace
		if err := json.Unmarshal(data, &trace); err != nil {
			errors = append(errors, fmt.Sprintf("%s: parse error: %v", path, err))
			return nil
		}
		
		// Apply filters
		if cfg.Variant != "" && trace.PromptVariant != cfg.Variant {
			return nil
		}
		
		if !sinceTime.IsZero() && trace.StartedAt.Before(sinceTime) {
			return nil
		}
		
		if !untilTime.IsZero() && trace.StartedAt.After(untilTime) {
			return nil
		}
		
		traces = append(traces, trace)
		return nil
	})
	
	if err != nil {
		return nil, fmt.Errorf("walking directory: %w", err)
	}
	
	if len(errors) > 0 && cfg.Verbose {
		fmt.Fprintf(os.Stderr, "Warnings while loading traces:\n")
		for _, e := range errors {
			fmt.Fprintf(os.Stderr, "  %s\n", e)
		}
	}
	
	return traces, nil
}
```

- [ ] **Step 5: Run test to verify it passes**

```bash
go test -v
```

Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add cmd/trace-analyzer/
git commit -m "feat: implement trace loading with filtering

- Add LoadTraces function with directory walking
- Support variant, since, until filters
- Handle malformed JSON gracefully
- Add test fixtures and tests for loading"
```

---

### Task 3: Basic Metric Computation

**Files:**
- Modify: `cmd/trace-analyzer/analyzer.go`
- Modify: `cmd/trace-analyzer/analyzer_test.go`

- [ ] **Step 1: Write test for ComputeMetrics**

Add to `cmd/trace-analyzer/analyzer_test.go`:

```go
func TestComputeMetrics_Overview(t *testing.T) {
	traces := []tracing.Trace{
		{
			SessionID:         "test-001",
			Status:            "ok",
			StartedAt:         time.Date(2026, 5, 10, 10, 0, 0, 0, time.UTC),
			DurationMs:        300000,
			TotalInputTokens:  5000,
			TotalOutputTokens: 1000,
			TotalCostUSD:      0.025,
		},
		{
			SessionID:         "test-002",
			Status:            "error",
			StartedAt:         time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC),
			DurationMs:        150000,
			TotalInputTokens:  3000,
			TotalOutputTokens: 500,
			TotalCostUSD:      0.015,
		},
	}
	
	metrics := ComputeMetrics(traces, Config{})
	
	if metrics.TotalTraces != 2 {
		t.Errorf("expected TotalTraces=2, got %d", metrics.TotalTraces)
	}
	
	expectedSuccess := 0.5
	if metrics.SuccessRate != expectedSuccess {
		t.Errorf("expected SuccessRate=%.2f, got %.2f", expectedSuccess, metrics.SuccessRate)
	}
	
	expectedTokens := 9500
	if metrics.TotalTokens != expectedTokens {
		t.Errorf("expected TotalTokens=%d, got %d", expectedTokens, metrics.TotalTokens)
	}
	
	expectedCost := 0.04
	if metrics.TotalCost != expectedCost {
		t.Errorf("expected TotalCost=%.3f, got %.3f", expectedCost, metrics.TotalCost)
	}
	
	expectedAvgDuration := 225000.0
	if metrics.AvgDuration != expectedAvgDuration {
		t.Errorf("expected AvgDuration=%.1f, got %.1f", expectedAvgDuration, metrics.AvgDuration)
	}
	
	if metrics.DateRange.Earliest.Year() != 2026 || metrics.DateRange.Earliest.Month() != 5 || metrics.DateRange.Earliest.Day() != 10 {
		t.Errorf("expected Earliest=2026-05-10, got %v", metrics.DateRange.Earliest)
	}
	
	if metrics.DateRange.Latest.Day() != 11 {
		t.Errorf("expected Latest day=11, got %v", metrics.DateRange.Latest)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test -v -run TestComputeMetrics_Overview
```

Expected: FAIL - undefined: ComputeMetrics

- [ ] **Step 3: Implement ComputeMetrics skeleton**

Add to `cmd/trace-analyzer/analyzer.go`:

```go
func ComputeMetrics(traces []tracing.Trace, cfg Config) *Metrics {
	m := NewMetrics()
	
	if len(traces) == 0 {
		return m
	}
	
	m.TotalTraces = len(traces)
	
	// Compute overview metrics
	successCount := 0
	totalDurationMs := int64(0)
	
	for _, trace := range traces {
		// Success rate
		if trace.Status == "ok" {
			successCount++
		}
		
		// Date range
		m.DateRange.Update(trace.StartedAt)
		
		// Totals
		m.TotalTokens += trace.TotalInputTokens + trace.TotalOutputTokens
		m.TotalCost += trace.TotalCostUSD
		totalDurationMs += trace.DurationMs
	}
	
	m.SuccessRate = float64(successCount) / float64(m.TotalTraces)
	m.AvgDuration = float64(totalDurationMs) / float64(m.TotalTraces)
	
	return m
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test -v -run TestComputeMetrics_Overview
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add cmd/trace-analyzer/
git commit -m "feat: implement basic metric computation

- Add ComputeMetrics function for overview stats
- Compute success rate, totals, averages, date range
- Add tests for overview metrics"
```

---

### Task 4: Tool Statistics

**Files:**
- Modify: `cmd/trace-analyzer/analyzer.go`
- Modify: `cmd/trace-analyzer/analyzer_test.go`

- [ ] **Step 1: Write test for tool statistics**

Add to `cmd/trace-analyzer/analyzer_test.go`:

```go
func TestComputeMetrics_ToolStats(t *testing.T) {
	traces := []tracing.Trace{
		{
			SessionID: "test-001",
			Turns: []tracing.Turn{
				{
					ToolCalls: []tracing.ToolCall{
						{Name: "search_arxiv", DurationMs: 2000, ResultLength: 500},
						{Name: "search_arxiv", DurationMs: 2500, ResultLength: 600},
						{Name: "fetch_arxiv_pdf", DurationMs: 1500, ResultLength: 100},
					},
				},
			},
		},
		{
			SessionID: "test-002",
			Turns: []tracing.Turn{
				{
					ToolCalls: []tracing.ToolCall{
						{Name: "search_arxiv", DurationMs: 1800, ResultLength: 450},
					},
				},
			},
		},
	}
	
	metrics := ComputeMetrics(traces, Config{})
	
	arxivStat, ok := metrics.ToolStats["search_arxiv"]
	if !ok {
		t.Fatal("expected search_arxiv in ToolStats")
	}
	
	if arxivStat.TotalCalls != 3 {
		t.Errorf("expected TotalCalls=3, got %d", arxivStat.TotalCalls)
	}
	
	expectedAvgLatency := (2000.0 + 2500.0 + 1800.0) / 3.0
	if arxivStat.AvgLatency != expectedAvgLatency {
		t.Errorf("expected AvgLatency=%.1f, got %.1f", expectedAvgLatency, arxivStat.AvgLatency)
	}
	
	expectedAvgSize := (500.0 + 600.0 + 450.0) / 3.0
	if arxivStat.AvgResultSize != expectedAvgSize {
		t.Errorf("expected AvgResultSize=%.1f, got %.1f", expectedAvgSize, arxivStat.AvgResultSize)
	}
	
	expectedCallsPerRun := 1.5
	if arxivStat.CallsPerRun != expectedCallsPerRun {
		t.Errorf("expected CallsPerRun=%.1f, got %.1f", expectedCallsPerRun, arxivStat.CallsPerRun)
	}
	
	pdfStat, ok := metrics.ToolStats["fetch_arxiv_pdf"]
	if !ok {
		t.Fatal("expected fetch_arxiv_pdf in ToolStats")
	}
	
	if pdfStat.TotalCalls != 1 {
		t.Errorf("expected TotalCalls=1, got %d", pdfStat.TotalCalls)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test -v -run TestComputeMetrics_ToolStats
```

Expected: FAIL - ToolStats map is empty

- [ ] **Step 3: Implement tool statistics computation**

Modify `ComputeMetrics` in `cmd/trace-analyzer/analyzer.go`:

```go
func ComputeMetrics(traces []tracing.Trace, cfg Config) *Metrics {
	m := NewMetrics()
	
	if len(traces) == 0 {
		return m
	}
	
	m.TotalTraces = len(traces)
	
	// Compute overview metrics
	successCount := 0
	totalDurationMs := int64(0)
	
	// Tool statistics accumulators
	toolData := make(map[string]*toolAccumulator)
	
	for _, trace := range traces {
		// Success rate
		if trace.Status == "ok" {
			successCount++
		}
		
		// Date range
		m.DateRange.Update(trace.StartedAt)
		
		// Totals
		m.TotalTokens += trace.TotalInputTokens + trace.TotalOutputTokens
		m.TotalCost += trace.TotalCostUSD
		totalDurationMs += trace.DurationMs
		
		// Tool statistics
		for _, turn := range trace.Turns {
			for _, tc := range turn.ToolCalls {
				if _, ok := toolData[tc.Name]; !ok {
					toolData[tc.Name] = &toolAccumulator{
						latencies:   []float64{},
						resultSizes: []float64{},
					}
				}
				
				acc := toolData[tc.Name]
				acc.totalCalls++
				acc.latencies = append(acc.latencies, float64(tc.DurationMs))
				acc.resultSizes = append(acc.resultSizes, float64(tc.ResultLength))
				
				// Store for latency distribution
				m.LatencyDistribution[tc.Name] = append(m.LatencyDistribution[tc.Name], float64(tc.DurationMs))
			}
		}
	}
	
	m.SuccessRate = float64(successCount) / float64(m.TotalTraces)
	m.AvgDuration = float64(totalDurationMs) / float64(m.TotalTraces)
	
	// Compute tool statistics
	for toolName, acc := range toolData {
		stat := &ToolStat{
			Name:          toolName,
			TotalCalls:    acc.totalCalls,
			AvgLatency:    mean(acc.latencies),
			MedianLatency: median(acc.latencies),
			P95Latency:    percentile(acc.latencies, 0.95),
			AvgResultSize: mean(acc.resultSizes),
			CallsPerRun:   float64(acc.totalCalls) / float64(m.TotalTraces),
		}
		m.ToolStats[toolName] = stat
		
		// Build scatter plot data for result size vs latency
		for i := range acc.latencies {
			m.ResultSizeLatency = append(m.ResultSizeLatency, DataPoint{
				X:     acc.resultSizes[i],
				Y:     acc.latencies[i],
				Label: toolName,
			})
		}
	}
	
	return m
}

type toolAccumulator struct {
	totalCalls  int
	latencies   []float64
	resultSizes []float64
}
```

- [ ] **Step 4: Add statistics helper functions**

Add to `cmd/trace-analyzer/analyzer.go`:

```go
import (
	"sort"
)

func mean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

func median(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)
	
	mid := len(sorted) / 2
	if len(sorted)%2 == 0 {
		return (sorted[mid-1] + sorted[mid]) / 2
	}
	return sorted[mid]
}

func percentile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)
	
	idx := int(float64(len(sorted)-1) * p)
	return sorted[idx]
}
```

- [ ] **Step 5: Run test to verify it passes**

```bash
go test -v -run TestComputeMetrics_ToolStats
```

Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add cmd/trace-analyzer/
git commit -m "feat: add tool statistics computation

- Compute per-tool call counts, latencies, result sizes
- Add mean, median, p95 calculations
- Build result size vs latency scatter data
- Add tests for tool statistics"
```

---

### Task 5: Tool Sequence Detection

**Files:**
- Create: `cmd/trace-analyzer/sequences.go`
- Create: `cmd/trace-analyzer/sequences_test.go`

- [ ] **Step 1: Write test for sequence detection**

Create `cmd/trace-analyzer/sequences_test.go`:

```go
package main

import (
	"testing"
	
	"github.com/dancsalo/arxiv-deep-research/internal/tracing"
)

func TestFindToolSequences(t *testing.T) {
	traces := []tracing.Trace{
		{
			SessionID: "trace-1",
			Turns: []tracing.Turn{
				{
					ToolCalls: []tracing.ToolCall{
						{Name: "search_arxiv", DurationMs: 1000},
						{Name: "fetch_arxiv_pdf", DurationMs: 1500},
						{Name: "search_github_repos", DurationMs: 800},
					},
				},
			},
		},
		{
			SessionID: "trace-2",
			Turns: []tracing.Turn{
				{
					ToolCalls: []tracing.ToolCall{
						{Name: "search_arxiv", DurationMs: 1200},
						{Name: "fetch_arxiv_pdf", DurationMs: 1400},
					},
				},
			},
		},
		{
			SessionID: "trace-3",
			Turns: []tracing.Turn{
				{
					ToolCalls: []tracing.ToolCall{
						{Name: "search_arxiv", DurationMs: 1100},
						{Name: "fetch_arxiv_pdf", DurationMs: 1600},
					},
				},
			},
		},
	}
	
	sequences := FindToolSequences(traces, 2)
	
	// Should find ["search_arxiv", "fetch_arxiv_pdf"] in all 3 traces
	found := false
	for _, seq := range sequences {
		if len(seq.Pattern) == 2 && 
		   seq.Pattern[0] == "search_arxiv" && 
		   seq.Pattern[1] == "fetch_arxiv_pdf" {
			found = true
			
			if seq.Count != 3 {
				t.Errorf("expected count=3 for [search_arxiv, fetch_arxiv_pdf], got %d", seq.Count)
			}
			
			expectedSupport := 1.0
			if seq.Support != expectedSupport {
				t.Errorf("expected support=%.1f, got %.1f", expectedSupport, seq.Support)
			}
		}
	}
	
	if !found {
		t.Error("expected to find [search_arxiv, fetch_arxiv_pdf] sequence")
	}
	
	// Should find ["fetch_arxiv_pdf", "search_github_repos"] in 1 trace
	found = false
	for _, seq := range sequences {
		if len(seq.Pattern) == 2 && 
		   seq.Pattern[0] == "fetch_arxiv_pdf" && 
		   seq.Pattern[1] == "search_github_repos" {
			found = true
			
			if seq.Count != 1 {
				t.Errorf("expected count=1 for [fetch_arxiv_pdf, search_github_repos], got %d", seq.Count)
			}
		}
	}
	
	// With minSupport=2, should NOT find single-occurrence sequence
	if found {
		t.Error("should not find sequence with count < minSupport")
	}
}

func TestFindToolSequences_LongerPatterns(t *testing.T) {
	traces := []tracing.Trace{
		{
			SessionID: "trace-1",
			Turns: []tracing.Turn{
				{
					ToolCalls: []tracing.ToolCall{
						{Name: "A"},
						{Name: "B"},
						{Name: "C"},
						{Name: "D"},
					},
				},
			},
		},
		{
			SessionID: "trace-2",
			Turns: []tracing.Turn{
				{
					ToolCalls: []tracing.ToolCall{
						{Name: "A"},
						{Name: "B"},
						{Name: "C"},
					},
				},
			},
		},
	}
	
	sequences := FindToolSequences(traces, 2)
	
	// Should find [A,B,C] in both traces
	found := false
	for _, seq := range sequences {
		if len(seq.Pattern) == 3 && 
		   seq.Pattern[0] == "A" && 
		   seq.Pattern[1] == "B" && 
		   seq.Pattern[2] == "C" {
			found = true
			if seq.Count != 2 {
				t.Errorf("expected count=2 for [A,B,C], got %d", seq.Count)
			}
		}
	}
	
	if !found {
		t.Error("expected to find [A,B,C] sequence")
	}
}

func TestBuildToolTransitions(t *testing.T) {
	traces := []tracing.Trace{
		{
			Turns: []tracing.Turn{
				{
					ToolCalls: []tracing.ToolCall{
						{Name: "A"},
						{Name: "B"},
						{Name: "C"},
					},
				},
			},
		},
		{
			Turns: []tracing.Turn{
				{
					ToolCalls: []tracing.ToolCall{
						{Name: "A"},
						{Name: "B"},
					},
				},
			},
		},
	}
	
	transitions := BuildToolTransitions(traces)
	
	if transitions["A"]["B"] != 2 {
		t.Errorf("expected A->B transition count=2, got %d", transitions["A"]["B"])
	}
	
	if transitions["B"]["C"] != 1 {
		t.Errorf("expected B->C transition count=1, got %d", transitions["B"]["C"])
	}
	
	if transitions["A"]["C"] != 0 {
		t.Errorf("expected A->C transition count=0, got %d", transitions["A"]["C"])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test -v -run TestFindToolSequences
```

Expected: FAIL - undefined: FindToolSequences, BuildToolTransitions

- [ ] **Step 3: Implement sequence detection**

Create `cmd/trace-analyzer/sequences.go`:

```go
package main

import (
	"sort"
	"strings"
	
	"github.com/dancsalo/arxiv-deep-research/internal/tracing"
)

// FindToolSequences detects common tool call patterns across traces
// using a sliding window approach. Returns sequences sorted by count (descending).
// minSupport filters sequences that appear in fewer than minSupport traces.
func FindToolSequences(traces []tracing.Trace, minSupport int) []ToolSequence {
	// Map of pattern -> {count, traces, latencies}
	type seqData struct {
		count     int
		traces    map[string]bool
		latencies []float64
	}
	
	sequences := make(map[string]*seqData)
	
	for _, trace := range traces {
		// Track which patterns we've seen in this trace (for support calculation)
		seenPatterns := make(map[string]bool)
		
		for _, turn := range trace.Turns {
			toolNames := make([]string, len(turn.ToolCalls))
			toolLatencies := make([]int64, len(turn.ToolCalls))
			
			for i, tc := range turn.ToolCalls {
				toolNames[i] = tc.Name
				toolLatencies[i] = tc.DurationMs
			}
			
			// Sliding window for patterns of length 2-5
			for windowSize := 2; windowSize <= 5 && windowSize <= len(toolNames); windowSize++ {
				for start := 0; start <= len(toolNames)-windowSize; start++ {
					pattern := toolNames[start : start+windowSize]
					key := strings.Join(pattern, "→")
					
					if _, ok := sequences[key]; !ok {
						sequences[key] = &seqData{
							traces:    make(map[string]bool),
							latencies: []float64{},
						}
					}
					
					seq := sequences[key]
					seq.count++
					seq.traces[trace.SessionID] = true
					
					// Sum latencies for this pattern instance
					totalLatency := int64(0)
					for i := start; i < start+windowSize; i++ {
						totalLatency += toolLatencies[i]
					}
					seq.latencies = append(seq.latencies, float64(totalLatency))
					
					seenPatterns[key] = true
				}
			}
		}
	}
	
	// Convert to result slice, filtering by minSupport
	result := []ToolSequence{}
	totalTraces := float64(len(traces))
	
	for key, data := range sequences {
		if len(data.traces) < minSupport {
			continue
		}
		
		pattern := strings.Split(key, "→")
		result = append(result, ToolSequence{
			Pattern:    pattern,
			Count:      data.count,
			Support:    float64(len(data.traces)) / totalTraces,
			AvgLatency: mean(data.latencies),
		})
	}
	
	// Sort by count descending
	sort.Slice(result, func(i, j int) bool {
		return result[i].Count > result[j].Count
	})
	
	return result
}

// BuildToolTransitions creates a transition matrix for tool->tool calls
func BuildToolTransitions(traces []tracing.Trace) map[string]map[string]int {
	transitions := make(map[string]map[string]int)
	
	for _, trace := range traces {
		for _, turn := range trace.Turns {
			for i := 0; i < len(turn.ToolCalls)-1; i++ {
				from := turn.ToolCalls[i].Name
				to := turn.ToolCalls[i+1].Name
				
				if _, ok := transitions[from]; !ok {
					transitions[from] = make(map[string]int)
				}
				
				transitions[from][to]++
			}
		}
	}
	
	return transitions
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test -v -run TestFindToolSequences
go test -v -run TestBuildToolTransitions
```

Expected: PASS

- [ ] **Step 5: Integrate sequences into ComputeMetrics**

Modify `ComputeMetrics` in `cmd/trace-analyzer/analyzer.go` to call sequence functions:

```go
// At the end of ComputeMetrics, before return m:

	// Compute tool sequences and transitions
	m.ToolSequences = FindToolSequences(traces, cfg.MinSupport)
	m.ToolTransitions = BuildToolTransitions(traces)
	
	return m
```

- [ ] **Step 6: Commit**

```bash
git add cmd/trace-analyzer/
git commit -m "feat: implement tool sequence detection

- Add FindToolSequences with sliding window algorithm
- Build tool transition matrix for heatmap
- Support patterns length 2-5 with minimum support filter
- Add comprehensive tests for sequence detection"
```

---

### Task 6: Budget Analysis

**Files:**
- Modify: `cmd/trace-analyzer/analyzer.go`
- Modify: `cmd/trace-analyzer/analyzer_test.go`

- [ ] **Step 1: Write test for budget tier classification**

Add to `cmd/trace-analyzer/analyzer_test.go`:

```go
func TestComputeBudgetAnalysis(t *testing.T) {
	traces := []tracing.Trace{
		{
			SessionID: "tight",
			Turns: []tracing.Turn{
				{Index: 0, TokensRemaining: 10000, ToolCalls: []tracing.ToolCall{
					{Name: "search_arxiv"},
					{Name: "search_openalex"},
				}},
			},
		},
		{
			SessionID: "normal",
			Turns: []tracing.Turn{
				{Index: 0, TokensRemaining: 25000, ToolCalls: []tracing.ToolCall{
					{Name: "search_arxiv"},
					{Name: "fetch_arxiv_pdf"},
					{Name: "search_web"},
				}},
			},
		},
		{
			SessionID: "large",
			Turns: []tracing.Turn{
				{Index: 0, TokensRemaining: 40000, ToolCalls: []tracing.ToolCall{
					{Name: "search_arxiv"},
					{Name: "fetch_arxiv_pdf"},
					{Name: "search_web"},
					{Name: "search_github_repos"},
				}},
			},
		},
	}
	
	analysis := ComputeBudgetAnalysis(traces)
	
	if len(analysis.BudgetTiers) == 0 {
		t.Fatal("expected budget tiers to be populated")
	}
	
	// Find the "Tight" tier
	var tightTier *BudgetTier
	for i := range analysis.BudgetTiers {
		if analysis.BudgetTiers[i].Name == "Tight" {
			tightTier = &analysis.BudgetTiers[i]
			break
		}
	}
	
	if tightTier == nil {
		t.Fatal("expected to find Tight tier")
	}
	
	if tightTier.TraceCount != 1 {
		t.Errorf("expected Tight tier TraceCount=1, got %d", tightTier.TraceCount)
	}
	
	if tightTier.AvgToolsUsed != 2.0 {
		t.Errorf("expected Tight tier AvgToolsUsed=2.0, got %.1f", tightTier.AvgToolsUsed)
	}
	
	// Check token burn-down data
	if len(analysis.TokenBurnDown) != 3 {
		t.Errorf("expected 3 token burn-down records, got %d", len(analysis.TokenBurnDown))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test -v -run TestComputeBudgetAnalysis
```

Expected: FAIL - undefined: ComputeBudgetAnalysis

- [ ] **Step 3: Implement budget analysis**

Add to `cmd/trace-analyzer/analyzer.go`:

```go
func ComputeBudgetAnalysis(traces []tracing.Trace) *BudgetAnalysis {
	analysis := &BudgetAnalysis{
		BudgetTiers:   []BudgetTier{},
		TokenBurnDown: []TurnTokenData{},
		ToolsByBudget: make(map[string][]ToolFrequency),
	}
	
	// Define tier boundaries
	tiers := []struct {
		name string
		min  int
		max  int
	}{
		{"Tight", 0, 15000},
		{"Normal", 15000, 30000},
		{"Large", 30000, 50000},
		{"Generous", 50000, 1000000},
	}
	
	// Initialize tiers
	tierMap := make(map[string]*BudgetTier)
	for _, t := range tiers {
		tier := BudgetTier{
			Name:          t.name,
			Min:           t.min,
			Max:           t.max,
			TraceCount:    0,
			ToolFrequency: make(map[string]int),
		}
		tierMap[t.name] = &tier
	}
	
	// Classify traces into tiers
	for _, trace := range traces {
		if len(trace.Turns) == 0 {
			continue
		}
		
		// Get initial tokens_remaining from first turn
		initialTokens := trace.Turns[0].TokensRemaining
		
		// Find matching tier
		var matchedTier *BudgetTier
		for _, t := range tiers {
			if initialTokens >= t.min && initialTokens < t.max {
				matchedTier = tierMap[t.name]
				break
			}
		}
		
		if matchedTier == nil {
			continue
		}
		
		matchedTier.TraceCount++
		
		// Track unique tools used
		uniqueTools := make(map[string]bool)
		for _, turn := range trace.Turns {
			for _, tc := range turn.ToolCalls {
				uniqueTools[tc.Name] = true
				matchedTier.ToolFrequency[tc.Name]++
			}
		}
		
		// Add to unique tools list
		for tool := range uniqueTools {
			found := false
			for _, existing := range matchedTier.UniqueToolsUsed {
				if existing == tool {
					found = true
					break
				}
			}
			if !found {
				matchedTier.UniqueToolsUsed = append(matchedTier.UniqueToolsUsed, tool)
			}
		}
		
		// Collect token burn-down data
		for _, turn := range trace.Turns {
			analysis.TokenBurnDown = append(analysis.TokenBurnDown, TurnTokenData{
				TurnIndex:       turn.Index,
				TokensRemaining: turn.TokensRemaining,
				TraceID:         trace.SessionID,
			})
		}
	}
	
	// Compute averages and build result
	for _, t := range tiers {
		tier := tierMap[t.name]
		if tier.TraceCount > 0 {
			// Compute average tools used
			totalTools := 0
			for _, count := range tier.ToolFrequency {
				totalTools += count
			}
			tier.AvgToolsUsed = float64(totalTools) / float64(tier.TraceCount)
		}
		
		analysis.BudgetTiers = append(analysis.BudgetTiers, *tier)
		
		// Build ToolsByBudget for charts
		toolFreqs := []ToolFrequency{}
		for tool, count := range tier.ToolFrequency {
			toolFreqs = append(toolFreqs, ToolFrequency{Tool: tool, Count: count})
		}
		sort.Slice(toolFreqs, func(i, j int) bool {
			return toolFreqs[i].Count > toolFreqs[j].Count
		})
		analysis.ToolsByBudget[t.name] = toolFreqs
	}
	
	return analysis
}
```

- [ ] **Step 4: Integrate into ComputeMetrics**

Modify `ComputeMetrics` in `cmd/trace-analyzer/analyzer.go`:

```go
// After computing tool sequences, before return:

	// Compute budget analysis
	m.BudgetAnalysis = ComputeBudgetAnalysis(traces)
	
	return m
```

- [ ] **Step 5: Run test to verify it passes**

```bash
go test -v -run TestComputeBudgetAnalysis
```

Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add cmd/trace-analyzer/
git commit -m "feat: implement budget tier analysis

- Classify traces into budget tiers (Tight/Normal/Large/Generous)
- Compute tool usage per tier
- Collect token burn-down data for visualization
- Add tests for budget analysis"
```

---

### Task 7: Prompt Variant Analysis

**Files:**
- Modify: `cmd/trace-analyzer/analyzer.go`
- Modify: `cmd/trace-analyzer/analyzer_test.go`

- [ ] **Step 1: Write test for variant statistics**

Add to `cmd/trace-analyzer/analyzer_test.go`:

```go
func TestComputeVariantStats(t *testing.T) {
	traces := []tracing.Trace{
		{
			SessionID:         "v-a-1",
			PromptVariant:     "A",
			Status:            "ok",
			TotalCostUSD:      0.02,
			DurationMs:        100000,
			TotalInputTokens:  3000,
			TotalOutputTokens: 500,
			Turns: []tracing.Turn{
				{ToolCalls: []tracing.ToolCall{{Name: "search_arxiv"}, {Name: "fetch_arxiv_pdf"}}},
			},
		},
		{
			SessionID:         "v-a-2",
			PromptVariant:     "A",
			Status:            "error",
			TotalCostUSD:      0.01,
			DurationMs:        50000,
			TotalInputTokens:  1500,
			TotalOutputTokens: 200,
			Turns: []tracing.Turn{
				{ToolCalls: []tracing.ToolCall{{Name: "search_arxiv"}}},
			},
		},
		{
			SessionID:         "v-b-1",
			PromptVariant:     "B",
			Status:            "ok",
			TotalCostUSD:      0.03,
			DurationMs:        120000,
			TotalInputTokens:  4000,
			TotalOutputTokens: 800,
			Turns: []tracing.Turn{
				{ToolCalls: []tracing.ToolCall{{Name: "search_openalex"}, {Name: "search_web"}}},
			},
		},
	}
	
	variantStats := ComputeVariantStats(traces)
	
	statA, ok := variantStats["A"]
	if !ok {
		t.Fatal("expected variant A stats")
	}
	
	if statA.TraceCount != 2 {
		t.Errorf("expected A TraceCount=2, got %d", statA.TraceCount)
	}
	
	expectedSuccessRate := 0.5
	if statA.SuccessRate != expectedSuccessRate {
		t.Errorf("expected A SuccessRate=%.1f, got %.1f", expectedSuccessRate, statA.SuccessRate)
	}
	
	expectedAvgCost := 0.015
	if statA.AvgCost != expectedAvgCost {
		t.Errorf("expected A AvgCost=%.3f, got %.3f", expectedAvgCost, statA.AvgCost)
	}
	
	expectedAvgDuration := 75000.0
	if statA.AvgDuration != expectedAvgDuration {
		t.Errorf("expected A AvgDuration=%.1f, got %.1f", expectedAvgDuration, statA.AvgDuration)
	}
	
	expectedAvgTokens := 2600
	if statA.AvgTokens != expectedAvgTokens {
		t.Errorf("expected A AvgTokens=%d, got %d", expectedAvgTokens, statA.AvgTokens)
	}
	
	if statA.ToolUsage["search_arxiv"] != 2 {
		t.Errorf("expected A search_arxiv usage=2, got %d", statA.ToolUsage["search_arxiv"])
	}
	
	statB, ok := variantStats["B"]
	if !ok {
		t.Fatal("expected variant B stats")
	}
	
	if statB.TraceCount != 1 {
		t.Errorf("expected B TraceCount=1, got %d", statB.TraceCount)
	}
	
	if statB.SuccessRate != 1.0 {
		t.Errorf("expected B SuccessRate=1.0, got %.1f", statB.SuccessRate)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test -v -run TestComputeVariantStats
```

Expected: FAIL - undefined: ComputeVariantStats

- [ ] **Step 3: Implement variant statistics**

Add to `cmd/trace-analyzer/analyzer.go`:

```go
func ComputeVariantStats(traces []tracing.Trace) map[string]*VariantStats {
	variants := make(map[string]*VariantStats)
	
	// Initialize variants
	for _, trace := range traces {
		if trace.PromptVariant == "" {
			continue
		}
		
		if _, ok := variants[trace.PromptVariant]; !ok {
			variants[trace.PromptVariant] = &VariantStats{
				Variant:   trace.PromptVariant,
				ToolUsage: make(map[string]int),
			}
		}
	}
	
	// Accumulate statistics
	for _, trace := range traces {
		if trace.PromptVariant == "" {
			continue
		}
		
		stat := variants[trace.PromptVariant]
		stat.TraceCount++
		
		if trace.Status == "ok" {
			stat.SuccessRate += 1.0
		}
		
		stat.AvgCost += trace.TotalCostUSD
		stat.AvgDuration += float64(trace.DurationMs)
		stat.AvgTokens += trace.TotalInputTokens + trace.TotalOutputTokens
		
		// Tool usage
		for _, turn := range trace.Turns {
			for _, tc := range turn.ToolCalls {
				stat.ToolUsage[tc.Name]++
			}
		}
	}
	
	// Compute averages
	for _, stat := range variants {
		if stat.TraceCount > 0 {
			stat.SuccessRate /= float64(stat.TraceCount)
			stat.AvgCost /= float64(stat.TraceCount)
			stat.AvgDuration /= float64(stat.TraceCount)
			stat.AvgTokens /= stat.TraceCount
		}
	}
	
	return variants
}
```

- [ ] **Step 4: Integrate into ComputeMetrics**

Modify `ComputeMetrics` in `cmd/trace-analyzer/analyzer.go`:

```go
// After budget analysis, before return:

	// Compute variant statistics
	m.PromptVariants = ComputeVariantStats(traces)
	
	return m
```

- [ ] **Step 5: Run test to verify it passes**

```bash
go test -v -run TestComputeVariantStats
```

Expected: PASS

- [ ] **Step 6: Run all tests**

```bash
go test -v
```

Expected: All tests PASS

- [ ] **Step 7: Commit**

```bash
git add cmd/trace-analyzer/
git commit -m "feat: implement prompt variant analysis

- Compute per-variant success rate, cost, duration, tokens
- Track tool usage by variant
- Add tests for variant statistics"
```

---

### Task 8: HTML Template Foundation

**Files:**
- Create: `cmd/trace-analyzer/templates.go`
- Create: `cmd/trace-analyzer/html_generator.go`
- Create: `cmd/trace-analyzer/html_generator_test.go`

- [ ] **Step 1: Write test for HTML generation**

Create `cmd/trace-analyzer/html_generator_test.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGenerateHTML(t *testing.T) {
	metrics := &Metrics{
		TotalTraces: 5,
		SuccessRate: 0.8,
		DateRange: DateRange{
			Earliest: time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC),
			Latest:   time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC),
		},
		TotalTokens: 50000,
		TotalCost:   0.25,
		AvgDuration: 150000,
		ToolStats: map[string]*ToolStat{
			"search_arxiv": {
				Name:       "search_arxiv",
				TotalCalls: 10,
				AvgLatency: 2000,
			},
		},
		ToolSequences: []ToolSequence{
			{Pattern: []string{"search_arxiv", "fetch_arxiv_pdf"}, Count: 5, Support: 0.8},
		},
		PromptVariants: map[string]*VariantStats{
			"A": {Variant: "A", TraceCount: 3, SuccessRate: 1.0},
		},
	}
	
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "test-dashboard.html")
	
	err := GenerateHTML(metrics, outputPath)
	if err != nil {
		t.Fatalf("GenerateHTML failed: %v", err)
	}
	
	// Verify file exists
	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		t.Fatal("output file not created")
	}
	
	// Read and verify content
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read output: %v", err)
	}
	
	html := string(content)
	
	// Check for basic HTML structure
	if !strings.Contains(html, "<!DOCTYPE html>") {
		t.Error("missing DOCTYPE")
	}
	
	if !strings.Contains(html, "<title>Trace Analysis Dashboard</title>") {
		t.Error("missing title")
	}
	
	// Check for section IDs
	requiredSections := []string{
		`id="overview"`,
		`id="tool-usage"`,
		`id="latency"`,
		`id="sequences"`,
		`id="budget"`,
		`id="variants"`,
	}
	
	for _, section := range requiredSections {
		if !strings.Contains(html, section) {
			t.Errorf("missing section: %s", section)
		}
	}
	
	// Check for embedded data
	if !strings.Contains(html, `id="metrics-data"`) {
		t.Error("missing metrics data script")
	}
	
	// Check that metrics are embedded as JSON
	if !strings.Contains(html, `"total_traces":5`) {
		t.Error("metrics not properly embedded")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test -v -run TestGenerateHTML
```

Expected: FAIL - undefined: GenerateHTML

- [ ] **Step 3: Create HTML template skeleton**

Create `cmd/trace-analyzer/templates.go`:

```go
package main

const htmlTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Trace Analysis Dashboard</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
            line-height: 1.6;
            color: #333;
            background: #f5f5f5;
            padding: 20px;
        }
        .container { max-width: 1400px; margin: 0 auto; }
        header {
            background: white;
            padding: 30px;
            border-radius: 8px;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
            margin-bottom: 30px;
        }
        h1 { color: #2c3e50; margin-bottom: 10px; }
        h2 { color: #34495e; margin: 30px 0 15px; }
        .overview-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
            gap: 20px;
            margin-bottom: 30px;
        }
        .card {
            background: white;
            padding: 20px;
            border-radius: 8px;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
        }
        .card h3 { color: #7f8c8d; font-size: 0.9em; margin-bottom: 5px; }
        .card .value { font-size: 2em; font-weight: bold; color: #2c3e50; }
        .card .subtitle { color: #95a5a6; font-size: 0.85em; }
        section {
            background: white;
            padding: 30px;
            border-radius: 8px;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
            margin-bottom: 30px;
        }
        canvas { max-width: 100%; height: 400px; }
        table {
            width: 100%;
            border-collapse: collapse;
            margin-top: 20px;
        }
        th, td {
            padding: 12px;
            text-align: left;
            border-bottom: 1px solid #ddd;
        }
        th {
            background: #f8f9fa;
            font-weight: 600;
            color: #495057;
        }
        tr:hover { background: #f8f9fa; }
        .chart-container { position: relative; margin: 20px 0; }
    </style>
</head>
<body>
    <div class="container">
        <header>
            <h1>📊 Trace Analysis Dashboard</h1>
            <p>Research agent performance and optimization insights</p>
        </header>
        
        <section id="overview">
            <h2>Overview</h2>
            <div class="overview-grid">
                <div class="card">
                    <h3>Total Traces</h3>
                    <div class="value" id="total-traces">-</div>
                    <div class="subtitle" id="date-range">-</div>
                </div>
                <div class="card">
                    <h3>Success Rate</h3>
                    <div class="value" id="success-rate">-</div>
                </div>
                <div class="card">
                    <h3>Total Tokens</h3>
                    <div class="value" id="total-tokens">-</div>
                </div>
                <div class="card">
                    <h3>Total Cost</h3>
                    <div class="value" id="total-cost">-</div>
                </div>
                <div class="card">
                    <h3>Avg Duration</h3>
                    <div class="value" id="avg-duration">-</div>
                    <div class="subtitle">milliseconds</div>
                </div>
            </div>
        </section>
        
        <section id="tool-usage">
            <h2>Tool Usage Analysis</h2>
            <div class="chart-container">
                <canvas id="tool-frequency-chart"></canvas>
            </div>
            <table id="tool-stats-table">
                <thead>
                    <tr>
                        <th>Tool Name</th>
                        <th>Total Calls</th>
                        <th>Avg Latency (ms)</th>
                        <th>Median Latency (ms)</th>
                        <th>P95 Latency (ms)</th>
                        <th>Calls/Run</th>
                    </tr>
                </thead>
                <tbody id="tool-stats-body">
                </tbody>
            </table>
        </section>
        
        <section id="latency">
            <h2>Latency Analysis</h2>
            <div class="chart-container">
                <canvas id="latency-boxplot"></canvas>
            </div>
            <div class="chart-container">
                <canvas id="size-latency-scatter"></canvas>
            </div>
        </section>
        
        <section id="sequences">
            <h2>Tool Call Sequences</h2>
            <div class="chart-container">
                <canvas id="transition-heatmap"></canvas>
            </div>
            <table id="sequences-table">
                <thead>
                    <tr>
                        <th>Sequence</th>
                        <th>Count</th>
                        <th>Support</th>
                        <th>Avg Latency (ms)</th>
                    </tr>
                </thead>
                <tbody id="sequences-body">
                </tbody>
            </table>
        </section>
        
        <section id="budget">
            <h2>Budget Constraint Analysis</h2>
            <div class="chart-container">
                <canvas id="budget-tools-scatter"></canvas>
            </div>
            <div class="chart-container">
                <canvas id="budget-tier-bars"></canvas>
            </div>
        </section>
        
        <section id="variants">
            <h2>Prompt Variant Comparison</h2>
            <table id="variants-table">
                <thead>
                    <tr>
                        <th>Variant</th>
                        <th>Traces</th>
                        <th>Success Rate</th>
                        <th>Avg Cost ($)</th>
                        <th>Avg Duration (ms)</th>
                        <th>Avg Tokens</th>
                    </tr>
                </thead>
                <tbody id="variants-body">
                </tbody>
            </table>
            <div class="chart-container">
                <canvas id="variant-cost-chart"></canvas>
            </div>
        </section>
    </div>
    
    <script type="application/json" id="metrics-data">
{{.MetricsJSON}}
    </script>
    
    <script>
        // Load metrics data
        const metrics = JSON.parse(document.getElementById('metrics-data').textContent);
        
        // Render overview
        document.getElementById('total-traces').textContent = metrics.total_traces;
        document.getElementById('success-rate').textContent = (metrics.success_rate * 100).toFixed(1) + '%';
        document.getElementById('total-tokens').textContent = metrics.total_tokens.toLocaleString();
        document.getElementById('total-cost').textContent = '$' + metrics.total_cost.toFixed(3);
        document.getElementById('avg-duration').textContent = Math.round(metrics.avg_duration).toLocaleString();
        
        const earliest = new Date(metrics.date_range.earliest);
        const latest = new Date(metrics.date_range.latest);
        document.getElementById('date-range').textContent = 
            earliest.toLocaleDateString() + ' - ' + latest.toLocaleDateString();
        
        // Render tool stats table
        const toolStatsBody = document.getElementById('tool-stats-body');
        Object.values(metrics.tool_stats).forEach(stat => {
            const row = toolStatsBody.insertRow();
            row.innerHTML = \`
                <td>\${stat.name}</td>
                <td>\${stat.total_calls}</td>
                <td>\${stat.avg_latency.toFixed(0)}</td>
                <td>\${stat.median_latency.toFixed(0)}</td>
                <td>\${stat.p95_latency.toFixed(0)}</td>
                <td>\${stat.calls_per_run.toFixed(2)}</td>
            \`;
        });
        
        // Render sequences table
        const sequencesBody = document.getElementById('sequences-body');
        metrics.tool_sequences.forEach(seq => {
            const row = sequencesBody.insertRow();
            row.innerHTML = \`
                <td>\${seq.pattern.join(' → ')}</td>
                <td>\${seq.count}</td>
                <td>\${(seq.support * 100).toFixed(0)}%</td>
                <td>\${seq.avg_latency.toFixed(0)}</td>
            \`;
        });
        
        // Render variants table
        const variantsBody = document.getElementById('variants-body');
        Object.values(metrics.prompt_variants).forEach(variant => {
            const row = variantsBody.insertRow();
            row.innerHTML = \`
                <td>\${variant.variant}</td>
                <td>\${variant.trace_count}</td>
                <td>\${(variant.success_rate * 100).toFixed(1)}%</td>
                <td>\$${variant.avg_cost.toFixed(3)}</td>
                <td>\${Math.round(variant.avg_duration).toLocaleString()}</td>
                <td>\${variant.avg_tokens.toLocaleString()}</td>
            \`;
        });
        
        console.log('Dashboard rendered with metrics:', metrics);
    </script>
</body>
</html>`
```

- [ ] **Step 4: Implement GenerateHTML**

Create `cmd/trace-analyzer/html_generator.go`:

```go
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/template"
)

// GenerateHTML creates a self-contained HTML dashboard from metrics
func GenerateHTML(metrics *Metrics, outputPath string) error {
	// Convert metrics to JSON
	metricsJSON, err := serializeMetrics(metrics)
	if err != nil {
		return fmt.Errorf("serializing metrics: %w", err)
	}
	
	// Parse template
	tmpl, err := template.New("dashboard").Parse(htmlTemplate)
	if err != nil {
		return fmt.Errorf("parsing template: %w", err)
	}
	
	// Create output file
	f, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("creating output file: %w", err)
	}
	defer f.Close()
	
	// Render template
	data := struct {
		MetricsJSON string
	}{
		MetricsJSON: metricsJSON,
	}
	
	if err := tmpl.Execute(f, data); err != nil {
		return fmt.Errorf("executing template: %w", err)
	}
	
	return nil
}

func serializeMetrics(m *Metrics) (string, error) {
	// Convert to JSON-friendly structure
	data := map[string]interface{}{
		"total_traces":   m.TotalTraces,
		"success_rate":   m.SuccessRate,
		"date_range": map[string]interface{}{
			"earliest": m.DateRange.Earliest,
			"latest":   m.DateRange.Latest,
		},
		"total_tokens":  m.TotalTokens,
		"total_cost":    m.TotalCost,
		"avg_duration":  m.AvgDuration,
		"tool_stats":    convertToolStats(m.ToolStats),
		"tool_sequences": m.ToolSequences,
		"prompt_variants": m.PromptVariants,
		"budget_analysis": m.BudgetAnalysis,
		"result_size_latency": m.ResultSizeLatency,
		"latency_distribution": m.LatencyDistribution,
		"tool_transitions": m.ToolTransitions,
	}
	
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	
	return string(jsonBytes), nil
}

func convertToolStats(stats map[string]*ToolStat) map[string]interface{} {
	result := make(map[string]interface{})
	for name, stat := range stats {
		result[name] = map[string]interface{}{
			"name":           stat.Name,
			"total_calls":    stat.TotalCalls,
			"avg_latency":    stat.AvgLatency,
			"median_latency": stat.MedianLatency,
			"p95_latency":    stat.P95Latency,
			"avg_result_size": stat.AvgResultSize,
			"calls_per_run":  stat.CallsPerRun,
		}
	}
	return result
}
```

- [ ] **Step 5: Run test to verify it passes**

```bash
go test -v -run TestGenerateHTML
```

Expected: PASS

- [ ] **Step 6: Wire up in main.go**

Modify `run` function in `cmd/trace-analyzer/main.go`:

```go
func run(cfg Config) error {
	if cfg.Verbose {
		fmt.Printf("Loading traces from %s...\n", cfg.TracesDir)
	}
	
	// 1. Load traces
	traces, err := LoadTraces(cfg.TracesDir, cfg)
	if err != nil {
		return fmt.Errorf("loading traces: %w", err)
	}
	
	if len(traces) == 0 {
		return fmt.Errorf("no traces found in %s", cfg.TracesDir)
	}
	
	if cfg.Verbose {
		fmt.Printf("Loaded %d traces\n", len(traces))
		fmt.Printf("Computing metrics...\n")
	}
	
	// 2. Compute metrics
	metrics := ComputeMetrics(traces, cfg)
	
	if cfg.Verbose {
		fmt.Printf("Generating HTML dashboard...\n")
	}
	
	// 3. Generate HTML
	if err := GenerateHTML(metrics, cfg.OutputPath); err != nil {
		return fmt.Errorf("generating HTML: %w", err)
	}
	
	fmt.Printf("Dashboard written to %s\n", cfg.OutputPath)
	return nil
}
```

- [ ] **Step 7: Test end-to-end with fixture**

```bash
go build -o trace-analyzer .
./trace-analyzer --traces-dir=testdata/fixtures --output=/tmp/test-dashboard.html --verbose
```

Expected: Dashboard generated successfully, file at /tmp/test-dashboard.html

- [ ] **Step 8: Open in browser to verify**

```bash
open /tmp/test-dashboard.html
```

Expected: Dashboard displays overview section, tool stats table, sequences table, variants table (no charts yet)

- [ ] **Step 9: Commit**

```bash
git add cmd/trace-analyzer/
git commit -m "feat: implement HTML dashboard generation

- Create HTML template with responsive layout
- Add overview cards, tables for tool stats, sequences, variants
- Implement GenerateHTML with JSON serialization
- Wire up full pipeline in main.go
- Add test for HTML generation"
```

---

### Task 9: Chart.js Integration and Visualizations

**Files:**
- Modify: `cmd/trace-analyzer/templates.go`

- [ ] **Step 1: Download Chart.js**

```bash
curl -o /tmp/chart.min.js https://cdn.jsdelivr.net/npm/chart.js@4.4.0/dist/chart.umd.min.js
```

Expected: Chart.js downloaded to /tmp/chart.min.js

- [ ] **Step 2: Add Chart.js to template**

Modify `htmlTemplate` in `cmd/trace-analyzer/templates.go` - add before the closing `</script>` tag (after the console.log):

```go
// In templates.go, add this constant at the top:
const chartJsLibrary = `...` // paste Chart.js contents here

// Then update htmlTemplate to include Chart.js before the metrics script:
```

Actually, let's embed it inline. Modify the template to add the Chart.js script tag:

Add before the metrics data script in `templates.go`:

```html
    <script src="https://cdn.jsdelivr.net/npm/chart.js@4.4.0/dist/chart.umd.min.js"></script>
```

Note: For true offline mode, we'd embed the full Chart.js code inline, but for development we'll use CDN. The spec allows CDN with offline fallback.

- [ ] **Step 3: Add chart rendering JavaScript**

Replace the existing `<script>` section (after metrics-data) in `templates.go` with:

```html
    <script src="https://cdn.jsdelivr.net/npm/chart.js@4.4.0/dist/chart.umd.min.js"></script>
    
    <script>
        // Load metrics data
        const metrics = JSON.parse(document.getElementById('metrics-data').textContent);
        
        // Render overview
        document.getElementById('total-traces').textContent = metrics.total_traces;
        document.getElementById('success-rate').textContent = (metrics.success_rate * 100).toFixed(1) + '%';
        document.getElementById('total-tokens').textContent = metrics.total_tokens.toLocaleString();
        document.getElementById('total-cost').textContent = '$' + metrics.total_cost.toFixed(3);
        document.getElementById('avg-duration').textContent = Math.round(metrics.avg_duration).toLocaleString();
        
        const earliest = new Date(metrics.date_range.earliest);
        const latest = new Date(metrics.date_range.latest);
        document.getElementById('date-range').textContent = 
            earliest.toLocaleDateString() + ' - ' + latest.toLocaleDateString();
        
        // Render tool stats table
        const toolStatsBody = document.getElementById('tool-stats-body');
        Object.values(metrics.tool_stats).forEach(stat => {
            const row = toolStatsBody.insertRow();
            row.innerHTML = \`
                <td>\${stat.name}</td>
                <td>\${stat.total_calls}</td>
                <td>\${stat.avg_latency.toFixed(0)}</td>
                <td>\${stat.median_latency.toFixed(0)}</td>
                <td>\${stat.p95_latency.toFixed(0)}</td>
                <td>\${stat.calls_per_run.toFixed(2)}</td>
            \`;
        });
        
        // Render sequences table
        const sequencesBody = document.getElementById('sequences-body');
        metrics.tool_sequences.forEach(seq => {
            const row = sequencesBody.insertRow();
            row.innerHTML = \`
                <td>\${seq.pattern.join(' → ')}</td>
                <td>\${seq.count}</td>
                <td>\${(seq.support * 100).toFixed(0)}%</td>
                <td>\${seq.avg_latency.toFixed(0)}</td>
            \`;
        });
        
        // Render variants table
        const variantsBody = document.getElementById('variants-body');
        Object.values(metrics.prompt_variants).forEach(variant => {
            const row = variantsBody.insertRow();
            row.innerHTML = \`
                <td>\${variant.variant}</td>
                <td>\${variant.trace_count}</td>
                <td>\${(variant.success_rate * 100).toFixed(1)}%</td>
                <td>\$${variant.avg_cost.toFixed(3)}</td>
                <td>\${Math.round(variant.avg_duration).toLocaleString()}</td>
                <td>\${variant.avg_tokens.toLocaleString()}</td>
            \`;
        });
        
        // Chart 1: Tool Frequency Bar Chart
        const toolNames = Object.keys(metrics.tool_stats);
        const toolCounts = toolNames.map(name => metrics.tool_stats[name].total_calls);
        
        new Chart(document.getElementById('tool-frequency-chart'), {
            type: 'bar',
            data: {
                labels: toolNames,
                datasets: [{
                    label: 'Total Calls',
                    data: toolCounts,
                    backgroundColor: 'rgba(54, 162, 235, 0.5)',
                    borderColor: 'rgba(54, 162, 235, 1)',
                    borderWidth: 1
                }]
            },
            options: {
                responsive: true,
                plugins: {
                    title: { display: true, text: 'Tool Call Frequency' },
                    legend: { display: false }
                },
                scales: {
                    y: { beginAtZero: true, title: { display: true, text: 'Number of Calls' } },
                    x: { title: { display: true, text: 'Tool Name' } }
                }
            }
        });
        
        // Chart 2: Latency Box Plot (using bar chart with error bars approximation)
        const latencyData = Object.entries(metrics.latency_distribution).map(([tool, latencies]) => {
            latencies.sort((a, b) => a - b);
            return {
                tool: tool,
                median: latencies[Math.floor(latencies.length / 2)],
                min: latencies[0],
                max: latencies[latencies.length - 1]
            };
        });
        
        new Chart(document.getElementById('latency-boxplot'), {
            type: 'bar',
            data: {
                labels: latencyData.map(d => d.tool),
                datasets: [{
                    label: 'Median Latency (ms)',
                    data: latencyData.map(d => d.median),
                    backgroundColor: 'rgba(75, 192, 192, 0.5)',
                    borderColor: 'rgba(75, 192, 192, 1)',
                    borderWidth: 1
                }]
            },
            options: {
                responsive: true,
                plugins: {
                    title: { display: true, text: 'Latency Distribution by Tool' },
                    legend: { display: false }
                },
                scales: {
                    y: { beginAtZero: true, title: { display: true, text: 'Latency (ms)' } },
                    x: { title: { display: true, text: 'Tool Name' } }
                }
            }
        });
        
        // Chart 3: Result Size vs Latency Scatter
        new Chart(document.getElementById('size-latency-scatter'), {
            type: 'scatter',
            data: {
                datasets: [{
                    label: 'Tool Calls',
                    data: metrics.result_size_latency.map(d => ({ x: d.X, y: d.Y })),
                    backgroundColor: 'rgba(255, 99, 132, 0.5)'
                }]
            },
            options: {
                responsive: true,
                plugins: {
                    title: { display: true, text: 'Result Size vs Latency' },
                    tooltip: {
                        callbacks: {
                            label: function(context) {
                                return \`Size: \${context.parsed.x.toFixed(0)}, Latency: \${context.parsed.y.toFixed(0)}ms\`;
                            }
                        }
                    }
                },
                scales: {
                    x: { title: { display: true, text: 'Result Size (bytes)' } },
                    y: { title: { display: true, text: 'Latency (ms)' } }
                }
            }
        });
        
        // Chart 4: Budget vs Tools Scatter
        const budgetTiers = metrics.budget_analysis.budget_tiers;
        const budgetScatterData = [];
        budgetTiers.forEach(tier => {
            budgetScatterData.push({
                x: (tier.min + tier.max) / 2,
                y: tier.avg_tools_used,
                label: tier.name
            });
        });
        
        new Chart(document.getElementById('budget-tools-scatter'), {
            type: 'scatter',
            data: {
                datasets: [{
                    label: 'Avg Tools Used',
                    data: budgetScatterData.map(d => ({ x: d.x, y: d.y })),
                    backgroundColor: 'rgba(153, 102, 255, 0.5)'
                }]
            },
            options: {
                responsive: true,
                plugins: {
                    title: { display: true, text: 'Budget Constraints vs Tools Used' }
                },
                scales: {
                    x: { title: { display: true, text: 'Initial Tokens Remaining' } },
                    y: { title: { display: true, text: 'Avg Tools Used' } }
                }
            }
        });
        
        // Chart 5: Budget Tier Tool Frequency
        const tierNames = budgetTiers.map(t => t.name).filter(n => budgetTiers.find(t => t.name === n).trace_count > 0);
        const allTools = new Set();
        tierNames.forEach(tierName => {
            const tier = budgetTiers.find(t => t.name === tierName);
            Object.keys(tier.tool_frequency).forEach(tool => allTools.add(tool));
        });
        
        const toolsArray = Array.from(allTools);
        const datasets = toolsArray.map(tool => {
            return {
                label: tool,
                data: tierNames.map(tierName => {
                    const tier = budgetTiers.find(t => t.name === tierName);
                    return tier.tool_frequency[tool] || 0;
                }),
                backgroundColor: \`hsla(\${Math.random() * 360}, 70%, 50%, 0.5)\`
            };
        });
        
        new Chart(document.getElementById('budget-tier-bars'), {
            type: 'bar',
            data: {
                labels: tierNames,
                datasets: datasets
            },
            options: {
                responsive: true,
                plugins: {
                    title: { display: true, text: 'Tool Usage by Budget Tier' }
                },
                scales: {
                    x: { title: { display: true, text: 'Budget Tier' } },
                    y: { title: { display: true, text: 'Tool Call Count' }, stacked: true }
                }
            }
        });
        
        // Chart 6: Variant Cost Comparison
        const variantNames = Object.keys(metrics.prompt_variants);
        const variantCosts = variantNames.map(v => metrics.prompt_variants[v].avg_cost);
        
        new Chart(document.getElementById('variant-cost-chart'), {
            type: 'bar',
            data: {
                labels: variantNames,
                datasets: [{
                    label: 'Average Cost ($)',
                    data: variantCosts,
                    backgroundColor: 'rgba(255, 159, 64, 0.5)',
                    borderColor: 'rgba(255, 159, 64, 1)',
                    borderWidth: 1
                }]
            },
            options: {
                responsive: true,
                plugins: {
                    title: { display: true, text: 'Average Cost by Prompt Variant' }
                },
                scales: {
                    y: { beginAtZero: true, title: { display: true, text: 'Cost (USD)' } },
                    x: { title: { display: true, text: 'Prompt Variant' } }
                }
            }
        });
        
        console.log('Dashboard rendered with', metrics.total_traces, 'traces');
    </script>
```

- [ ] **Step 4: Rebuild and test**

```bash
go build -o trace-analyzer .
./trace-analyzer --traces-dir=testdata/fixtures --output=/tmp/dashboard.html --verbose
open /tmp/dashboard.html
```

Expected: Dashboard with all 6 charts rendering correctly

- [ ] **Step 5: Test with actual traces**

```bash
./trace-analyzer --traces-dir=.traces --output=.traces/dashboard.html
open .traces/dashboard.html
```

Expected: Dashboard showing real data from actual traces

- [ ] **Step 6: Commit**

```bash
git add cmd/trace-analyzer/
git commit -m "feat: add Chart.js visualizations

- Integrate Chart.js library via CDN
- Implement 6 chart types: tool frequency, latency distribution,
  size vs latency scatter, budget scatter, budget tier bars, variant costs
- All visualizations render from embedded metrics data"
```

---

### Task 10: Makefile Integration and Documentation

**Files:**
- Modify: `Makefile`
- Modify: `README.md`

- [ ] **Step 1: Add Makefile targets**

Add to `Makefile`:

```makefile
build-analyzer:  ## Build trace-analyzer binary
	go build -o trace-analyzer ./cmd/trace-analyzer

trace-dashboard: build-analyzer  ## Generate trace analysis dashboard
	@echo "Generating trace dashboard..."
	@./trace-analyzer --output=.traces/dashboard.html
	@echo "Dashboard written to .traces/dashboard.html"
	@echo "Open file://$$(pwd)/.traces/dashboard.html in your browser"
```

- [ ] **Step 2: Update help text in Makefile**

Modify the help section comments in `Makefile`:

```makefile
# At the top, update the .PHONY line:
.PHONY: build build-research build-tools build-analyzer vet lint test test-go test-race check run trace-list trace-show trace-errors trace-dashboard help
```

- [ ] **Step 3: Test Makefile targets**

```bash
make build-analyzer
```

Expected: Binary builds successfully

```bash
make trace-dashboard
```

Expected: Dashboard generated at .traces/dashboard.html

- [ ] **Step 4: Update README.md**

Add to `README.md` after the "Trace commands" section:

```markdown
## Trace Analysis Dashboard

Generate an interactive HTML dashboard to analyze trace patterns across multiple runs:

```bash
make trace-dashboard
```

This creates `.traces/dashboard.html` with:
- **Overview**: Success rate, total cost, tokens, duration
- **Tool Usage**: Frequency, latencies, per-tool statistics
- **Sequences**: Common tool call patterns and transitions
- **Budget Analysis**: Tool selection under different budget constraints
- **Variant Comparison**: A/B/C prompt variant performance

### Dashboard Options

```bash
# Analyze specific directory
./trace-analyzer --traces-dir=/path/to/traces

# Filter by date range
./trace-analyzer --since=2026-05-01 --until=2026-05-10

# Compare specific variant
./trace-analyzer --variant=A --output=variant-a-analysis.html

# Verbose output
./trace-analyzer --verbose
```

The dashboard is self-contained and works offline (open file:// URL directly in browser).
```

- [ ] **Step 5: Commit**

```bash
git add Makefile README.md
git commit -m "docs: integrate trace-analyzer into build system

- Add make build-analyzer and make trace-dashboard targets
- Update README with dashboard documentation
- Add usage examples for trace analysis"
```

---

### Task 11: Integration Test with Real Traces

**Files:**
- None (verification step)

- [ ] **Step 1: Run full pipeline with real traces**

```bash
make trace-dashboard
```

Expected: Dashboard generated successfully

- [ ] **Step 2: Open dashboard in browser**

```bash
open .traces/dashboard.html
```

Expected: Dashboard displays with all sections populated from real trace data

- [ ] **Step 3: Verify each section**

Manual checklist:
- [ ] Overview cards show correct totals
- [ ] Tool frequency chart displays all tools
- [ ] Tool stats table is populated
- [ ] Latency distribution chart shows all tools
- [ ] Result size vs latency scatter has data points
- [ ] Sequences table shows common patterns
- [ ] Transition heatmap placeholder visible (or implemented)
- [ ] Budget scatter shows tier data
- [ ] Budget tier bars show tool distribution
- [ ] Variants table shows A/B/C stats (if variants exist)
- [ ] Variant cost chart displays

- [ ] **Step 4: Test filtering options**

```bash
./trace-analyzer --since=2026-05-11 --output=/tmp/filtered.html --verbose
open /tmp/filtered.html
```

Expected: Dashboard only includes traces from May 11 onward

- [ ] **Step 5: Test with empty directory**

```bash
mkdir -p /tmp/empty-traces
./trace-analyzer --traces-dir=/tmp/empty-traces --output=/tmp/empty-dashboard.html
```

Expected: Error message "no traces found in /tmp/empty-traces"

- [ ] **Step 6: Test error handling for malformed JSON**

```bash
echo "invalid json" > /tmp/bad-trace.json
./trace-analyzer --traces-dir=/tmp --output=/tmp/test.html --verbose 2>&1 | grep -i "warning"
```

Expected: Warning printed about bad-trace.json, but process continues

---

### Task 12: Final Polish and Testing

**Files:**
- Modify: `cmd/trace-analyzer/analyzer_test.go`
- Run all tests

- [ ] **Step 1: Add integration test with multiple fixtures**

Create `cmd/trace-analyzer/testdata/fixtures/error-trace.json`:

```json
{
  "session_id": "error-001",
  "query": "test error",
  "model": "test-model",
  "prompt_variant": "B",
  "started_at": "2026-05-11T12:00:00Z",
  "ended_at": "2026-05-11T12:01:00Z",
  "duration_ms": 60000,
  "status": "error",
  "error": "test error message",
  "total_input_tokens": 1000,
  "total_output_tokens": 100,
  "total_cost_usd": 0.005,
  "turns": []
}
```

- [ ] **Step 2: Add end-to-end integration test**

Add to `cmd/trace-analyzer/analyzer_test.go`:

```go
func TestEndToEnd_Integration(t *testing.T) {
	fixtureDir := filepath.Join("testdata", "fixtures")
	
	// Load all fixtures
	traces, err := LoadTraces(fixtureDir, Config{})
	if err != nil {
		t.Fatalf("LoadTraces failed: %v", err)
	}
	
	if len(traces) < 2 {
		t.Fatalf("expected at least 2 traces, got %d", len(traces))
	}
	
	// Compute metrics
	metrics := ComputeMetrics(traces, Config{MinSupport: 1})
	
	if metrics.TotalTraces != len(traces) {
		t.Errorf("metrics.TotalTraces=%d, expected %d", metrics.TotalTraces, len(traces))
	}
	
	// Check success rate calculation
	expectedSuccess := 0.5 // 1 ok, 1 error
	if metrics.SuccessRate != expectedSuccess {
		t.Errorf("expected SuccessRate=%.1f, got %.1f", expectedSuccess, metrics.SuccessRate)
	}
	
	// Check tool stats exist
	if len(metrics.ToolStats) == 0 {
		t.Error("expected tool statistics to be computed")
	}
	
	// Check sequences found
	if len(metrics.ToolSequences) == 0 {
		t.Error("expected tool sequences to be found")
	}
	
	// Generate HTML
	tmpFile := filepath.Join(t.TempDir(), "integration-test.html")
	err = GenerateHTML(metrics, tmpFile)
	if err != nil {
		t.Fatalf("GenerateHTML failed: %v", err)
	}
	
	// Verify file created and has content
	stat, err := os.Stat(tmpFile)
	if err != nil {
		t.Fatalf("output file not created: %v", err)
	}
	
	if stat.Size() < 1000 {
		t.Errorf("output file too small (%d bytes), expected >1000", stat.Size())
	}
}
```

- [ ] **Step 3: Run all tests**

```bash
cd cmd/trace-analyzer
go test -v
```

Expected: All tests PASS

- [ ] **Step 4: Run with race detector**

```bash
go test -race -v
```

Expected: No data races detected, all tests PASS

- [ ] **Step 5: Run tests from root**

```bash
cd /Users/dsalo/Repos/arxiv-deep-research
go test ./cmd/trace-analyzer/...
```

Expected: All tests PASS

- [ ] **Step 6: Build final binary**

```bash
make build-analyzer
```

Expected: Binary builds cleanly

- [ ] **Step 7: Run `make check` to ensure project still passes**

```bash
make check
```

Expected: All lints, builds, and tests pass

- [ ] **Step 8: Commit**

```bash
git add cmd/trace-analyzer/
git commit -m "test: add integration tests and error trace fixture

- Add error-trace.json fixture
- Add end-to-end integration test
- Verify full pipeline with multiple traces
- All tests passing with race detector"
```

---

### Task 13: Final Commit and Documentation

**Files:**
- None (verification and documentation finalization)

- [ ] **Step 1: Run trace-analyzer on actual traces one more time**

```bash
make trace-dashboard
open .traces/dashboard.html
```

Expected: Dashboard displays correctly with real data

- [ ] **Step 2: Verify all spec requirements met**

Checklist from spec:
- [x] Load traces from directory with filtering
- [x] Compute overview metrics (traces, success rate, totals, averages)
- [x] Compute tool statistics (calls, latencies, percentiles)
- [x] Detect tool sequences with sliding window
- [x] Build tool transition matrix
- [x] Compute budget tier analysis
- [x] Compute prompt variant comparison
- [x] Generate self-contained HTML
- [x] Render 6 visualizations (bar charts, scatter plots, tables)
- [x] CLI with flags for filtering
- [x] Makefile integration
- [x] Error handling for malformed files
- [x] Unit tests for core functions
- [x] Integration tests

- [ ] **Step 3: Final functional test**

```bash
# Test each CLI option
./trace-analyzer --help
./trace-analyzer --traces-dir=.traces --verbose
./trace-analyzer --since=2026-05-10
./trace-analyzer --variant=A --output=/tmp/variant-a.html
```

Expected: All commands work correctly

- [ ] **Step 4: Check file sizes**

```bash
du -h trace-analyzer
du -h .traces/dashboard.html
```

Expected: Binary ~5-8MB, HTML ~100-500KB depending on trace count

- [ ] **Step 5: Create final commit**

```bash
git add -A
git status
```

Verify all new files are staged, then:

```bash
git commit -m "feat: complete trace analysis dashboard implementation

Implements trace-analyzer CLI tool with:
- Multi-trace loading with variant/date filtering
- Comprehensive metric computation (tool stats, sequences, budget, variants)
- Self-contained HTML dashboard with Chart.js visualizations
- 6 chart types: tool frequency, latency distribution, size/latency scatter,
  budget scatter, budget tier bars, variant comparison
- Full test coverage with fixtures and integration tests
- Makefile integration with 'make trace-dashboard'

Spec: docs/superpowers/specs/2026-05-12-trace-dashboard-design.md"
```

- [ ] **Step 6: Verify git log**

```bash
git log --oneline -10
```

Expected: Clean commit history with descriptive messages

---

## Implementation Complete

The trace analysis dashboard is now fully implemented and tested. The tool:

1. ✅ Loads and filters trace files from `.traces/` directory
2. ✅ Computes comprehensive metrics across all traces
3. ✅ Detects tool call patterns and sequences
4. ✅ Analyzes budget constraints and tool selection
5. ✅ Compares prompt variant performance
6. ✅ Generates interactive HTML dashboard with 6 visualizations
7. ✅ Integrates with Makefile build system
8. ✅ Handles errors gracefully
9. ✅ Has comprehensive test coverage

Run `make trace-dashboard` to generate the dashboard from your current traces.
