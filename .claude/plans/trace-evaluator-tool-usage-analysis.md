# Plan: Trace Evaluator for Tool Usage Analysis

## Context

This evaluator agent will analyze trace files from the research-demo agent to extract insights about tool usage patterns. Specifically, for each tool call in a trace, it will determine:

1. **Why the agent decided to use that tool** - extracted from the "Turn N: Planning" section where the agent states its rationale
2. **What the tool output was** - the actual result returned by the tool
3. **Whether the agent found value in the output** - determined from the "Turn N+1: Reflection" section where the agent reflects on surprises and findings

This analysis will help understand which tools provide value, how the agent reasons about tool selection, and where improvements can be made to the research process.

### Trace Structure Context

From exploration of the codebase:
- Traces are JSON files in `.traces/` directory with session data, turns, and tool calls
- Each `Turn` contains an `LLMCall` with the agent's text response and a `ToolCalls` array
- The metacognitive prompt (variant-b) instructs the agent to write structured Planning and Reflection sections
- **Planning sections** appear in Turn N's LLM output before tools are called, with format:
  - `## Turn N Planning`
  - `**What I Learned:**`, `**Current Gaps:**`, `**This Turn's Tools & Rationale:**`, `**Strategy:**`
- **Reflection sections** appear in Turn N+1's LLM output after receiving tool results:
  - `## Turn N Reflection`  
  - `**Surprises:**`, `**Tool Coverage:**`, `**Breadth vs Depth Check:**`, `**Template Readiness:**`

### Why This Change

The research-demo agent generates rich metacognitive traces, but there's currently no automated way to analyze tool effectiveness. This evaluator will enable:
- Quantifying which tools provide valuable insights vs. noise
- Understanding the agent's reasoning for tool selection
- Identifying patterns where tools under-deliver on expected value
- Creating datasets for training better tool-use policies

## Architecture Decision

Create a new standalone binary `cmd/trace-evaluator/` as a completely self-contained tool. This will replace the existing `cmd/trace-viewer/` which will be deleted.

**Rationale:**
- `tools-cli` is for exercising research tools, not analyzing traces
- Self-contained implementation avoids dependencies and tight coupling
- Provides focused tool usage analysis rather than generic visualization
- trace-viewer is redundant once evaluator provides structured analysis

## Implementation Plan

### 1. Core Data Structures

**File:** `internal/tracing/evaluation.go` (new)

Define evaluation result types:

```go
type ToolEvaluation struct {
    TurnIndex       int        `json:"turn_index"`
    ToolIndex       int        `json:"tool_index"`     // Index within turn
    ToolName        string     `json:"tool_name"`
    Rationale       string     `json:"rationale"`      // From Planning section
    Output          string     `json:"output"`         // First 500 chars
    OutputLength    int        `json:"output_length"`  // Full length
    ReflectionNotes string     `json:"reflection_notes"` // From Reflection
    FoundValue      bool       `json:"found_value"`    // Heuristic determination
    DurationMs      int64      `json:"duration_ms"`
    StartedAt       *time.Time `json:"started_at,omitempty"`
}

type TraceEvaluation struct {
    SessionID   string           `json:"session_id"`
    Query       string           `json:"query"`
    TotalTurns  int              `json:"total_turns"`
    TotalTools  int              `json:"total_tools"`
    Evaluations []ToolEvaluation `json:"evaluations"`
    Summary     EvaluationSummary `json:"summary"`
}

type EvaluationSummary struct {
    ToolUsageCounts   map[string]int `json:"tool_usage_counts"`
    ValueFoundCount   int            `json:"value_found_count"`
    NoReflectionCount int            `json:"no_reflection_count"`
    AvgToolsPerTurn   float64        `json:"avg_tools_per_turn"`
}
```

### 2. Text Parsing Logic

**File:** `internal/tracing/text_parser.go` (new)

Implement parsing functions for Planning and Reflection sections:

```go
type PlanningSection struct {
    TurnNumber        int
    WhatILearned      string
    CurrentGaps       string
    ToolsAndRationale map[string]string // tool_name -> rationale
    Strategy          string
}

type ReflectionSection struct {
    TurnNumber       int
    Surprises        string
    ToolCoverage     string
    BreadthVsDepth   string
    TemplateReadiness string
    MentionedTools   []string
}

// ParsePlanning extracts planning section from LLM text output
func ParsePlanning(text string, turnIndex int) (*PlanningSection, error)

// ParseReflection extracts reflection section from next turn's output
func ParseReflection(text string, targetTurnIndex int) (*ReflectionSection, error)

// ExtractToolRationale finds specific tool's reasoning in planning text
func ExtractToolRationale(planning *PlanningSection, toolName string) string

// CheckToolReflection determines if tool was mentioned positively
func CheckToolReflection(reflection *ReflectionSection, toolName string) (foundValue bool, notes string)
```

**Parsing Strategy:**
- Use regex to find section headers: `## Turn (\d+) (Planning|Reflection)`
- Extract subsections using header markers (`**X:**` or `X:`)
- For tool rationale, look for patterns:
  - Backtick-enclosed tool names: `` `search_arxiv` ``
  - Phrases like "I'll use X because..." or "calling X to..."
