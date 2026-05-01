package contextmanager

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/anthropics/anthropic-sdk-go"
)

const maxCompactionInputTokens = 50_000
const maxSummarizeDepth = 3

type CompactionClient interface {
	CreateMessage(ctx context.Context, params anthropic.MessageNewParams) (*anthropic.Message, error)
}

func (m *ContextManager) CompactTurn(ctx context.Context, turnIdx int) error {
	if turnIdx < 0 || turnIdx >= m.log.Len() {
		return fmt.Errorf("turn index %d out of range", turnIdx)
	}
	turn := m.log.GetTurn(turnIdx)
	if turn.CompactionLevel >= 3 {
		return nil
	}

	nextLevel := turn.CompactionLevel + 1
	content := m.extractTurnText(*turn)

	var text string
	switch nextLevel {
	case 1:
		text = extractiveTruncate(content, turn.EstimatedTokens/2)
	case 2:
		summary, err := m.summarize(ctx, content, nextLevel, 0)
		if err != nil {
			return fmt.Errorf("compaction failed for turn %d: %w", turnIdx, err)
		}
		text = summary
	case 3:
		text = buildTombstone(turnIdx, content)
	}

	m.applyCompaction(turnIdx, nextLevel, text)
	m.cached.dirty = true
	return nil
}

func (m *ContextManager) applyCompaction(turnIdx, level int, text string) {
	m.log.turns[turnIdx].Assistant = anthropic.NewAssistantMessage(anthropic.NewTextBlock(text))
	m.log.turns[turnIdx].ToolResults = nil
	m.log.turns[turnIdx].EstimatedTokens = m.estimator.EstimateFast(text, ContentProse)
	m.log.turns[turnIdx].CompactionLevel = level
	m.log.turns[turnIdx].Compacted = true
}

func (m *ContextManager) CompactConversationHistory(ctx context.Context) error {
	if m.log.Len() <= 3 {
		return nil
	}

	keepCount := 3
	oldTurns := m.log.turns[:m.log.Len()-keepCount]

	var compactable []Turn
	for _, turn := range oldTurns {
		if turn.CompactionLevel < 3 {
			compactable = append(compactable, turn)
		}
	}
	if len(compactable) == 0 {
		return nil
	}

	oldContent := m.extractMultiTurnText(compactable)
	summary, err := m.summarize(ctx, oldContent, 2, 0)
	if err != nil {
		return fmt.Errorf("conversation compaction failed: %w", err)
	}

	summaryTurn := Turn{
		Index:           0,
		Assistant:       anthropic.NewAssistantMessage(anthropic.NewTextBlock(summary)),
		EstimatedTokens: m.estimator.EstimateFast(summary, ContentProse),
		Compacted:       true,
		CompactionLevel: 2,
		Priority:        PriorityCore,
	}

	recent := make([]Turn, keepCount)
	copy(recent, m.log.turns[m.log.Len()-keepCount:])

	m.log.turns = append([]Turn{summaryTurn}, recent...)
	for i := range m.log.turns {
		m.log.turns[i].Index = i
	}
	m.cached.dirty = true
	return nil
}

func (m *ContextManager) autoCompact(ctx context.Context, tokensNeeded int) ([]int, error) {
	freed := 0
	var compactedIdxs []int
	var firstErr error

	candidates := m.compactionCandidates()

	for _, idx := range candidates {
		if freed >= tokensNeeded {
			break
		}
		turn := m.log.turns[idx]
		if turn.CompactionLevel >= 3 {
			continue
		}

		beforeTokens := turn.EstimatedTokens
		err := m.CompactTurn(ctx, idx)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}

		freed += beforeTokens - m.log.turns[idx].EstimatedTokens
		compactedIdxs = append(compactedIdxs, idx)
	}

	if freed < tokensNeeded {
		msg := fmt.Sprintf("could only free %d of %d tokens needed (%d turns compacted)",
			freed, tokensNeeded, len(compactedIdxs))
		if firstErr != nil {
			msg += fmt.Sprintf("; first compaction error: %v", firstErr)
		}
		return compactedIdxs, fmt.Errorf("%s", msg)
	}

	m.cached.dirty = true
	return compactedIdxs, nil
}

// compactionCandidates returns turn indices sorted for compaction:
// lowest-importance first (highest TurnPriority number), then oldest first.
func (m *ContextManager) compactionCandidates() []int {
	type candidate struct {
		index    int
		priority TurnPriority
	}
	var candidates []candidate
	for i := 0; i < m.log.Len(); i++ {
		turn := m.log.GetTurn(i)
		if turn.CompactionLevel < 3 {
			candidates = append(candidates, candidate{index: i, priority: turn.Priority})
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].priority != candidates[j].priority {
			// Higher numeric priority = lower importance = compact first
			return candidates[i].priority > candidates[j].priority
		}
		return candidates[i].index < candidates[j].index
	})
	result := make([]int, len(candidates))
	for i, c := range candidates {
		result[i] = c.index
	}
	return result
}

func (m *ContextManager) SetCompactionClient(client CompactionClient) {
	m.compactionClient = client
}

func (m *ContextManager) summarize(ctx context.Context, content string, level int, depth int) (string, error) {
	if m.compactionClient == nil {
		return extractiveTruncate(content, maxCompactionInputTokens/2), nil
	}

	contentTokens := m.estimator.EstimateFast(content, ContentMixed)
	if contentTokens > maxCompactionInputTokens {
		if depth >= maxSummarizeDepth {
			return extractiveTruncate(content, maxCompactionInputTokens/2), nil
		}
		return m.summarizeChunked(ctx, content, level, depth)
	}

	targetChars := utf8.RuneCountInString(content) / (2 * (level + 1))
	if targetChars < 100 {
		targetChars = 100
	}
	prompt := fmt.Sprintf(
		"Summarize in under %d characters, preserving key facts, numbers, and conclusions:\n\n%s",
		targetChars, content,
	)

	resp, err := m.compactionClient.CreateMessage(ctx, anthropic.MessageNewParams{
		Model:     anthropic.ModelClaudeHaiku4_5,
		MaxTokens: int64(targetChars / 3),
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
		},
	})
	if err != nil {
		return "", fmt.Errorf("summarization API call failed: %w", err)
	}

	for _, block := range resp.Content {
		if block.Type == "text" {
			return block.Text, nil
		}
	}
	return "", fmt.Errorf("summarization returned no text")
}

func (m *ContextManager) summarizeChunked(ctx context.Context, content string, level int, depth int) (string, error) {
	chunkSize := int(float64(maxCompactionInputTokens) * 3.5)
	runes := []rune(content)
	var summaries []string
	for i := 0; i < len(runes); i += chunkSize {
		end := i + chunkSize
		if end > len(runes) {
			end = len(runes)
		}
		s, err := m.summarize(ctx, string(runes[i:end]), level, depth+1)
		if err != nil {
			return "", err
		}
		summaries = append(summaries, s)
	}
	if len(summaries) > 1 {
		combined := strings.Join(summaries, "\n\n")
		return m.summarize(ctx, combined, level, depth+1)
	}
	return summaries[0], nil
}

func extractiveTruncate(content string, maxTokens int) string {
	maxChars := int(float64(maxTokens) * 3.5)
	runes := []rune(content)
	if len(runes) <= maxChars {
		return content
	}
	return string(runes[:maxChars]) + "\n[...truncated]"
}

func buildTombstone(turnIdx int, content string) string {
	runes := []rune(content)
	preview := string(runes)
	if len(runes) > 80 {
		preview = string(runes[:80])
	}
	return fmt.Sprintf("Turn %d: %s", turnIdx, preview)
}
