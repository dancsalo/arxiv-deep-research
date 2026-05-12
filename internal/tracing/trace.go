package tracing

import (
	"encoding/json"
	"log/slog"
	"time"
)

type Trace struct {
	SessionID          string                `json:"session_id"`
	Query              string                `json:"query"`
	Model              string                `json:"model"`
	PromptVariant      string                `json:"prompt_variant"`     // NEW: "A", "B", or "C"
	PromptHash         string                `json:"prompt_hash"`        // NEW: First 8 chars of SHA256
	StartedAt          time.Time             `json:"started_at"`
	EndedAt            time.Time             `json:"ended_at"`
	DurationMs         int64                 `json:"duration_ms"`
	Status             string                `json:"status"`
	Error              string                `json:"error,omitempty"`
	TotalInputTokens   int                   `json:"total_input_tokens"`
	TotalOutputTokens  int                   `json:"total_output_tokens"`
	TotalCostUSD       float64               `json:"total_cost_usd"`
	Turns              []Turn                `json:"turns"`
	GuardrailDecisions []GuardrailDecision   `json:"guardrail_decisions,omitempty"`
}

type Turn struct {
	Index              int                 `json:"index"`
	StartedAt          time.Time           `json:"started_at"`
	EndedAt            time.Time           `json:"ended_at"`
	DurationMs         int64               `json:"duration_ms"`
	TokensUsed         int                 `json:"tokens_used"`
	TokensRemaining    int                 `json:"tokens_remaining"`
	CostUSD            float64             `json:"cost_usd"`
	LLMCall            *LLMCall            `json:"llm_call,omitempty"`
	ToolCalls          []ToolCall          `json:"tool_calls"`
	GuardrailDecisions []GuardrailDecision `json:"guardrail_decisions,omitempty"`
	Display            *TurnDisplay        `json:"display,omitempty"`
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
	StartedAt    *time.Time      `json:"started_at,omitempty"`
	EndedAt      *time.Time      `json:"ended_at,omitempty"`
	InputSummary *InputSummary   `json:"input_summary,omitempty"`
}

type ToolCall struct {
	Name            string          `json:"name"`
	Input           json.RawMessage `json:"input"`
	Output          json.RawMessage `json:"output,omitempty"`
	InputLength     int             `json:"input_length"`
	ResultLength    int             `json:"result_length"`
	DurationMs      int64           `json:"duration_ms"`
	StartedAt       *time.Time      `json:"started_at,omitempty"`
	EndedAt         *time.Time      `json:"ended_at,omitempty"`
	ParentToolIndex *int            `json:"parent_tool_index,omitempty"`
	ExecutionMode   string          `json:"execution_mode,omitempty"`
	Error           *ToolError      `json:"error,omitempty"`
}

// InputSummary captures context window state at the start of a turn
type InputSummary struct {
	SystemTokens      int `json:"system_tokens"`
	UserMessages      int `json:"user_messages"`
	AssistantMessages int `json:"assistant_messages"`
	ToolResults       int `json:"tool_results"`
	TotalMessages     int `json:"total_messages"`
	OldestMessageTurn int `json:"oldest_message_turn"`
}

// TurnDisplay provides human-readable metadata for a turn
type TurnDisplay struct {
	Label       string `json:"label"`
	Summary     string `json:"summary"`
	Status      string `json:"status"`
	PrimaryTool string `json:"primary_tool"`
}

// ToolError captures structured error information from tool execution
type ToolError struct {
	Type             string `json:"type"`
	Message          string `json:"message"`
	Retryable        bool   `json:"retryable"`
	AttemptedRetries int    `json:"attempted_retries"`
	SuggestedAction  string `json:"suggested_action"`
}

// RemovedContent tracks what was removed during context compaction
type RemovedContent struct {
	ToolResultsCount int `json:"tool_results_count"`
	MessageCount     int `json:"message_count"`
	SummaryTokens    int `json:"summary_tokens"`
}

type GuardrailDecision struct {
	ToolName        string          `json:"tool_name"`
	Proceed         bool            `json:"proceed"`
	Reason          string          `json:"reason,omitempty"`
	EstimatedTokens int             `json:"estimated_tokens"`
	TokensRemaining int             `json:"tokens_remaining"`
	SafetyMargin    int             `json:"safety_margin"`
	ArgsModified    bool            `json:"args_modified,omitempty"`
	Compacted       bool            `json:"compacted,omitempty"`
	CompactedTurns  []int           `json:"compacted_turns,omitempty"`
	RemovedContent  *RemovedContent `json:"removed_content,omitempty"`
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