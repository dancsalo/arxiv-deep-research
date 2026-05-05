package agentic

import (
	"context"
	"encoding/json"

	"github.com/anthropics/anthropic-sdk-go"
)

type MessageClient interface {
	CreateMessage(ctx context.Context, params anthropic.MessageNewParams) (*anthropic.Message, error)
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
		"finish_loop",
	)
	t.OfTool.Description = anthropic.String(
		"Call this when the task is complete. Provide the final synthesized summary.",
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
