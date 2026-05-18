package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Trace structures
type Trace struct {
	SessionID   string `json:"session_id"`
	Query       string `json:"query"`
	Model       string `json:"model"`
	Turns       []Turn `json:"turns"`
	PromptHash  string `json:"prompt_hash"`
	TotalCostUSD float64 `json:"total_cost_usd"`
}

type Turn struct {
	Index     int       `json:"index"`
	LLMCall   LLMCall   `json:"llm_call"`
	ToolCalls []ToolCall `json:"tool_calls"`
}

type LLMCall struct {
	Output Output `json:"output"`
}

type Output struct {
	Content []ContentItem `json:"content"`
}

type ContentItem struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type ToolCall struct {
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
	Output json.RawMessage `json:"output"`
}

// Dataset structures
type Dataset struct {
	SessionID    string           `json:"session_id"`
	Query        string           `json:"query"`
	Model        string           `json:"model"`
	ExtractedAt  string           `json:"extracted_at"`
	TotalEntries int              `json:"total_entries"`
	Entries      []DatasetEntry   `json:"entries"`
}

type DatasetEntry struct {
	Turn            int             `json:"turn"`
	ToolName        string          `json:"tool_name"`
	Rationale       string          `json:"rationale"`
	RationaleFound  bool            `json:"rationale_found"`
	Input           json.RawMessage `json:"input"`
	Output          json.RawMessage `json:"output"`
	OutputLength    int             `json:"output_length"`
}

// Validation structures
type ValidationReport struct {
	SessionID        string              `json:"session_id"`
	TotalEntries     int                 `json:"total_entries"`
	AccuracyScore    float64             `json:"accuracy_score"`
	CompletionScore  float64             `json:"completion_score"`
	ValidatedAt      string              `json:"validated_at"`
	EntryValidations []EntryValidation   `json:"entry_validations"`
	Summary          string              `json:"summary"`
}

type EntryValidation struct {
	Turn             int     `json:"turn"`
	ToolName         string  `json:"tool_name"`
	RationaleFound   bool    `json:"rationale_found"`
	RationaleQuality string  `json:"rationale_quality"` // "complete", "partial", "missing", "incorrect"
	Issues           []string `json:"issues,omitempty"`
	Score            float64 `json:"score"` // 0.0 to 1.0
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <trace-file.json>\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\nExtracts tool evaluation dataset from trace and validates extraction.\n")
		os.Exit(1)
	}

	traceFile := os.Args[1]

	// Extract dataset
	fmt.Println("=== EXTRACTING TOOL EVALUATION DATASET ===")
	fmt.Println()
	dataset, err := extractDataset(traceFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error extracting dataset: %v\n", err)
		os.Exit(1)
	}

	// Save dataset
	datasetFile := strings.TrimSuffix(traceFile, filepath.Ext(traceFile)) + "_dataset.json"
	if err := saveDataset(dataset, datasetFile); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving dataset: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✓ Dataset saved to: %s\n", datasetFile)
	fmt.Printf("  Total entries: %d\n\n", dataset.TotalEntries)

	// Validate extraction
	fmt.Println("=== VALIDATING EXTRACTION ===")
	fmt.Println()
	validation := validateExtraction(traceFile, dataset)

	// Save validation report
	validationFile := strings.TrimSuffix(traceFile, filepath.Ext(traceFile)) + "_validation.json"
	if err := saveValidation(validation, validationFile); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving validation: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✓ Validation report saved to: %s\n\n", validationFile)

	// Print summary
	printValidationSummary(validation)
}

