# Research Prompt Improvement Design

## Problem Statement

The current research-demo system prompt produces limited research depth and breadth:

**Current behavior:**
- Stops after 4-6 turns (max 10 configured)
- Calls only 1 tool per turn
- Sequential search pattern (no parallel exploration)
- No structured output format
- No tool variety enforcement
- No sequential chaining patterns

**User needs:**
1. Structured markdown output matching specific template
2. More turns utilized (10-15 range)
3. Intelligent breadth-first tool selection
4. Variety in tool usage (avoid over-relying on 1-2 tools)
5. Both parallel tool calls AND sequential chaining

## Requirements

### Functional Requirements

**FR1: Three Prompt Variants**

Implement three distinct prompting approaches:
- **Variant A (Explicit)**: Turn-by-turn guidance, prescribed tool patterns, enforced parallelism
- **Variant B (Metacognitive)**: Strategy planning, self-reflection, adaptive selection
- **Variant C (Reward-Shaping)**: Quality metrics, scoring rubric, achievement-based completion

**FR2: Variant Selection**

- Add `--prompt-variant` flag to research-demo (values: "A", "B", "C", default: "A")
- Store prompts in text files: `cmd/research-demo/prompts/variant-{a,b,c}.txt`
- Include variant identifier and prompt hash in trace output

**FR3: Structured Output Template**

All variants enforce identical markdown output structure:
- `## Overview` - 2-3 paragraph executive summary
- `## Key Papers and Contributions` - 5-7 papers with full details
- `## Technical Deep Dive` - Detailed explanation of approaches/methods
- `## Leading Researchers and Groups` - 3-5 key researchers
- `## Code Implementations` - 2-3 notable repositories (if relevant)
- `## Current Trends and Future Directions` - 2-3 paragraphs
- `## Additional Resources` - Web tutorials, documentation

**FR4: Turn Target**

- All variants target 10-15 turns
- Prompts discourage early stopping
- Variant A prescribes turns 1-4, 5-10, 11-15 phases
- Variants B/C use criteria-based stopping that requires depth

**FR5: Tool Usage Patterns**

**Parallel calls:**
- Variant A: MUST call 2+ tools in parallel for exploratory turns
- Variants B/C: Encourage parallel calls when exploring different aspects

**Sequential chaining:**
- All variants support: find paper → fetch PDF → search author → find implementations
- Variant A provides explicit chaining examples
- Variants B/C discover chains through metacognition/optimization

**Tool variety:**
- Variant A: Requires using all 5 tools at least once
- Variant B: Self-reflection prompts ask "What tools haven't I used?"
- Variant C: Quality score includes tool diversity metric (4+ tool types)

**FR6: Trace Comparison Tool**

Create `cmd/compare-traces/main.go` that:
- Loads multiple trace JSON files
- Displays side-by-side metrics table (turns, tools, diversity, cost, time)
- Shows parallel vs sequential tool usage patterns
- Optionally diffs final summaries

### Non-Functional Requirements

**NFR1: Backward Compatibility**
- Existing research-demo behavior unchanged when `--prompt-variant` not specified (defaults to A)
- Trace format extended, not replaced (new fields added)

**NFR2: Prompt Maintainability**
- Prompts in separate text files (not hardcoded in Go)
- Prompt hash in trace enables detecting drift/changes

**NFR3: Cost Management**
- All variants respect existing `--max-cost` flag
- Longer research sessions expected, but hard cap remains

## Design

### Architecture

```
cmd/research-demo/
├── main.go                      # Add --prompt-variant flag, load prompt file
├── prompts/
│   ├── variant-a-explicit.txt       # Explicit multi-tool prompting
│   ├── variant-b-metacognitive.txt  # Metacognitive prompting
│   └── variant-c-reward.txt         # Reward-shaping prompting

cmd/compare-traces/
└── main.go                      # Trace comparison tool

internal/tracing/
└── types.go                     # Add PromptVariant, PromptHash fields
```

### Prompt Variant A: Explicit Multi-Tool

**Core strategy:** Prescriptive turn-by-turn guidance with explicit tool patterns.

