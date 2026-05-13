package tracing

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/dancsalo/arxiv-deep-research/internal/agentic"
)

func NewTracingHooks(cfg Config) (*agentic.LoopHooks, *Recorder) {
	rec := NewRecorder(cfg)
	if !cfg.Enabled() {
		return nil, rec
	}
	return &agentic.LoopHooks{
		OnTurnStart:  rec.onTurnStart,
		OnTurnEnd:    rec.onTurnEnd,
		OnToolCall:   rec.onToolCall,
		OnToolResult: rec.onToolResult,
		OnGuardrail:  rec.onGuardrail,
		OnLLMCall:    rec.onLLMCall,
	}, rec
}

func (r *Recorder) onTurnStart(_ context.Context, state agentic.TurnState) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.currentTurn = &Turn{
		Index:     state.TurnIndex,
		StartedAt: time.Now(),
	}
	r.toolStartStack = r.toolStartStack[:0]
	r.toolIndexStack = r.toolIndexStack[:0]
	return nil
}

func (r *Recorder) onTurnEnd(_ context.Context, state agentic.TurnState) error {
	r.mu.Lock()
	if r.currentTurn == nil {
		r.mu.Unlock()
		return nil
	}
	now := time.Now()
	r.currentTurn.EndedAt = now
	r.currentTurn.DurationMs = now.Sub(r.currentTurn.StartedAt).Milliseconds()
	r.currentTurn.TokensUsed = state.TokensUsed
	r.currentTurn.TokensRemaining = state.TokensRemaining
	r.currentTurn.CostUSD = state.TotalCostUSD - r.prevCostUSD
	r.prevCostUSD = state.TotalCostUSD

	if r.pendingLLMCall != nil {
		r.currentTurn.LLMCall = r.pendingLLMCall
		r.trace.TotalInputTokens += r.pendingLLMCall.InputTokens
		r.trace.TotalOutputTokens += r.pendingLLMCall.OutputTokens
		r.pendingLLMCall = nil
	}

	// Generate display metadata
	r.currentTurn.Display = generateTurnDisplay(r.currentTurn)

	r.trace.TotalCostUSD = state.TotalCostUSD
	r.trace.Turns = append(r.trace.Turns, *r.currentTurn)
	r.currentTurn = nil
	r.mu.Unlock()

	// Flush trace after each turn to enable real-time viewing
	if err := r.flushIncremental(); err != nil && r.cfg.Logger != nil {
		r.cfg.Logger.Warn("failed to flush trace incrementally", "err", err)
	}
	return nil
}

func generateTurnDisplay(turn *Turn) *TurnDisplay {
	display := &TurnDisplay{
		Status: "success",
	}

	// Check for errors
	hasError := false
	if turn.LLMCall != nil && turn.LLMCall.Error != "" {
		hasError = true
	}
	for _, tc := range turn.ToolCalls {
		if tc.Error != nil {
			hasError = true
			break
		}
	}
	if hasError {
		display.Status = "error"
	}

	// Generate label based on tool calls
	toolCount := len(turn.ToolCalls)
	if toolCount == 0 {
		display.Label = "Reasoning"
		display.Summary = "LLM reasoning without tool calls"
	} else if toolCount == 1 {
		display.Label = turn.ToolCalls[0].Name
		display.PrimaryTool = turn.ToolCalls[0].Name
		display.Summary = "1 tool call"
	} else {
		// Find most common tool
		toolCounts := make(map[string]int)
		for _, tc := range turn.ToolCalls {
			toolCounts[tc.Name]++
		}
		maxCount := 0
		var primaryTool string
		for name, count := range toolCounts {
			if count > maxCount {
				maxCount = count
				primaryTool = name
			}
		}
		display.PrimaryTool = primaryTool
		display.Label = primaryTool
		display.Summary = fmt.Sprintf("%d tool calls", toolCount)
	}

	return display
}

