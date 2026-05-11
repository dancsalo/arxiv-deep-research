---
name: Research Plan Validation Tool  
description: DEFERRED - Implement create_research_plan tool only if prompt-based planning fails
type: feature
parent: stage5-master
status: deferred
---

## Status: DEFERRED PENDING EVIDENCE

**DO NOT IMPLEMENT in Stage 5.** This plan is ready for implementation but should only be executed if baseline testing shows the agent fails to self-organize research plans.

## Activation Criteria

Implement this tool ONLY IF baseline evaluation (10-20 research sessions with stage5-01, stage5-02, stage5-03 tools) shows:
- **>30% of outputs lack clear section structure**, OR
- **>20% of outputs miss one of the three required areas** (seminal papers, fundamental techniques, survey papers), OR  
- **User feedback indicates confusion or missing coverage**

If the agent already produces well-structured outputs with prompt-based guidance (as observed in current traces), this tool is unnecessary overhead.

## Context

This plan implements the `create_research_plan` tool that would require the research agent to declare its plan before starting research. The tool validates that the plan includes three required sections:
1. Seminal Papers (or similar: pivotal, highly-cited, foundational)
2. Fundamental Techniques (or similar: algorithms, methods, math)  
3. Survey Papers (or similar: review, landscape, applications)

The validation uses word-boundary regex matching to prevent false positives while accepting natural variations in section names.

### Why Deferred

Current evidence suggests this is **solving a hypothetical problem**:
- Existing traces show the agent produces structured output without validation
- System prompt updates in stage5-05 already mandate planning
- The tool validates naming conventions, not plan quality
- Engineering cost (440 lines, tests, maintenance) isn't justified without observed failures

### Rationale (If Implemented)

If baseline testing reveals planning failures, this tool would serve two purposes:
1. **Observability**: Measurable data on agent planning compliance
2. **Guardrails**: Prevention of silent degradation

### Alternative Approach (RECOMMENDED FOR STAGE 5)

**Prompt-only baseline**: Stage5-05 system prompt explicitly instructs structured planning. Test this first:
```
BEFORE searching, consider your research plan covering three areas:
1. Seminal/highly-cited papers that established the field
2. Fundamental techniques/algorithms that define how it works  
3. Survey/review papers that synthesize current state
```

Measure baseline compliance before adding enforcement tooling.

## Implementation Steps

### Step 1: Register Tool

**File**: `tools/research/research.go`

**ADD** the fourth tool registration to the existing `Register()` method (currently lines 18-22):

```go
func (r *ResearchToolSet) Register(reg *registry.ToolRegistry) {
	reg.Register("search_arxiv", BuildSearchArxivTool(), r.handleSearchArxiv)
	reg.Register("search_openalex", BuildSearchOpenAlexTool(), r.handleSearchOpenAlex)
	reg.Register("fetch_arxiv_pdf", BuildFetchArxivPdfTool(), r.handleFetchArxivPdf)
	reg.Register("create_research_plan", BuildCreateResearchPlanTool(), r.handleCreateResearchPlan) // ADD THIS LINE
}
```

This is the critical integration step. Without this line, the tool exists but is never callable.

### Step 2: Add Tool Definition

**File**: `tools/research/definitions.go`

Add the tool definition:

```go
func BuildCreateResearchPlanTool() anthropic.ToolUnionParam {
	t := anthropic.ToolUnionParamOfTool(
		anthropic.ToolInputSchemaParam{
			Type: "object",
			Properties: map[string]any{
				"topic": map[string]any{
					"type":        "string",
					"description": "The research topic being investigated",
				},
				"sections": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"name": map[string]any{
								"type":        "string",
								"description": "Section name (e.g., 'Seminal Papers', 'Fundamental Techniques')",
							},
							"description": map[string]any{
								"type":        "string",
								"description": "Brief description of what this section will cover",
							},
						},
						"required": []string{"name", "description"},
					},
					"description": "Planned research sections. Must include: 1) Seminal/pivotal papers, 2) Fundamental techniques/algorithms, 3) Survey/review papers",
				},
			},
			Required: []string{"topic", "sections"},
		},
		"create_research_plan",
	)
	t.OfTool.Description = anthropic.String(
		"Create and validate a research plan before starting research. This tool MUST be called first. Validates that your plan includes the three required sections: seminal papers, fundamental techniques, and survey papers. Returns approval once plan is validated.",
	)
	return t
}
```

### Step 3: Add Result Types

**File**: `tools/research/handlers.go`

Add result struct:

