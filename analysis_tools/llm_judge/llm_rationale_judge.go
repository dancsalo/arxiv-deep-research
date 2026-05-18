package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/bedrock"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// Dataset structures (from extractor)
type Dataset struct {
	SessionID    string         `json:"session_id"`
	Query        string         `json:"query"`
	Model        string         `json:"model"`
	ExtractedAt  string         `json:"extracted_at"`
	TotalEntries int            `json:"total_entries"`
	Entries      []DatasetEntry `json:"entries"`
}

type DatasetEntry struct {
	Turn           int             `json:"turn"`
	ToolName       string          `json:"tool_name"`
	Rationale      string          `json:"rationale"`
	RationaleFound bool            `json:"rationale_found"`
	Input          json.RawMessage `json:"input"`
	Output         json.RawMessage `json:"output"`
	OutputLength   int             `json:"output_length"`
}

// Judgment structures
type JudgmentReport struct {
	SessionID       string             `json:"session_id"`
	Query           string             `json:"query"`
	JudgedAt        string             `json:"judged_at"`
	TotalEntries    int                `json:"total_entries"`
	OverallAccuracy float64            `json:"overall_accuracy"`
	ByTurn          map[int]*TurnStats `json:"by_turn"`
	ByTool          map[string]*ToolStats `json:"by_tool"`
	Judgments       []Judgment         `json:"judgments"`
	Summary         string             `json:"summary"`
}

type TurnStats struct {
	Turn      int     `json:"turn"`
	Total     int     `json:"total"`
	Fulfilled int     `json:"fulfilled"`
	Accuracy  float64 `json:"accuracy"`
}

type ToolStats struct {
	ToolName  string  `json:"tool_name"`
	Total     int     `json:"total"`
	Fulfilled int     `json:"fulfilled"`
	Accuracy  float64 `json:"accuracy"`
}

type Judgment struct {
	Turn       int    `json:"turn"`
	ToolName   string `json:"tool_name"`
	Rationale  string `json:"rationale"`
	Fulfilled  bool   `json:"fulfilled"`
	Confidence string `json:"confidence"` // "high", "medium", "low"
	Reasoning  string `json:"reasoning"`
	Score      float64 `json:"score"` // 0.0 to 1.0
}

type LLMJudgeResponse struct {
	Fulfilled  bool   `json:"fulfilled"`
	Confidence string `json:"confidence"`
	Reasoning  string `json:"reasoning"`
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <dataset.json>\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\nUses LLM to judge whether rationales were fulfilled by tool executions.\n")
		os.Exit(1)
	}

	datasetFile := os.Args[1]

	// Load dataset
	dataset, err := loadDataset(datasetFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading dataset: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("=== LLM RATIONALE FULFILLMENT JUDGE ===\n")
	fmt.Printf("Dataset: %s\n", datasetFile)
	fmt.Printf("Session: %s\n", dataset.SessionID)
	fmt.Printf("Query: %s\n", dataset.Query)
	fmt.Printf("Total entries: %d\n\n", dataset.TotalEntries)

	// Initialize LLM client
	ctx := context.Background()
	client, err := initClient(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing LLM client: %v\n", err)
		os.Exit(1)
	}

	// Judge each entry
	fmt.Println("Judging rationale fulfillment...")
	fmt.Println()

	report := &JudgmentReport{
		SessionID: dataset.SessionID,
		Query:     dataset.Query,
		JudgedAt:  time.Now().Format(time.RFC3339),
		TotalEntries: dataset.TotalEntries,
		ByTurn:    make(map[int]*TurnStats),
		ByTool:    make(map[string]*ToolStats),
		Judgments: []Judgment{},
	}

	for i, entry := range dataset.Entries {
		if !entry.RationaleFound {
			// Skip entries without rationales
			fmt.Printf("[%d/%d] Turn %d: %-20s SKIPPED (no rationale)\n",
				i+1, dataset.TotalEntries, entry.Turn, entry.ToolName)
			continue
		}

		fmt.Printf("[%d/%d] Turn %d: %-20s Judging... ",
			i+1, dataset.TotalEntries, entry.Turn, entry.ToolName)

		judgment, err := judgeEntry(ctx, client, entry)
		if err != nil {
			fmt.Printf("ERROR: %v\n", err)
			continue
		}

		report.Judgments = append(report.Judgments, judgment)

		// Print result
		symbol := "✗"
		if judgment.Fulfilled {
			symbol = "✓"
		}
		fmt.Printf("%s %s (%.0f%% confidence)\n", symbol, strings.ToUpper(confidenceLabel(judgment.Confidence)), judgment.Score*100)

		// Update stats
		updateStats(report, judgment)

		// Small delay to avoid rate limiting
		time.Sleep(500 * time.Millisecond)
	}

	// Calculate overall accuracy
	calculateAccuracy(report)

	// Save report
	reportFile := strings.TrimSuffix(datasetFile, "_dataset.json") + "_judgment.json"
	if err := saveReport(report, reportFile); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving report: %v\n", err)
		os.Exit(1)
	}

	// Print summary
	fmt.Println()
	printSummary(report)

	fmt.Printf("\n✓ Judgment report saved to: %s\n", reportFile)
}

