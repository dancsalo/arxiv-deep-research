package agentic

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/dancsalo/arxiv-deep-research/internal/ctxmgr"
	"github.com/dancsalo/arxiv-deep-research/internal/registry"
)

type RecalledMemory struct {
	ID    int64
	Type  string
	Title string
	Score float64
}

type MemoryRecaller interface {
	RecallMemories(ctx context.Context, query string, mode string, limit int) ([]RecalledMemory, error)
}

type MemoryRecallConfig struct {
	Enabled      bool
	MaxResults   int
	MaxTokens    int
	SearchMode   string
	SkipFirstN   int
	RecallEveryN int
}

type TurnState struct {
	TurnIndex         int
	TotalCostUSD      float64
	TokensUsed        int
	TokensRemaining   int
	LastToolCalls     []string
	RecalledMemoryIDs []int64
	AssistantText     string
	ToolResultTexts   map[string]string
}

type LoopHooks struct {
	OnTurnStart     func(ctx context.Context, state TurnState) error
	OnTurnEnd       func(ctx context.Context, state TurnState) error
	OnToolCall      func(ctx context.Context, toolName string, input json.RawMessage, state TurnState) error
	OnToolResult    func(ctx context.Context, toolName string, result string, state TurnState) error
	OnMemoryRecall  func(ctx context.Context, memories []RecalledMemory, state TurnState) ([]RecalledMemory, error)
	OnMemoryPersist func(ctx context.Context, state TurnState) error
}

type LoopConfig struct {
	MaxTurns        int
	MaxCostUSD      float64
	MaxDepth        int
	Model           anthropic.Model
	SessionID       string
	FinishTool      string
	DefaultPriority ctxmgr.TurnPriority
	MemoryRecall    MemoryRecallConfig
	Hooks           *LoopHooks
	Logger          *slog.Logger
}

type Loop struct {
	client   MessageClient
	manager  *ctxmgr.ContextManager
	registry *registry.ToolRegistry
	recaller MemoryRecaller
	cfg      LoopConfig
	system   []anthropic.TextBlockParam
	hooks    *LoopHooks

	query         string
	totalCostUSD  float64
	turnIndex     int
	finished      bool
	seenMemoryIDs map[int64]bool
	logger        *slog.Logger
	depth         int
	mu            sync.Mutex
}

func NewLoop(
	client MessageClient,
	manager *ctxmgr.ContextManager,
	reg *registry.ToolRegistry,
	recaller MemoryRecaller,
	cfg LoopConfig,
	system []anthropic.TextBlockParam,
) *Loop {
	if cfg.MemoryRecall.Enabled {
		if cfg.MemoryRecall.MaxResults == 0 {
			cfg.MemoryRecall.MaxResults = 5
		}
		if cfg.MemoryRecall.MaxTokens == 0 {
			cfg.MemoryRecall.MaxTokens = 2000
		}
		if cfg.MemoryRecall.SearchMode == "" {
			cfg.MemoryRecall.SearchMode = "hybrid"
		}
		if cfg.MemoryRecall.RecallEveryN == 0 {
			cfg.MemoryRecall.RecallEveryN = 1
		}
	}

	if cfg.DefaultPriority == 0 {
		cfg.DefaultPriority = ctxmgr.PriorityCore
	}

	if cfg.MaxDepth == 0 {
		cfg.MaxDepth = 3
	}

	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	hooks := cfg.Hooks
	if hooks == nil {
		hooks = &LoopHooks{}
	}

	return &Loop{
		client:   client,
		manager:  manager,
		registry: reg,
		recaller: recaller,
		cfg:      cfg,
		system:   system,
		hooks:    hooks,
		logger:   logger,
	}
}

func (l *Loop) TotalCost() float64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.totalCostUSD
}

func (l *Loop) addChildCost(cost float64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.totalCostUSD += cost
}

// Backward-compatible aliases
type AgenticLoop = Loop
type AgenticLoopConfig = LoopConfig

func NewAgenticLoop(
	client MessageClient,
	manager *ctxmgr.ContextManager,
	reg *registry.ToolRegistry,
	recaller MemoryRecaller,
	cfg LoopConfig,
	system []anthropic.TextBlockParam,
) *Loop {
	return NewLoop(client, manager, reg, recaller, cfg, system)
}
