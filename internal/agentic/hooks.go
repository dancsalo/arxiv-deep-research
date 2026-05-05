package agentic

import (
	"context"
	"encoding/json"
)

func ComposeHooks(a, b *LoopHooks) *LoopHooks {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}

	return &LoopHooks{
		OnTurnStart: func(ctx context.Context, state TurnState) error {
			if a.OnTurnStart != nil {
				if err := a.OnTurnStart(ctx, state); err != nil {
					return err
				}
			}
			if b.OnTurnStart != nil {
				return b.OnTurnStart(ctx, state)
			}
			return nil
		},
		OnTurnEnd: func(ctx context.Context, state TurnState) error {
			if a.OnTurnEnd != nil {
				if err := a.OnTurnEnd(ctx, state); err != nil {
					return err
				}
			}
			if b.OnTurnEnd != nil {
				return b.OnTurnEnd(ctx, state)
			}
			return nil
		},
		OnToolCall: func(ctx context.Context, toolName string, input json.RawMessage, state TurnState) error {
			if a.OnToolCall != nil {
				if err := a.OnToolCall(ctx, toolName, input, state); err != nil {
					return err
				}
			}
			if b.OnToolCall != nil {
				return b.OnToolCall(ctx, toolName, input, state)
			}
			return nil
		},
		OnToolResult: func(ctx context.Context, toolName string, result string, state TurnState) error {
			if a.OnToolResult != nil {
				if err := a.OnToolResult(ctx, toolName, result, state); err != nil {
					return err
				}
			}
			if b.OnToolResult != nil {
				return b.OnToolResult(ctx, toolName, result, state)
			}
			return nil
		},
		OnMemoryRecall: func(ctx context.Context, memories []RecalledMemory, state TurnState) ([]RecalledMemory, error) {
			result := memories
			if a.OnMemoryRecall != nil {
				var err error
				result, err = a.OnMemoryRecall(ctx, result, state)
				if err != nil {
					return nil, err
				}
			}
			if b.OnMemoryRecall != nil {
				return b.OnMemoryRecall(ctx, result, state)
			}
			return result, nil
		},
		OnMemoryPersist: func(ctx context.Context, state TurnState) error {
			if a.OnMemoryPersist != nil {
				if err := a.OnMemoryPersist(ctx, state); err != nil {
					return err
				}
			}
			if b.OnMemoryPersist != nil {
				return b.OnMemoryPersist(ctx, state)
			}
			return nil
		},
	}
}