**Structure:**
1. Role & Goal (existing)
2. Available Tools (existing + usage hints)
3. **Research Phases** (NEW)
   - Phase 1 (Turns 1-4): Broad Exploration
   - Phase 2 (Turns 5-10): Deep Investigation
   - Phase 3 (Turns 11-15): Synthesis & Validation
4. **Tool Usage Rules** (NEW)
   - Parallel call requirements
   - Sequential chaining examples
   - Tool variety enforcement
5. Output Template Specification (NEW)
6. Finish Tool Requirements (existing + template compliance)

**Key instructions:**

```
Research Phases:

Phase 1 (Turns 1-4): Broad Exploration
- Turn 1: Call search_arxiv + search_openalex + search_web in parallel to understand the landscape
  Example: Search all three for your main query to see academic, code, and tutorial perspectives
- Turns 2-3: Based on initial findings, explore 2-3 specific angles in parallel
  Example: If you found "transformers for NLP" and "transformers for vision", search both paths
- Turn 4: Identify key papers, authors, and research directions for deep investigation

Phase 2 (Turns 5-10): Deep Investigation
- For each key paper/direction, use sequential chains:
  Example chain: search_openalex for paper → fetch_arxiv_pdf → search author's other work → search_github_repos for implementations
- Use parallel calls when investigating independent aspects
  Example: Investigate "training efficiency" angle + "architecture variants" angle in parallel
- Target: 3-4 complete investigation chains

Phase 3 (Turns 11-15): Synthesis & Validation
- Verify key claims across multiple sources
- Fill remaining gaps in understanding
- Ensure all template sections have sufficient content
- Finalize comprehensive summary

Tool Usage Rules:
- MUST call 2+ tools in parallel when exploring different aspects of the topic
- Single tool calls are only appropriate when chaining (result from tool N feeds into tool N+1)
- Use all 5 tool types at least once during your research
- Parallel example: search_arxiv("transformers") + search_openalex("transformers attention") + search_web("transformer tutorial")
- Chain example: search_openalex("attention is all you need") → fetch_arxiv_pdf("1706.03762") → search_github_repos("transformer implementation pytorch")
```

### Prompt Variant B: Metacognitive

**Core strategy:** Strategy planning before turns, self-reflection after turns, adaptive execution.

**Structure:**
1. Role & Goal (existing)
2. Available Tools (existing)
3. **Metacognitive Framework** (NEW)
   - Pre-turn planning template
   - Post-turn reflection template
   - Quality gates for stopping
4. Output Template Specification (NEW)
5. Finish Tool Requirements (existing + quality criteria)

**Key instructions:**

```
Metacognitive Research Process:

Before each turn, explicitly state:
1. What I learned from previous searches (or "first turn" if starting)
2. What gaps remain in my understanding
3. Which tools I'll use this turn and why
4. What angles I'm exploring in parallel (if any)

Example pre-turn planning:
"From previous turns, I learned transformers dominate NLP but I haven't explored vision applications yet. 
Gaps: No code implementations found, limited understanding of training techniques.
This turn: search_github_repos for implementations + search_web for training tutorials (parallel).
Angles: Practical implementation perspective."

After each turn, reflect:
- What surprised me in these results?
- What tools haven't I used yet that might provide different insights?
- Am I exploring breadth (multiple angles) or depth (one angle deeply) right now?
- Do I have enough information to fill the output template sections?

Stop searching when you can confidently answer ALL of these:
- What are the key papers? [Need 5-7 papers with authors, contributions, citations]
- Who are the leading researchers? [Need 3-5 researchers with affiliations and focus]
- What are the main technical approaches? [Need detailed understanding of 2-3 methods]
- What are current trends and future directions? [Need evidence-based claims]
- What implementations exist? [Need 2-3 notable repositories]

If you cannot answer all questions with confidence, continue researching. Target 10-15 turns.

When exploring breadth, use parallel tool calls to investigate multiple aspects simultaneously.
When going deep, use sequential chains to thoroughly investigate one aspect.
```

### Prompt Variant C: Reward-Shaping

**Core strategy:** Quality metrics and scoring, incentive alignment, self-regulation.

**Structure:**
1. Role & Goal (existing)
2. Available Tools (existing)
3. **Quality Criteria & Scoring** (NEW)
   - Research quality dimensions
   - Scoring rubric
   - Excellence threshold for completion
