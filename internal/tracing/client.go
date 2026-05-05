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
	elapsed := time.Since(start)

	call := &LLMCall{
		Model:      string(params.Model),
		DurationMs: elapsed.Milliseconds(),
	}
	if err != nil {
		call.Error = err.Error()
	} else {
		call.InputTokens = int(resp.Usage.InputTokens)
		call.OutputTokens = int(resp.Usage.OutputTokens)
		call.StopReason = string(resp.StopReason)
	}
	tc.Recorder.mu.Lock()
	tc.Recorder.pendingLLMCall = call
	tc.Recorder.mu.Unlock()

	return resp, err
}
