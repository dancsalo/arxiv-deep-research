package tools

import "testing"

func TestBuildSearchMemoriesTool(t *testing.T) {
	tool := BuildSearchMemoriesTool()
	if tool.OfTool.Name != "search_memories" {
		t.Errorf("expected name 'search_memories', got %q", tool.OfTool.Name)
	}
	if tool.OfTool.Description.Value == "" {
		t.Error("expected non-empty description")
	}
	props, ok := tool.OfTool.InputSchema.Properties.(map[string]any)
	if !ok {
		t.Fatal("expected properties to be map[string]any")
	}
	if _, ok := props["query"]; !ok {
		t.Error("expected 'query' property")
	}
	if _, ok := props["mode"]; !ok {
		t.Error("expected 'mode' property")
	}
	if len(tool.OfTool.InputSchema.Required) != 1 || tool.OfTool.InputSchema.Required[0] != "query" {
		t.Errorf("expected required=['query'], got %v", tool.OfTool.InputSchema.Required)
	}
}

func TestBuildGetMemoryDetailsTool(t *testing.T) {
	tool := BuildGetMemoryDetailsTool()
	if tool.OfTool.Name != "get_memory_details" {
		t.Errorf("expected name 'get_memory_details', got %q", tool.OfTool.Name)
	}
	if tool.OfTool.Description.Value == "" {
		t.Error("expected non-empty description")
	}
	props, ok := tool.OfTool.InputSchema.Properties.(map[string]any)
	if !ok {
		t.Fatal("expected properties to be map[string]any")
	}
	if _, ok := props["ids"]; !ok {
		t.Error("expected 'ids' property")
	}
}

func TestBuildGetMemorySourceTool(t *testing.T) {
	tool := BuildGetMemorySourceTool()
	if tool.OfTool.Name != "get_memory_source" {
		t.Errorf("expected name 'get_memory_source', got %q", tool.OfTool.Name)
	}
	if tool.OfTool.Description.Value == "" {
		t.Error("expected non-empty description")
	}
	props, ok := tool.OfTool.InputSchema.Properties.(map[string]any)
	if !ok {
		t.Fatal("expected properties to be map[string]any")
	}
	if _, ok := props["id"]; !ok {
		t.Error("expected 'id' property")
	}
}

func TestBuildStoreMemoryTool(t *testing.T) {
	tool := BuildStoreMemoryTool()
	if tool.OfTool.Name != "store_memory" {
		t.Errorf("expected name 'store_memory', got %q", tool.OfTool.Name)
	}
	if tool.OfTool.Description.Value == "" {
		t.Error("expected non-empty description")
	}
	required := tool.OfTool.InputSchema.Required
	if len(required) != 3 {
		t.Fatalf("expected 3 required fields, got %d: %v", len(required), required)
	}
	wantRequired := map[string]bool{"memory_type": true, "title": true, "content": true}
	for _, r := range required {
		if !wantRequired[r] {
			t.Errorf("unexpected required field: %q", r)
		}
	}

	props, ok := tool.OfTool.InputSchema.Properties.(map[string]any)
	if !ok {
		t.Fatal("expected properties to be map[string]any")
	}
	memType, ok := props["memory_type"].(map[string]any)
	if !ok {
		t.Fatal("expected memory_type to be map[string]any")
	}
	enumVals, ok := memType["enum"].([]string)
	if !ok {
		t.Fatal("expected enum to be []string")
	}
	if len(enumVals) != 9 {
		t.Errorf("expected 9 enum values, got %d", len(enumVals))
	}
}

func TestBuildAllTools(t *testing.T) {
	tools := BuildAllTools()
	if len(tools) != 4 {
		t.Fatalf("expected 4 tools, got %d", len(tools))
	}
	names := map[string]bool{}
	for _, tool := range tools {
		names[tool.OfTool.Name] = true
	}
	for _, want := range []string{"search_memories", "get_memory_details", "get_memory_source", "store_memory"} {
		if !names[want] {
			t.Errorf("missing tool: %q", want)
		}
	}
}
