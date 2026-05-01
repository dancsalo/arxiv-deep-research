package agentic

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/dancsalo/arxiv-deep-research/internal/ctxmgr"
)

func (a *AgenticLoop) Run(ctx context.Context, query string) (string, error) {
	a.query = query
	a.totalCostUSD = 0
	a.turnIndex = 0
	a.finished = false
	a.seenMemoryIDs = make(map[int64]bool)

	tools := a.registry.Definitions()

	for a.turnIndex < a.cfg.MaxTurns && !a.finished {
		if ctx.Err() != nil {
			a.logger.Info("loop.cancelled", "turn", a.turnIndex, "reason", ctx.Err())
			return "", ctx.Err()
		}

		tokensUsed := a.manager.EstimateAllTokens()
		tokensRemaining := a.manager.Budget().Remaining(tokensUsed)

		state := TurnState{
			TurnIndex:       a.turnIndex,
			TotalCostUSD:    a.totalCostUSD,
			TokensUsed:      tokensUsed,
			TokensRemaining: tokensRemaining,
		}

		a.logger.Info("turn.start", "turn", a.turnIndex, "tokens_used", tokensUsed, "tokens_remaining", tokensRemaining)

		if a.hooks.OnTurnStart != nil {
			if err := a.hooks.OnTurnStart(ctx, state); err != nil {
				a.logger.Warn("hook.error", "hook", "OnTurnStart", "err", err)
			}
		}

		// Memory recall phase
		var memoryBlock string
		var recalledIDs []int64
		if a.recaller != nil && a.cfg.MemoryRecall.Enabled && a.shouldRecall() {
			recalledIDs, memoryBlock = a.doRecall(ctx, state)
		}

		if err := a.manager.PreAPICheck(ctx); err != nil {
			return "", fmt.Errorf("turn %d pre-API check failed: %w", a.turnIndex, err)
		}

		messages := a.manager.BuildMessages()
		if memoryBlock != "" {
			messages = injectMemories(messages, memoryBlock)
		}

		resp, err := a.client.CreateMessage(ctx, anthropic.MessageNewParams{
			Model:     a.cfg.Model,
			MaxTokens: 8192,
			Tools:     tools,
			System:    a.system,
			Messages:  messages,
		})
		if err != nil {
			return "", fmt.Errorf("turn %d API error: %w", a.turnIndex, err)
		}

		cost := estimateCost(resp.Usage, a.cfg.Model)
		a.totalCostUSD += cost
		a.manager.OutputTracker().Record(int(resp.Usage.OutputTokens))

		a.logger.Info("llm.call", "turn", a.turnIndex, "model", string(a.cfg.Model),
			"input_tokens", resp.Usage.InputTokens, "output_tokens", resp.Usage.OutputTokens,
			"cost_usd", cost)

		if ctx.Err() != nil {
			a.logger.Info("loop.cancelled", "turn", a.turnIndex, "reason", ctx.Err())
			return "", ctx.Err()
		}

		if a.totalCostUSD > a.cfg.MaxCostUSD {
			return "", fmt.Errorf("cost limit exceeded: $%.4f > $%.4f", a.totalCostUSD, a.cfg.MaxCostUSD)
		}

		// Process response blocks
		var toolResults []anthropic.ContentBlockParamUnion
		var toolCalls []string
		toolResultTexts := make(map[string]string)
		var assistantText string

		for _, block := range resp.Content {
			switch block.Type {
			case "text":
				assistantText += block.Text

			case "tool_use":
				toolName := block.Name
				toolID := block.ID
				toolInput := block.Input

				if toolName == a.cfg.FinishTool {
					a.finished = true
					result, execErr := a.registry.Execute(ctx, toolName, toolInput)
					if execErr != nil {
						toolResults = append(toolResults,
							anthropic.NewToolResultBlock(toolID, fmt.Sprintf("error: %v", execErr), true))
					} else {
						toolResults = append(toolResults,
							anthropic.NewToolResultBlock(toolID, result, false))
						toolResultTexts[toolName] = result
					}
					toolCalls = append(toolCalls, toolName)
					continue
				}

				if a.hooks.OnToolCall != nil {
					if err := a.hooks.OnToolCall(ctx, toolName, toolInput, state); err != nil {
						a.logger.Warn("hook.error", "hook", "OnToolCall", "err", err)
					}
				}

				args := jsonToMap(toolInput)
				decision, guardErr := a.manager.PreToolGuardrail(ctx, toolName, args)
				if guardErr != nil {
					toolResults = append(toolResults,
						anthropic.NewToolResultBlock(toolID, fmt.Sprintf("error: %v", guardErr), true))
					toolCalls = append(toolCalls, toolName)
					continue
				}
				if !decision.Proceed {
					toolResults = append(toolResults,
						anthropic.NewToolResultBlock(toolID, fmt.Sprintf("REJECTED: %s", decision.Reason), true))
					toolCalls = append(toolCalls, toolName)
					continue
				}

				execInput := toolInput
				if decision.ModifiedArgs != nil {
					modifiedJSON, err := json.Marshal(decision.ModifiedArgs)
					if err == nil {
						execInput = json.RawMessage(modifiedJSON)
					}
				}

				start := time.Now()
				result, execErr := a.registry.Execute(ctx, toolName, execInput)
				latency := time.Since(start).Milliseconds()

				if execErr != nil {
					a.logger.Warn("tool.unknown", "tool", toolName)
					toolResults = append(toolResults,
						anthropic.NewToolResultBlock(toolID, fmt.Sprintf("error: %v", execErr), true))
				} else {
					a.logger.Info("tool.execute", "turn", a.turnIndex, "tool", toolName, "latency_ms", latency)
					if postErr := a.manager.PostToolCheck(ctx, toolName, result); postErr != nil {
						_ = postErr
					}
					toolResults = append(toolResults,
						anthropic.NewToolResultBlock(toolID, result, false))
					toolResultTexts[toolName] = result
				}

				if a.hooks.OnToolResult != nil {
					resultText := result
					if execErr != nil {
						resultText = execErr.Error()
					}
					if err := a.hooks.OnToolResult(ctx, toolName, resultText, state); err != nil {
						a.logger.Warn("hook.error", "hook", "OnToolResult", "err", err)
					}
				}

				toolCalls = append(toolCalls, toolName)
			}
		}

		// Record turn
		assistantMsg := responseToAssistantParam(resp)
		var toolResultMsg *anthropic.MessageParam
		if len(toolResults) > 0 {
			msg := anthropic.NewUserMessage(toolResults...)
			toolResultMsg = &msg
		}
		a.manager.AddTurn(assistantMsg, toolResultMsg, a.cfg.DefaultPriority)

		// Post-turn state for hooks
		tokensUsed = a.manager.EstimateAllTokens()
		postState := TurnState{
			TurnIndex:         a.turnIndex,
			TotalCostUSD:      a.totalCostUSD,
			TokensUsed:        tokensUsed,
			TokensRemaining:   a.manager.Budget().Remaining(tokensUsed),
			LastToolCalls:     toolCalls,
			RecalledMemoryIDs: recalledIDs,
			AssistantText:     assistantText,
			ToolResultTexts:   toolResultTexts,
		}

		if a.hooks.OnMemoryPersist != nil {
			if err := a.hooks.OnMemoryPersist(ctx, postState); err != nil {
				a.logger.Warn("hook.error", "hook", "OnMemoryPersist", "err", err)
			}
		}

		// Termination check
		if ctx.Err() != nil {
			a.logger.Info("loop.cancelled", "turn", a.turnIndex, "reason", ctx.Err())
			return "", ctx.Err()
		}
		if a.finished || resp.StopReason == "end_turn" {
			if a.hooks.OnTurnEnd != nil {
				if err := a.hooks.OnTurnEnd(ctx, postState); err != nil {
					a.logger.Warn("hook.error", "hook", "OnTurnEnd", "err", err)
				}
			}
			break
		}

		if a.hooks.OnTurnEnd != nil {
			if err := a.hooks.OnTurnEnd(ctx, postState); err != nil {
				a.logger.Warn("hook.error", "hook", "OnTurnEnd", "err", err)
			}
		}

		a.turnIndex++
	}

	return a.manager.ExtractFinalAnswer(), nil
}