func loadDataset(filename string) (*Dataset, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var dataset Dataset
	if err := json.Unmarshal(data, &dataset); err != nil {
		return nil, err
	}

	return &dataset, nil
}

func initClient(ctx context.Context) (*anthropic.Client, error) {
	// Try Bedrock first
	opts := []option.RequestOption{bedrock.WithLoadDefaultConfig(ctx)}
	client := anthropic.NewClient(opts...)
	return &client, nil
}

func judgeEntry(ctx context.Context, client *anthropic.Client, entry DatasetEntry) (Judgment, error) {
	// Prepare input/output as pretty JSON strings
	inputStr := formatJSON(entry.Input)
	outputStr := formatJSON(entry.Output)

	// Truncate output if too long (keep first 2000 chars)
	if len(outputStr) > 2000 {
		outputStr = outputStr[:2000] + "\n... (truncated)"
	}

	prompt := fmt.Sprintf(`You are judging whether a tool execution fulfilled its stated rationale.

RATIONALE: "%s"

TOOL: %s

INPUT:
%s

OUTPUT:
%s

Your task: Determine if the tool's output successfully fulfilled the stated rationale.

Consider:
1. Did the tool execute as intended based on the rationale?
2. Did the output provide the information the rationale promised?
3. Is the output quality sufficient to fulfill the rationale's goal?

Respond ONLY with a JSON object in this exact format:
{
  "fulfilled": true or false,
  "confidence": "high" or "medium" or "low",
  "reasoning": "Brief explanation (1-2 sentences)"
}`, entry.Rationale, entry.ToolName, inputStr, outputStr)

	// Call LLM
	maxTokens := int64(500)
	msg, err := client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model("us.anthropic.claude-sonnet-4-5-20250929-v1:0"),
		MaxTokens: maxTokens,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
		},
	})
	if err != nil {
		return Judgment{}, err
	}

	// Parse response
	responseText := ""
	for _, block := range msg.Content {
		if block.Type == "text" {
			responseText = block.Text
			break
		}
	}

	// Extract JSON from response (in case there's extra text)
	jsonStart := strings.Index(responseText, "{")
	jsonEnd := strings.LastIndex(responseText, "}")
	if jsonStart != -1 && jsonEnd != -1 {
		responseText = responseText[jsonStart : jsonEnd+1]
	}

	var llmResp LLMJudgeResponse
	if err := json.Unmarshal([]byte(responseText), &llmResp); err != nil {
		return Judgment{}, fmt.Errorf("parsing LLM response: %w (response: %s)", err, responseText)
	}

	// Convert to judgment
	score := 0.0
	if llmResp.Fulfilled {
		switch llmResp.Confidence {
		case "high":
			score = 1.0
		case "medium":
			score = 0.8
		case "low":
			score = 0.6
		default:
			score = 0.7
		}
	} else {
		switch llmResp.Confidence {
		case "high":
			score = 0.0
		case "medium":
			score = 0.2
		case "low":
			score = 0.4
		default:
			score = 0.3
		}
	}

	return Judgment{
		Turn:       entry.Turn,
		ToolName:   entry.ToolName,
		Rationale:  entry.Rationale,
		Fulfilled:  llmResp.Fulfilled,
		Confidence: llmResp.Confidence,
		Reasoning:  llmResp.Reasoning,
		Score:      score,
	}, nil
}

