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
	Index           int        `json:"index"`
	StartedAt       time.Time  `json:"started_at"`
	EndedAt         time.Time  `json:"ended_at"`
	DurationMs      int64      `json:"duration_ms"`
	TokensUsed      int        `json:"tokens_used"`
	TokensRemaining int        `json:"tokens_remaining"`
	CostUSD         float64    `json:"cost_usd"`
	LLMCall         *LLMCall   `json:"llm_call,omitempty"`
	ToolCalls       []ToolCall `json:"tool_calls"`
}

type LLMCall struct {
	Model        string `json:"model"`
	InputTokens  int    `json:"input_tokens"`
	OutputTokens int    `json:"output_tokens"`
	DurationMs   int64  `json:"duration_ms"`
	StopReason   string `json:"stop_reason"`
	Error        string `json:"error,omitempty"`
}

type ToolCall struct {
	Name         string          `json:"name"`
	Input        json.RawMessage `json:"input"`
	Output       json.RawMessage `json:"output,omitempty"`
	InputLength  int             `json:"input_length"`
	ResultLength int             `json:"result_length"`
	DurationMs   int64           `json:"duration_ms"`
}

type Config struct {
	Dir       string
	SessionID string
	Query     string
	Model     string
	Logger    *slog.Logger
}

func (cfg Config) Enabled() bool {
	return cfg.Dir != ""
}