```go
type ResearchPlanResult struct {
	Status   string   `json:"status"`   // "approved" or "rejected"
	Sections []string `json:"sections"` // List of approved section names
	Message  string   `json:"message"`  // Additional feedback
}
```

### Step 4: Implement Handler

**File**: `tools/research/handlers.go`

First, add the required import at the top of the file:

```go
import (
	// ... existing imports ...
	"regexp"
)
```

Then add the handler implementation and validation functions:

```go
func (r *ResearchToolSet) handleCreateResearchPlan(ctx context.Context, input json.RawMessage) (string, error) {
	var params struct {
		Topic    string `json:"topic"`
		Sections []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"sections"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return toolError("invalid input: "+err.Error(), false), nil
	}

	if params.Topic == "" {
		return toolError("topic is required", false), nil
	}

	if len(params.Sections) == 0 {
		return toolError("sections array is required and cannot be empty", false), nil
	}

	// Validate required sections are present
	hasSeminal := false
	hasTechniques := false
	hasSurvey := false

	sectionNames := make([]string, 0, len(params.Sections))
	for _, section := range params.Sections {
		name := strings.ToLower(section.Name)
		sectionNames = append(sectionNames, section.Name)

		// Check for seminal papers section
		if matchesSeminalSection(name) {
			hasSeminal = true
		}

		// Check for fundamental techniques section
		if matchesTechniquesSection(name) {
			hasTechniques = true
		}

		// Check for survey papers section
		if matchesSurveySection(name) {
			hasSurvey = true
		}
	}

	// Build validation result
	var missing []string
	if !hasSeminal {
		missing = append(missing, "seminal/pivotal/highly-cited papers")
	}
	if !hasTechniques {
		missing = append(missing, "fundamental techniques/algorithms/methods")
	}
	if !hasSurvey {
		missing = append(missing, "survey/review papers")
	}

	if len(missing) > 0 {
		result := ResearchPlanResult{
			Status:   "rejected",
			Sections: sectionNames,
			Message:  fmt.Sprintf("Plan is missing required sections: %s. Please revise your plan to include all three required areas.", strings.Join(missing, ", ")),
		}
		b, err := json.Marshal(result)
		if err != nil {
			return toolError("failed to marshal rejection response: "+err.Error(), false), nil
		}
		return string(b), nil
	}

	// All sections present - approve plan
	result := ResearchPlanResult{
		Status:   "approved",
		Sections: sectionNames,
		Message:  fmt.Sprintf("Research plan approved for topic: %s. You may now proceed with your research.", params.Topic),
	}
	b, err := json.Marshal(result)
	if err != nil {
		return toolError("failed to marshal approval response: "+err.Error(), false), nil
	}
	return string(b), nil
}

// Compiled regexes for section matching (word boundaries prevent false positives)
var (
	seminalRegex = regexp.MustCompile(`(?i)\b(seminal|pivotal|foundational|highly.cited|landmark|influential)\b`)
	techniquesRegex = regexp.MustCompile(`(?i)\b(fundamental|technique|algorithm|method|approach|mathematics?|mechanism|architecture)\b`)
	surveyRegex = regexp.MustCompile(`(?i)\b(survey|review|landscape|overview|state.of.the.art|comparison)\b`)
)

// matchesSeminalSection checks if a section name indicates seminal/pivotal papers.
// Uses word boundaries to prevent false positives like "fundamental papers" matching techniques.
func matchesSeminalSection(name string) bool {
	return seminalRegex.MatchString(name)
}

// matchesTechniquesSection checks if a section name indicates fundamental techniques.
// Uses word boundaries to prevent false positives.
func matchesTechniquesSection(name string) bool {
	return techniquesRegex.MatchString(name)
}

// matchesSurveySection checks if a section name indicates survey papers.
// Uses word boundaries to prevent false positives.
func matchesSurveySection(name string) bool {
	return surveyRegex.MatchString(name)
}
```

### Step 5: Create Tests

**File**: `tools/research/create_plan_test.go`

