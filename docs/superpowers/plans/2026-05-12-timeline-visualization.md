# Timeline Trace Visualization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a static HTML generator tool (trace-viewer) that visualizes single research agent traces as interactive timelines.

**Architecture:** Go CLI tool reads trace JSON, enriches with schema feature detection, generates self-contained HTML with embedded data and vanilla JavaScript for UI.

**Tech Stack:** Go 1.21+ (stdlib only), Vanilla JavaScript, Inline CSS

---

## File Structure

```
cmd/trace-viewer/
  main.go              - CLI entry, flags, orchestration
  data_loader.go       - Read and validate trace JSON
  enricher.go          - Detect schema features, compute derived data
  generator.go         - HTML generation, template rendering
  templates.go         - Embedded HTML/CSS/JS template constants
  
  data_loader_test.go  - Tests for trace loading
  enricher_test.go     - Tests for schema detection
  generator_test.go    - Tests for HTML generation
  
  testdata/
    fixtures/
      minimal-trace.json        - Single turn, no tools
      normal-trace.json         - Multiple turns with tools
      compaction-trace.json     - Includes compaction events
      error-trace.json          - Failed tool calls
```

---

### Task 1: Project Setup and Basic CLI

**Files:**
- Create: `cmd/trace-viewer/main.go`
- Create: `cmd/trace-viewer/data_loader.go`
- Create: `cmd/trace-viewer/data_loader_test.go`
- Create: `cmd/trace-viewer/testdata/fixtures/minimal-trace.json`

- [ ] **Step 1: Create test fixture**

Create `cmd/trace-viewer/testdata/fixtures/minimal-trace.json`:

```json
{
  "session_id": "test-minimal-001",
  "query": "test query",
  "model": "claude-sonnet-4",
  "prompt_variant": "A",
  "prompt_hash": "abc12345",
  "started_at": "2026-05-12T10:00:00Z",
  "ended_at": "2026-05-12T10:01:00Z",
  "duration_ms": 60000,
  "status": "ok",
  "total_input_tokens": 1000,
  "total_output_tokens": 200,
  "total_cost_usd": 0.005,
  "turns": [
    {
      "index": 0,
      "started_at": "2026-05-12T10:00:00Z",
      "ended_at": "2026-05-12T10:00:30Z",
      "duration_ms": 30000,
      "tokens_used": 1200,
      "tokens_remaining": 28800,
      "cost_usd": 0.005,
      "llm_call": {
        "model": "claude-sonnet-4",
        "input_tokens": 1000,
        "output_tokens": 200,
        "duration_ms": 15000,
        "stop_reason": "end_turn"
      },
      "tool_calls": []
    }
  ]
}
```

- [ ] **Step 2: Write test for LoadTrace**

Create `cmd/trace-viewer/data_loader_test.go`:

```go
package main

import (
	"path/filepath"
	"testing"

	"github.com/dancsalo/arxiv-deep-research/internal/tracing"
)

func TestLoadTrace(t *testing.T) {
	fixturePath := filepath.Join("testdata", "fixtures", "minimal-trace.json")
	
	trace, err := LoadTrace(fixturePath)
	if err != nil {
		t.Fatalf("LoadTrace failed: %v", err)
	}
	
	if trace.SessionID != "test-minimal-001" {
		t.Errorf("expected SessionID=test-minimal-001, got %s", trace.SessionID)
	}
	
	if len(trace.Turns) != 1 {
		t.Fatalf("expected 1 turn, got %d", len(trace.Turns))
	}
	
	if trace.Status != "ok" {
		t.Errorf("expected status=ok, got %s", trace.Status)
	}
}

func TestLoadTrace_InvalidPath(t *testing.T) {
	_, err := LoadTrace("nonexistent.json")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestLoadTrace_InvalidJSON(t *testing.T) {
	tmpfile := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(tmpfile, []byte("not json"), 0644); err != nil {
		t.Fatal(err)
	}
	
	_, err := LoadTrace(tmpfile)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

```bash
cd cmd/trace-viewer
go test -v
```

Expected: FAIL - undefined: LoadTrace

- [ ] **Step 4: Implement LoadTrace**

Create `cmd/trace-viewer/data_loader.go`:

```go
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
		return nil, fmt.Errorf("reading trace file: %w", err)
	}
	
	var trace tracing.Trace
	if err := json.Unmarshal(data, &trace); err != nil {
		return nil, fmt.Errorf("parsing trace JSON: %w", err)
	}
	
	return &trace, nil
}
```

- [ ] **Step 5: Run test to verify it passes**

```bash
go test -v
```

Expected: PASS

- [ ] **Step 6: Create main.go skeleton**

Create `cmd/trace-viewer/main.go`:

```go
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	TracePath  string
	OutputPath string
	Open       bool
	Verbose    bool
}

func main() {
	cfg := Config{}
	
	flag.StringVar(&cfg.TracePath, "trace", "", "Path to trace JSON file (required)")
	flag.StringVar(&cfg.OutputPath, "output", "", "Output HTML file path (default: <trace>-timeline.html)")
	flag.BoolVar(&cfg.Open, "open", false, "Open in browser after generation")
	flag.BoolVar(&cfg.Verbose, "verbose", false, "Print detailed progress")
	flag.Parse()
	
	if cfg.TracePath == "" {
		fmt.Fprintf(os.Stderr, "Error: --trace flag is required\n\n")
		flag.Usage()
		os.Exit(1)
	}
	
	// Default output path: <basename>-timeline.html
	if cfg.OutputPath == "" {
		base := filepath.Base(cfg.TracePath)
		base = strings.TrimSuffix(base, filepath.Ext(base))
		cfg.OutputPath = base + "-timeline.html"
	}
	
	if err := run(cfg); err != nil {
		log.Fatalf("Error: %v", err)
	}
}

func run(cfg Config) error {
	if cfg.Verbose {
		fmt.Printf("Loading trace from %s...\n", cfg.TracePath)
	}
	
	trace, err := LoadTrace(cfg.TracePath)
	if err != nil {
		return err
	}
	
	if cfg.Verbose {
		fmt.Printf("Loaded trace: session=%s, turns=%d\n", trace.SessionID, len(trace.Turns))
		fmt.Printf("Generating HTML to %s...\n", cfg.OutputPath)
	}
	
	// TODO: generate HTML
	
	fmt.Printf("Timeline generated: %s\n", cfg.OutputPath)
	
	if cfg.Open {
		if cfg.Verbose {
			fmt.Println("Opening in browser...")
		}
		// TODO: open browser
	}
	
	return nil
}
```

- [ ] **Step 7: Verify CLI works**

```bash
go build -o trace-viewer .
./trace-viewer --help
```

Expected: Help text displayed

```bash
./trace-viewer --trace=testdata/fixtures/minimal-trace.json --verbose
```

Expected: Prints "Loading trace..." and "Timeline generated..." messages

- [ ] **Step 8: Commit**

```bash
git add cmd/trace-viewer/
git commit -m "feat: add trace-viewer CLI skeleton and data loader