- For reflection value, check if tool mentioned in "Surprises" or "Tool Coverage" with positive context

**Edge Cases:**
- Last turn has no reflection → mark as "No reflection (final turn)"
- Missing planning section → rationale = "No planning found"
- Tool mentioned multiple times → use first substantive rationale
- Format variations (## vs **, etc.) → support multiple patterns with fallbacks

### 3. Trace Loading

**File:** `cmd/trace-evaluator/loader.go` (new, self-contained)

Implement trace loading directly in the evaluator:

```go
func LoadTrace(filepath string) (*tracing.Trace, error) {
    data, err := os.ReadFile(filepath)
    if err != nil {
        return nil, fmt.Errorf("read file: %w", err)
    }
    
    var trace tracing.Trace
    if err := json.Unmarshal(data, &trace); err != nil {
        return nil, fmt.Errorf("parse JSON: %w", err)
    }
    
    return &trace, nil
}
```

Simple, self-contained loading that unmarshals the trace JSON using existing `internal/tracing/trace.go` structs.

### 4. LLM Output Extraction

**File:** `cmd/trace-evaluator/llm_parser.go` (new)

Extract text content from LLM response JSON:

```go
// ExtractLLMText parses the LLM output JSON and extracts text content
func ExtractLLMText(turn tracing.Turn) (string, error) {
    if turn.LLMCall == nil || turn.LLMCall.Output == nil {
        return "", nil
    }
    
    // Parse anthropic API response: {"content": [{"type": "text", "text": "..."}]}
    var response struct {
        Content []struct {
            Type string `json:"type"`
            Text string `json:"text"`
        } `json:"content"`
    }
    
    if err := json.Unmarshal(turn.LLMCall.Output, &response); err != nil {
        return "", fmt.Errorf("parse LLM output: %w", err)
    }
    
    var textParts []string
    for _, c := range response.Content {
        if c.Type == "text" {
            textParts = append(textParts, c.Text)
        }
    }
    
    return strings.Join(textParts, "\n"), nil
}
```

### 5. Evaluation Engine

**File:** `cmd/trace-evaluator/evaluator.go` (new)

Main evaluation orchestration:

```go
func EvaluateTrace(trace *tracing.Trace) *tracing.TraceEvaluation {
    evaluations := []tracing.ToolEvaluation{}
    
    for _, turn := range trace.Turns {
        // Extract planning from this turn's LLM output
        planning := extractPlanningFromTurn(turn)
        
        // Look ahead for reflection in next turn(s)
        reflection := findReflectionForTurn(trace, turn.Index)
        
        // Evaluate each tool call
        for toolIdx, toolCall := range turn.ToolCalls {
            eval := evaluateToolCall(
                turn.Index, toolIdx, toolCall,
                planning, reflection,
            )
            evaluations = append(evaluations, eval)
        }
    }
    
    return buildTraceEvaluation(trace, evaluations)
}

func extractPlanningFromTurn(turn tracing.Turn) *tracing.PlanningSection
func findReflectionForTurn(trace *tracing.Trace, turnIndex int) *tracing.ReflectionSection
func evaluateToolCall(...) tracing.ToolEvaluation
func buildTraceEvaluation(...) *tracing.TraceEvaluation
```

**Key Logic:**
- Use `llm_parser.ExtractLLMText()` to get text content from turn
- Call `text_parser.ParsePlanning()` on text
- Search turns N+1, N+2 (up to 2 ahead) for reflection mentioning turn N
- Match tool names between planning rationale and actual tool calls
- Truncate tool output to 500 chars for readability, store full length

### 6. Output Formatters

**File:** `cmd/trace-evaluator/formatters.go` (new)

Support JSON and Markdown output:

```go
func OutputJSON(eval *tracing.TraceEvaluation, outputPath string) error
func OutputMarkdown(eval *tracing.TraceEvaluation, outputPath string) error
```

**JSON Format:** Direct marshaling of TraceEvaluation struct

**Markdown Format:**
```markdown
# Tool Usage Evaluation: {Query}

Session: {session_id}
Total Turns: {n} | Total Tools: {n}

## Tool Usage Summary
| Tool Name | Count | Avg Duration (ms) |
|-----------|-------|-------------------|
| ... | ... | ... |

## Turn-by-Turn Analysis

### Turn 0 (3 tools)
| Tool | Rationale | Value Found | Reflection |
|------|-----------|-------------|------------|
| search_openalex | Find influential papers | ✓ | Found DQN with 29K citations |
| ... | ... | ... | ... |
```

### 7. CLI Entry Point

**File:** `cmd/trace-evaluator/main.go` (new)

```go
var (
    traceFile = flag.String("trace", "", "Path to trace JSON file (required)")
    format    = flag.String("format", "json", "Output format: json or markdown")
    output    = flag.String("output", "", "Output file (default: stdout)")
    verbose   = flag.Bool("verbose", false, "Enable verbose logging")
)

func main() {
    // Parse flags
    // Load trace using loader.LoadTrace() (self-contained)
    // Evaluate using evaluator.EvaluateTrace()
    // Format and output using formatters
}
```

**Usage Examples:**
```bash
# Evaluate trace to JSON (stdout)
./trace-evaluator --trace=.traces/demo-1778684958083.json

# Evaluate to markdown file
./trace-evaluator --trace=.traces/demo-1778684958083.json \
  --format=markdown --output=evaluation.md

# Verbose mode for debugging
./trace-evaluator --trace=.traces/demo-1778684958083.json --verbose
```

### 8. Delete trace-viewer

**Files to delete:**
- `cmd/trace-viewer/` (entire directory)
- Remove `build-trace-viewer`, `trace-view`, and any other trace-viewer targets from Makefile

**Rationale:** trace-viewer provides generic HTML visualization but doesn't offer the structured analysis needed for understanding tool effectiveness. The evaluator provides focused, actionable insights.

### 9. Makefile Integration

Add to `Makefile`:

```makefile
build-evaluator: ## Build trace-evaluator binary
	go build -o trace-evaluator ./cmd/trace-evaluator

trace-eval: build-evaluator ## Evaluate a trace: make trace-eval RUN=<session_id>
	./trace-evaluator --trace=$(TRACE_DIR)/$(RUN).json --format=markdown

trace-eval-latest: build-evaluator ## Evaluate most recent trace
	@LATEST=$$(ls -t $(TRACE_DIR)/*.json 2>/dev/null | head -1); \
	if [ -z "$$LATEST" ]; then \
		echo "No traces found in $(TRACE_DIR)/"; \
		exit 1; \
	fi; \
	./trace-evaluator --trace="$$LATEST" --format=markdown
```

## Critical Files

1. `internal/tracing/text_parser.go` (new) - Core parsing logic for Planning/Reflection sections
2. `internal/tracing/evaluation.go` (new) - Data structures for evaluation results
3. `cmd/trace-evaluator/loader.go` (new) - Self-contained trace loading
4. `cmd/trace-evaluator/llm_parser.go` (new) - Extract text from LLM output JSON
5. `cmd/trace-evaluator/evaluator.go` (new) - Main evaluation engine
6. `cmd/trace-evaluator/formatters.go` (new) - JSON and Markdown output formatting
7. `cmd/trace-evaluator/main.go` (new) - CLI entry point
8. `internal/tracing/trace.go` (existing) - Contains Turn and ToolCall structures

## Implementation Sequence

1. **Delete:** Remove `cmd/trace-viewer/` directory and Makefile references
2. **Setup:** Create `internal/tracing/evaluation.go` with data structures
3. **Parse:** Implement `internal/tracing/text_parser.go` with regex-based section extraction
4. **Load:** Create `cmd/trace-evaluator/loader.go` for self-contained trace loading
5. **LLM:** Implement `cmd/trace-evaluator/llm_parser.go` for extracting text from LLM responses
6. **Evaluate:** Build `cmd/trace-evaluator/evaluator.go` orchestration logic
7. **Format:** Implement `cmd/trace-evaluator/formatters.go` for JSON and Markdown
8. **CLI:** Create `cmd/trace-evaluator/main.go` with flag parsing
9. **Integrate:** Update Makefile with new commands
10. **Test:** Run against `demo-1778684958083.json` and verify output

## Verification

After implementation, test with:

```bash
# Build the evaluator
make build-evaluator

# Test with known trace
make trace-eval RUN=demo-1778684958083

# Verify expected output:
# - All 27 tools should be listed
# - Turn 0: search_openalex, search_arxiv, search_github_repos
# - Rationale for search_openalex should mention "Find most influential papers"
# - Reflection should note DQN's high citation count (29,572)
# - Should show value_found=true for tools mentioned in Surprises

# Test edge cases
# - Run against a trace that ends with finish_loop (last turn has no reflection)
# - Verify "No reflection (final turn)" appears
# - Verify summary statistics are accurate
```

Expected first evaluation entry for demo-1778684958083.json:
```json
{
  "turn_index": 0,
  "tool_index": 0,
  "tool_name": "search_openalex",
  "rationale": "Find most influential papers on RL agent training using citation sorting",
  "output_length": 11550,
  "reflection_notes": "Mentioned in Surprises: DQN paper has 29,572 citations",
  "found_value": true,
  "duration_ms": 485
}
```

## Challenges & Mitigations

**Challenge:** Text parsing brittleness due to format variations
- **Mitigation:** Support multiple header patterns (##, **, etc.), use fuzzy matching, fallback to substring search

**Challenge:** Ambiguous "value found" determination
- **Mitigation:** Use heuristics: mentioned in Surprises = value, not mentioned = unclear, negative mention = no value

**Challenge:** Tool name matching (search_arxiv vs "searching arxiv")
- **Mitigation:** Normalize names (lowercase, underscores), use fuzzy matching with Levenshtein distance

**Challenge:** Multi-turn reflection gaps (reflection may be delayed)
- **Mitigation:** Search up to 2 turns ahead for reflection, mark as "No reflection found" if missing
