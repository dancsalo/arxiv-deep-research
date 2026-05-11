---
name: Research Plan Validation Tool
description: Implement create_research_plan tool to enforce planning phase
type: feature
parent: stage5-master
---

## Context

This plan implements the `create_research_plan` tool that the research agent must call before starting its research. The tool validates that the agent's plan includes the three required sections:
1. Seminal Papers (or similar: pivotal, highly-cited, foundational)
2. Fundamental Techniques (or similar: algorithms, methods, math)
3. Survey Papers (or similar: review, landscape, applications)

The validation uses flexible matching to accept variations in section names while ensuring all three conceptual areas are covered.

## Implementation Steps

### Step 1: Add Tool Definition

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

### Step 2: Add Result Types

**File**: `tools/research/handlers.go`

Add result struct:

```go
type ResearchPlanResult struct {
	Status   string   `json:"status"`   // "approved" or "rejected"
	Sections []string `json:"sections"` // List of approved section names
	Message  string   `json:"message"`  // Additional feedback
}
```

### Step 3: Implement Handler

**File**: `tools/research/handlers.go`

Add the handler implementation:

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
		b, _ := json.Marshal(result)
		return string(b), nil
	}

	// All sections present - approve plan
	result := ResearchPlanResult{
		Status:   "approved",
		Sections: sectionNames,
		Message:  fmt.Sprintf("Research plan approved for topic: %s. You may now proceed with your research.", params.Topic),
	}
	b, _ := json.Marshal(result)
	return string(b), nil
}

// matchesSeminalSection checks if a section name indicates seminal/pivotal papers
func matchesSeminalSection(name string) bool {
	keywords := []string{
		"seminal", "pivotal", "foundational", "highly-cited", "highly cited",
		"key papers", "important papers", "landmark", "influential",
	}
	for _, keyword := range keywords {
		if strings.Contains(name, keyword) {
			return true
		}
	}
	return false
}

// matchesTechniquesSection checks if a section name indicates fundamental techniques
func matchesTechniquesSection(name string) bool {
	keywords := []string{
		"fundamental", "technique", "algorithm", "method", "approach",
		"math", "mathematics", "mechanism", "architecture", "how it works",
	}
	for _, keyword := range keywords {
		if strings.Contains(name, keyword) {
			return true
		}
	}
	return false
}

// matchesSurveySection checks if a section name indicates survey papers
func matchesSurveySection(name string) bool {
	keywords := []string{
		"survey", "review", "landscape", "overview", "state of the art",
		"state-of-the-art", "comparison", "applications",
	}
	for _, keyword := range keywords {
		if strings.Contains(name, keyword) {
			return true
		}
	}
	return false
}
```

### Step 4: Create Tests

**File**: `tools/research/create_plan_test.go`

```go
package research

import (
	"context"
	"encoding/json"
	"testing"
)

func TestHandleCreateResearchPlanValid(t *testing.T) {
	toolset := NewResearchToolSet(&http.Client{}, "")

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
	toolset := NewResearchToolSet(&http.Client{}, "")

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
	toolset := NewResearchToolSet(&http.Client{}, "")

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
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
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
		{"how it works", "how it works", matchesTechniquesSection, true},
		{"method", "main method", matchesTechniquesSection, true},
		{"random", "random section", matchesTechniquesSection, false},

		// Survey section matching
		{"survey", "survey papers", matchesSurveySection, true},
		{"review", "review literature", matchesSurveySection, true},
		{"landscape", "research landscape", matchesSurveySection, true},
		{"state of the art", "state of the art", matchesSurveySection, true},
		{"random", "random section", matchesSurveySection, false},
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
```

## Critical Files

- `tools/research/definitions.go` - Tool schema
- `tools/research/handlers.go` - Tool implementation and matching functions
- `tools/research/create_plan_test.go` - Tests

## Verification

1. **Build**:
   ```bash
   make build
   ```

2. **Run tests**:
   ```bash
   go test ./tools/research -run TestHandleCreateResearchPlan -v
   go test ./tools/research -run TestSectionMatching -v
   ```

3. **Manual test** (after integration):
   ```bash
   ./bin/research-demo --query "transformers"
   ```
   Check trace logs for:
   - `create_research_plan` called in turn 0 or 1
   - Status: "approved"
   - All three sections detected

4. **Test rejection**:
   Modify system prompt temporarily to only require 2 sections, verify rejection message.

## Notes

- Section matching is case-insensitive for flexibility
- Multiple keywords per section type to handle variations
- Clear error messages guide the agent if plan is rejected
- Tool can be extended to validate other plan aspects (e.g., minimum number of papers)
- The approval flow prevents the agent from starting research without a plan