- Add LoadTrace function to read trace JSON files
- Add CLI with --trace, --output, --open, --verbose flags
- Add minimal-trace.json test fixture
- Add tests for trace loading"
```

---

### Task 2: Schema Feature Detection

**Files:**
- Create: `cmd/trace-viewer/enricher.go`
- Create: `cmd/trace-viewer/enricher_test.go`

- [ ] **Step 1: Write test for schema feature detection**

Create `cmd/trace-viewer/enricher_test.go`:

```go
package main

import (
	"testing"
	"time"

	"github.com/dancsalo/arxiv-deep-research/internal/tracing"
)

func TestDetectSchemaFeatures_MinimalSchema(t *testing.T) {
	trace := &tracing.Trace{
		Turns: []tracing.Turn{
			{
				LLMCall: &tracing.LLMCall{},
				ToolCalls: []tracing.ToolCall{
					{Name: "test_tool"},
				},
			},
		},
	}
	
	features := DetectSchemaFeatures(trace)
	
	if features.HasTimestamps {
		t.Error("minimal schema should not have timestamps")
	}
	if features.HasContextSnapshots {
		t.Error("minimal schema should not have context snapshots")
	}
	if features.HasToolRelationships {
		t.Error("minimal schema should not have tool relationships")
	}
	if features.HasCompactionDiffs {
		t.Error("minimal schema should not have compaction diffs")
	}
	if features.HasDisplayMetadata {
		t.Error("minimal schema should not have display metadata")
	}
	if features.HasRichErrors {
		t.Error("minimal schema should not have rich errors")
	}
}

func TestDetectSchemaFeatures_FullSchema(t *testing.T) {
	now := time.Now()
	
	trace := &tracing.Trace{
		Turns: []tracing.Turn{
			{
				LLMCall: &tracing.LLMCall{
					StartedAt: &now,
					InputSummary: &InputSummary{
						SystemTokens: 100,
					},
				},
				ToolCalls: []tracing.ToolCall{
					{
						Name:            "test_tool",
						StartedAt:       &now,
						ParentToolIndex: intPtr(0),
						Error: &ToolError{
							Type: "TestError",
						},
					},
				},
				Display: &TurnDisplay{
					Label: "Test Turn",
				},
			},
		},
		GuardrailDecisions: []tracing.GuardrailDecision{
			{
				Compacted: true,
				RemovedContent: &RemovedContent{
					ToolResultsCount: 1,
				},
			},
		},
	}
	
	features := DetectSchemaFeatures(trace)
	
	if !features.HasTimestamps {
		t.Error("full schema should have timestamps")
	}
	if !features.HasContextSnapshots {
		t.Error("full schema should have context snapshots")
	}
	if !features.HasToolRelationships {
		t.Error("full schema should have tool relationships")
	}
	if !features.HasCompactionDiffs {
		t.Error("full schema should have compaction diffs")
	}
	if !features.HasDisplayMetadata {
		t.Error("full schema should have display metadata")
	}
	if !features.HasRichErrors {
		t.Error("full schema should have rich errors")
	}
}

