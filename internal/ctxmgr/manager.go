package ctxmgr

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
)

type CachedCount struct {
	tokens    int
	dirty     bool
	countedAt time.Time
}

type ContextManager struct {
	estimator        *TokenEstimator
	budget           *ContextBudget
	log              *MessageLog
	outputTracker    *OutputTracker
	cached           CachedCount
	system           []anthropic.TextBlockParam
	tools            []anthropic.MessageCountTokensToolUnionParam
	nowFunc          func() time.Time
	compactionClient CompactionClient
	toolSizeEst      *ToolSizeEstimator
}

type ContextManagerConfig struct {
	Estimator *TokenEstimator
	Budget    *ContextBudget
	System    []anthropic.TextBlockParam
	Tools     []anthropic.MessageCountTokensToolUnionParam
	NowFunc   func() time.Time
}

func NewContextManager(cfg ContextManagerConfig, initialMessage anthropic.MessageParam) *ContextManager {
	nf := cfg.NowFunc
	if nf == nil {
		nf = time.Now
	}
	return &ContextManager{
		estimator:     cfg.Estimator,
		budget:        cfg.Budget,
		log:           NewMessageLog(initialMessage),
		outputTracker: NewOutputTracker(),
		cached:        CachedCount{dirty: true},
		system:        cfg.System,
		tools:         cfg.Tools,
		nowFunc:       nf,
	}
}

func (m *ContextManager) AddTurn(assistant anthropic.MessageParam, toolResults *anthropic.MessageParam, priority TurnPriority) {
	tokenEst := m.estimateTurnTokens(assistant, toolResults)
	m.log.AddTurn(assistant, toolResults, tokenEst, priority)
	m.cached.dirty = true
}

func (m *ContextManager) GetTurn(index int) *Turn {
	return m.log.GetTurn(index)
}

func (m *ContextManager) TurnCount() int {
	return m.log.Len()
}

func (m *ContextManager) BuildMessages() []anthropic.MessageParam {
	return m.log.BuildMessages()
}

func (m *ContextManager) SystemBlocks() []anthropic.TextBlockParam {
	return m.system
}

func (m *ContextManager) OutputTracker() *OutputTracker {
	return m.outputTracker
}

func (m *ContextManager) Budget() *ContextBudget {
	return m.budget
}

func (m *ContextManager) GetTokenCount(ctx context.Context) (int, error) {
	now := m.nowFunc()
	if !m.cached.dirty && now.Sub(m.cached.countedAt) < 30*time.Second {
		return m.cached.tokens, nil
	}
	if m.estimator.hasDirect {
		msgs := m.BuildMessages()
		exact, err := m.estimator.CountExact(ctx, msgs, m.system, m.tools)
		if err != nil {
			return m.estimateAll(), nil
		}
		m.cached = CachedCount{tokens: exact, dirty: false, countedAt: now}
		m.estimator.Calibrate(m.messagesText(), exact, ContentMixed)
		return exact, nil
	}
	return m.estimateAll(), nil
}

func (m *ContextManager) WillFit(estimatedTokens int) bool {
	est := m.estimateAll()
	return m.budget.Remaining(est) >= estimatedTokens
}

func (m *ContextManager) SpaceNeeded(estimatedTokens int) int {
	est := m.estimateAll()
	remaining := m.budget.Remaining(est)
	if remaining >= estimatedTokens {
		return 0
	}
	return estimatedTokens - remaining
}

func (m *ContextManager) estimateAll() int {
	total := 0
	for i := 0; i < m.log.Len(); i++ {
		turn := m.log.GetTurn(i)
		if turn != nil {
			total += turn.EstimatedTokens
		}
	}
	initialText := m.extractMessageText(m.log.initialUserMessage)
	total += m.estimator.EstimateFast(initialText, ContentProse)
	return total
}

func (m *ContextManager) estimateTurnTokens(assistant anthropic.MessageParam, toolResults *anthropic.MessageParam) int {
	assistantText := m.extractMessageText(assistant)
	tokens := m.estimator.EstimateFast(assistantText, ContentMixed)
	if toolResults != nil {
		toolText := m.extractMessageText(*toolResults)
		tokens += m.estimator.EstimateFast(toolText, ContentJSON)
	}
	return tokens
}

func (m *ContextManager) messagesText() string {
	var sb strings.Builder
	msgs := m.BuildMessages()
	for _, msg := range msgs {
		sb.WriteString(m.extractMessageText(msg))
		sb.WriteString("\n")
	}
	return sb.String()
}

func (m *ContextManager) extractMessageText(msg anthropic.MessageParam) string {
	var sb strings.Builder
	for _, block := range msg.Content {
		if tb := block.OfText; tb != nil {
			sb.WriteString(tb.Text)
		} else if tr := block.OfToolResult; tr != nil {
			for _, inner := range tr.Content {
				if it := inner.OfText; it != nil {
					sb.WriteString(it.Text)
				}
			}
		}
	}
	return sb.String()
}

func (m *ContextManager) extractTurnText(turn Turn) string {
	var sb strings.Builder
	sb.WriteString(m.extractMessageText(turn.Assistant))
	if turn.ToolResults != nil {
		sb.WriteString("\n")
		sb.WriteString(m.extractMessageText(*turn.ToolResults))
	}
	return sb.String()
}

func (m *ContextManager) extractMultiTurnText(turns []Turn) string {
	var sb strings.Builder
	for _, turn := range turns {
		sb.WriteString(fmt.Sprintf("Turn %d:\n", turn.Index))
		sb.WriteString(m.extractTurnText(turn))
		sb.WriteString("\n\n")
	}
	return sb.String()
}

func (m *ContextManager) EstimateAllTokens() int {
	return m.estimateAll()
}

func (m *ContextManager) EstimateText(text string, ct ContentType) int {
	return m.estimator.EstimateFast(text, ct)
}

func (m *ContextManager) ExtractInitialQuery() string {
	return m.extractMessageText(m.log.initialUserMessage)
}

func (m *ContextManager) ExtractFinalAnswer() string {
	if m.log.Len() == 0 {
		return ""
	}
	lastTurn := m.log.GetTurn(m.log.Len() - 1)
	if lastTurn == nil {
		return ""
	}
	return m.extractMessageText(lastTurn.Assistant)
}
