# Timeline Trace Visualization Design

**Date:** 2026-05-12  
**Status:** Draft  
**Type:** New Feature

## Overview

A single-trace visualization tool that makes the timeline the central, interactive surface for exploring research agent execution. The tool enables debugging, performance optimization, and behavioral analysis through a chronological view of LLM calls, tool invocations, context management events, and guardrail decisions.

## Goals

**Primary use cases:**
- Debug trace failures and unexpected behavior
- Optimize performance bottlenecks (token usage, latency)
- Understand agent reasoning flow and decision-making
- Analyze context management effectiveness (compaction impact)

**Non-goals:**
- Multi-trace comparison or aggregation (handled by trace-analyzer dashboard)
- Real-time monitoring or live trace viewing
- Collaborative features or annotations
- Advanced comparison modes (out of scope for MVP)

## Architecture

### Implementation Approach

**Static HTML Generator (Go)** — follows the trace-analyzer pattern.

```
┌──────────────┐
│ Trace JSON   │  Single trace file from .traces/
│   File       │
└──────┬───────┘
       │
       ▼
┌──────────────────┐
│  Go CLI Tool     │  Read, validate, enrich
│  (trace-viewer)  │
└──────┬───────────┘
       │
       ▼
┌──────────────────┐
│ Self-contained   │  Open in browser
│    HTML file     │  No server needed
│                  │
│ • Embedded JSON  │
│ • JS for UI      │
│ • Inline CSS     │
└──────────────────┘
```

**Technology stack:**
- **Backend:** Go 1.21+, stdlib only (consistent with project conventions)
- **Frontend:** Vanilla JavaScript (no framework dependencies)
- **Styling:** Inline CSS (self-contained HTML)
- **Data format:** JSON embedded in `<script type="application/json">` tag

**Why static generation:**
- Consistent with existing trace-analyzer tool
- No server required (simple deployment and usage)
- Works offline
- Fast to implement (reuse existing patterns)
- Sufficient for current trace sizes (< 100 turns typical)

## User Experience

### Timeline Structure

**Primary layout:** Horizontal timeline with expandable turn nodes.

**Node types:**

1. **Turn Nodes (circles)** — represent reasoning cycles
   - Display: turn index, tokens remaining
   - State: collapsed (default) or expanded
   - Color: blue (default), purple (expanded), red (error)

2. **Event Nodes (dashed rectangles)** — context management events
   - Compaction events (red): "COMPACT -3 turns"
   - Guardrail events (orange): "BLOCKED (budget)"
   - Inline between turn nodes

3. **Tool Calls** — shown inside expanded turn detail panels
   - Not visible on collapsed timeline
   - Displayed as list within detail panel

**Default view (all collapsed):**

```
┌─────┐      ┌─────┐      ┌─────────┐      ┌─────┐      ┌─────┐
│Turn 0│ ━━━ │Turn 1│ ━━━ │ COMPACT │ ━━━ │Turn 2│ ━━━ │Turn 3│
│12.5k │      │11.2k │      │-3 turns │      │ 9.8k │      │ 9.5k │
└─────┘      └─────┘      └─────────┘      └─────┘      └─────┘
```

**Expanded view (Turn 1 expanded):**

```
┌─────┐      ┌───────────────────────────────┐
│Turn 0│ ━━━ │         Turn 1 (expanded)      │
│12.5k │      │         ▲ click to collapse   │
└─────┘      └───────────────────────────────┘
                    ┌────────────────────────────────────┐
                    │ Turn 1 Details                   × │
                    ├────────────────────────────────────┤
                    │ Duration: 3.2s                     │
                    │ Tokens: 5,200 → 850                │
                    │ Remaining: 11,250                  │
                    │                                    │
                    │ Tool Calls (3):                    │
                    │  • search_arxiv → 3 results (2.1s) │
                    │  • fetch_arxiv_pdf → 127KB (1.5s)  │
                    │  • search_github_repos → 5 (0.8s)  │
                    │                                    │
                    │ [View Full Input/Output →]         │
                    └────────────────────────────────────┘
```

**Multiple expansions supported:**
- Users can expand multiple turns simultaneously
- Each gets its own inline detail panel
- Useful for manual comparison (scroll between panels)
- No explicit comparison mode (out of scope)

### Interactions

| User Action | System Response |
|-------------|-----------------|
| Click collapsed turn node | Expands inline detail panel below node |
| Click expanded turn node | Collapses detail panel |
| Click × in detail panel | Closes that specific panel |
| Click event node (COMPACT/BLOCKED) | Expands inline event details |
| Click "View Full I/O" button | Opens modal overlay with complete JSON data |
| Scroll | Natural vertical scrolling through timeline |

