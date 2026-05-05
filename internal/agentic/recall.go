package agentic

import (
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
)

func buildRecallQuery(query string, lastAssistantText string, turnIndex int) string {
	if turnIndex == 0 || lastAssistantText == "" {
		return query
	}
	truncated := lastAssistantText
	if len(truncated) > 200 {
		truncated = truncated[:200]
	}
	return fmt.Sprintf("%s\n%s", truncated, query)
}

func buildMemoryBlock(memories []RecalledMemory) string {
	if len(memories) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("[Prior Knowledge]\n")
	for _, m := range memories {
		fmt.Fprintf(&b, "- [%s] %q\n", m.Type, m.Title)
	}
	b.WriteString("[End Prior Knowledge]")
	return b.String()
}

func injectMemories(messages []anthropic.MessageParam, block string) []anthropic.MessageParam {
	copied := make([]anthropic.MessageParam, len(messages))
	copy(copied, messages)

	if len(copied) > 0 {
		orig := copied[0]
		newContent := make([]anthropic.ContentBlockParamUnion, len(orig.Content), len(orig.Content)+1)
		copy(newContent, orig.Content)
		newContent = append(newContent, anthropic.NewTextBlock(block))
		copied[0] = anthropic.MessageParam{
			Role:    orig.Role,
			Content: newContent,
		}
	}
	return copied
}

func (l *Loop) filterNewMemories(memories []RecalledMemory) []RecalledMemory {
	var filtered []RecalledMemory
	for _, m := range memories {
		if !l.seenMemoryIDs[m.ID] {
			filtered = append(filtered, m)
			l.seenMemoryIDs[m.ID] = true
		}
	}
	return filtered
}