func (r *Recorder) onToolCall(_ context.Context, toolName string, input json.RawMessage, _ agentic.TurnState) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.currentTurn != nil {
		idx := len(r.currentTurn.ToolCalls)
		startTime := time.Now()

		toolCall := ToolCall{
			Name:        toolName,
			Input:       input,
			InputLength: len(input),
			StartedAt:   &startTime,
		}

		// Set parent tool index for nested calls
		if len(r.toolIndexStack) > 0 {
			parentIdx := r.toolIndexStack[len(r.toolIndexStack)-1]
			toolCall.ParentToolIndex = &parentIdx
			toolCall.ExecutionMode = "sequential"
		} else {
			toolCall.ExecutionMode = "parallel"
		}

		r.currentTurn.ToolCalls = append(r.currentTurn.ToolCalls, toolCall)
		r.toolStartStack = append(r.toolStartStack, startTime)
		r.toolIndexStack = append(r.toolIndexStack, idx)
	}
	return nil
}

func (r *Recorder) onToolResult(_ context.Context, _ string, result string, _ agentic.TurnState) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.toolStartStack) == 0 || r.currentTurn == nil {
		return nil
	}
	n := len(r.toolStartStack) - 1
	startTime := r.toolStartStack[n]
	idx := r.toolIndexStack[n]
	r.toolStartStack = r.toolStartStack[:n]
	r.toolIndexStack = r.toolIndexStack[:n]

	if idx < len(r.currentTurn.ToolCalls) {
		endTime := time.Now()
		r.currentTurn.ToolCalls[idx].EndedAt = &endTime
		r.currentTurn.ToolCalls[idx].DurationMs = endTime.Sub(startTime).Milliseconds()
		r.currentTurn.ToolCalls[idx].ResultLength = len(result)

		// Check if result is an error
		if strings.HasPrefix(result, "Error:") || strings.HasPrefix(result, "error:") {
			r.currentTurn.ToolCalls[idx].Error = &ToolError{
				Type:            "ToolExecutionError",
				Message:         result,
				Retryable:       false,
				AttemptedRetries: 0,
				SuggestedAction: "",
			}
		}

		// Try to parse result as JSON; if it fails, wrap it as a JSON string
		var outputJSON json.RawMessage
		if json.Valid([]byte(result)) {
			outputJSON = json.RawMessage(result)
		} else {
			// Wrap the result as a JSON string
			wrapped, err := json.Marshal(result)
			if err != nil {
				// Fallback: use the raw string as-is
				outputJSON = json.RawMessage(result)
			} else {
				outputJSON = wrapped
			}
		}
		r.currentTurn.ToolCalls[idx].Output = outputJSON
	}
	return nil
}

func (r *Recorder) onGuardrail(_ context.Context, info agentic.GuardrailInfo, _ agentic.TurnState) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.currentTurn == nil {
		return nil
	}

	decision := GuardrailDecision{
		ToolName:        info.ToolName,
		Proceed:         info.Proceed,
		Reason:          info.Reason,
		EstimatedTokens: info.Estimated,
		TokensRemaining: info.Remaining,
		SafetyMargin:    info.SafetyMargin,
		ArgsModified:    info.ArgsModified,
		Compacted:       info.Compacted,
		CompactedTurns:  info.CompactedTurns,
	}

	// Add removed content details if compaction occurred
	if info.Compacted && (info.ToolResultsRemoved > 0 || info.MessagesRemoved > 0) {
		decision.RemovedContent = &RemovedContent{
			ToolResultsCount: info.ToolResultsRemoved,
			MessageCount:     info.MessagesRemoved,
			SummaryTokens:    info.SummaryTokens,
		}
	}

	r.currentTurn.GuardrailDecisions = append(r.currentTurn.GuardrailDecisions, decision)
	return nil
}

func (r *Recorder) onLLMCall(_ context.Context, input, output json.RawMessage, _ agentic.TurnState) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.pendingLLMCall != nil {
		r.pendingLLMCall.Input = input
		r.pendingLLMCall.Output = output
	}
	return nil
}