**No features:**
- No drag-and-drop
- No zoom controls
- No filtering UI (use CLI flags for that)
- No comparison mode (rely on multiple expansions)
- No animations or transitions (instant expand/collapse)

### Detail Panel Content

**Turn detail panel shows:**

```
┌─────────────────────────────────────┐
│ Turn N Details                    × │
├─────────────────────────────────────┤
│ Duration: 3.2s                      │
│ Tokens: 5,200 input → 850 output    │
│ Remaining: 11,250                   │
│ Cost: $0.015                        │
│ Status: success                     │
│                                     │
│ Context Summary:                    │
│  • System: 450 tokens               │
│  • User messages: 2                 │
│  • Assistant messages: 1            │
│  • Tool results: 3                  │
│  • Oldest message: Turn 0           │
│                                     │
│ Tool Calls (3):                     │
│  • search_arxiv                     │
│    → 3 results, 2.1s                │
│  • fetch_arxiv_pdf                  │
│    → 127 KB, 1.5s                   │
│  • search_github_repos              │
│    → 5 repos, 0.8s                  │
│                                     │
│ [View Full Input/Output →]          │
└─────────────────────────────────────┘
```

**Event detail panel shows:**

For compaction:
- Turns removed (e.g., turns 0-2)
- Before/after token counts
- Content summary: messages removed, tool results removed
- Compaction type (truncate/summarize)

For guardrail:
- Decision reason (budget exceeded, safety)
- Action taken (blocked, args modified)
- Estimated tokens vs remaining
- Tool name that was blocked

**Modal (Full I/O view):**
- Complete LLM input (all messages as JSON)
- Complete LLM output (full response as JSON)
- Token breakdown
- Timestamps
- Copy/export buttons

## Data Model

### Schema Extensions

To enable rich visualization, extend the existing trace schema with these fields:

#### A. Per-Event Timestamps

Add precise timestamps for every event within a turn.

**Current:**
```json
{
  "turns": [{
    "started_at": "2026-05-12T10:23:45.123Z",
    "ended_at": "2026-05-12T10:23:48.456Z"
  }]
}
```

**New:**
```json
{
  "turns": [{
    "started_at": "2026-05-12T10:23:45.123Z",
    "ended_at": "2026-05-12T10:23:48.456Z",
    "llm_call": {
      "started_at": "2026-05-12T10:23:45.150Z",
      "ended_at": "2026-05-12T10:23:47.890Z"
    },
    "tool_calls": [{
      "name": "search_arxiv",
      "started_at": "2026-05-12T10:23:48.100Z",
      "ended_at": "2026-05-12T10:23:50.200Z"
    }]
  }]
}
```

**Enables:**
- Temporal waterfall visualization (optional enhancement)
- Parallel vs sequential tool execution detection
- Wall-clock time scrubbing

#### B. Context Snapshots

Capture input message structure for each LLM call.

**New field in `LLMCall`:**
```json
{
  "llm_call": {
    "input_summary": {
      "system_tokens": 450,
      "user_messages": 2,
      "assistant_messages": 1,
      "tool_results": 3,
      "total_messages": 7,
      "oldest_message_turn": 0
    }
  }
}
```

