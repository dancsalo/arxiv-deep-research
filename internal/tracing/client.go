package tracing

import (
	"context"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/dancsalo/arxiv-deep-research/internal/agentic"
)

type TracedClient struct {
	Inner    agentic.MessageClient
	Recorder *Recorder
}

func (tc *TracedClient) CreateMessage(ctx context.Context, params anthropic.MessageNewParams) (*anthropic.Message, error) {
	start := time.Now()
	resp, err := tc.Inner.CreateMessage(ctx, params)
	end := time.Now()
	elapsed := end.Sub(start)

	call := &LLMCall{
		Model:      string(params.Model),
		DurationMs: elapsed.Milliseconds(),
		StartedAt:  &start,
		EndedAt:    &end,
	}
	if err != nil {
		call.Error = err.Error()
	} else {
		call.InputTokens = int(resp.Usage.InputTokens)
		call.OutputTokens = int(resp.Usage.OutputTokens)
		call.StopReason = string(resp.StopReason)
	}

	// Compute input summary from messages
	call.InputSummary = computeInputSummary(params.Messages, params.System)

	tc.Recorder.mu.Lock()
	tc.Recorder.pendingLLMCall = call
	tc.Recorder.mu.Unlock()

	return resp, err
}

func computeInputSummary(messages []anthropic.MessageParam, system []anthropic.TextBlockParam) *InputSummary {
	summary := &InputSummary{}

	// Count system tokens (rough estimate based on system block count)
	summary.SystemTokens = len(system) * 100 // rough estimate

	// Count messages by role
	for _, msg := range messages {
		summary.TotalMessages++

		if msg.Role == anthropic.MessageParamRoleUser {
			summary.UserMessages++
		} else if msg.Role == anthropic.MessageParamRoleAssistant {
			summary.AssistantMessages++

			// Count content blocks as proxy for tool results
			if msg.Content != nil {
				summary.ToolResults += len(msg.Content) - 1 // -1 for text content
				if summary.ToolResults < 0 {
					summary.ToolResults = 0
				}
			}
		}
	}

	// Oldest turn is 0 if we have any messages (simplified - could be improved)
	if summary.TotalMessages > 0 {
		summary.OldestMessageTurn = 0
	}

	return summary
}