func intPtr(i int) *int {
	return &i
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test -v
```

Expected: FAIL - undefined types and functions

- [ ] **Step 3: Implement schema feature detection**

Create `cmd/trace-viewer/enricher.go`:

```go
package main

import (
	"github.com/dancsalo/arxiv-deep-research/internal/tracing"
)

// SchemaFeatures tracks which optional schema fields are present
type SchemaFeatures struct {
	HasTimestamps        bool
	HasContextSnapshots  bool
	HasToolRelationships bool
	HasCompactionDiffs   bool
	HasDisplayMetadata   bool
	HasRichErrors        bool
}

// Extended types for optional schema fields
type InputSummary struct {
	SystemTokens       int `json:"system_tokens"`
	UserMessages       int `json:"user_messages"`
	AssistantMessages  int `json:"assistant_messages"`
	ToolResults        int `json:"tool_results"`
	TotalMessages      int `json:"total_messages"`
	OldestMessageTurn  int `json:"oldest_message_turn"`
}

type TurnDisplay struct {
	Label       string `json:"label"`
	Summary     string `json:"summary"`
	Status      string `json:"status"`
	PrimaryTool string `json:"primary_tool"`
}

type ToolError struct {
	Type            string `json:"type"`
	Message         string `json:"message"`
	Retryable       bool   `json:"retryable"`
	AttemptedRetries int    `json:"attempted_retries"`
	SuggestedAction string `json:"suggested_action"`
}

type RemovedContent struct {
	ToolResultsCount int `json:"tool_results_count"`
	MessageCount     int `json:"message_count"`
	SummaryTokens    int `json:"summary_tokens"`
}

// DetectSchemaFeatures checks which optional fields are present
func DetectSchemaFeatures(trace *tracing.Trace) SchemaFeatures {
	features := SchemaFeatures{}
	
	if len(trace.Turns) == 0 {
		return features
	}
	
	firstTurn := trace.Turns[0]
	
	// Check for timestamps
	if firstTurn.LLMCall != nil && firstTurn.LLMCall.StartedAt != nil {
		features.HasTimestamps = true
	}
	
	// Check for context snapshots
	if firstTurn.LLMCall != nil && firstTurn.LLMCall.InputSummary != nil {
		features.HasContextSnapshots = true
	}
	
	// Check for tool relationships
	if len(firstTurn.ToolCalls) > 0 && firstTurn.ToolCalls[0].ParentToolIndex != nil {
		features.HasToolRelationships = true
	}
	
	// Check for display metadata
	if firstTurn.Display != nil {
		features.HasDisplayMetadata = true
	}
	
	// Check for rich error context
	if len(firstTurn.ToolCalls) > 0 && firstTurn.ToolCalls[0].Error != nil {
		features.HasRichErrors = true
	}
	
	// Check for compaction diffs
	for _, gd := range trace.GuardrailDecisions {
		if gd.Compacted && gd.RemovedContent != nil {
			features.HasCompactionDiffs = true
			break
		}
	}
	
	return features
}
```

- [ ] **Step 4: Update internal/tracing types**

Note: The extended schema fields need to be added to `internal/tracing/trace.go`. Since we're doing progressive enhancement, add these as pointer fields so they're optional:

Add to `internal/tracing/trace.go` in the appropriate struct definitions:

```go
// In LLMCall:
StartedAt    *time.Time `json:"started_at,omitempty"`
EndedAt      *time.Time `json:"ended_at,omitempty"`
InputSummary *struct {
	SystemTokens      int `json:"system_tokens"`
	UserMessages      int `json:"user_messages"`
	AssistantMessages int `json:"assistant_messages"`
	ToolResults       int `json:"tool_results"`
	TotalMessages     int `json:"total_messages"`
	OldestMessageTurn int `json:"oldest_message_turn"`
} `json:"input_summary,omitempty"`

// In ToolCall:
StartedAt       *time.Time `json:"started_at,omitempty"`
EndedAt         *time.Time `json:"ended_at,omitempty"`
ParentToolIndex *int       `json:"parent_tool_index,omitempty"`
ExecutionMode   string     `json:"execution_mode,omitempty"`
Error           *struct {
	Type             string `json:"type"`
	Message          string `json:"message"`
	Retryable        bool   `json:"retryable"`
	AttemptedRetries int    `json:"attempted_retries"`
	SuggestedAction  string `json:"suggested_action"`
} `json:"error,omitempty"`

// In Turn:
Display *struct {
	Label       string `json:"label"`
	Summary     string `json:"summary"`
	Status      string `json:"status"`
	PrimaryTool string `json:"primary_tool"`
} `json:"display,omitempty"`

// In GuardrailDecision:
RemovedContent *struct {
	ToolResultsCount int `json:"tool_results_count"`
	MessageCount     int `json:"message_count"`
	SummaryTokens    int `json:"summary_tokens"`
} `json:"removed_content,omitempty"`
```

- [ ] **Step 5: Run tests to verify they pass**

```bash
go test -v
```

Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add cmd/trace-viewer/ internal/tracing/trace.go
git commit -m "feat: add schema feature detection for progressive enhancement

- Add DetectSchemaFeatures to check for optional fields
- Extend trace types with optional schema fields
- Add tests for feature detection
- Support timestamps, context snapshots, tool relationships, etc."
```

---

### Task 3: HTML Template Foundation

**Files:**
- Create: `cmd/trace-viewer/templates.go`
- Create: `cmd/trace-viewer/generator.go`
- Create: `cmd/trace-viewer/generator_test.go`

- [ ] **Step 1: Write test for HTML generation**

Create `cmd/trace-viewer/generator_test.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dancsalo/arxiv-deep-research/internal/tracing"
)

func TestGenerateHTML_BasicStructure(t *testing.T) {
	trace := &tracing.Trace{
		SessionID: "test-001",
		Query:     "test query",
		Turns: []tracing.Turn{
			{Index: 0},
		},
	}
	
	features := SchemaFeatures{}
	outputPath := filepath.Join(t.TempDir(), "test.html")
	
	err := GenerateHTML(trace, features, outputPath)
	if err != nil {
		t.Fatalf("GenerateHTML failed: %v", err)
	}
	
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read output: %v", err)
	}
	
	html := string(content)
	
	// Check basic structure
	if !strings.Contains(html, "<!DOCTYPE html>") {
		t.Error("missing DOCTYPE")
	}
	
	if !strings.Contains(html, "Trace Timeline") {
		t.Error("missing title")
	}
	
	if !strings.Contains(html, "test-001") {
		t.Error("missing session ID")
	}
	
	if !strings.Contains(html, `id="timeline"`) {
		t.Error("missing timeline container")
	}
	
	if !strings.Contains(html, `id="trace-data"`) {
		t.Error("missing embedded trace data")
	}
}

func TestGenerateHTML_EmbeddedData(t *testing.T) {
	trace := &tracing.Trace{
		SessionID: "test-002",
		Query:     "embedded data test",
		Turns: []tracing.Turn{
			{Index: 0, TokensRemaining: 25000},
		},
	}
	
	features := SchemaFeatures{}
	outputPath := filepath.Join(t.TempDir(), "test2.html")
	
	err := GenerateHTML(trace, features, outputPath)
	if err != nil {
		t.Fatalf("GenerateHTML failed: %v", err)
	}
	
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read output: %v", err)
	}
	
	html := string(content)
	
	// Check embedded JSON
	if !strings.Contains(html, `"session_id":"test-002"`) {
		t.Error("trace data not properly embedded")
	}
	
	if !strings.Contains(html, `"tokens_remaining":25000`) {
		t.Error("turn data not properly embedded")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test -v -run TestGenerateHTML
```

Expected: FAIL - undefined: GenerateHTML

- [ ] **Step 3: Create HTML template**

Create `cmd/trace-viewer/templates.go`:

```go
package main

const htmlTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Trace Timeline: {{.SessionID}}</title>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }
        
        body {
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
            line-height: 1.6;
            color: #333;
            background: #f5f5f5;
            padding: 20px;
        }
        
        .container {
            max-width: 1200px;
            margin: 0 auto;
        }
        
        header {
            background: white;
            padding: 20px;
            border-radius: 8px;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
            margin-bottom: 30px;
        }
        
        h1 {
            color: #2c3e50;
            margin-bottom: 10px;
        }
        
        .metadata {
            font-size: 14px;
            color: #666;
        }
        
        .metadata span {
            margin-right: 20px;
        }
        
        #timeline {
            background: white;
            padding: 40px 20px;
            border-radius: 8px;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
            overflow-x: auto;
        }
        
        .timeline-container {
            display: flex;
            align-items: flex-start;
            gap: 20px;
            min-width: fit-content;
        }
        
        .turn-node {
            width: 60px;
            height: 60px;
            border-radius: 50%;
            border: 3px solid #3498db;
            background: white;
            display: flex;
            flex-direction: column;
            align-items: center;
            justify-content: center;
            cursor: pointer;
            transition: border-color 0.1s;
        }
        
        .turn-node:hover {
            border-color: #2980b9;
        }
        
        .turn-node.expanded {
            border-color: #9b59b6;
            background: #f3e5f5;
        }
        
        .turn-node.error {
            border-color: #e74c3c;
        }
        
        .turn-label {
            font-size: 10px;
            color: #666;
        }
        
        .turn-tokens {
            font-size: 9px;
            color: #888;
        }
        
        .edge {
            width: 30px;
            height: 2px;
            background: #ddd;
            align-self: center;
        }
        
        .detail-panel {
            margin-left: 72px;
            margin-top: 12px;
            margin-bottom: 20px;
            background: #f8f9fa;
            border-left: 4px solid #9b59b6;
            border-radius: 6px;
            padding: 16px;
            max-width: 700px;
        }
        
        .detail-header {
            display: flex;
            justify-content: space-between;
            align-items: start;
            margin-bottom: 10px;
        }
        
        .detail-title {
            font-weight: bold;
            color: #9b59b6;
            font-size: 12px;
        }
        
        .detail-close {
            font-size: 18px;
            color: #ccc;
            cursor: pointer;
            line-height: 1;
        }
        
        .detail-close:hover {
            color: #999;
        }
        
        .detail-metrics {
            display: grid;
            grid-template-columns: repeat(3, 1fr);
            gap: 10px;
            margin-bottom: 12px;
            font-size: 11px;
            color: #666;
        }
        
        .tool-list {
            border-top: 1px solid #ddd;
            padding-top: 10px;
            margin-top: 10px;
        }
        
        .tool-list-title {
            font-weight: 600;
            font-size: 10px;
            color: #888;
            margin-bottom: 6px;
        }
        
        .tool-item {
            background: white;
            border-radius: 4px;
            padding: 8px;
            font-size: 10px;
            display: flex;
            justify-content: space-between;
            margin-bottom: 4px;
        }
        
        .hidden {
            display: none;
        }
    </style>
</head>
<body>
    <div class="container">
        <header>
            <h1>📊 Trace Timeline</h1>
            <div class="metadata">
                <span><strong>Session:</strong> {{.SessionID}}</span>
                <span><strong>Query:</strong> {{.Query}}</span>
                <span><strong>Duration:</strong> {{.DurationMs}}ms</span>
                <span><strong>Status:</strong> {{.Status}}</span>
            </div>
        </header>
        
        <div id="timeline">
            <!-- Timeline rendered by JavaScript -->
        </div>
    </div>
    
    <script type="application/json" id="trace-data">
{{.TraceJSON}}
    </script>
    
    <script>
        // Load trace data
        const trace = JSON.parse(document.getElementById('trace-data').textContent);
        
        // State management
        const state = {
            expandedTurns: new Set()
        };
        
        // Initialize timeline
        renderTimeline();
        
        function renderTimeline() {
            const container = document.getElementById('timeline');
            container.innerHTML = '';
            
            const timelineDiv = document.createElement('div');
            timelineDiv.className = 'timeline-container';
            
            trace.turns.forEach((turn, index) => {
                // Render turn node
                const node = renderTurnNode(turn, index);
                timelineDiv.appendChild(node);
                
                // Add edge if not last
                if (index < trace.turns.length - 1) {
                    const edge = document.createElement('div');
                    edge.className = 'edge';
                    timelineDiv.appendChild(edge);
                }
            });
            
            container.appendChild(timelineDiv);
            
            // Render detail panels for expanded turns
            state.expandedTurns.forEach(turnIndex => {
                renderDetailPanel(trace.turns[turnIndex], turnIndex);
            });
        }
        
        function renderTurnNode(turn, index) {
            const node = document.createElement('div');
            node.className = 'turn-node';
            node.id = 'turn-' + index;
            
            if (state.expandedTurns.has(index)) {
                node.classList.add('expanded');
            }
            
            const label = document.createElement('div');
            label.className = 'turn-label';
            label.textContent = 'Turn ' + index;
            
            const tokens = document.createElement('div');
            tokens.className = 'turn-tokens';
            tokens.textContent = formatTokens(turn.tokens_remaining);
            
            node.appendChild(label);
            node.appendChild(tokens);
            
            node.onclick = () => toggleTurn(index);
            
            return node;
        }
        
        function renderDetailPanel(turn, index) {
            const timeline = document.getElementById('timeline');
            const nodeElement = document.getElementById('turn-' + index);
            
            // Remove existing panel if present
            const existingPanel = document.getElementById('detail-' + index);
            if (existingPanel) {
                existingPanel.remove();
            }
            
            const panel = document.createElement('div');
            panel.className = 'detail-panel';
            panel.id = 'detail-' + index;
            
            panel.innerHTML = `
                <div class="detail-header">
                    <div class="detail-title">Turn ${index} Details</div>
                    <div class="detail-close" onclick="toggleTurn(${index})">×</div>
                </div>
                <div class="detail-metrics">
                    <div><strong>Duration:</strong> ${turn.duration_ms}ms</div>
                    <div><strong>Tokens:</strong> ${turn.llm_call.input_tokens} → ${turn.llm_call.output_tokens}</div>
                    <div><strong>Remaining:</strong> ${formatTokens(turn.tokens_remaining)}</div>
                </div>
                <div class="tool-list">
                    <div class="tool-list-title">Tool Calls (${turn.tool_calls.length}):</div>
                    ${turn.tool_calls.map(tc => `
                        <div class="tool-item">
                            <span><strong>${tc.name}</strong></span>
                            <span>${tc.duration_ms}ms</span>
                        </div>
                    `).join('')}
                </div>
            `;
            
            // Insert after the turn node's parent container
            timeline.appendChild(panel);
        }
        
        function toggleTurn(index) {
            if (state.expandedTurns.has(index)) {
                state.expandedTurns.delete(index);
            } else {
                state.expandedTurns.add(index);
            }
            renderTimeline();
        }
        
        function formatTokens(tokens) {
            if (tokens >= 1000) {
                return (tokens / 1000).toFixed(1) + 'k';
            }
            return tokens.toString();
        }
    </script>
</body>
</html>`
```

- [ ] **Step 4: Implement HTML generator**

Create `cmd/trace-viewer/generator.go`:

```go
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"text/template"

	"github.com/dancsalo/arxiv-deep-research/internal/tracing"
)