```go
package research

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// Helper function for string containment checks
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

func TestHandleCreateResearchPlanValid(t *testing.T) {
	toolset := NewResearchToolSet(&http.Client{})

	tests := []struct {
		name  string
		input string
	}{
		{
			name: "exact section names",
			input: `{
				"topic": "transformers",
				"sections": [
					{"name": "Seminal Papers", "description": "Key papers"},
					{"name": "Fundamental Techniques", "description": "How attention works"},
					{"name": "Survey Papers", "description": "Applications and tradeoffs"}
				]
			}`,
		},
		{
			name: "alternative section names",
			input: `{
				"topic": "transformers",
				"sections": [
					{"name": "Pivotal Works", "description": "Highly-cited papers"},
					{"name": "Core Algorithms", "description": "Self-attention mechanism"},
					{"name": "Review Literature", "description": "Survey papers"}
				]
			}`,
		},
		{
			name: "mixed case and phrasing",
			input: `{
				"topic": "transformers",
				"sections": [
					{"name": "HIGHLY-CITED PAPERS", "description": "Most influential"},
					{"name": "how it works", "description": "Algorithm details"},
					{"name": "State of the Art", "description": "Recent surveys"}
				]
			}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := toolset.handleCreateResearchPlan(context.Background(), []byte(tt.input))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			var planResult ResearchPlanResult
			if err := json.Unmarshal([]byte(result), &planResult); err != nil {
				t.Fatalf("failed to parse result: %v", err)
			}

			if planResult.Status != "approved" {
				t.Errorf("expected approved status, got: %s (message: %s)", planResult.Status, planResult.Message)
			}

			if len(planResult.Sections) != 3 {
				t.Errorf("expected 3 sections, got: %d", len(planResult.Sections))
			}
		})
	}
}

func TestHandleCreateResearchPlanMissingSections(t *testing.T) {
	toolset := NewResearchToolSet(&http.Client{})

	tests := []struct {
		name          string
		input         string
		missingShouldContain string
	}{
		{
			name: "missing survey section",
			input: `{
				"topic": "transformers",
				"sections": [
					{"name": "Seminal Papers", "description": "Key papers"},
					{"name": "Algorithms", "description": "How it works"}
				]
			}`,
			missingShouldContain: "survey",
		},
		{
			name: "missing techniques section",
			input: `{
				"topic": "transformers",
				"sections": [
					{"name": "Important Papers", "description": "Highly cited"},
					{"name": "Review Papers", "description": "Surveys"}
				]
			}`,
			missingShouldContain: "techniques",
		},
		{
			name: "missing seminal section",
			input: `{
				"topic": "transformers",
				"sections": [
					{"name": "Algorithms", "description": "How it works"},
					{"name": "Surveys", "description": "Review papers"}
				]
			}`,
			missingShouldContain: "seminal",
		},
		{
			name: "only one section",
			input: `{
				"topic": "transformers",
				"sections": [
					{"name": "Random Section", "description": "Something"}
				]
			}`,
			missingShouldContain: "missing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := toolset.handleCreateResearchPlan(context.Background(), []byte(tt.input))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			var planResult ResearchPlanResult
			if err := json.Unmarshal([]byte(result), &planResult); err != nil {
				t.Fatalf("failed to parse result: %v", err)
			}

			if planResult.Status != "rejected" {
				t.Errorf("expected rejected status, got: %s", planResult.Status)
			}

			if !contains(strings.ToLower(planResult.Message), tt.missingShouldContain) {
				t.Errorf("expected message to contain %q, got: %s", tt.missingShouldContain, planResult.Message)
			}
		})
	}
}

func TestHandleCreateResearchPlanValidation(t *testing.T) {
	toolset := NewResearchToolSet(&http.Client{})

	tests := []struct {
		name        string
		input       string
		expectError bool
	}{
		{
			name:        "empty topic",
			input:       `{"topic": "", "sections": [{"name": "Test", "description": "Test"}]}`,
			expectError: true,
		},
		{
			name:        "empty sections",
			input:       `{"topic": "test", "sections": []}`,
			expectError: true,
		},
		{
			name:        "missing sections field",
			input:       `{"topic": "test"}`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := toolset.handleCreateResearchPlan(context.Background(), []byte(tt.input))
			// Note: Handler returns errors as JSON strings, not Go errors
			// So err is always nil, error info is in the result string
			if err != nil {
				t.Fatalf("unexpected Go error: %v", err)
			}

			hasError := contains(result, "error")
			if tt.expectError && !hasError {
				t.Errorf("expected error in result, got: %s", result)
			}
		})
	}
}