4. Output Template Specification (NEW)
5. Finish Tool Requirements (existing + quality score requirement)

**Key instructions:**

```
Research Quality Framework:

Your research will be evaluated on these dimensions:

✓ Tool Diversity (0-20 points)
  - 20 pts: Used all 5 tool types, each providing unique insights
  - 15 pts: Used 4 tool types effectively
  - 10 pts: Used 3 tool types
  - <10 pts: Over-relied on 1-2 tools

✓ Source Triangulation (0-20 points)
  - 20 pts: All key claims verified across 2+ independent sources
  - 15 pts: Most key claims triangulated
  - 10 pts: Some triangulation attempted
  - <10 pts: Single-source claims

✓ Depth of Exploration (0-20 points)
  - 20 pts: 12+ tool calls showing iterative refinement and follow-up
  - 15 pts: 10-11 tool calls with some iteration
  - 10 pts: 8-9 tool calls
  - <10 pts: Superficial exploration (<8 calls)

✓ Breadth of Coverage (0-20 points)
  - 20 pts: Explored 4+ distinct aspects/approaches/angles
  - 15 pts: Explored 3 aspects
  - 10 pts: Explored 2 aspects
  - <10 pts: Single narrow angle

✓ Implementation Grounding (0-20 points)
  - 20 pts: Found 3+ code repositories with analysis of their approaches
  - 15 pts: Found 2 repositories
  - 10 pts: Found 1 repository
  - <10 pts: No code implementations found (0 points if topic has implementations)

Target Score: 90+ points (Excellent)
Acceptable: 75-89 points (Good)
Insufficient: <75 points (Continue researching)

Strategies to maximize score:
- Use parallel tool calls to explore breadth (multiple aspects at once)
- Use sequential chains for depth (paper → PDF → author → implementations)
- Diversify tool usage early (don't save some tools for later)
- Verify important claims across academic (arXiv/OpenAlex) + practical (GitHub/web) sources
- Aim for 10-15 turns with purposeful, strategic tool usage

Only call finish_loop when you've achieved 90+ points (Excellent quality).
Self-assess your score before finishing.
```

### Shared Output Template

All three variants require this exact format in `finish_loop` call:

```markdown
# Research Summary: {Topic}

## Overview
[2-3 paragraph executive summary covering: what this field is, why it matters, current state, key developments]

## Key Papers and Contributions
### Paper 1: {Title} ({Authors}, {Year})
- **Main Contribution:** [1-2 sentences]
- **Key Findings:** [Bullet points]
- **Citations/Impact:** [If available from OpenAlex]
- **Links:** [arXiv URL, PDF URL if fetched]

[Repeat for 5-7 most important papers]

## Technical Deep Dive
### Approach/Method 1: {Name}
[Detailed explanation of technical approach, 2-3 paragraphs]

### Approach/Method 2: {Name}
[If there are competing approaches or important variations]

## Leading Researchers and Groups
- **{Researcher Name}** ({Institution}): [Focus area, key contributions]
[3-5 key researchers]

## Code Implementations
### {Repository Name}
- **Stars:** {count}
- **Language:** {language}
- **Description:** [What it implements]
- **URL:** {github url}

[2-3 notable repositories]

## Current Trends and Future Directions
[2-3 paragraphs covering: where the field is heading, open problems, emerging directions]

## Additional Resources
- [Web tutorials, documentation, blog posts found via search_web]

---
*Research conducted using: [list tools used] | Total sources: [count]*
```

**Minimum content requirements:**
- At least 5 papers with full citations and contributions
- At least 3 leading researchers identified
- At least 2 code implementations (if relevant to topic; state "No implementations found" if genuinely none)
- Technical Deep Dive with sufficient detail for technical reader
- Overview and Future Directions are mandatory sections

### Trace Format Changes

Add to trace JSON:

```json
{
  "prompt_variant": "A",
  "prompt_hash": "a3f5c2d1",
  "turns": [...],
  ...
}
```

**prompt_variant**: "A", "B", or "C"
**prompt_hash**: First 8 characters of SHA256 hash of the prompt text (detects prompt changes)

### Compare Tool Design

**Usage:**
```bash
compare-traces .traces/demo-1234.json .traces/demo-5678.json .traces/demo-9012.json
```

