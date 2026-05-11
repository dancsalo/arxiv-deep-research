package tracing

import (
	"context"
	"encoding/json"
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
	defer r.mu.Unlock()
	if r.currentTurn == nil {
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

	r.trace.TotalCostUSD = state.TotalCostUSD
	r.trace.Turns = append(r.trace.Turns, *r.currentTurn)
	r.currentTurn = nil
	return nil
}

func (r *Recorder) onToolCall(_ context.Context, toolName string, input json.RawMessage, _ agentic.TurnState) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.currentTurn != nil {
		idx := len(r.currentTurn.ToolCalls)
		r.currentTurn.ToolCalls = append(r.currentTurn.ToolCalls, ToolCall{
			Name:        toolName,
			Input:       input,
			InputLength: len(input),
		})
		r.toolStartStack = append(r.toolStartStack, time.Now())
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
		r.currentTurn.ToolCalls[idx].DurationMs = time.Since(startTime).Milliseconds()
		r.currentTurn.ToolCalls[idx].ResultLength = len(result)

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

	r.currentTurn.GuardrailDecisions = append(r.currentTurn.GuardrailDecisions, GuardrailDecision{
		ToolName:        info.ToolName,
		Proceed:         info.Proceed,
		Reason:          info.Reason,
		EstimatedTokens: info.Estimated,
		TokensRemaining: info.Remaining,
		SafetyMargin:    info.SafetyMargin,
		ArgsModified:    info.ArgsModified,
		Compacted:       info.Compacted,
		CompactedTurns:  info.CompactedTurns,
	})
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
