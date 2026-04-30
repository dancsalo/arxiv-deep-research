package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/bedrock"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/log"
	"golang.org/x/sync/errgroup"
)

// ---------------------------------------------------------------------------
// Styles
// ---------------------------------------------------------------------------

var (
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#7D56F4")).
			BorderStyle(lipgloss.DoubleBorder()).
			BorderForeground(lipgloss.Color("#7D56F4")).
			Padding(0, 1)

	toolStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FF6B6B"))

	assistantStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#04B575"))

	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#666666"))

	parallelStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFD700"))
)

// ---------------------------------------------------------------------------
// Tool handlers
// ---------------------------------------------------------------------------

type ToolHandler func(inputJSON []byte) (string, error)

func handleGetWeather(input []byte) (string, error) {
	var p struct {
		City string `json:"city"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	result := map[string]any{
		"city":       p.City,
		"temp_f":     68,
		"conditions": "sunny",
	}
	b, _ := json.Marshal(result)
	return string(b), nil
}

func handleCalculate(input []byte) (string, error) {
	var p struct {
		Operation string  `json:"operation"`
		A         float64 `json:"a"`
		B         float64 `json:"b"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}

	var value float64
	switch p.Operation {
	case "add":
		value = p.A + p.B
	case "subtract":
		value = p.A - p.B
	case "multiply":
		value = p.A * p.B
	case "divide":
		if p.B == 0 {
			return `{"error": "division by zero"}`, nil
		}
		value = p.A / p.B
	case "power":
		value = math.Pow(p.A, p.B)
	case "modulo":
		if p.B == 0 {
			return `{"error": "modulo by zero"}`, nil
		}
		value = math.Mod(p.A, p.B)
	default:
		return fmt.Sprintf(`{"error": "unknown operation: %s"}`, p.Operation), nil
	}

	result := map[string]any{
		"operation": p.Operation,
		"a":         p.A,
		"b":         p.B,
		"result":    value,
	}
	b, _ := json.Marshal(result)
	return string(b), nil
}

