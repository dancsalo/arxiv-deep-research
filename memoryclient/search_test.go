package memoryclient

import (
	"context"
	"strings"
	"testing"
)

func TestSearchMemories_DefaultMode(t *testing.T) {
	c := &Client{}
	_, err := c.SearchMemories(context.Background(), "test", "magic", 10, "")
	if err == nil {
		t.Fatal("expected error for unknown mode")
	}
	if !strings.Contains(err.Error(), "unknown search mode: magic") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSearchResult_Instructions(t *testing.T) {
	result := &SearchResult{
		Matches:    nil,
		SearchMode: "hybrid",
		Instructions: "Use get_memory_details with IDs from this index to fetch full content. " +
			"Types: 🎯=session-goal 🔴=gotcha 🟡=problem-fix 🔵=how-it-works 🟢=what-changed 🟣=discovery 🟠=why-it-exists 🟤=decision ⚖️=trade-off",
	}
	if !strings.Contains(result.Instructions, "get_memory_details") {
		t.Error("instructions should mention get_memory_details")
	}
}

func TestDetailsResult_Instructions(t *testing.T) {
	result := &DetailsResult{
		Instructions: "Use get_memory_source with an ID to fetch the original raw content (if has_source is true).",
	}
	if !strings.Contains(result.Instructions, "get_memory_source") {
		t.Error("instructions should mention get_memory_source")
	}
}