func extractDataset(traceFile string) (*Dataset, error) {
	data, err := os.ReadFile(traceFile)
	if err != nil {
		return nil, fmt.Errorf("reading file: %w", err)
	}

	var trace Trace
	if err := json.Unmarshal(data, &trace); err != nil {
		return nil, fmt.Errorf("parsing JSON: %w", err)
	}

	dataset := &Dataset{
		SessionID:   trace.SessionID,
		Query:       trace.Query,
		Model:       trace.Model,
		ExtractedAt: time.Now().Format(time.RFC3339),
		Entries:     []DatasetEntry{},
	}

	for _, turn := range trace.Turns {
		// Extract the planning text
		planningText := extractPlanningText(turn.LLMCall.Output.Content)

		// Extract rationales for each tool
		toolRationales := extractToolRationales(planningText)

		// Match tool calls with rationales
		for _, toolCall := range turn.ToolCalls {
			rationale, found := toolRationales[toolCall.Name]

			entry := DatasetEntry{
				Turn:           turn.Index + 1,
				ToolName:       toolCall.Name,
				Rationale:      rationale,
				RationaleFound: found,
				Input:          toolCall.Input,
				Output:         toolCall.Output,
				OutputLength:   len(toolCall.Output),
			}
			dataset.Entries = append(dataset.Entries, entry)

			// Print extraction status
			status := "✗"
			if found {
				status = "✓"
			}
			fmt.Printf("Turn %d: %s %-25s %s\n",
				entry.Turn, status, entry.ToolName,
				truncate(entry.Rationale, 60))
		}
	}

	dataset.TotalEntries = len(dataset.Entries)
	return dataset, nil
}

func extractPlanningText(content []ContentItem) string {
	var text strings.Builder
	for _, item := range content {
		if item.Type == "text" {
			text.WriteString(item.Text)
			text.WriteString("\n")
		}
	}
	return text.String()
}

func extractToolRationales(planningText string) map[string]string {
	rationales := make(map[string]string)

	// Find the "This Turn's Tools" section
	lowerText := strings.ToLower(planningText)
	sectionStart := strings.Index(lowerText, "this turn's tools")
	if sectionStart == -1 {
		sectionStart = strings.Index(lowerText, "tools & rationale")
	}
	if sectionStart == -1 {
		return rationales // No tools section found
	}

	// Extract the tools section
	toolsSection := planningText[sectionStart:]
	nextHeadingIdx := strings.Index(toolsSection[50:], "##")
	if nextHeadingIdx != -1 {
		toolsSection = toolsSection[:50+nextHeadingIdx]
	}

	// Parse each line looking for tool definitions
	// Format: 1. `tool_name(...)` - Rationale text
	lines := strings.Split(toolsSection, "\n")

	// Regex to match: optional number, backtick, tool_name, optional params, backtick, dash, rationale
	toolPattern := regexp.MustCompile("`([a-z_]+)(?:\\([^)]*\\))?`\\s*-\\s*(.+)")

	for _, line := range lines {
		matches := toolPattern.FindStringSubmatch(line)
		if len(matches) >= 3 {
			toolName := matches[1]
			rationale := strings.TrimSpace(matches[2])

			// Store the rationale
			// Note: If tool is called multiple times, first rationale is used
			if _, exists := rationales[toolName]; !exists {
				rationales[toolName] = rationale
			}
		}
	}

	return rationales
}

