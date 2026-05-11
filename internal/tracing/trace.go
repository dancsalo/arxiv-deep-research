package tracing

import (
	"encoding/json"
	"log/slog"
	"time"
)

type Trace struct {
	SessionID         string  `json:"session_id"`
	Query             string  `json:"query"`
	Model             string  `json:"model"`
	PromptVariant     string  `json:"prompt_variant"`     // NEW: "A", "B", or "C"
	PromptHash        string  `json:"prompt_hash"`        // NEW: First 8 chars of SHA256
	StartedAt         time.Time `json:"started_at"`
	EndedAt           time.Time `json:"ended_at"`
	DurationMs        int64   `json:"duration_ms"`
	Status            string  `json:"status"`
	Error             string  `json:"error,omitempty"`
	TotalInputTokens  int     `json:"total_input_tokens"`
	TotalOutputTokens int     `json:"total_output_tokens"`
	TotalCostUSD      float64 `json:"total_cost_usd"`
	Turns             []Turn  `json:"turns"`
}

type Turn struct {
	Index             int              `json:"index"`
	StartedAt         time.Time        `json:"started_at"`
	EndedAt           time.Time        `json:"ended_at"`
	DurationMs        int64            `json:"duration_ms"`
	TokensUsed        int              `json:"tokens_used"`
	TokensRemaining   int              `json:"tokens_remaining"`
	CostUSD           float64          `json:"cost_usd"`
	LLMCall           *LLMCall         `json:"llm_call,omitempty"`
	ToolCalls         []ToolCall       `json:"tool_calls"`
	GuardrailDecisions []GuardrailDecision `json:"guardrail_decisions,omitempty"`
}

type LLMCall struct {
	Model        string          `json:"model"`
	InputTokens  int             `json:"input_tokens"`
	OutputTokens int             `json:"output_tokens"`
	DurationMs   int64           `json:"duration_ms"`
	StopReason   string          `json:"stop_reason"`
	Error        string          `json:"error,omitempty"`
	Input        json.RawMessage `json:"input,omitempty"`
	Output       json.RawMessage `json:"output,omitempty"`
}

type ToolCall struct {
	Name         string          `json:"name"`
	Input        json.RawMessage `json:"input"`
	Output       json.RawMessage `json:"output,omitempty"`
	InputLength  int             `json:"input_length"`
	ResultLength int             `json:"result_length"`
	DurationMs   int64           `json:"duration_ms"`
}

type GuardrailDecision struct {
	ToolName          string   `json:"tool_name"`
	Proceed           bool     `json:"proceed"`
	Reason            string   `json:"reason,omitempty"`
	EstimatedTokens   int      `json:"estimated_tokens"`
	TokensRemaining   int      `json:"tokens_remaining"`
	SafetyMargin      int      `json:"safety_margin"`
	ArgsModified      bool     `json:"args_modified,omitempty"`
	Compacted         bool     `json:"compacted,omitempty"`
	CompactedTurns    []int    `json:"compacted_turns,omitempty"`
}

type Config struct {
	Dir           string
	SessionID     string
	Query         string
	Model         string
	PromptVariant string   // NEW: "A", "B", or "C"
	PromptHash    string   // NEW: First 8 chars of SHA256
	Logger        *slog.Logger
}

func (cfg Config) Enabled() bool {
	return cfg.Dir != ""
}