func TestSectionMatching(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		fn       func(string) bool
		expected bool
	}{
		// Seminal section matching
		{"seminal exact", "seminal papers", matchesSeminalSection, true},
		{"pivotal", "pivotal works", matchesSeminalSection, true},
		{"highly-cited", "highly-cited papers", matchesSeminalSection, true},
		{"influential", "influential papers", matchesSeminalSection, true},
		{"random", "random section", matchesSeminalSection, false},

		// Techniques section matching
		{"algorithm", "core algorithm", matchesTechniquesSection, true},
		{"technique", "fundamental techniques", matchesTechniquesSection, true},
		{"method", "main method", matchesTechniquesSection, true},
		{"random", "random section", matchesTechniquesSection, false},

		// Survey section matching
		{"survey", "survey papers", matchesSurveySection, true},
		{"review", "review literature", matchesSurveySection, true},
		{"landscape", "research landscape", matchesSurveySection, true},
		{"state of the art", "state of the art", matchesSurveySection, true},
		{"random", "random section", matchesSurveySection, false},
		
		// Edge cases: word boundaries prevent false positives
		{"substring seminal", "fundamental papers", matchesSeminalSection, false},
		{"partial match", "tech stuff", matchesTechniquesSection, false},
		
		// Multi-category matches (section names that match multiple categories)
		{"review methods", "review methods", matchesSurveySection, true},
		{"review methods techniques", "review methods", matchesTechniquesSection, true},
		{"fundamental applications", "fundamental applications", matchesTechniquesSection, true},
		{"fundamental applications survey", "fundamental applications", matchesSurveySection, false}, // "applications" alone doesn't match
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.fn(tt.input)
			if result != tt.expected {
				t.Errorf("expected %v for input %q, got %v", tt.expected, tt.input, result)
			}
		})
	}
}

