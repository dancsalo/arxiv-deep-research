package agentic

import (
	"context"
	"encoding/json"
	"fmt"
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

type GuardrailInfo struct {
	ToolName        string
	Proceed         bool
	Reason          string
	Estimated       int
	Remaining       int
	SafetyMargin    int
	ArgsModified    bool
	Compacted       bool
	CompactedTurns  []int
}

type LoopHooks struct {
	OnTurnStart        func(ctx context.Context, state TurnState) error
	OnTurnEnd          func(ctx context.Context, state TurnState) error
	OnToolCall         func(ctx context.Context, toolName string, input json.RawMessage, state TurnState) error
	OnToolResult       func(ctx context.Context, toolName string, result string, state TurnState) error
	OnGuardrail        func(ctx context.Context, info GuardrailInfo, state TurnState) error
	OnLLMCall          func(ctx context.Context, input, output json.RawMessage, state TurnState) error
	OnMemoryRecall     func(ctx context.Context, memories []RecalledMemory, state TurnState) ([]RecalledMemory, error)
	OnMemoryPersist    func(ctx context.Context, state TurnState) error
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

type ChildConfig struct {
	Query      string
	MaxTurns   int
	MaxCostUSD float64
	Priority   ctxmgr.TurnPriority
	Tools      *registry.ToolRegistry
	System     []anthropic.TextBlockParam
	Model      anthropic.Model
}

func (l *Loop) Spawn(cfg ChildConfig) (*Loop, error) {
	nextDepth := l.depth + 1
	if nextDepth >= l.cfg.MaxDepth {
		return nil, fmt.Errorf("max recursion depth %d reached", l.cfg.MaxDepth)
	}

	tokensUsed := l.manager.EstimateAllTokens()
	parentRemaining := l.manager.Budget().Remaining(tokensUsed)
	childBudget := int(float64(parentRemaining) * 0.5)
	if childBudget > 100_000 {
		childBudget = 100_000
	}

	initialMsg := anthropic.NewUserMessage(anthropic.NewTextBlock(cfg.Query))
	childManager := ctxmgr.NewContextManager(ctxmgr.ContextManagerConfig{
		Estimator: l.manager.Estimator(),
		Budget: &ctxmgr.ContextBudget{
			ModelContextLimit: childBudget,
			MaxOutputTokens:   8192,
			SafetyMargin:      1000,
		},
	}, initialMsg)

	if cc := l.manager.CompactionClient(); cc != nil {
		childManager.SetCompactionClient(cc)
	}

	childReg := cfg.Tools
	if childReg == nil {
		childReg = registry.NewToolRegistry()
	}
	childReg.Register("finish_loop", BuildFinishTool(), func(_ context.Context, input json.RawMessage) (string, error) {
		return string(input), nil
	})

	model := cfg.Model
	if model == "" {
		model = l.cfg.Model
	}

	system := cfg.System
	if system == nil {
		system = l.system
	}

	priority := cfg.Priority
	if priority == 0 {
		priority = ctxmgr.PriorityResearch
	}

	childLogger := l.logger.With("depth", nextDepth)

	child := &Loop{
		client:   l.client,
		manager:  childManager,
		registry: childReg,
		recaller: nil,
		cfg: LoopConfig{
			MaxTurns:        cfg.MaxTurns,
			MaxCostUSD:      cfg.MaxCostUSD,
			MaxDepth:        l.cfg.MaxDepth,
			Model:           model,
			FinishTool:      "finish_loop",
			DefaultPriority: priority,
			Logger:          childLogger,
		},
		system: system,
		hooks:  l.hooks,
		logger: childLogger,
		depth:  nextDepth,
	}

	return child, nil
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