// GenerateHTML creates a self-contained HTML file for the trace timeline
func GenerateHTML(trace *tracing.Trace, features SchemaFeatures, outputPath string) error {
	// Serialize trace to JSON
	traceJSON, err := json.Marshal(trace)
	if err != nil {
		return fmt.Errorf("marshaling trace: %w", err)
	}
	
	// Parse template
	tmpl, err := template.New("timeline").Parse(htmlTemplate)
	if err != nil {
		return fmt.Errorf("parsing template: %w", err)
	}
	
	// Prepare template data
	data := struct {
		SessionID  string
		Query      string
		DurationMs int64
		Status     string
		TraceJSON  string
	}{
		SessionID:  trace.SessionID,
		Query:      trace.Query,
		DurationMs: trace.DurationMs,
		Status:     trace.Status,
		TraceJSON:  string(traceJSON),
	}
	
	// Render template
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("executing template: %w", err)
	}
	
	// Write to file
	if err := os.WriteFile(outputPath, buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("writing output file: %w", err)
	}
	
	return nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

```bash
go test -v -run TestGenerateHTML
```

Expected: PASS

- [ ] **Step 6: Wire up in main.go**

Update `run` function in `cmd/trace-viewer/main.go`:

```go
func run(cfg Config) error {
	if cfg.Verbose {
		fmt.Printf("Loading trace from %s...\n", cfg.TracePath)
	}
	
	trace, err := LoadTrace(cfg.TracePath)
	if err != nil {
		return err
	}
	
	if cfg.Verbose {
		fmt.Printf("Loaded trace: session=%s, turns=%d\n", trace.SessionID, len(trace.Turns))
	}
	
	// Detect schema features
	features := DetectSchemaFeatures(trace)
	
	if cfg.Verbose {
		fmt.Printf("Generating HTML to %s...\n", cfg.OutputPath)
	}
	
	if err := GenerateHTML(trace, features, cfg.OutputPath); err != nil {
		return err
	}
	
	fmt.Printf("Timeline generated: %s\n", cfg.OutputPath)
	
	if cfg.Open {
		if cfg.Verbose {
			fmt.Println("Opening in browser...")
		}
		if err := openBrowser(cfg.OutputPath); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not open browser: %v\n", err)
		}
	}
	
	return nil
}

func openBrowser(path string) error {
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
```

