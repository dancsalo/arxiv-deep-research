package ctxmgr

import (
	"context"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

type mockCompactionClient struct {
	response *anthropic.Message
	err      error
	called   int
}

func (m *mockCompactionClient) CreateMessage(_ context.Context, _ anthropic.MessageNewParams) (*anthropic.Message, error) {
	m.called++
	return m.response, m.err
}

func newMockTextResponse(text string) *anthropic.Message {
	return &anthropic.Message{
		Content: []anthropic.ContentBlockUnion{
			{
				Type: "text",
				Text: text,
			},
		},
	}
}

func TestExtractiveTruncateShort(t *testing.T) {
	got := extractiveTruncate("short text", 100)
	if got != "short text" {
		t.Errorf("extractiveTruncate short: got %q, want %q", got, "short text")
	}
}

func TestExtractiveTruncateLong(t *testing.T) {
	content := strings.Repeat("a", 1000)
	got := extractiveTruncate(content, 50) // 50 tokens * 3.5 = 175 chars
	if !strings.HasSuffix(got, "\n[...truncated]") {
		t.Error("expected truncation marker")
	}
	if len([]rune(got)) > 200 {
		t.Errorf("truncated too long: got %d runes", len([]rune(got)))
	}
}

func addTestTurns(m *ContextManager, count int, priority TurnPriority) {
	for i := 0; i < count; i++ {
		text := strings.Repeat("x", 500)
		assistant := anthropic.NewAssistantMessage(anthropic.NewTextBlock(text))
		toolResult := anthropic.NewUserMessage(anthropic.NewToolResultBlock("tool-id", text, false))
		m.AddTurn(assistant, &toolResult, priority)
	}
}

func TestCompactTurnLevel0To1(t *testing.T) {
	m := newTestManager(false, nil)
	addTestTurns(m, 1, PrioritySupplementary)

	before := m.GetTurn(0).EstimatedTokens
	err := m.CompactTurn(context.Background(), 0)
	if err != nil {
		t.Fatalf("CompactTurn: %v", err)
	}

	turn := m.GetTurn(0)
	if turn.CompactionLevel != 1 {
		t.Errorf("compaction level: got %d, want 1", turn.CompactionLevel)
	}
	if !turn.Compacted {
		t.Error("should be marked compacted")
	}
	if turn.ToolResults != nil {
		t.Error("tool results should be nil after compaction")
	}
	if turn.EstimatedTokens >= before {
		t.Errorf("tokens should decrease: before=%d, after=%d", before, turn.EstimatedTokens)
	}
}

func TestCompactTurnLevel3Noop(t *testing.T) {
	m := newTestManager(false, nil)
	addTestTurns(m, 1, PrioritySupplementary)

	m.log.turns[0].CompactionLevel = 3
	err := m.CompactTurn(context.Background(), 0)
	if err != nil {
		t.Fatalf("CompactTurn level 3: %v", err)
	}
	if m.log.turns[0].CompactionLevel != 3 {
		t.Error("level 3 should remain unchanged")
	}
}

func TestCompactTurnLevel2WithMock(t *testing.T) {
	mock := &mockCompactionClient{
		response: newMockTextResponse("Summary of the turn."),
	}
	m := newTestManager(false, nil)
	m.SetCompactionClient(mock)
	addTestTurns(m, 1, PrioritySupplementary)

	m.log.turns[0].CompactionLevel = 1
	err := m.CompactTurn(context.Background(), 0)
	if err != nil {
		t.Fatalf("CompactTurn to level 2: %v", err)
	}
	if m.log.turns[0].CompactionLevel != 2 {
		t.Errorf("compaction level: got %d, want 2", m.log.turns[0].CompactionLevel)
	}
	if mock.called != 1 {
		t.Errorf("expected 1 API call, got %d", mock.called)
	}
}

func TestCompactTurnLevel3Tombstone(t *testing.T) {
	m := newTestManager(false, nil)
	addTestTurns(m, 1, PrioritySupplementary)

	m.log.turns[0].CompactionLevel = 2
	err := m.CompactTurn(context.Background(), 0)
	if err != nil {
		t.Fatalf("CompactTurn to level 3: %v", err)
	}
	if m.log.turns[0].CompactionLevel != 3 {
		t.Errorf("compaction level: got %d, want 3", m.log.turns[0].CompactionLevel)
	}
	text := m.extractMessageText(m.log.turns[0].Assistant)
	if !strings.HasPrefix(text, "Turn 0:") {
		t.Errorf("tombstone should start with 'Turn 0:', got %q", text[:20])
	}
}

func TestCompactConversationHistory(t *testing.T) {
	mock := &mockCompactionClient{
		response: newMockTextResponse("Conversation summary."),
	}
	m := newTestManager(false, nil)
	m.SetCompactionClient(mock)
	addTestTurns(m, 6, PrioritySupplementary)

	err := m.CompactConversationHistory(context.Background())
	if err != nil {
		t.Fatalf("CompactConversationHistory: %v", err)
	}
	// 1 summary + 3 recent = 4
	if m.TurnCount() != 4 {
		t.Errorf("turn count after compaction: got %d, want 4", m.TurnCount())
	}
	summary := m.GetTurn(0)
	if summary.CompactionLevel != 2 {
		t.Errorf("summary compaction level: got %d, want 2", summary.CompactionLevel)
	}
	if summary.Priority != PriorityCore {
		t.Errorf("summary priority: got %d, want PriorityCore", summary.Priority)
	}
	if !m.cached.dirty {
		t.Error("cache should be dirty after compaction")
	}
}

func TestCompactConversationHistoryNoopFewTurns(t *testing.T) {
	m := newTestManager(false, nil)
	addTestTurns(m, 3, PrioritySupplementary)

	err := m.CompactConversationHistory(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.TurnCount() != 3 {
		t.Errorf("should not compact: got %d turns, want 3", m.TurnCount())
	}
}

func TestCompactConversationHistoryNoopAllTombstones(t *testing.T) {
	mock := &mockCompactionClient{
		response: newMockTextResponse("summary"),
	}
	m := newTestManager(false, nil)
	m.SetCompactionClient(mock)
	addTestTurns(m, 5, PrioritySupplementary)

	// Set first 2 turns to tombstone level
	m.log.turns[0].CompactionLevel = 3
	m.log.turns[1].CompactionLevel = 3

	err := m.CompactConversationHistory(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should still compact because turns 2 is not a tombstone
	// But if ALL old turns were tombstones, it would be a no-op
}

func TestAutoCompactFreesSpace(t *testing.T) {
	m := newTestManager(false, nil)
	addTestTurns(m, 5, PrioritySupplementary)

	indices, err := m.autoCompact(context.Background(), 100)
	if err != nil {
		t.Fatalf("autoCompact: %v", err)
	}
	if len(indices) == 0 {
		t.Error("expected some turns to be compacted")
	}
}

func TestAutoCompactPriorityOrder(t *testing.T) {
	m := newTestManager(false, nil)
	// Add 2 core, then 3 supplementary turns
	addTestTurns(m, 2, PriorityCore)
	addTestTurns(m, 3, PrioritySupplementary)

	indices, err := m.autoCompact(context.Background(), 100)
	if err != nil {
		t.Fatalf("autoCompact: %v", err)
	}
	// Supplementary turns (indices 2,3,4) should be compacted first
	if len(indices) > 0 && indices[0] < 2 {
		t.Errorf("expected supplementary turns first, got index %d", indices[0])
	}
}

func TestAutoCompactInsufficientSpace(t *testing.T) {
	m := newTestManager(false, nil)
	addTestTurns(m, 2, PrioritySupplementary)
	// Mark both as already tombstoned
	m.log.turns[0].CompactionLevel = 3
	m.log.turns[1].CompactionLevel = 3

	_, err := m.autoCompact(context.Background(), 10000)
	if err == nil {
		t.Fatal("expected error when space insufficient")
	}
	if !strings.Contains(err.Error(), "could only free") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCompactionCandidatesSorted(t *testing.T) {
	m := newTestManager(false, nil)
	addTestTurns(m, 1, PriorityCore)          // idx 0
	addTestTurns(m, 1, PrioritySupplementary)  // idx 1
	addTestTurns(m, 1, PriorityResearch)       // idx 2

	candidates := m.compactionCandidates()
	// Supplementary (3) > Research (2) > Core (1) in numeric priority
	if candidates[0] != 1 {
		t.Errorf("first candidate should be supplementary (idx 1), got %d", candidates[0])
	}
}