**Enables:**
- Context window visualization (what's in the context at each turn)
- Understanding compaction impact on conversation structure
- Debugging "why did it forget X?" scenarios

#### C. Tool Call Relationships

Track dependencies between tool calls.

**New fields in `ToolCall`:**
```json
{
  "tool_calls": [{
    "name": "fetch_arxiv_pdf",
    "parent_tool_index": 0,
    "execution_mode": "sequential"
  }]
}
```

**Enables:**
- Tool call dependency graph visualization
- Distinguish parallel vs sequential execution
- Identify bottlenecks in tool chains

#### D. Compaction Diffs

Store what was removed/summarized during compaction.

**New fields in `GuardrailDecision` (compaction type):**
```json
{
  "guardrail_decisions": [{
    "compacted": true,
    "compacted_turns": [0, 1, 2],
    "before_tokens": 8500,
    "after_tokens": 1200,
    "removed_content": {
      "tool_results_count": 5,
      "message_count": 8,
      "summary_tokens": 1200
    }
  }]
}
```

**Enables:**
- Before/after comparison in event detail panel
- Understanding what information was lost
- Debugging compaction strategy effectiveness

#### E. Node Display Metadata

Add labels and summaries optimized for visualization.

**New field in `Turn`:**
```json
{
  "turns": [{
    "display": {
      "label": "Search & Fetch",
      "summary": "Found 3 papers, fetched 2 PDFs",
      "status": "success",
      "primary_tool": "search_arxiv"
    }
  }]
}
```

**Enables:**
- Rich node labels without parsing raw data
- At-a-glance status indicators
- Semantic grouping and filtering

#### F. Rich Error Context

Structured error information for failures.

**New field in `ToolCall`:**
```json
{
  "tool_calls": [{
    "error": {
      "type": "RateLimitError",
      "message": "API rate limit exceeded",
      "retryable": true,
      "attempted_retries": 2,
      "suggested_action": "backoff"
    }
  }]
}
```

**Enables:**
- Clear error visualization on timeline
- Understanding failure patterns
- Suggested fixes in detail panel

### Progressive Enhancement

The visualization should work with current schema and progressively enhance as new fields become available.

**Detection strategy:**
```go
func detectSchemaFeatures(trace Trace) SchemaFeatures {
    return SchemaFeatures{
        HasTimestamps:      trace.Turns[0].LLMCall.StartedAt != nil,
        HasContextSnapshots: trace.Turns[0].LLMCall.InputSummary != nil,
        HasToolRelationships: trace.Turns[0].ToolCalls[0].ParentToolIndex != nil,
        // ... etc
    }
}
```

**Fallback behavior:**
- Missing timestamps: show only durations, no waterfall view
- Missing context snapshots: skip context breakdown section
- Missing display metadata: derive simple labels from tool names
- Missing error context: show basic error string

## Visual Design

### Style Guidelines

**Polish level:** Functional & minimal

**Colors:**
- Turns (default): `#3498db` (blue)
- Turns (expanded): `#9b59b6` (purple)
- Turns (error): `#e74c3c` (red)
- Compaction events: `#e74c3c` (red)
- Guardrail events: `#f39c12` (orange)
- Success indicators: `#27ae60` (green)
- Text: `#333` (dark gray)
- Background: `#f5f5f5` (light gray)

**Typography:**
- Font: System UI font stack (`-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto`)
- Sizes: 14px body, 16px headings, 12px metadata

**Shapes:**
- Turn nodes: 60px circles with border
- Event nodes: Rounded rectangles (border-radius: 8px)
- Detail panels: Cards with 4px left border accent

**Layout:**
- Timeline: Horizontal, left-to-right
- Spacing: 15-20px between nodes
- Detail panels: Indented 72px (node width + margin)
- Max content width: 800px

**Interaction states:**
- Hover: `cursor: pointer`, subtle highlight
- Active/Expanded: Purple accent color
- No animations or transitions (instant state changes)

### Responsive Behavior

**Out of scope for MVP:**
- Mobile layout
- Touch gestures
- Responsive breakpoints

**Acceptable:** Horizontal scrolling on narrow screens.

## Implementation

### File Structure

```
cmd/trace-viewer/
  main.go              - CLI entry point, flag parsing, orchestration
  generator.go         - HTML generation, template rendering
  data_loader.go       - Read and validate trace JSON
  enricher.go          - Detect schema features, compute derived data
  templates.go         - Embedded HTML/CSS/JS templates
  
  generator_test.go    - Unit tests for HTML generation
  data_loader_test.go  - Unit tests for trace loading
  enricher_test.go     - Unit tests for data enrichment
  
  testdata/
    fixtures/          - Sample trace files for testing
```

### CLI Interface

```bash
# Basic usage
trace-viewer --trace=.traces/session-001.json --output=timeline.html

# Open in browser automatically
trace-viewer --trace=.traces/session-001.json --open

# Verbose output
trace-viewer --trace=session.json --verbose

# Help
trace-viewer --help
```

**Flags:**
- `--trace` (required): Path to trace JSON file
- `--output` (default: `<trace-basename>-timeline.html`): Output HTML path
- `--open` (default: false): Open in browser after generation
- `--verbose` (default: false): Print detailed progress

### Makefile Integration

```makefile
build-viewer:  ## Build trace-viewer binary
	go build -o trace-viewer ./cmd/trace-viewer

trace-view:  ## Generate timeline for most recent trace
	@LATEST=$$(ls -t .traces/*.json | head -1) && \
	./trace-viewer --trace=$$LATEST --open

.PHONY: build-viewer trace-view
```

### HTML Template Structure

```html
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Trace Timeline: {{.SessionID}}</title>
    <style>
        /* Inline CSS here */
    </style>
</head>
<body>
    <div class="container">
        <header>
            <h1>Trace Timeline</h1>
            <div class="metadata">
                <span>Session: {{.SessionID}}</span>
                <span>Query: {{.Query}}</span>
                <span>Duration: {{.DurationMs}}ms</span>
            </div>
        </header>

        <div id="timeline">
            <!-- Timeline rendered by JavaScript -->
        </div>

        <div id="modal" class="modal hidden">
            <!-- Modal content -->
        </div>
    </div>

    <script type="application/json" id="trace-data">
        {{.TraceJSON}}
    </script>

    <script>
        // Vanilla JavaScript for timeline rendering and interaction
    </script>
</body>
</html>
```

### JavaScript Architecture

**Module structure:**

```javascript
// Data loading
const trace = JSON.parse(document.getElementById('trace-data').textContent);

// State management
const state = {
  expandedNodes: new Set(),
  modalVisible: false,
  modalContent: null
};

// Rendering functions
function renderTimeline(trace) { /* ... */ }
function renderTurnNode(turn, index) { /* ... */ }
function renderEventNode(event) { /* ... */ }
function renderDetailPanel(turn) { /* ... */ }

// Event handlers
function handleNodeClick(nodeId) { /* ... */ }
function handleDetailClose(nodeId) { /* ... */ }
function handleModalOpen(content) { /* ... */ }

// Initialize
renderTimeline(trace);
```

**No framework dependencies** — keep it simple with vanilla JS and DOM manipulation.

## Implementation Phases

### Phase 1: Core Timeline (MVP)

**Goal:** Basic timeline visualization with turn nodes and inline expansion.

**Deliverables:**
- Read trace JSON file
- Generate self-contained HTML
- Render horizontal timeline with turn nodes
- Click to expand inline detail panel
- Show basic turn metrics (tokens, duration)
- Show tool calls list in detail panel

**Acceptance criteria:**
- Can visualize existing trace files (current schema)
- Multiple turns can be expanded simultaneously
- HTML file works offline in browser
- Basic styling (circles, cards, colors)

### Phase 2: Events & Schema v1

**Goal:** Add context management events and first schema enhancements.

**Deliverables:**
- Render compaction/guardrail event nodes inline
- Add per-event timestamps support
- Add context snapshots support
- Progressive enhancement detection

**Acceptance criteria:**
- Compaction and guardrail events visible on timeline
- Event detail panels show relevant info
- Works with both old and new schema versions

### Phase 3: Remaining Schema Fields

**Goal:** Full schema support for all six extensions.

**Deliverables:**
- Tool relationships visualization (dependency notes)
- Compaction diffs (before/after comparison)
- Display metadata (rich labels, status)
- Rich error context (structured errors)

**Acceptance criteria:**
- All schema extensions supported
- Detail panels show comprehensive information
- Error states clearly visualized

### Phase 4: Polish & UX

**Goal:** Improve usability and add convenience features.

**Deliverables:**
- Modal for full I/O viewing (JSON formatted)
- Keyboard shortcuts (Escape to close panels)
- URL state (link to specific expanded nodes)
- Copy/export buttons in modal
- Makefile integration

**Acceptance criteria:**
- Smooth user experience for common workflows
- Easy to generate timelines from command line
- Can share links to specific trace states

## Testing Strategy

### Unit Tests

**Backend (Go):**
- `data_loader_test.go`: Read and validate trace JSON
- `enricher_test.go`: Schema feature detection, derived data
- `generator_test.go`: HTML generation, template rendering

**Test fixtures:**
- `minimal-trace.json`: Single turn, no tools
- `normal-trace.json`: Multiple turns with tools
- `compaction-trace.json`: Includes compaction events
- `error-trace.json`: Contains failed tool calls
- `full-schema-trace.json`: All schema extensions present

### Integration Tests

**End-to-end:**
- Generate HTML from fixture
- Verify HTML structure (DOM nodes present)
- Check embedded JSON data
- Verify self-contained (no external deps)

### Manual Testing

**Browser compatibility:**
- Chrome (primary)
- Firefox
- Safari
- Edge

**Trace size testing:**
- Small: 5-10 turns
- Medium: 20-50 turns
- Large: 100+ turns (performance check)

## Open Questions

None — design is complete and ready for implementation.

## Success Metrics

**Adoption:**
- Tool is used by team for debugging traces
- Reduces time to diagnose trace issues

**Quality:**
- No critical bugs in first month
- Handles all existing trace formats correctly

**Scalability:**
- Handles traces up to 100 turns without performance issues
- HTML files remain under 5MB for typical traces

## Future Enhancements

**Not in scope for initial release, but could be added later:**

1. **Advanced comparison mode** — explicit UI for comparing multiple turns side-by-side
2. **Temporal waterfall view** — visualize timing overlaps and parallelism
3. **Filtering UI** — interactive filters for tool types, token ranges, etc.
4. **Server mode** — for very large traces or real-time monitoring
5. **Export formats** — PDF, PNG screenshots, or shareable links
6. **Dark mode** — toggle between light/dark themes
7. **Minimap** — overview of entire timeline for quick navigation
8. **Search** — find specific tools or events in timeline
9. **Annotations** — add notes or bookmarks to specific turns
10. **Multi-trace view** — compare timelines across multiple runs

These can be prioritized based on user feedback after initial release.