**Output:**
```
Comparing 3 traces:

Query: "transformers"

┌──────────┬───────┬──────┬───────────┬──────────┬──────────┐
│ Variant  │ Turns │ Cost │ Duration  │ Tools    │ Unique   │
├──────────┼───────┼──────┼───────────┼──────────┼──────────┤
│ A        │ 12    │ $0.42│ 45s       │ 18 calls │ 5 types  │
│ B        │ 9     │ $0.31│ 38s       │ 11 calls │ 4 types  │
│ C        │ 14    │ $0.48│ 52s       │ 21 calls │ 5 types  │
└──────────┴───────┴──────┴───────────┴──────────┴──────────┘

Tool Usage Breakdown:
  A: search_arxiv(5) search_openalex(4) fetch_arxiv_pdf(3) search_github_repos(3) search_web(3)
  B: search_arxiv(4) search_openalex(4) search_github_repos(2) search_web(1)
  C: search_arxiv(6) search_openalex(6) fetch_arxiv_pdf(4) search_github_repos(3) search_web(2)

Parallel Tool Usage:
  A: 8 turns with 2+ tools (67%)
  B: 3 turns with 2+ tools (33%)
  C: 6 turns with 2+ tools (43%)

Sequential Chains Detected:
  A: 3 chains (search → fetch → search)
  B: 1 chain
  C: 2 chains

Summary Lengths:
  A: 2,450 words
  B: 1,820 words
  C: 2,680 words
```

**Optional flags:**
- `--diff-summaries` - Show side-by-side summary comparison
- `--json` - Output as JSON for programmatic analysis

## Implementation Phases

### Phase 1: Prompt Files
1. Write variant-a-explicit.txt with full prompt text
2. Write variant-b-metacognitive.txt with full prompt text
3. Write variant-c-reward.txt with full prompt text
4. All three include identical output template specification

### Phase 2: Variant Selection
1. Add `--prompt-variant` flag to cmd/research-demo/main.go
2. Implement prompt file loading logic
3. Compute and store prompt hash (SHA256)
4. Add prompt_variant and prompt_hash to trace JSON

### Phase 3: Compare Tool
1. Create cmd/compare-traces/main.go
2. Implement trace loading and parsing
3. Implement metrics calculation (turns, tools, parallel usage, chains)
4. Implement table output formatting
5. Add optional --diff-summaries and --json flags

### Phase 4: Testing & Validation
1. Run same query with all 3 variants
2. Verify turn counts in 10-15 range
3. Verify parallel tool usage (A > C > B expected)
4. Verify output template compliance
5. Compare quality and cost trade-offs

## Success Metrics

**Turn Utilization:**
- Baseline: 4-6 turns
- Target: 10-15 turns
- Success: All variants average 10+ turns

**Parallel Tool Usage:**
- Baseline: 0% of turns have parallel calls
- Target: 30-50% of turns have parallel calls
- Success: Variant A >60%, Variant C >40%, Variant B >25%

**Tool Diversity:**
- Baseline: Typically 2-3 tool types used
- Target: All 5 tool types used
- Success: Variants A and C use all 5 types consistently

**Output Quality:**
- All variants produce complete template compliance
- Minimum content requirements met (5 papers, 3 researchers, 2 repos)
- Technical depth sufficient for expert readers

**Comparison Tool:**
- Can load and compare 2-10 traces
- Metrics accurately reflect trace content
- Table output is readable and informative

## Open Questions

**Q: Should we enforce template compliance programmatically?**
**A:** No. Prompt provides clear template, but we trust model to follow it. Manual inspection during testing validates compliance.

**Q: What if a topic genuinely has no code implementations?**
**A:** Prompt includes "(if relevant to topic)" and allows "No implementations found" statement. Not all research has GitHub repos.

**Q: How do we detect sequential chains programmatically?**
**A:** Look for tool call patterns where output of call N appears in input of call N+1 (e.g., arXiv ID from search appears in fetch_pdf call).

**Q: Should compare tool show actual summaries or just metrics?**
**A:** Metrics by default (concise comparison). Add `--diff-summaries` flag for users who want to see content differences.

**Q: Default variant for production use?**
**A:** Variant A (explicit) gives most control and predictability. Users can experiment with B/C but A is the recommended default.