func formatJSON(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "{}"
	}

	var obj interface{}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return string(raw)
	}

	pretty, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		return string(raw)
	}

	return string(pretty)
}

func updateStats(report *JudgmentReport, judgment Judgment) {
	// Update by turn
	if _, exists := report.ByTurn[judgment.Turn]; !exists {
		report.ByTurn[judgment.Turn] = &TurnStats{Turn: judgment.Turn}
	}
	report.ByTurn[judgment.Turn].Total++
	if judgment.Fulfilled {
		report.ByTurn[judgment.Turn].Fulfilled++
	}

	// Update by tool
	if _, exists := report.ByTool[judgment.ToolName]; !exists {
		report.ByTool[judgment.ToolName] = &ToolStats{ToolName: judgment.ToolName}
	}
	report.ByTool[judgment.ToolName].Total++
	if judgment.Fulfilled {
		report.ByTool[judgment.ToolName].Fulfilled++
	}
}

func calculateAccuracy(report *JudgmentReport) {
	// Calculate turn accuracies
	for _, stats := range report.ByTurn {
		if stats.Total > 0 {
			stats.Accuracy = float64(stats.Fulfilled) / float64(stats.Total)
		}
	}

	// Calculate tool accuracies
	for _, stats := range report.ByTool {
		if stats.Total > 0 {
			stats.Accuracy = float64(stats.Fulfilled) / float64(stats.Total)
		}
	}

	// Calculate overall accuracy
	totalFulfilled := 0
	for _, j := range report.Judgments {
		if j.Fulfilled {
			totalFulfilled++
		}
	}
	if len(report.Judgments) > 0 {
		report.OverallAccuracy = float64(totalFulfilled) / float64(len(report.Judgments))
	}

	// Generate summary
	report.Summary = fmt.Sprintf(
		"Judged %d tool calls. %d fulfilled (%.1f%% overall accuracy). Average score: %.2f/1.0",
		len(report.Judgments),
		totalFulfilled,
		report.OverallAccuracy*100,
		averageScore(report.Judgments),
	)
}

func averageScore(judgments []Judgment) float64 {
	if len(judgments) == 0 {
		return 0.0
	}
	sum := 0.0
	for _, j := range judgments {
		sum += j.Score
	}
	return sum / float64(len(judgments))
}

func printSummary(report *JudgmentReport) {
	fmt.Println("=== JUDGMENT SUMMARY ===")
	fmt.Println()

	// Overall
	fmt.Printf("Overall Accuracy: %.1f%% (%d/%d fulfilled)\n",
		report.OverallAccuracy*100,
		countFulfilled(report.Judgments),
		len(report.Judgments))
	fmt.Printf("Average Score: %.2f/1.0\n", averageScore(report.Judgments))
	fmt.Println()

	// By turn
	fmt.Println("Accuracy by Turn:")
	for turn := 1; turn <= len(report.ByTurn); turn++ {
		if stats, exists := report.ByTurn[turn]; exists {
			fmt.Printf("  Turn %d: %.1f%% (%d/%d) %s\n",
				turn, stats.Accuracy*100, stats.Fulfilled, stats.Total,
				accuracyBar(stats.Accuracy))
		}
	}
	fmt.Println()

	// By tool
	fmt.Println("Accuracy by Tool:")
	for tool, stats := range report.ByTool {
		fmt.Printf("  %-25s: %.1f%% (%d/%d) %s\n",
			tool, stats.Accuracy*100, stats.Fulfilled, stats.Total,
			accuracyBar(stats.Accuracy))
	}
	fmt.Println()

	fmt.Println(report.Summary)
}

func countFulfilled(judgments []Judgment) int {
	count := 0
	for _, j := range judgments {
		if j.Fulfilled {
			count++
		}
	}
	return count
}

func accuracyBar(accuracy float64) string {
	bars := int(accuracy * 10)
	filled := strings.Repeat("█", bars)
	empty := strings.Repeat("░", 10-bars)
	return "[" + filled + empty + "]"
}

func confidenceLabel(conf string) string {
	switch conf {
	case "high":
		return "✓✓✓"
	case "medium":
		return "✓✓~"
	case "low":
		return "✓~~"
	default:
		return "???"
	}
}

func saveReport(report *JudgmentReport, filename string) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filename, data, 0644)
}