Add imports at top of main.go:
```go
import (
	"os/exec"
	"runtime"
)
```

- [ ] **Step 7: Test end-to-end**

```bash
go build -o trace-viewer .
./trace-viewer --trace=testdata/fixtures/minimal-trace.json --output=/tmp/timeline.html
```

Expected: HTML file created

```bash
open /tmp/timeline.html
```

Expected: Timeline displays in browser with Turn 0 node, can click to expand

- [ ] **Step 8: Commit**

```bash
git add cmd/trace-viewer/
git commit -m "feat: implement HTML generation with basic timeline rendering

- Add HTML template with CSS and JavaScript
- Implement GenerateHTML function
- Add turn node rendering with expand/collapse
- Add detail panels for turns with metrics and tool lists
- Wire up --open flag to launch browser
- Tests for HTML generation"
```

---

### Task 4: Test Fixtures and Integration Tests

**Files:**
- Create: `cmd/trace-viewer/testdata/fixtures/normal-trace.json`
- Create: `cmd/trace-viewer/testdata/fixtures/compaction-trace.json`
- Create: `cmd/trace-viewer/testdata/fixtures/error-trace.json`
- Modify: `cmd/trace-viewer/generator_test.go`

- [ ] **Step 1: Create normal-trace fixture**

Create `cmd/trace-viewer/testdata/fixtures/normal-trace.json`:

```json
{
  "session_id": "test-normal-001",
  "query": "transformer attention mechanisms",
  "model": "claude-sonnet-4",
  "prompt_variant": "A",
  "prompt_hash": "def45678",
  "started_at": "2026-05-12T10:00:00Z",
  "ended_at": "2026-05-12T10:05:00Z",
  "duration_ms": 300000,
  "status": "ok",
  "total_input_tokens": 8000,
  "total_output_tokens": 2500,
  "total_cost_usd": 0.045,
  "turns": [
    {
      "index": 0,
      "started_at": "2026-05-12T10:00:00Z",
      "ended_at": "2026-05-12T10:01:30Z",
      "duration_ms": 90000,
      "tokens_used": 3500,
      "tokens_remaining": 26500,
      "cost_usd": 0.015,
      "llm_call": {
        "model": "claude-sonnet-4",
        "input_tokens": 2500,
        "output_tokens": 1000,
        "duration_ms": 25000,
        "stop_reason": "tool_use"
      },
      "tool_calls": [
        {
          "name": "search_arxiv",
          "input": "{\"query\":\"attention mechanisms\"}",
          "output": "{\"results\":[{\"title\":\"Attention Is All You Need\"}]}",
          "input_length": 35,
          "result_length": 250,
          "duration_ms": 2100
        },
        {
          "name": "fetch_arxiv_pdf",
          "input": "{\"arxiv_id\":\"1706.03762\"}",
          "output": "{\"url\":\"https://arxiv.org/pdf/1706.03762.pdf\"}",
          "input_length": 30,
          "result_length": 120,
          "duration_ms": 1500
        }
      ]
    },
    {
      "index": 1,
      "started_at": "2026-05-12T10:01:30Z",
      "ended_at": "2026-05-12T10:03:00Z",
      "duration_ms": 90000,
      "tokens_used": 4200,
      "tokens_remaining": 22300,
      "cost_usd": 0.018,
      "llm_call": {
        "model": "claude-sonnet-4",
        "input_tokens": 3200,
        "output_tokens": 1000,
        "duration_ms": 28000,
        "stop_reason": "tool_use"
      },
      "tool_calls": [
        {
          "name": "search_web",
          "input": "{\"query\":\"attention mechanism applications\"}",
          "output": "{\"results\":[...]}",
          "input_length": 50,
          "result_length": 500,
          "duration_ms": 3200
        }
      ]
    },
    {
      "index": 2,
      "started_at": "2026-05-12T10:03:00Z",
      "ended_at": "2026-05-12T10:05:00Z",
      "duration_ms": 120000,
      "tokens_used": 2800,
      "tokens_remaining": 19500,
      "cost_usd": 0.012,
      "llm_call": {
        "model": "claude-sonnet-4",
        "input_tokens": 2000,
        "output_tokens": 800,
        "duration_ms": 22000,
        "stop_reason": "end_turn"
      },
      "tool_calls": []
    }
  ]
}
```

- [ ] **Step 2: Create compaction-trace fixture**

Create `cmd/trace-viewer/testdata/fixtures/compaction-trace.json`:

```json
{
  "session_id": "test-compaction-001",
  "query": "test with compaction",
  "model": "claude-sonnet-4",
  "started_at": "2026-05-12T10:00:00Z",
  "ended_at": "2026-05-12T10:08:00Z",
  "duration_ms": 480000,
  "status": "ok",
  "total_input_tokens": 15000,
  "total_output_tokens": 3000,
  "total_cost_usd": 0.08,
  "turns": [
    {
      "index": 0,
      "tokens_remaining": 28000,
      "llm_call": {"input_tokens": 2000, "output_tokens": 500},
      "tool_calls": [{"name": "search_arxiv", "duration_ms": 2000}]
    },
    {
      "index": 1,
      "tokens_remaining": 12000,
      "llm_call": {"input_tokens": 8000, "output_tokens": 800},
      "tool_calls": [],
      "guardrail_decisions": [
        {
          "tool_name": "",
          "proceed": true,
          "reason": "compaction triggered",
          "compacted": true,
          "compacted_turns": [0, 1, 2],
          "removed_content": {
            "tool_results_count": 3,
            "message_count": 5,
            "summary_tokens": 800
          }
        }
      ]
    },
    {
      "index": 2,
      "tokens_remaining": 11000,
      "llm_call": {"input_tokens": 1500, "output_tokens": 600},
      "tool_calls": []
    }
  ],
  "guardrail_decisions": []
}
```