func TestAntiGaming(t *testing.T) {
	// Test that agents can't cheat by cramming keywords into one section
	toolset := NewResearchToolSet(&http.Client{})

	tests := []struct {
		name        string
		input       string
		shouldPass  bool
		description string
	}{
		{
			name: "single section with all keywords",
			input: `{
				"topic": "transformers",
				"sections": [
					{"name": "Seminal Fundamental Survey Papers", "description": "All in one"}
				]
			}`,
			shouldPass:  true, // Current implementation allows this
			description: "Single section matches all categories (known limitation)",
		},
		{
			name: "overlapping keywords in different sections",
			input: `{
				"topic": "transformers",
				"sections": [
					{"name": "Important Review Papers", "description": "Seminal + survey"},
					{"name": "Core Algorithms", "description": "Techniques"},
					{"name": "Research Landscape", "description": "Survey"}
				]
			}`,
			shouldPass:  true,
			description: "Multiple sections, some overlap - acceptable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := toolset.handleCreateResearchPlan(context.Background(), []byte(tt.input))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			var planResult ResearchPlanResult
			if err := json.Unmarshal([]byte(result), &planResult); err != nil {
				t.Fatalf("failed to parse result: %v", err)
			}

			passed := (planResult.Status == "approved")
			if passed != tt.shouldPass {
				t.Errorf("%s: expected pass=%v, got status=%s", tt.description, tt.shouldPass, planResult.Status)
			}

			if tt.shouldPass && passed {
				t.Logf("KNOWN: %s", tt.description)
			}
		})
	}
}
```

## Success Metrics

Track these metrics to evaluate whether this tool adds value:

1. **Compliance rate**: % of research sessions where `create_research_plan` is called in turn 0-2
2. **Rejection rate**: % of plan submissions that get rejected
3. **Retry count**: Average number of plan submissions before approval
4. **Research quality**: Compare output quality with/without validation (subjective, sampled evaluation)

**Decision criteria** (evaluate after 20-30 research sessions):
- If compliance rate < 80% → Tool is being ignored, consider removal
- If rejection rate < 5% → Validation is redundant, consider removal
- If retry count > 2 → Validation is too strict, relax matching rules
- If research quality shows no improvement → Remove tool, rely on prompt alone

## Critical Files

- `tools/research/research.go` - Tool registration
- `tools/research/definitions.go` - Tool schema
- `tools/research/handlers.go` - Tool implementation and matching functions
- `tools/research/create_plan_test.go` - Tests
- `cmd/research-demo/main.go` - System prompt integration (stage5-05)

## Verification

1. **Build**:
   ```bash
   make build
   ```
   Verify no compilation errors.

2. **Run tests**:
   ```bash
   go test ./tools/research -run TestHandleCreateResearchPlan -v
   go test ./tools/research -run TestSectionMatching -v
   go test ./tools/research -run TestAntiGaming -v
   ```
   All tests should pass. Note that `TestAntiGaming` documents a known limitation (see below).

3. **Integration test** (after stage5-05 system prompt update):
   ```bash
   ./bin/research-demo --query "transformers" --output ./output/test_validation.json
   ```
   Check the trace log at `./output/test_validation.json`:
   - Find turn 0 or 1
   - Look for tool use: `create_research_plan`
   - Verify result has `"status":"approved"`
   - Verify agent proceeds with searches only after approval

4. **Rejection test**:
   Create a temporary test binary that modifies the system prompt to request only 2 sections (e.g., remove "survey papers"). Run research demo and verify:
   - Agent's plan gets `"status":"rejected"`
   - Agent retries with revised plan
   - Eventually submits a compliant plan and gets approved

5. **Manual inspection**:
   Read several research outputs and verify section structure matches expectations.

## Known Limitations

1. **Gaming via keyword stuffing**: An agent can pass validation by creating a single section named "Seminal Fundamental Survey Papers" that matches all three categories. The current logic checks if ANY section matches each category, not that DISTINCT sections match. This is acceptable for v1 because:
   - Agents following the prompt in good faith won't exploit this
   - If exploited frequently, metrics (rejection rate < 5%) will reveal the issue
   - Fix is straightforward: track which sections match which categories and require distinctness

2. **No enforcement of call ordering**: The tool validates plan structure but doesn't enforce that it's called FIRST. The agent can ignore the tool entirely or call it after starting research. The system prompt (stage5-05) must strongly emphasize "MUST be called first." Future improvement: agentic loop can enforce this at the framework level.

3. **False negatives**: Novel section names that don't match keywords (e.g., "Core Foundations") will fail validation even if conceptually correct. Mitigated by broad keyword coverage and clear error messages that guide revision.

## Decisions & Tradeoffs

| Decision | Rationale | Tradeoff |
|----------|-----------|----------|
| Regex with word boundaries | Prevents false positives (e.g., "tech" matching "technique") | Slightly more complex than simple substring matching |
| Three separate regexes | Clear separation of concerns, easy to extend keywords per category | Could be unified into one complex regex |
| Allow keyword variations | Agents can use natural phrasing ("highly-cited papers" vs "seminal papers") | Increases surface area for gaming |
| Return both approval and rejection as JSON | Consistent response schema for the agent | More complex than returning error strings for rejections |
| Implement as tool, not loop logic | Provides observability (tool use logs) and can be toggled via toolset | Adds turn overhead (1-2 turns for plan validation) |
| Experiment with metrics | Allows data-driven decision to keep/remove/refine | Requires instrumentation and evaluation effort |

**PM concern addressed**: This is explicitly an experiment with defined success criteria. If metrics show the tool adds friction without improving outcomes, we remove it. The cost of building it (1-2 hours) is worth the data we'll gain about agent planning behavior.

## Revision Log

### Revision 2 (2026-05-11) - DEFERRED STATUS
**Major decision**: Changed plan status to DEFERRED based on second-round critiques revealing lack of evidence for the problem this solves.

**Critical fixes** (making plan implementation-ready if activated):
- **Clarified**: Step 1 now explicitly shows tool registration as an ADDITION (line 42) with clear annotation
- **Fixed**: Added `contains()` helper function definition to test file imports
- **Fixed**: Updated all test references to use `contains()` consistently  
- **Enhanced**: `TestAntiGaming` now tests multiple edge cases, not just one
- **Added**: Multi-category matching tests in `TestSectionMatching`
- **Clarified**: Test error handling expectations with explanatory comment
- **Added**: Activation Criteria section — when to implement this plan
- **Added**: "Why Deferred" section — documenting evidence gap
- **Changed**: Status frontmatter to `status: deferred`
- **Added**: Recommended alternative approach (prompt-only baseline)

**Key insight from critiques**:
- Engineer: "Tool registration omission is inexcusable" → Fixed with clear ADD annotation
- PM: "This solves a hypothetical problem without evidence" → Changed to deferred, added baseline testing requirement

### Revision 1 (2026-05-11)
- **Added**: Tool registration step (research.go) — critical integration gap
- **Fixed**: Test constructor signatures (removed phantom second parameter)  
- **Fixed**: Missing `strings` import in tests
- **Changed**: Replaced substring matching with word-boundary regex to prevent false positives
- **Added**: Import statement for `regexp` in handlers.go
- **Fixed**: JSON marshal error handling (no longer ignores errors)
- **Added**: Success metrics section with decision criteria
- **Added**: `TestAntiGaming` test documenting gaming limitation
- **Added**: Known Limitations section acknowledging gaming, enforcement, and false negative issues
- **Added**: Decisions & Tradeoffs section justifying design choices
- **Added**: Rationale and alternatives in Context section (addressing PM critique)
- **Removed**: "how it works" from techniques matching (too vague, caused false positives)
- **Updated**: Verification instructions with concrete file paths and expected behaviors
