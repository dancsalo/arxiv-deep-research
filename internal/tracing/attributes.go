package tracing

import "go.opentelemetry.io/otel/attribute"

const (
	AttrTurnIndex      = attribute.Key("gen_ai.turn.index")
	AttrModel          = attribute.Key("gen_ai.request.model")
	AttrInputTokens    = attribute.Key("gen_ai.usage.input_tokens")
	AttrOutputTokens   = attribute.Key("gen_ai.usage.output_tokens")
	AttrCostUSD        = attribute.Key("gen_ai.usage.cost")
	AttrToolName       = attribute.Key("gen_ai.tool.name")
	AttrToolResultLen  = attribute.Key("gen_ai.tool.result_length")
	AttrMemoryCount    = attribute.Key("gen_ai.memory.recall_count")
	AttrSessionID      = attribute.Key("session.id")
	AttrTokensUsed     = attribute.Key("gen_ai.context.tokens_used")
	AttrTokensRemain   = attribute.Key("gen_ai.context.tokens_remaining")
	AttrToolCalls      = attribute.Key("gen_ai.turn.tool_calls")
	AttrQuery          = attribute.Key("gen_ai.query")
)