func validateExtraction(traceFile string, dataset *Dataset) *ValidationReport {
	// Re-read the trace to validate
	data, _ := os.ReadFile(traceFile)
	var trace Trace
	json.Unmarshal(data, &trace)

	report := &ValidationReport{
		SessionID:        dataset.SessionID,
		TotalEntries:     dataset.TotalEntries,
		ValidatedAt:      time.Now().Format(time.RFC3339),
		EntryValidations: []EntryValidation{},
	}

	totalScore := 0.0
	foundCount := 0

	for _, entry := range dataset.Entries {
		validation := EntryValidation{
			Turn:           entry.Turn,
			ToolName:       entry.ToolName,
			RationaleFound: entry.RationaleFound,
			Issues:         []string{},
		}

		if !entry.RationaleFound {
			validation.RationaleQuality = "missing"
			validation.Issues = append(validation.Issues, "No rationale found in planning section")
			validation.Score = 0.0
		} else {
			foundCount++

			// Check rationale quality
			rationaleLen := len(entry.Rationale)

			if rationaleLen < 10 {
				validation.RationaleQuality = "incomplete"
				validation.Issues = append(validation.Issues, "Rationale too short (< 10 chars)")
				validation.Score = 0.3
			} else if rationaleLen < 30 {
				validation.RationaleQuality = "partial"
				validation.Issues = append(validation.Issues, "Rationale brief (< 30 chars)")
				validation.Score = 0.6
			} else {
				validation.RationaleQuality = "complete"
				validation.Score = 1.0

				// Bonus checks for quality
				lowerRat := strings.ToLower(entry.Rationale)
				if !strings.Contains(lowerRat, "find") &&
				   !strings.Contains(lowerRat, "get") &&
				   !strings.Contains(lowerRat, "search") &&
				   !strings.Contains(lowerRat, "identify") &&
				   !strings.Contains(lowerRat, "understand") &&
				   !strings.Contains(lowerRat, "explore") &&
				   !strings.Contains(lowerRat, "target") {
					validation.Issues = append(validation.Issues, "Rationale lacks clear action verb")
					validation.Score = 0.8
				}
			}

			// Check if output is present
			if len(entry.Output) == 0 {
				validation.Issues = append(validation.Issues, "Tool output is empty")
			}
		}

		report.EntryValidations = append(report.EntryValidations, validation)
		totalScore += validation.Score

		// Print validation status
		scoreBar := generateScoreBar(validation.Score)
		fmt.Printf("Turn %d: %s %-25s [%s] %s\n",
			validation.Turn, scoreBar, validation.ToolName,
			validation.RationaleQuality,
			strings.Join(validation.Issues, "; "))
	}

	// Calculate overall scores
	if dataset.TotalEntries > 0 {
		report.AccuracyScore = totalScore / float64(dataset.TotalEntries)
		report.CompletionScore = float64(foundCount) / float64(dataset.TotalEntries)
	}

	// Generate summary
	report.Summary = fmt.Sprintf(
		"Extracted %d tool calls. Found rationales for %d (%.1f%%). Average quality score: %.2f/1.0",
		dataset.TotalEntries, foundCount, report.CompletionScore*100, report.AccuracyScore,
	)

	return report
}

func saveDataset(dataset *Dataset, filename string) error {
	data, err := json.MarshalIndent(dataset, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filename, data, 0644)
}

func saveValidation(validation *ValidationReport, filename string) error {
	data, err := json.MarshalIndent(validation, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filename, data, 0644)
}

func printValidationSummary(validation *ValidationReport) {
	fmt.Println("=== VALIDATION SUMMARY ===")
	fmt.Printf("Session ID: %s\n", validation.SessionID)
	fmt.Printf("Total Entries: %d\n", validation.TotalEntries)
	fmt.Printf("Completion Score: %.1f%% (rationales found)\n", validation.CompletionScore*100)
	fmt.Printf("Accuracy Score: %.1f%% (average quality)\n", validation.AccuracyScore*100)
	fmt.Println()
	fmt.Printf("Quality Distribution:\n")

	qualityCounts := make(map[string]int)
	for _, ev := range validation.EntryValidations {
		qualityCounts[ev.RationaleQuality]++
	}

	for quality, count := range qualityCounts {
		pct := float64(count) / float64(validation.TotalEntries) * 100
		fmt.Printf("  %-12s: %2d (%.1f%%)\n", quality, count, pct)
	}
	fmt.Println()
	fmt.Println(validation.Summary)
}

func generateScoreBar(score float64) string {
	if score >= 0.9 {
		return "✓✓"
	} else if score >= 0.6 {
		return "✓~"
	} else if score > 0.0 {
		return "~✗"
	}
	return "✗✗"
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
