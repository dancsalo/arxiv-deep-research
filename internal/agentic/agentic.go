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

func (l *Loop) Run(ctx context.Context, query string) (string, error) {
	l.query = query
	l.totalCostUSD = 0
	l.turnIndex = 0
	l.finished = false
	l.seenMemoryIDs = make(map[int64]bool)

	var finishResult string
	tools := l.registry.Definitions()

	for l.turnIndex < l.cfg.MaxTurns && !l.finished {
		if ctx.Err() != nil {
			l.logger.Info("loop.cancelled", "turn", l.turnIndex, "reason", ctx.Err())
			return l.partialResult, ctx.Err()
		}

		tokensUsed := l.manager.EstimateAllTokens()
		tokensRemaining := l.manager.Budget().Remaining(tokensUsed)

		state := TurnState{
			TurnIndex:       l.turnIndex,
			TotalCostUSD:    l.TotalCost(),
			TokensUsed:      tokensUsed,
			TokensRemaining: tokensRemaining,
		}

		l.logger.Info("turn.start", "turn", l.turnIndex, "tokens_used", tokensUsed, "tokens_remaining", tokensRemaining)

		if l.hooks.OnTurnStart != nil {
			if err := l.hooks.OnTurnStart(ctx, state); err != nil {
				l.logger.Warn("hook.error", "hook", "OnTurnStart", "err", err)
			}
		}

		var memoryBlock string
		var recalledIDs []int64
		if l.recaller != nil && l.cfg.MemoryRecall.Enabled && l.depth == 0 && l.shouldRecall() {
			recalledIDs, memoryBlock = l.doRecall(ctx, state)
		}

		if err := l.manager.PreAPICheck(ctx); err != nil {
			return "", fmt.Errorf("turn %d pre-API check failed: %w", l.turnIndex, err)
		}

		messages := l.manager.BuildMessages()
		if memoryBlock != "" {
			messages = injectMemories(messages, memoryBlock)
		}

		params := anthropic.MessageNewParams{
			Model:     l.cfg.Model,
			MaxTokens: 8192,
			Tools:     tools,
			System:    l.system,
			Messages:  messages,
		}

		resp, err := l.client.CreateMessage(ctx, params)
		if err != nil {
			return "", fmt.Errorf("turn %d API error: %w", l.turnIndex, err)
		}

		// Call OnLLMCall hook with full input/output
		if l.hooks.OnLLMCall != nil {
			inputJSON, _ := json.Marshal(params)
			outputJSON, _ := json.Marshal(resp)
			if err := l.hooks.OnLLMCall(ctx, inputJSON, outputJSON, state); err != nil {
				l.logger.Warn("hook.error", "hook", "OnLLMCall", "err", err)
			}
		}

		cost := estimateCost(resp.Usage, l.cfg.Model)
		l.mu.Lock()
		l.totalCostUSD += cost
		l.mu.Unlock()
		l.manager.OutputTracker().Record(int(resp.Usage.OutputTokens))

		l.logger.Info("llm.call", "turn", l.turnIndex, "model", string(l.cfg.Model),
			"input_tokens", resp.Usage.InputTokens, "output_tokens", resp.Usage.OutputTokens,
			"cost_usd", cost)

		if ctx.Err() != nil {
			l.logger.Info("loop.cancelled", "turn", l.turnIndex, "reason", ctx.Err())
			return l.partialResult, ctx.Err()
		}

		if l.TotalCost() > l.cfg.MaxCostUSD {
			return l.partialResult, fmt.Errorf("cost limit exceeded: $%.4f > $%.4f", l.TotalCost(), l.cfg.MaxCostUSD)
		}

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

				if toolName == l.cfg.FinishTool {
					l.finished = true
					var parsed struct {
						Summary string `json:"summary"`
					}
					if err := json.Unmarshal(toolInput, &parsed); err == nil && parsed.Summary != "" {
						finishResult = parsed.Summary
					}
					if l.hooks.OnToolCall != nil {
						if err := l.hooks.OnToolCall(ctx, toolName, toolInput, state); err != nil {
							l.logger.Warn("hook.error", "hook", "OnToolCall", "err", err)
						}
					}
					result, execErr := l.registry.Execute(ctx, toolName, toolInput)
					if l.hooks.OnToolResult != nil {
						if err := l.hooks.OnToolResult(ctx, toolName, result, state); err != nil {
							l.logger.Warn("hook.error", "hook", "OnToolResult", "err", err)
						}
					}
					if execErr != nil {
						l.logger.Warn("tool.error", "turn", l.turnIndex, "tool", toolName, "error", execErr.Error())
						toolResults = append(toolResults,
							anthropic.NewToolResultBlock(toolID, fmt.Sprintf("error: %v", execErr), true))
					} else {
						l.logger.Info("tool.execute", "turn", l.turnIndex, "tool", toolName, "latency_ms", 0)
						toolResults = append(toolResults,
							anthropic.NewToolResultBlock(toolID, result, false))
						toolResultTexts[toolName] = result
					}
					toolCalls = append(toolCalls, toolName)
					continue
				}

				if l.hooks.OnToolCall != nil {
					if err := l.hooks.OnToolCall(ctx, toolName, toolInput, state); err != nil {
						l.logger.Warn("hook.error", "hook", "OnToolCall", "err", err)
					}
				}

				args := jsonToMap(toolInput)
				decision, guardErr := l.manager.PreToolGuardrail(ctx, toolName, args)

				// Call OnGuardrail hook with decision info
				if l.hooks.OnGuardrail != nil {
					tokensUsed := l.manager.EstimateAllTokens()
					tokensRemaining := l.manager.Budget().Remaining(tokensUsed)
					safetyMargin := l.manager.Budget().SafetyMargin
					estimated := l.manager.EstimateToolResult(toolName, args)

					guardInfo := GuardrailInfo{
						ToolName:           toolName,
						Proceed:            guardErr == nil && decision.Proceed,
						Reason:             decision.Reason,
						Estimated:          estimated,
						Remaining:          tokensRemaining,
						SafetyMargin:       safetyMargin,
						ArgsModified:       decision.ModifiedArgs != nil,
						Compacted:          len(decision.Compacted) > 0,
						CompactedTurns:     decision.Compacted,
						ToolResultsRemoved: decision.ToolResultsRemoved,
						MessagesRemoved:    decision.MessagesRemoved,
						SummaryTokens:      decision.SummaryTokens,
					}
					if err := l.hooks.OnGuardrail(ctx, guardInfo, state); err != nil {
						l.logger.Warn("hook.error", "hook", "OnGuardrail", "err", err)
					}
				}

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
				result, execErr := l.registry.Execute(ctx, toolName, execInput)
				latency := time.Since(start).Milliseconds()

				if execErr != nil {
					l.logger.Warn("tool.error", "turn", l.turnIndex, "tool", toolName, "error", execErr.Error())
					toolResults = append(toolResults,
						anthropic.NewToolResultBlock(toolID, fmt.Sprintf("error: %v", execErr), true))
				} else {
					l.logger.Info("tool.execute", "turn", l.turnIndex, "tool", toolName, "latency_ms", latency)
					if postErr := l.manager.PostToolCheck(ctx, toolName, result); postErr != nil {
						_ = postErr
					}
					toolResults = append(toolResults,
						anthropic.NewToolResultBlock(toolID, result, false))
					toolResultTexts[toolName] = result
				}

				if l.hooks.OnToolResult != nil {
					resultText := result
					if execErr != nil {
						resultText = execErr.Error()
					}
					if err := l.hooks.OnToolResult(ctx, toolName, resultText, state); err != nil {
						l.logger.Warn("hook.error", "hook", "OnToolResult", "err", err)
					}
				}

				toolCalls = append(toolCalls, toolName)
			}
		}

		// Accumulate assistant text for partial results
		if assistantText != "" {
			l.partialResult += assistantText
		}

		assistantMsg := responseToAssistantParam(resp)
		var toolResultMsg *anthropic.MessageParam
		if len(toolResults) > 0 {
			msg := anthropic.NewUserMessage(toolResults...)
			toolResultMsg = &msg
		}
		l.manager.AddTurn(assistantMsg, toolResultMsg, l.cfg.DefaultPriority)

		tokensUsed = l.manager.EstimateAllTokens()
		postState := TurnState{
			TurnIndex:         l.turnIndex,
			TotalCostUSD:      l.TotalCost(),
			TokensUsed:        tokensUsed,
			TokensRemaining:   l.manager.Budget().Remaining(tokensUsed),
			LastToolCalls:     toolCalls,
			RecalledMemoryIDs: recalledIDs,
			AssistantText:     assistantText,
			ToolResultTexts:   toolResultTexts,
		}

		if l.hooks.OnMemoryPersist != nil {
			if err := l.hooks.OnMemoryPersist(ctx, postState); err != nil {
				l.logger.Warn("hook.error", "hook", "OnMemoryPersist", "err", err)
			}
		}

		if ctx.Err() != nil {
			l.logger.Info("loop.cancelled", "turn", l.turnIndex, "reason", ctx.Err())
			return l.partialResult, ctx.Err()
		}
		if l.finished || resp.StopReason == "end_turn" {
			if l.hooks.OnTurnEnd != nil {
				if err := l.hooks.OnTurnEnd(ctx, postState); err != nil {
					l.logger.Warn("hook.error", "hook", "OnTurnEnd", "err", err)
				}
			}
			break
		}

		if l.hooks.OnTurnEnd != nil {
			if err := l.hooks.OnTurnEnd(ctx, postState); err != nil {
				l.logger.Warn("hook.error", "hook", "OnTurnEnd", "err", err)
			}
		}

		l.turnIndex++
	}

	if finishResult != "" {
		return finishResult, nil
	}
	if l.partialResult != "" {
		return l.partialResult, nil
	}
	return l.manager.ExtractFinalAnswer(), nil
}

