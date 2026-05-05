package tracing

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/dancsalo/arxiv-deep-research/internal/agentic"
)

type TracedClient struct {
	Inner agentic.MessageClient
	Hooks *TracingHooks
}

func (tc *TracedClient) CreateMessage(ctx context.Context, params anthropic.MessageNewParams) (*anthropic.Message, error) {
	parentCtx := tc.Hooks.TurnContext()

	parentCtx, span := tc.Hooks.tracer.Start(parentCtx, "llm.call",
		trace.WithAttributes(
			AttrModel.String(string(params.Model)),
		),
	)
	_ = parentCtx
	defer span.End()

	resp, err := tc.Inner.CreateMessage(ctx, params)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	span.SetAttributes(
		AttrInputTokens.Int(int(resp.Usage.InputTokens)),
		AttrOutputTokens.Int(int(resp.Usage.OutputTokens)),
		attribute.String("gen_ai.response.stop_reason", string(resp.StopReason)),
	)
	return resp, nil
}
