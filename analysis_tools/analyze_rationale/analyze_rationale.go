package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type Trace struct {
	Turns []Turn `json:"turns"`
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
	Name string `json:"name,omitempty"`
}

type ToolCall struct {
	Name string `json:"name"`
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <trace-file.json>\n", os.Args[0])
		os.Exit(1)
	}

	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		os.Exit(1)
	}

	var trace Trace
	if err := json.Unmarshal(data, &trace); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing JSON: %v\n", err)
		os.Exit(1)
	}

	totalToolCalls := 0
	toolsWithRationale := 0

	for _, turn := range trace.Turns {
		// Extract all text content from LLM output
		var textBeforeTools strings.Builder
		for _, content := range turn.LLMCall.Output.Content {
			if content.Type == "text" {
				textBeforeTools.WriteString(content.Text)
				textBeforeTools.WriteString("\n")
			}
		}
		precedingText := textBeforeTools.String()

		// Analyze each tool call
		for _, toolCall := range turn.ToolCalls {
			totalToolCalls++
			toolName := toolCall.Name

			// STRICTER REQUIREMENT: Tool must be explicitly mentioned with a reason
			hasRationale := checkToolRationale(precedingText, toolName)

			if hasRationale {
				toolsWithRationale++
				fmt.Printf("Turn %d: Tool '%s' HAS rationale ✓\n", turn.Index+1, toolName)
			} else {
				fmt.Printf("Turn %d: Tool '%s' called WITHOUT clear rationale ✗\n", turn.Index+1, toolName)
			}
		}

		// Debug output for each turn
		if len(turn.ToolCalls) > 0 {
			toolNames := make([]string, len(turn.ToolCalls))
			for i, tc := range turn.ToolCalls {
				toolNames[i] = tc.Name
			}
			fmt.Printf("\n--- Turn %d ---\n", turn.Index+1)
			fmt.Printf("Tools called: %v\n", toolNames)
			fmt.Printf("Preceding text (first 300 chars):\n%s...\n\n",
				truncate(precedingText, 300))
		}
	}

	fmt.Printf("\n=== RATIONALE ANALYSIS ===\n")
	fmt.Printf("Total tool calls: %d\n", totalToolCalls)
	fmt.Printf("Tools with rationale: %d\n", toolsWithRationale)

	if totalToolCalls > 0 {
		accuracy := float64(toolsWithRationale) / float64(totalToolCalls) * 100
		fmt.Printf("Rationale accuracy: %.1f%%\n", accuracy)
	} else {
		fmt.Println("No tool calls found in trace")
	}
}

func checkToolRationale(text, toolName string) bool {
	lowerText := strings.ToLower(text)
	lowerTool := strings.ToLower(toolName)

	// Extract the "This Turn's Tools" section if it exists
	toolsSectionStart := strings.Index(lowerText, "this turn's tools")
	if toolsSectionStart == -1 {
		// Fallback to older format
		toolsSectionStart = strings.Index(lowerText, "tools & rationale")
	}

	// If no tools section found, fail
	if toolsSectionStart == -1 {
		return false
	}

	// Extract the tools section (everything after the header until next ## heading or end)
	toolsSection := lowerText[toolsSectionStart:]
	nextHeadingIdx := strings.Index(toolsSection[50:], "##")
	if nextHeadingIdx != -1 {
		toolsSection = toolsSection[:50+nextHeadingIdx]
	}

	// Look for tool mentions - could be just `tool_name` or `tool_name(...)`
	tickTool := "`" + lowerTool

	// Find the line containing the tool
	lines := strings.Split(toolsSection, "\n")
	for _, line := range lines {
		if strings.Contains(line, tickTool) {
			// Check if the line contains a rationale (has a hyphen/dash and explanation)
			if strings.Contains(line, " - ") {
				// Make sure there's actual text after the dash
				parts := strings.SplitN(line, " - ", 2)
				if len(parts) > 1 && len(strings.TrimSpace(parts[1])) > 10 {
					return true
				}
			}
		}
	}

	return false
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}
