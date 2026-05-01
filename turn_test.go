package contextmanager

import (
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

func TestBuildMessagesNoTurns(t *testing.T) {
	initial := anthropic.NewUserMessage(anthropic.NewTextBlock("What is quantum computing?"))
	ml := NewMessageLog(initial)
	msgs := ml.BuildMessages()
	if len(msgs) != 1 {
		t.Errorf("BuildMessages no turns: got %d messages, want 1", len(msgs))
	}
}

func TestBuildMessagesWithToolResult(t *testing.T) {
	initial := anthropic.NewUserMessage(anthropic.NewTextBlock("search something"))
	ml := NewMessageLog(initial)

	assistant := anthropic.NewAssistantMessage(anthropic.NewTextBlock("I'll search for that."))
	toolResult := anthropic.NewUserMessage(anthropic.NewToolResultBlock("tool-1", "result data", false))
	ml.AddTurn(assistant, &toolResult, 100, PriorityResearch)

	msgs := ml.BuildMessages()
	if len(msgs) != 3 {
		t.Errorf("BuildMessages with tool: got %d messages, want 3", len(msgs))
	}
}

func TestBuildMessagesNoToolResult(t *testing.T) {
	initial := anthropic.NewUserMessage(anthropic.NewTextBlock("hello"))
	ml := NewMessageLog(initial)

	assistant := anthropic.NewAssistantMessage(anthropic.NewTextBlock("Hi there!"))
	ml.AddTurn(assistant, nil, 50, PriorityResearch)

	msgs := ml.BuildMessages()
	if len(msgs) != 2 {
		t.Errorf("BuildMessages no tool: got %d messages, want 2", len(msgs))
	}
}

func TestBuildMessagesMultipleTurns(t *testing.T) {
	initial := anthropic.NewUserMessage(anthropic.NewTextBlock("research topic"))
	ml := NewMessageLog(initial)

	// Turn 1: with tool result
	a1 := anthropic.NewAssistantMessage(anthropic.NewTextBlock("searching..."))
	tr1 := anthropic.NewUserMessage(anthropic.NewToolResultBlock("t1", "results", false))
	ml.AddTurn(a1, &tr1, 100, PriorityResearch)

	// Turn 2: no tool result
	a2 := anthropic.NewAssistantMessage(anthropic.NewTextBlock("Here's what I found."))
	ml.AddTurn(a2, nil, 50, PriorityCore)

	// Turn 3: with tool result
	a3 := anthropic.NewAssistantMessage(anthropic.NewTextBlock("fetching more..."))
	tr3 := anthropic.NewUserMessage(anthropic.NewToolResultBlock("t3", "more data", false))
	ml.AddTurn(a3, &tr3, 200, PrioritySupplementary)

	msgs := ml.BuildMessages()
	// initial + (a1 + tr1) + (a2) + (a3 + tr3) = 1 + 2 + 1 + 2 = 6
	if len(msgs) != 6 {
		t.Errorf("BuildMessages multiple turns: got %d messages, want 6", len(msgs))
	}
}

func TestTurnDefaultValues(t *testing.T) {
	initial := anthropic.NewUserMessage(anthropic.NewTextBlock("test"))
	ml := NewMessageLog(initial)

	assistant := anthropic.NewAssistantMessage(anthropic.NewTextBlock("response"))
	ml.AddTurn(assistant, nil, 50, PriorityResearch)

	turn := ml.GetTurn(0)
	if turn == nil {
		t.Fatal("GetTurn returned nil")
	}
	if turn.Compacted {
		t.Error("new turn should not be compacted")
	}
	if turn.CompactionLevel != 0 {
		t.Errorf("new turn compaction level: got %d, want 0", turn.CompactionLevel)
	}
}

func TestGetTurnOutOfBounds(t *testing.T) {
	initial := anthropic.NewUserMessage(anthropic.NewTextBlock("test"))
	ml := NewMessageLog(initial)

	if ml.GetTurn(0) != nil {
		t.Error("GetTurn(0) on empty log should return nil")
	}
	if ml.GetTurn(-1) != nil {
		t.Error("GetTurn(-1) should return nil")
	}
}

func TestMessageLogLen(t *testing.T) {
	initial := anthropic.NewUserMessage(anthropic.NewTextBlock("test"))
	ml := NewMessageLog(initial)

	if ml.Len() != 0 {
		t.Errorf("empty log Len: got %d, want 0", ml.Len())
	}

	assistant := anthropic.NewAssistantMessage(anthropic.NewTextBlock("response"))
	ml.AddTurn(assistant, nil, 50, PriorityResearch)

	if ml.Len() != 1 {
		t.Errorf("one turn Len: got %d, want 1", ml.Len())
	}
}