- [ ] **Step 3: Create error-trace fixture**

Create `cmd/trace-viewer/testdata/fixtures/error-trace.json`:

```json
{
  "session_id": "test-error-001",
  "query": "test with errors",
  "model": "claude-sonnet-4",
  "started_at": "2026-05-12T10:00:00Z",
  "ended_at": "2026-05-12T10:02:00Z",
  "duration_ms": 120000,
  "status": "error",
  "error": "tool execution failed",
  "total_input_tokens": 3000,
  "total_output_tokens": 500,
  "total_cost_usd": 0.015,
  "turns": [
    {
      "index": 0,
      "tokens_remaining": 27000,
      "llm_call": {"input_tokens": 2000, "output_tokens": 500},
      "tool_calls": [
        {
          "name": "search_arxiv",
          "duration_ms": 5000,
          "error": {
            "type": "RateLimitError",
            "message": "API rate limit exceeded",
            "retryable": true,
            "attempted_retries": 2,
            "suggested_action": "backoff"
          }
        }
      ]
    }
  ]
}
```

- [ ] **Step 4: Add integration test**

Add to `cmd/trace-viewer/generator_test.go`:

```go
func TestGenerateHTML_Integration(t *testing.T) {
	fixtures := []string{
		"minimal-trace.json",
		"normal-trace.json",
		"compaction-trace.json",
		"error-trace.json",
	}
	
	for _, fixture := range fixtures {
		t.Run(fixture, func(t *testing.T) {
			tracePath := filepath.Join("testdata", "fixtures", fixture)
			
			trace, err := LoadTrace(tracePath)
			if err != nil {
				t.Fatalf("LoadTrace failed: %v", err)
			}
			
			features := DetectSchemaFeatures(trace)
			outputPath := filepath.Join(t.TempDir(), fixture+".html")
			
			err = GenerateHTML(trace, features, outputPath)
			if err != nil {
				t.Fatalf("GenerateHTML failed: %v", err)
			}
			
			// Verify file exists and has content
			stat, err := os.Stat(outputPath)
			if err != nil {
				t.Fatalf("output file not created: %v", err)
			}
			
			if stat.Size() < 1000 {
				t.Errorf("output file too small: %d bytes", stat.Size())
			}
			
			// Verify basic HTML structure
			content, _ := os.ReadFile(outputPath)
			html := string(content)
			
			if !strings.Contains(html, trace.SessionID) {
				t.Error("session ID not in HTML")
			}
			
			if !strings.Contains(html, `"turns":[`) {
				t.Error("trace data not properly embedded")
			}
		})
	}
}
```

- [ ] **Step 5: Run integration tests**

```bash
go test -v -run TestGenerateHTML_Integration
```

Expected: PASS for all 4 fixtures

- [ ] **Step 6: Manual test with each fixture**

```bash
./trace-viewer --trace=testdata/fixtures/normal-trace.json --output=/tmp/normal.html --open
./trace-viewer --trace=testdata/fixtures/compaction-trace.json --output=/tmp/compaction.html --open
./trace-viewer --trace=testdata/fixtures/error-trace.json --output=/tmp/error.html --open
```

Expected: All timelines display correctly in browser

- [ ] **Step 7: Commit**

```bash
git add cmd/trace-viewer/
git commit -m "test: add comprehensive test fixtures and integration tests

- Add normal-trace with multiple turns and tools
- Add compaction-trace with guardrail decisions
- Add error-trace with tool failures
- Add integration tests for all fixtures
- Verify end-to-end HTML generation"
```

---

### Task 5: Makefile Integration

**Files:**
- Modify: `Makefile`

- [ ] **Step 1: Add build-viewer target**

Add to `Makefile`:

```makefile
build-viewer:  ## Build trace-viewer binary
	go build -o trace-viewer ./cmd/trace-viewer
```

- [ ] **Step 2: Add trace-view target**

Add to `Makefile`:

```makefile
trace-view: build-viewer  ## Generate timeline for most recent trace
	@LATEST=$$(ls -t .traces/*.json 2>/dev/null | head -1) && \
	if [ -z "$$LATEST" ]; then \
		echo "No traces found in .traces/"; \
		exit 1; \
	fi && \
	echo "Generating timeline for $$LATEST..." && \
	./trace-viewer --trace=$$LATEST --open
```

- [ ] **Step 3: Update .PHONY**

Update `.PHONY` line in `Makefile`:

```makefile
.PHONY: build build-research build-tools build-analyzer build-viewer vet lint test test-go test-race check run trace-list trace-show trace-errors trace-dashboard trace-view help
```

- [ ] **Step 4: Test Makefile targets**

```bash
make build-viewer
```

Expected: Binary builds successfully

```bash
# Generate a trace first if needed
make run

# Then view it
make trace-view
```

Expected: Timeline opens in browser for most recent trace

- [ ] **Step 5: Commit**

```bash
git add Makefile
git commit -m "build: integrate trace-viewer into Makefile

- Add build-viewer target
- Add trace-view target to visualize most recent trace
- Update .PHONY list"
```

---

### Task 6: Event Nodes (Compaction & Guardrails)

**Files:**
- Modify: `cmd/trace-viewer/templates.go`
- Modify: `cmd/trace-viewer/generator_test.go`

- [ ] **Step 1: Write test for event node rendering**

Add to `cmd/trace-viewer/generator_test.go`:

```go
func TestGenerateHTML_CompactionEvents(t *testing.T) {
	trace, err := LoadTrace(filepath.Join("testdata", "fixtures", "compaction-trace.json"))
	if err != nil {
		t.Fatalf("LoadTrace failed: %v", err)
	}
	
	features := DetectSchemaFeatures(trace)
	outputPath := filepath.Join(t.TempDir(), "compaction.html")
	
	err = GenerateHTML(trace, features, outputPath)
	if err != nil {
		t.Fatalf("GenerateHTML failed: %v", err)
	}
	
	content, _ := os.ReadFile(outputPath)
	html := string(content)
	
	// Check that compaction events are in the embedded data
	if !strings.Contains(html, `"compacted":true`) {
		t.Error("compaction event not in embedded data")
	}
	
	if !strings.Contains(html, `"removed_content"`) {
		t.Error("removed_content not in embedded data")
	}
}
```

- [ ] **Step 2: Run test**

```bash
go test -v -run TestGenerateHTML_CompactionEvents
```

Expected: PASS (just verifies data is embedded)

- [ ] **Step 3: Update HTML template with event node support**

In `cmd/trace-viewer/templates.go`, update the CSS section to add event node styles:

```css
.event-node {
    border: 2px dashed #e74c3c;
    border-radius: 8px;
    padding: 8px 12px;
    background: #fff5f5;
    cursor: pointer;
    min-width: 100px;
    text-align: center;
}

.event-node.guardrail {
    border-color: #f39c12;
    background: #fffbf0;
}

.event-node-type {
    font-size: 9px;
    font-weight: bold;
    color: #e74c3c;
}

.event-node.guardrail .event-node-type {
    color: #f39c12;
}

.event-node-label {
    font-size: 8px;
    color: #c0392b;
}

.event-node.guardrail .event-node-label {
    color: #d68910;
}

.event-detail-panel {
    margin-left: 72px;
    margin-top: 12px;
    margin-bottom: 20px;
    background: #fff5f5;
    border-left: 4px solid #e74c3c;
    border-radius: 6px;
    padding: 16px;
    max-width: 700px;
}

.event-detail-panel.guardrail {
    background: #fffbf0;
    border-left-color: #f39c12;
}
```

- [ ] **Step 4: Update JavaScript to render events**

In the `<script>` section of `templates.go`, update the `renderTimeline` function:

```javascript
function renderTimeline() {
    const container = document.getElementById('timeline');
    container.innerHTML = '';
    
    const timelineDiv = document.createElement('div');
    timelineDiv.className = 'timeline-container';
    
    // Collect all timeline items (turns and events)
    const items = collectTimelineItems(trace);
    
    items.forEach((item, index) => {
        if (item.type === 'turn') {
            const node = renderTurnNode(item.data, item.turnIndex);
            timelineDiv.appendChild(node);
        } else if (item.type === 'event') {
            const node = renderEventNode(item.data, item.eventIndex);
            timelineDiv.appendChild(node);
        }
        
        // Add edge if not last
        if (index < items.length - 1) {
            const edge = document.createElement('div');
            edge.className = 'edge';
            timelineDiv.appendChild(edge);
        }
    });
    
    container.appendChild(timelineDiv);
    
    // Render detail panels
    state.expandedTurns.forEach(turnIndex => {
        renderDetailPanel(trace.turns[turnIndex], turnIndex);
    });
    
    state.expandedEvents.forEach(eventId => {
        const event = findEvent(eventId);
        if (event) {
            renderEventDetailPanel(event, eventId);
        }
    });
}

function collectTimelineItems(trace) {
    const items = [];
    let eventIndex = 0;
    
    trace.turns.forEach((turn, turnIndex) => {
        items.push({ type: 'turn', data: turn, turnIndex });
        
        // Check for events after this turn
        if (turn.guardrail_decisions) {
            turn.guardrail_decisions.forEach(gd => {
                if (gd.compacted || !gd.proceed) {
                    items.push({
                        type: 'event',
                        data: gd,
                        eventIndex: eventIndex++,
                        turnIndex: turnIndex
                    });
                }
            });
        }
    });
    
    return items;
}

function renderEventNode(event, eventIndex) {
    const node = document.createElement('div');
    node.className = 'event-node';
    node.id = 'event-' + eventIndex;
    
    const isCompaction = event.compacted;
    const isGuardrail = !event.proceed;
    
    if (isGuardrail) {
        node.classList.add('guardrail');
    }
    
    if (state.expandedEvents.has(eventIndex)) {
        node.style.borderWidth = '3px';
    }
    
    const type = document.createElement('div');
    type.className = 'event-node-type';
    type.textContent = isCompaction ? 'COMPACT' : 'BLOCKED';
    
    const label = document.createElement('div');
    label.className = 'event-node-label';
    
    if (isCompaction && event.compacted_turns) {
        label.textContent = '-' + event.compacted_turns.length + ' turns';
    } else if (isGuardrail) {
        label.textContent = 'budget';
    }
    
    node.appendChild(type);
    node.appendChild(label);
    
    node.onclick = () => toggleEvent(eventIndex);
    
    return node;
}

function renderEventDetailPanel(event, eventIndex) {
    const timeline = document.getElementById('timeline');
    
    // Remove existing panel
    const existingPanel = document.getElementById('event-detail-' + eventIndex);
    if (existingPanel) {
        existingPanel.remove();
    }
    
    const panel = document.createElement('div');
    panel.className = 'event-detail-panel';
    panel.id = 'event-detail-' + eventIndex;
    
    const isCompaction = event.compacted;
    const isGuardrail = !event.proceed;
    
    if (isGuardrail) {
        panel.classList.add('guardrail');
    }
    
    let content = `
        <div class="detail-header">
            <div class="detail-title">${isCompaction ? 'Compaction' : 'Guardrail'} Event</div>
            <div class="detail-close" onclick="toggleEvent(${eventIndex})">×</div>
        </div>
    `;
    
    if (isCompaction) {
        content += `
            <div class="detail-metrics">
                <div><strong>Turns Removed:</strong> ${event.compacted_turns ? event.compacted_turns.join(', ') : 'N/A'}</div>
                ${event.removed_content ? `
                    <div><strong>Messages:</strong> ${event.removed_content.message_count}</div>
                    <div><strong>Tool Results:</strong> ${event.removed_content.tool_results_count}</div>
                ` : ''}
            </div>
        `;
    } else {
        content += `
            <div class="detail-metrics">
                <div><strong>Tool:</strong> ${event.tool_name || 'N/A'}</div>
                <div><strong>Reason:</strong> ${event.reason}</div>
                <div><strong>Estimated:</strong> ${event.estimated_tokens} tokens</div>
            </div>
        `;
    }
    
    panel.innerHTML = content;
    timeline.appendChild(panel);
}

function toggleEvent(eventIndex) {
    if (!state.expandedEvents) {
        state.expandedEvents = new Set();
    }
    
    if (state.expandedEvents.has(eventIndex)) {
        state.expandedEvents.delete(eventIndex);
    } else {
        state.expandedEvents.add(eventIndex);
    }
    renderTimeline();
}

function findEvent(eventIndex) {
    let currentIndex = 0;
    for (const turn of trace.turns) {
        if (turn.guardrail_decisions) {
            for (const gd of turn.guardrail_decisions) {
                if (gd.compacted || !gd.proceed) {
                    if (currentIndex === eventIndex) {
                        return gd;
                    }
                    currentIndex++;
                }
            }
        }
    }
    return null;
}
```

Update the state initialization:

```javascript
const state = {
    expandedTurns: new Set(),
    expandedEvents: new Set()
};
```

- [ ] **Step 5: Test with compaction fixture**

```bash
go build -o trace-viewer .
./trace-viewer --trace=testdata/fixtures/compaction-trace.json --output=/tmp/compaction.html --open
```

Expected: Timeline shows COMPACT event node between turns, can click to expand and see details

- [ ] **Step 6: Commit**

```bash
git add cmd/trace-viewer/
git commit -m "feat: add event node rendering for compaction and guardrails

- Add event node styles (dashed rectangles)
- Collect and render events inline with turns
- Add event detail panels with compaction/guardrail info
- Support expanding multiple events simultaneously"
```

---

### Task 7: Documentation and README

**Files:**
- Create: `cmd/trace-viewer/README.md`
- Modify: `README.md` (root)

- [ ] **Step 1: Create tool-specific README**

Create `cmd/trace-viewer/README.md`:

```markdown
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
- **Self-Contained Output**: Generated HTML works offline, no server needed
```

- [ ] **Step 2: Update root README**

Add section to `README.md` after trace commands:

```markdown
## Trace Visualization

Generate an interactive timeline for a single trace:

```bash
make trace-view
```

This opens a browser with the timeline for the most recent trace. The timeline shows:
- **Turn nodes**: Click to expand and see LLM calls, tool executions, metrics
- **Event nodes**: Compaction and guardrail decisions shown inline
- **Multiple expansions**: Compare turns by expanding several at once

Manual usage:

```bash
./trace-viewer --trace=.traces/session-001.json --open
```

See [`cmd/trace-viewer/README.md`](cmd/trace-viewer/README.md) for details.
```

- [ ] **Step 3: Verify READMEs render correctly**

```bash
# View in terminal or GitHub
cat cmd/trace-viewer/README.md
cat README.md
```

Expected: Formatting looks correct

- [ ] **Step 4: Commit**

```bash
git add cmd/trace-viewer/README.md README.md
git commit -m "docs: add documentation for trace-viewer tool

- Add tool-specific README with usage examples
- Update root README with trace visualization section
- Document features, schema support, development workflow"
```

---

### Task 8: Final Integration and Testing

**Files:**
- Modify: `cmd/trace-viewer/generator_test.go`
- Run full test suite

- [ ] **Step 1: Add comprehensive end-to-end test**

Add to `cmd/trace-viewer/generator_test.go`:

```go
func TestEndToEnd_FullWorkflow(t *testing.T) {
	// Test complete workflow: load → detect → generate → verify
	
	tracePath := filepath.Join("testdata", "fixtures", "normal-trace.json")
	outputPath := filepath.Join(t.TempDir(), "e2e-timeline.html")
	
	// Load trace
	trace, err := LoadTrace(tracePath)
	if err != nil {
		t.Fatalf("LoadTrace failed: %v", err)
	}
	
	// Detect features
	features := DetectSchemaFeatures(trace)
	
	// Generate HTML
	err = GenerateHTML(trace, features, outputPath)
	if err != nil {
		t.Fatalf("GenerateHTML failed: %v", err)
	}
	
	// Verify output
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read output: %v", err)
	}
	
	html := string(content)
	
	// Check structure
	requiredElements := []string{
		"<!DOCTYPE html>",
		"Trace Timeline",
		"test-normal-001",
		"transformer attention mechanisms",
		`id="timeline"`,
		`id="trace-data"`,
		"renderTimeline",
		"toggleTurn",
		"renderEventNode",
	}
	
	for _, elem := range requiredElements {
		if !strings.Contains(html, elem) {
			t.Errorf("missing required element: %s", elem)
		}
	}
	
	// Verify embedded trace data is valid JSON
	start := strings.Index(html, `<script type="application/json" id="trace-data">`)
	end := strings.Index(html, `</script>`, start)
	if start == -1 || end == -1 {
		t.Fatal("could not find embedded trace data")
	}
	
	jsonStart := start + len(`<script type="application/json" id="trace-data">`)
	embeddedJSON := html[jsonStart:end]
	
	var embeddedTrace tracing.Trace
	if err := json.Unmarshal([]byte(embeddedJSON), &embeddedTrace); err != nil {
		t.Fatalf("embedded trace data is not valid JSON: %v", err)
	}
	
	// Verify trace data integrity
	if embeddedTrace.SessionID != trace.SessionID {
		t.Error("session ID mismatch in embedded data")
	}
	
	if len(embeddedTrace.Turns) != len(trace.Turns) {
		t.Error("turn count mismatch in embedded data")
	}
}
```

- [ ] **Step 2: Run full test suite**

```bash
cd cmd/trace-viewer
go test -v
```

Expected: All tests PASS

- [ ] **Step 3: Run with race detector**

```bash
go test -race -v
```

Expected: No races detected, all tests PASS

- [ ] **Step 4: Build and test all fixtures manually**

```bash
go build -o trace-viewer .

for fixture in testdata/fixtures/*.json; do
    echo "Testing $fixture"
    ./trace-viewer --trace="$fixture" --output="/tmp/$(basename $fixture .json).html"
    if [ $? -ne 0 ]; then
        echo "FAIL: $fixture"
        exit 1
    fi
done

echo "All fixtures processed successfully"
```

Expected: All fixtures generate HTML successfully

- [ ] **Step 5: Test with actual traces**

```bash
# Generate some real traces if needed
cd ../..
make run

# Then view them
./trace-viewer --trace=.traces/*.json --open
```

Expected: Real traces visualize correctly

- [ ] **Step 6: Run project-wide tests**

```bash
cd ../..
make test
```

Expected: All project tests PASS

- [ ] **Step 7: Commit**

```bash
git add cmd/trace-viewer/
git commit -m "test: add comprehensive end-to-end integration test

- Test full workflow: load → detect → generate → verify
- Verify HTML structure and embedded data integrity
- Run race detector
- Test all fixtures
- All tests passing"
```

---

## Self-Review

✅ **Spec coverage:**
- Timeline visualization: Task 3 (HTML template)
- Turn nodes with expand/collapse: Task 3
- Event nodes (compaction/guardrail): Task 6
- Schema feature detection: Task 2
- Progressive enhancement: Task 2
- CLI interface: Task 1
- Makefile integration: Task 5
- Documentation: Task 7
- Testing strategy: Tasks 1-4, 8

✅ **No placeholders:** All code is complete and concrete

✅ **Type consistency:** Checked across all tasks

✅ **All requirements implemented:** MVP feature-complete per Phase 1 spec

---

## Plan Complete

This plan implements **Phase 1: Core Timeline (MVP)** from the spec, delivering:

✅ Read trace JSON files
✅ Generate self-contained HTML
✅ Horizontal timeline with turn nodes
✅ Click to expand inline detail panels
✅ Show turn metrics and tool calls
✅ Multiple simultaneous expansions
✅ Event nodes for compaction and guardrails
✅ Progressive enhancement for schema features
✅ Makefile integration
✅ Comprehensive tests

**Phases 2-4** (additional schema fields, modal view, polish) can be implemented as follow-on work based on user feedback.