func (a *AgenticLoop) shouldRecall() bool {
	if a.turnIndex < a.cfg.MemoryRecall.SkipFirstN {
		return false
	}
	offset := a.turnIndex - a.cfg.MemoryRecall.SkipFirstN
	return offset%a.cfg.MemoryRecall.RecallEveryN == 0
}

func (a *AgenticLoop) doRecall(ctx context.Context, state TurnState) ([]int64, string) {
	lastText := a.manager.ExtractFinalAnswer()
	recallQuery := buildRecallQuery(a.query, lastText, a.turnIndex)

	memories, err := a.recaller.RecallMemories(ctx, recallQuery, a.cfg.MemoryRecall.SearchMode, a.cfg.MemoryRecall.MaxResults)
	if err != nil {
		a.logger.Warn("memory.recall.failed", "err", err)
		return nil, ""
	}

	memories = a.filterNewMemories(memories)
	if len(memories) == 0 {
		a.logger.Debug("memory.recall.skip", "reason", "no new memories")
		return nil, ""
	}

	if a.hooks.OnMemoryRecall != nil {
		filtered, err := a.hooks.OnMemoryRecall(ctx, memories, state)
		if err != nil {
			a.logger.Warn("hook.error", "hook", "OnMemoryRecall", "err", err)
			return nil, ""
		}
		memories = filtered
	}

	if len(memories) == 0 {
		return nil, ""
	}

	block := buildMemoryBlock(memories)
	memTokens := a.manager.EstimateText(block, ctxmgr.ContentProse)

	// Trim lowest-score memories until block fits or is empty
	for !a.manager.WillFit(memTokens) && len(memories) > 0 {
		sort.Slice(memories, func(i, j int) bool {
			return memories[i].Score < memories[j].Score
		})
		memories = memories[1:]
		if len(memories) == 0 {
			a.logger.Debug("memory.recall.skip", "reason", "budget exhausted")
			return nil, ""
		}
		block = buildMemoryBlock(memories)
		memTokens = a.manager.EstimateText(block, ctxmgr.ContentProse)
	}

	var ids []int64
	for _, m := range memories {
		ids = append(ids, m.ID)
	}

	a.logger.Info("memory.recall", "turn", a.turnIndex, "query", recallQuery, "results", len(memories), "injected", len(memories))

	return ids, block
}
