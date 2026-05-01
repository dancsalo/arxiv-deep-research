package contextmanager

import (
	"log/slog"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

func TestNewAgenticLoopAppliesDefaults(t *testing.T) {
	client := &scriptedMessageClient{}
	manager := newLoopManager()
	registry := NewToolRegistry()

	tests := []struct {
		name     string
		cfg      AgenticLoopConfig
		checkFn  func(t *testing.T, loop *AgenticLoop)
	}{
		{
			name: "enabled recall with zero fields gets defaults",
			cfg: AgenticLoopConfig{
				MemoryRecall: MemoryRecallConfig{Enabled: true},
			},
			checkFn: func(t *testing.T, loop *AgenticLoop) {
				if loop.cfg.MemoryRecall.MaxResults != 5 {
					t.Errorf("MaxResults = %d, want 5", loop.cfg.MemoryRecall.MaxResults)
				}
				if loop.cfg.MemoryRecall.MaxTokens != 2000 {
					t.Errorf("MaxTokens = %d, want 2000", loop.cfg.MemoryRecall.MaxTokens)
				}
				if loop.cfg.MemoryRecall.SearchMode != "hybrid" {
					t.Errorf("SearchMode = %q, want hybrid", loop.cfg.MemoryRecall.SearchMode)
				}
				if loop.cfg.MemoryRecall.RecallEveryN != 1 {
					t.Errorf("RecallEveryN = %d, want 1", loop.cfg.MemoryRecall.RecallEveryN)
				}
			},
		},
		{
			name: "enabled recall with explicit values not overwritten",
			cfg: AgenticLoopConfig{
				MemoryRecall: MemoryRecallConfig{Enabled: true, MaxResults: 10, MaxTokens: 3000, SearchMode: "semantic", RecallEveryN: 3},
			},
			checkFn: func(t *testing.T, loop *AgenticLoop) {
				if loop.cfg.MemoryRecall.MaxResults != 10 {
					t.Errorf("MaxResults = %d, want 10", loop.cfg.MemoryRecall.MaxResults)
				}
				if loop.cfg.MemoryRecall.MaxTokens != 3000 {
					t.Errorf("MaxTokens = %d, want 3000", loop.cfg.MemoryRecall.MaxTokens)
				}
				if loop.cfg.MemoryRecall.SearchMode != "semantic" {
					t.Errorf("SearchMode = %q, want semantic", loop.cfg.MemoryRecall.SearchMode)
				}
				if loop.cfg.MemoryRecall.RecallEveryN != 3 {
					t.Errorf("RecallEveryN = %d, want 3", loop.cfg.MemoryRecall.RecallEveryN)
				}
			},
		},
		{
			name: "disabled recall does not apply defaults",
			cfg: AgenticLoopConfig{
				MemoryRecall: MemoryRecallConfig{Enabled: false},
			},
			checkFn: func(t *testing.T, loop *AgenticLoop) {
				if loop.cfg.MemoryRecall.MaxResults != 0 {
					t.Errorf("MaxResults = %d, want 0", loop.cfg.MemoryRecall.MaxResults)
				}
				if loop.cfg.MemoryRecall.MaxTokens != 0 {
					t.Errorf("MaxTokens = %d, want 0", loop.cfg.MemoryRecall.MaxTokens)
				}
			},
		},
		{
			name: "zero DefaultPriority defaults to PriorityCore",
			cfg:  AgenticLoopConfig{},
			checkFn: func(t *testing.T, loop *AgenticLoop) {
				if loop.cfg.DefaultPriority != PriorityCore {
					t.Errorf("DefaultPriority = %d, want %d", loop.cfg.DefaultPriority, PriorityCore)
				}
			},
		},
		{
			name: "nil Logger defaults to slog.Default",
			cfg:  AgenticLoopConfig{},
			checkFn: func(t *testing.T, loop *AgenticLoop) {
				if loop.logger == nil {
					t.Error("logger should not be nil")
				}
			},
		},
		{
			name: "explicit Logger is used",
			cfg: AgenticLoopConfig{
				Logger: slog.Default(),
			},
			checkFn: func(t *testing.T, loop *AgenticLoop) {
				if loop.logger != slog.Default() {
					t.Error("expected explicit logger to be used")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.cfg.Model = anthropic.ModelClaudeHaiku4_5
			tt.cfg.MaxTurns = 1
			loop := NewAgenticLoop(client, manager, registry, nil, tt.cfg, nil)
			tt.checkFn(t, loop)
		})
	}
}

func TestNewAgenticLoopNilRecaller(t *testing.T) {
	client := &scriptedMessageClient{}
	manager := newLoopManager()
	registry := NewToolRegistry()

	loop := NewAgenticLoop(client, manager, registry, nil, AgenticLoopConfig{
		MaxTurns: 1,
		Model:    anthropic.ModelClaudeHaiku4_5,
	}, nil)

	if loop == nil {
		t.Fatal("expected non-nil loop")
	}
	if loop.recaller != nil {
		t.Error("expected nil recaller")
	}
}

func TestNewAgenticLoopNilHooks(t *testing.T) {
	client := &scriptedMessageClient{}
	manager := newLoopManager()
	registry := NewToolRegistry()

	loop := NewAgenticLoop(client, manager, registry, nil, AgenticLoopConfig{
		MaxTurns: 1,
		Model:    anthropic.ModelClaudeHaiku4_5,
		Hooks:    nil,
	}, nil)

	if loop.hooks == nil {
		t.Error("hooks should be initialized to empty struct, not nil")
	}
}