func handleCalendar(input []byte) (string, error) {
	var p struct {
		Action string `json:"action"`
		Date   string `json:"date"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}

	now := time.Now()

	switch p.Action {
	case "today":
		result := map[string]any{
			"date":        now.Format("2006-01-02"),
			"day_of_week": now.Weekday().String(),
			"week_number": now.YearDay() / 7,
			"year":        now.Year(),
		}
		b, _ := json.Marshal(result)
		return string(b), nil

	case "days_until":
		target, err := time.Parse("2006-01-02", p.Date)
		if err != nil {
			return fmt.Sprintf(`{"error": "invalid date format, use YYYY-MM-DD: %s"}`, err), nil
		}
		days := int(target.Sub(now).Hours() / 24)
		result := map[string]any{
			"target_date": p.Date,
			"days_until":  days,
			"is_past":     days < 0,
			"day_of_week": target.Weekday().String(),
		}
		b, _ := json.Marshal(result)
		return string(b), nil

	case "day_of_week":
		target, err := time.Parse("2006-01-02", p.Date)
		if err != nil {
			return fmt.Sprintf(`{"error": "invalid date format, use YYYY-MM-DD: %s"}`, err), nil
		}
		result := map[string]any{
			"date":        p.Date,
			"day_of_week": target.Weekday().String(),
		}
		b, _ := json.Marshal(result)
		return string(b), nil

	default:
		return fmt.Sprintf(`{"error": "unknown action: %s"}`, p.Action), nil
	}
}

func handleCPUInfo(input []byte) (string, error) {
	result := map[string]any{
		"os":         runtime.GOOS,
		"arch":       runtime.GOARCH,
		"num_cpus":   runtime.NumCPU(),
		"go_version": runtime.Version(),
		"compiler":   runtime.Compiler,
		"goroutines": runtime.NumGoroutine(),
	}

	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	result["memory_alloc_mb"] = float64(mem.Alloc) / 1024 / 1024
	result["memory_sys_mb"] = float64(mem.Sys) / 1024 / 1024

	b, _ := json.Marshal(result)
	return string(b), nil
}

var handlers = map[string]ToolHandler{
	"get_weather": handleGetWeather,
	"calculate":   handleCalculate,
	"calendar":    handleCalendar,
	"cpu_info":    handleCPUInfo,
}

// ---------------------------------------------------------------------------
// Tool definitions
// ---------------------------------------------------------------------------

func buildTools() []anthropic.ToolUnionParam {
	weatherTool := anthropic.ToolUnionParamOfTool(
		anthropic.ToolInputSchemaParam{
			Type: "object",
			Properties: map[string]any{
				"city": map[string]any{
					"type":        "string",
					"description": "City name",
				},
			},
			Required: []string{"city"},
		},
		"get_weather",
	)
	weatherTool.OfTool.Description = anthropic.String("Get current weather for a city")

	calcTool := anthropic.ToolUnionParamOfTool(
		anthropic.ToolInputSchemaParam{
			Type: "object",
			Properties: map[string]any{
				"operation": map[string]any{
					"type":        "string",
					"enum":        []string{"add", "subtract", "multiply", "divide", "power", "modulo"},
					"description": "The arithmetic operation to perform",
				},
				"a": map[string]any{
					"type":        "number",
					"description": "First operand",
				},
				"b": map[string]any{
					"type":        "number",
					"description": "Second operand",
				},
			},
			Required: []string{"operation", "a", "b"},
		},
		"calculate",
	)
	calcTool.OfTool.Description = anthropic.String("Perform arithmetic calculations: add, subtract, multiply, divide, power, modulo")

	calendarTool := anthropic.ToolUnionParamOfTool(
		anthropic.ToolInputSchemaParam{
			Type: "object",
			Properties: map[string]any{
				"action": map[string]any{
					"type":        "string",
					"enum":        []string{"today", "days_until", "day_of_week"},
					"description": "Action: 'today' for current date info, 'days_until' to count days until a date, 'day_of_week' to get the day name for a date",
				},
				"date": map[string]any{
					"type":        "string",
					"description": "Target date in YYYY-MM-DD format (required for days_until and day_of_week)",
				},
			},
			Required: []string{"action"},
		},
		"calendar",
	)
	calendarTool.OfTool.Description = anthropic.String("Get today's date, count days until a target date, or find the day of the week for any date")

	cpuTool := anthropic.ToolUnionParamOfTool(
		anthropic.ToolInputSchemaParam{
			Type:       "object",
			Properties: map[string]any{},
		},
		"cpu_info",
	)
	cpuTool.OfTool.Description = anthropic.String("Get system info: OS, architecture, CPU count, memory usage, and Go runtime details")

	return []anthropic.ToolUnionParam{weatherTool, calcTool, calendarTool, cpuTool}
}

// ---------------------------------------------------------------------------
// Parallel tool execution
// ---------------------------------------------------------------------------

type pendingToolCall struct {
	block anthropic.ToolUseBlock
	index int
}

type toolResult struct {
	block  anthropic.ContentBlockParamUnion
	result string
	name   string
}

func executeToolsParallel(logger *log.Logger, calls []pendingToolCall) []anthropic.ContentBlockParamUnion {
	results := make([]toolResult, len(calls))
	var mu sync.Mutex

	g, _ := errgroup.WithContext(context.Background())

	for _, tc := range calls {
		tc := tc
		g.Go(func() error {
			handler, ok := handlers[tc.block.Name]
			if !ok {
				return fmt.Errorf("unknown tool: %s", tc.block.Name)
			}

			start := time.Now()
			resultJSON, err := handler(tc.block.Input)
			if err != nil {
				return fmt.Errorf("tool %s failed: %w", tc.block.Name, err)
			}
			elapsed := time.Since(start)

			mu.Lock()
			logger.Info("Tool executed",
				"tool", tc.block.Name,
				"index", tc.index,
				"latency", elapsed.Round(time.Microsecond),
			)
			mu.Unlock()

			results[tc.index] = toolResult{
				block:  anthropic.NewToolResultBlock(tc.block.ID, resultJSON, false),
				result: resultJSON,
				name:   tc.block.Name,
			}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		logger.Fatal("Parallel tool execution failed", "err", err)
	}

	blocks := make([]anthropic.ContentBlockParamUnion, len(results))
	for i, r := range results {
		blocks[i] = r.block
		fmt.Println(dimStyle.Render(fmt.Sprintf("  [%d] %s result: %s", i, r.name, r.result)))
	}
	return blocks
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

func main() {
	useBedrock := flag.Bool("bedrock", true, "use AWS Bedrock (default true)")
	model := flag.String("model", "", "model ID override")
	flag.Parse()

	logger := log.NewWithOptions(os.Stderr, log.Options{
		ReportTimestamp: true,
		TimeFormat:      time.Kitchen,
	})

	ctx := context.Background()

	var opts []option.RequestOption
	if *useBedrock {
		opts = append(opts, bedrock.WithLoadDefaultConfig(ctx))
		logger.Info("Using AWS Bedrock")
	} else {
		logger.Info("Using direct Anthropic API")
	}
	client := anthropic.NewClient(opts...)

	var modelID anthropic.Model
	if *model != "" {
		modelID = anthropic.Model(*model)
	} else if *useBedrock {
		modelID = "us.anthropic.claude-3-5-haiku-20241022-v1:0"
	} else {
		modelID = anthropic.ModelClaudeHaiku4_5
	}
	logger.Info("Model selected", "model", string(modelID))

	fmt.Println(headerStyle.Render("Parallel Tool Use Demo"))
	fmt.Println()

	tools := buildTools()
	toolNames := make([]string, 0, len(handlers))
	for name := range handlers {
		toolNames = append(toolNames, name)
	}
	logger.Info("Registered tools", "count", len(tools), "names", toolNames)

	userQuery := "What's the weather in Tokyo? Also, what's 1234 * 5678? What day of the week is Christmas 2026? And tell me about the CPU running this."
	logger.Info("Sending user message", "query", userQuery)

	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(
			anthropic.NewTextBlock(userQuery),
		),
	}

	turn := 0
	for {
		turn++
		logger.Info("API call", "turn", turn)
		start := time.Now()

		resp, err := client.Messages.New(ctx, anthropic.MessageNewParams{
			Model:     modelID,
			MaxTokens: 4096,
			Tools:     tools,
			System: []anthropic.TextBlockParam{{Text: "IMPORTANT: When the user asks multiple independent questions, you MUST call ALL relevant tools in a single response. Do NOT call them one at a time across multiple turns. Return all tool_use blocks together in one response.", Type: "text"}},
			Messages:  messages,
		})
		if err != nil {
			logger.Fatal("API error", "err", err)
		}

		elapsed := time.Since(start)
		logger.Info("Response received",
			"turn", turn,
			"latency", elapsed.Round(time.Millisecond),
			"stop_reason", resp.StopReason,
			"input_tokens", resp.Usage.InputTokens,
			"output_tokens", resp.Usage.OutputTokens,
		)

		messages = append(messages, resp.ToParam())

		// Pass 1: collect text blocks and tool calls separately
		var pendingCalls []pendingToolCall

		logger.Info("Response content blocks", "count", len(resp.Content))
		for i, block := range resp.Content {
			logger.Info("Block", "index", i, "type", block.Type)
			switch b := block.AsAny().(type) {
			case anthropic.TextBlock:
				fmt.Println(assistantStyle.Render("assistant: " + b.Text))

			case anthropic.ToolUseBlock:
				inputJSON, _ := json.MarshalIndent(json.RawMessage(b.Input), "  ", "  ")
				fmt.Println(toolStyle.Render(fmt.Sprintf("tool call: %s", b.Name)))
				fmt.Println(dimStyle.Render(fmt.Sprintf("  id: %s", b.ID)))
				fmt.Println(dimStyle.Render(fmt.Sprintf("  input: %s", inputJSON)))

				pendingCalls = append(pendingCalls, pendingToolCall{
					block: b,
					index: len(pendingCalls),
				})
			}
		}

		if len(pendingCalls) == 0 {
			logger.Info("Loop complete", "total_turns", turn)
			break
		}

		// Pass 2: execute all tool calls in parallel
		fmt.Println()
		fmt.Println(parallelStyle.Render(fmt.Sprintf("executing %d tools in parallel...", len(pendingCalls))))
		execStart := time.Now()

		toolResults := executeToolsParallel(logger, pendingCalls)

		execElapsed := time.Since(execStart)
		logger.Info("All tools complete",
			"count", len(pendingCalls),
			"total_latency", execElapsed.Round(time.Microsecond),
		)
		fmt.Println()

		messages = append(messages, anthropic.NewUserMessage(toolResults...))
	}
}
