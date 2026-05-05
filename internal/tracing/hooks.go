package tracing

import (
	"context"
	"encoding/json"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/dancsalo/arxiv-deep-research/internal/agentic"
)

type TracingHooks struct {
	tracer          trace.Tracer
	currentTurnCtx  context.Context
	currentTurnSpan trace.Span
	toolSpanStack   []trace.Span
}

func NewTracingHooks(tp trace.TracerProvider) (*agentic.LoopHooks, *TracingHooks) {
	th := &TracingHooks{
		tracer: tp.Tracer("arxiv-deep-research/agentic"),
	}

	hooks := &agentic.LoopHooks{
		OnTurnStart:    th.onTurnStart,
		OnTurnEnd:      th.onTurnEnd,
		OnToolCall:     th.onToolCall,
		OnToolResult:   th.onToolResult,
		OnMemoryRecall: th.onMemoryRecall,
	}

	return hooks, th
}

func (th *TracingHooks) TurnContext() context.Context {
	if th.currentTurnCtx != nil {
		return th.currentTurnCtx
	}
	return context.Background()
}

func (th *TracingHooks) onTurnStart(ctx context.Context, state agentic.TurnState) error {
	th.currentTurnCtx, th.currentTurnSpan = th.tracer.Start(ctx, "turn",
		trace.WithAttributes(
			AttrTurnIndex.Int(state.TurnIndex),
			AttrTokensUsed.Int(state.TokensUsed),
			AttrTokensRemain.Int(state.TokensRemaining),
		),
	)
	th.toolSpanStack = th.toolSpanStack[:0]
	return nil
}

func (th *TracingHooks) onTurnEnd(_ context.Context, state agentic.TurnState) error {
	for _, s := range th.toolSpanStack {
		s.SetStatus(codes.Error, "tool span not closed properly")
		s.End()
	}
	th.toolSpanStack = th.toolSpanStack[:0]

	if th.currentTurnSpan != nil {
		th.currentTurnSpan.SetAttributes(
			AttrCostUSD.Float64(state.TotalCostUSD),
			AttrToolCalls.String(strings.Join(state.LastToolCalls, ",")),
		)
		th.currentTurnSpan.End()
		th.currentTurnSpan = nil
		th.currentTurnCtx = nil
	}
	return nil
}

func (th *TracingHooks) onToolCall(_ context.Context, toolName string, input json.RawMessage, _ agentic.TurnState) error {
	parentCtx := th.TurnContext()
	_, span := th.tracer.Start(parentCtx, "tool."+toolName,
		trace.WithAttributes(
			AttrToolName.String(toolName),
			attribute.Int("gen_ai.tool.input_length", len(input)),
		),
	)
	th.toolSpanStack = append(th.toolSpanStack, span)
	return nil
}

func (th *TracingHooks) onToolResult(_ context.Context, toolName string, result string, _ agentic.TurnState) error {
	if len(th.toolSpanStack) == 0 {
		return nil
	}

	span := th.toolSpanStack[len(th.toolSpanStack)-1]
	th.toolSpanStack = th.toolSpanStack[:len(th.toolSpanStack)-1]

	span.SetAttributes(
		AttrToolResultLen.Int(len(result)),
	)
	span.End()
	return nil
}

func (th *TracingHooks) onMemoryRecall(_ context.Context, memories []agentic.RecalledMemory, state agentic.TurnState) ([]agentic.RecalledMemory, error) {
	parentCtx := th.TurnContext()
	_, span := th.tracer.Start(parentCtx, "memory.recall",
		trace.WithAttributes(
			AttrMemoryCount.Int(len(memories)),
			AttrTurnIndex.Int(state.TurnIndex),
		),
	)
	span.End()
	return memories, nil
}
