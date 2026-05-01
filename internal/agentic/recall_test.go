package agentic

import (
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

func TestBuildRecallQueryTurn0(t *testing.T) {
	got := buildRecallQuery("what is X?", "", 0)
	if got != "what is X?" {
		t.Errorf("got %q, want %q", got, "what is X?")
	}
}

func TestBuildRecallQueryTurnN(t *testing.T) {
	longText := strings.Repeat("a", 300)
	got := buildRecallQuery("what is X?", longText, 3)

	if !strings.Contains(got, "what is X?") {
		t.Error("result should contain original query")
	}
	// Assistant text should be truncated to 200 chars
	lines := strings.SplitN(got, "\n", 2)
	if len(lines[0]) != 200 {
		t.Errorf("assistant text length = %d, want 200", len(lines[0]))
	}
}

func TestBuildRecallQueryEmptyAssistant(t *testing.T) {
	got := buildRecallQuery("what is X?", "", 5)
	if got != "what is X?" {
		t.Errorf("got %q, want %q", got, "what is X?")
	}
}

func TestBuildMemoryBlockFormats(t *testing.T) {
	memories := []RecalledMemory{
		{Type: "gotcha", Title: "pgvector needs HNSW"},
		{Type: "decision", Title: "Use hybrid search"},
	}
	got := buildMemoryBlock(memories)

	if !strings.HasPrefix(got, "[Prior Knowledge]") {
		t.Error("should start with [Prior Knowledge]")
	}
	if !strings.HasSuffix(got, "[End Prior Knowledge]") {
		t.Error("should end with [End Prior Knowledge]")
	}
	if !strings.Contains(got, `- [gotcha] "pgvector needs HNSW"`) {
		t.Error("should contain formatted gotcha memory")
	}
	if !strings.Contains(got, `- [decision] "Use hybrid search"`) {
		t.Error("should contain formatted decision memory")
	}
}

func TestBuildMemoryBlockEmpty(t *testing.T) {
	got := buildMemoryBlock(nil)
	if got != "" {
		t.Errorf("got %q, want empty string", got)
	}
}

func TestInjectMemoriesCopiesMessages(t *testing.T) {
	original := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("hello")),
	}
	origContentLen := len(original[0].Content)

	result := injectMemories(original, "memory block")

	// Original should be unmodified
	if len(original[0].Content) != origContentLen {
		t.Errorf("original content modified: len=%d, want %d", len(original[0].Content), origContentLen)
	}

	// Result should have the memory block appended
	if len(result[0].Content) != origContentLen+1 {
		t.Errorf("result content len = %d, want %d", len(result[0].Content), origContentLen+1)
	}
}

func TestInjectMemoriesPreservesExistingBlocks(t *testing.T) {
	original := []anthropic.MessageParam{
		anthropic.NewUserMessage(
			anthropic.NewTextBlock("block1"),
			anthropic.NewTextBlock("block2"),
		),
	}

	result := injectMemories(original, "memory block")

	if len(result[0].Content) != 3 {
		t.Fatalf("result content len = %d, want 3", len(result[0].Content))
	}
	// First two blocks should be preserved
	if result[0].Content[0].OfText.Text != "block1" {
		t.Error("first block should be preserved")
	}
	if result[0].Content[1].OfText.Text != "block2" {
		t.Error("second block should be preserved")
	}
	if result[0].Content[2].OfText.Text != "memory block" {
		t.Error("third block should be the memory block")
	}
}

func TestDeduplicationFiltersSeenIDs(t *testing.T) {
	loop := &AgenticLoop{
		seenMemoryIDs: map[int64]bool{1: true, 3: true},
	}

	memories := []RecalledMemory{
		{ID: 1, Title: "seen1"},
		{ID: 2, Title: "new1"},
		{ID: 3, Title: "seen2"},
		{ID: 4, Title: "new2"},
	}

	filtered := loop.filterNewMemories(memories)

	if len(filtered) != 2 {
		t.Fatalf("got %d filtered, want 2", len(filtered))
	}
	if filtered[0].ID != 2 || filtered[1].ID != 4 {
		t.Errorf("filtered IDs = [%d, %d], want [2, 4]", filtered[0].ID, filtered[1].ID)
	}

	// All IDs should now be in seenMemoryIDs
	for _, id := range []int64{1, 2, 3, 4} {
		if !loop.seenMemoryIDs[id] {
			t.Errorf("ID %d should be in seenMemoryIDs", id)
		}
	}
}
