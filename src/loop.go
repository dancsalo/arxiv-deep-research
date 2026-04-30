package contextmanager

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
)

type LoopConfig struct {
	MaxTurns   int
	MaxCostUSD float64
	Model      anthropic.Model
}

type FinishResult struct {
	Summary string `json:"summary"`
}

type MessageClient interface {
	CreateMessage(ctx context.Context, params anthropic.MessageNewParams) (*anthropic.Message, error)
}

type ToolExecutor func(toolName string, args map[string]any) (string, error)

func ResearchLoop(
	ctx context.Context,
	client MessageClient,
	query string,
	cfg LoopConfig,
	tools []anthropic.ToolUnionParam,
	system []anthropic.TextBlockParam,
	executor ToolExecutor,
	manager *ContextManager,
) (string, error) {
	var totalCostUSD float64
	finished := false

	for turn := 0; turn < cfg.MaxTurns && !finished; turn++ {
		if err := manager.PreAPICheck(ctx); err != nil {
			return "", fmt.Errorf("turn %d pre-API check failed: %w", turn, err)
		}

		messages := manager.BuildMessages()

		resp, err := client.CreateMessage(ctx, anthropic.MessageNewParams{
			Model:     cfg.Model,
			MaxTokens: 8192,
			Tools:     tools,
			System:    system,
			Messages:  messages,
		})
		if err != nil {
			return "", fmt.Errorf("turn %d API error: %w", turn, err)
		}

		totalCostUSD += estimateCost(resp.Usage, cfg.Model)
		if totalCostUSD > cfg.MaxCostUSD {
			return "", fmt.Errorf("cost limit exceeded: $%.4f > $%.4f", totalCostUSD, cfg.MaxCostUSD)
		}

		manager.OutputTracker().Record(int(resp.Usage.OutputTokens))

		var toolResults []anthropic.ContentBlockParamUnion

		for _, block := range resp.Content {
			switch block.Type {
			case "text":
				// text output — could be final answer

			case "tool_use":
				toolUse := block.AsToolUse()
				if toolUse.Name == "finish_research" {
					finished = true
					var result FinishResult
					if err := json.Unmarshal(toolUse.Input, &result); err == nil {
						toolResults = append(toolResults,
							anthropic.NewToolResultBlock(toolUse.ID, result.Summary, false),
						)
					} else {
						toolResults = append(toolResults,
							anthropic.NewToolResultBlock(toolUse.ID, "finished", false),
						)
					}
					continue
				}

				args := jsonToMap(toolUse.Input)
				decision, err := manager.PreToolGuardrail(ctx, toolUse.Name, args)
				if err != nil {
					toolResults = append(toolResults,
						anthropic.NewToolResultBlock(toolUse.ID, fmt.Sprintf("error: %v", err), true),
					)
					continue
				}
				if !decision.Proceed {
					toolResults = append(toolResults,
						anthropic.NewToolResultBlock(toolUse.ID,
							fmt.Sprintf("REJECTED: %s", decision.Reason), true),
					)
					continue
				}

				execArgs := args
				if decision.ModifiedArgs != nil {
					execArgs = decision.ModifiedArgs
				}

				resultJSON, err := executor(toolUse.Name, execArgs)
				if err != nil {
					toolResults = append(toolResults,
						anthropic.NewToolResultBlock(toolUse.ID, fmt.Sprintf("error: %v", err), true),
					)
					continue
				}

				if postErr := manager.PostToolCheck(ctx, toolUse.Name, resultJSON); postErr != nil {
					// log but don't fail
					_ = postErr
				}

				toolResults = append(toolResults,
					anthropic.NewToolResultBlock(toolUse.ID, resultJSON, false),
				)
			}
		}

		assistantMsg := responseToAssistantParam(resp)
		var toolResultMsg *anthropic.MessageParam
		if len(toolResults) > 0 {
			msg := anthropic.NewUserMessage(toolResults...)
			toolResultMsg = &msg
		}
		manager.AddTurn(assistantMsg, toolResultMsg, PriorityResearch)

		if resp.StopReason == "end_turn" || finished {
			break
		}
	}

	return manager.ExtractFinalAnswer(), nil
}

func BuildFinishTool() anthropic.ToolUnionParam {
	t := anthropic.ToolUnionParamOfTool(
		anthropic.ToolInputSchemaParam{
			Type: "object",
			Properties: map[string]any{
				"summary": map[string]any{
					"type":        "string",
					"description": "The final research summary",
				},
			},
			Required: []string{"summary"},
		},
		"finish_research",
	)
	t.OfTool.Description = anthropic.String(
		"Call this when research is complete. Provide the final synthesized summary.",
	)
	return t
}

func estimateCost(usage anthropic.Usage, model anthropic.Model) float64 {
	inputTokens := float64(usage.InputTokens)
	outputTokens := float64(usage.OutputTokens)

	var inputPer1M, outputPer1M float64
	switch model {
	case anthropic.ModelClaudeSonnet4_6:
		inputPer1M = 3.0
		outputPer1M = 15.0
	case anthropic.ModelClaudeHaiku4_5:
		inputPer1M = 0.80
		outputPer1M = 4.0
	default:
		inputPer1M = 3.0
		outputPer1M = 15.0
	}

	return (inputTokens/1_000_000)*inputPer1M + (outputTokens/1_000_000)*outputPer1M
}

func responseToAssistantParam(resp *anthropic.Message) anthropic.MessageParam {
	var blocks []anthropic.ContentBlockParamUnion
	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			blocks = append(blocks, anthropic.NewTextBlock(block.Text))
		case "tool_use":
			blocks = append(blocks, anthropic.ContentBlockParamUnion{
				OfToolUse: &anthropic.ToolUseBlockParam{
					ID:    block.ID,
					Name:  block.Name,
					Input: block.Input,
				},
			})
		}
	}
	if len(blocks) == 0 {
		blocks = append(blocks, anthropic.NewTextBlock(""))
	}
	return anthropic.NewAssistantMessage(blocks...)
}

func jsonToMap(data json.RawMessage) map[string]any {
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return make(map[string]any)
	}
	return m
}