func (l *Loop) shouldRecall() bool {
	if l.turnIndex < l.cfg.MemoryRecall.SkipFirstN {
		return false
	}
	offset := l.turnIndex - l.cfg.MemoryRecall.SkipFirstN
	return offset%l.cfg.MemoryRecall.RecallEveryN == 0
}

func (l *Loop) doRecall(ctx context.Context, state TurnState) ([]int64, string) {
	lastText := l.manager.ExtractFinalAnswer()
	recallQuery := buildRecallQuery(l.query, lastText, l.turnIndex)

	memories, err := l.recaller.RecallMemories(ctx, recallQuery, l.cfg.MemoryRecall.SearchMode, l.cfg.MemoryRecall.MaxResults)
	if err != nil {
		l.logger.Warn("memory.recall.failed", "err", err)
		return nil, ""
	}

	memories = l.filterNewMemories(memories)
	if len(memories) == 0 {
		l.logger.Debug("memory.recall.skip", "reason", "no new memories")
		return nil, ""
	}

	if l.hooks.OnMemoryRecall != nil {
		filtered, err := l.hooks.OnMemoryRecall(ctx, memories, state)
		if err != nil {
			l.logger.Warn("hook.error", "hook", "OnMemoryRecall", "err", err)
			return nil, ""
		}
		memories = filtered
	}

	if len(memories) == 0 {
		return nil, ""
	}

	block := buildMemoryBlock(memories)
	memTokens := l.manager.EstimateText(block, ctxmgr.ContentProse)

	for !l.manager.WillFit(memTokens) && len(memories) > 0 {
		sort.Slice(memories, func(i, j int) bool {
			return memories[i].Score < memories[j].Score
		})
		memories = memories[1:]
		if len(memories) == 0 {
			l.logger.Debug("memory.recall.skip", "reason", "budget exhausted")
			return nil, ""
		}
		block = buildMemoryBlock(memories)
		memTokens = l.manager.EstimateText(block, ctxmgr.ContentProse)
	}

	var ids []int64
	for _, m := range memories {
		ids = append(ids, m.ID)
	}

	l.logger.Info("memory.recall", "turn", l.turnIndex, "query", recallQuery, "results", len(memories), "injected", len(memories))

	return ids, block
}
