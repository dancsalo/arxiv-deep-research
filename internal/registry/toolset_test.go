package registry

import (
	"context"
	"encoding/json"
	"testing"
)

type mockToolSet struct {
	tools map[string]string
}

func (m *mockToolSet) Register(registry *ToolRegistry) {
	for name, resp := range m.tools {
		resp := resp
		registry.Register(name, minimalToolDef(name), func(_ context.Context, _ json.RawMessage) (string, error) {
			return resp, nil
		})
	}
}

func TestRegisterToolSetsSingle(t *testing.T) {
	r := NewToolRegistry()
	set := &mockToolSet{tools: map[string]string{"a": "1", "b": "2"}}
	RegisterToolSets(r, set)

	defs := r.Definitions()
	if len(defs) != 2 {
		t.Errorf("got %d definitions, want 2", len(defs))
	}
}

func TestRegisterToolSetsMultiple(t *testing.T) {
	r := NewToolRegistry()
	set1 := &mockToolSet{tools: map[string]string{"a": "1", "b": "2"}}
	set2 := &mockToolSet{tools: map[string]string{"c": "3"}}
	set3 := &mockToolSet{tools: map[string]string{"d": "4", "e": "5", "f": "6"}}
	RegisterToolSets(r, set1, set2, set3)

	defs := r.Definitions()
	if len(defs) != 6 {
		t.Errorf("got %d definitions, want 6", len(defs))
	}
}

func TestRegisterToolSetsZero(t *testing.T) {
	r := NewToolRegistry()
	RegisterToolSets(r)

	defs := r.Definitions()
	if len(defs) != 0 {
		t.Errorf("got %d definitions, want 0", len(defs))
	}
}

func TestRegisterToolSetsDuplicate(t *testing.T) {
	r := NewToolRegistry()
	set1 := &mockToolSet{tools: map[string]string{"search": "v1"}}
	set2 := &mockToolSet{tools: map[string]string{"search": "v2"}}
	RegisterToolSets(r, set1, set2)

	defs := r.Definitions()
	if len(defs) != 1 {
		t.Fatalf("got %d definitions, want 1", len(defs))
	}

	result, err := r.Execute(context.Background(), "search", json.RawMessage("{}"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "v2" {
		t.Errorf("result = %q, want %q (last wins)", result, "v2")
	}
}

type configToolSet struct {
	prefix string
}

func (c *configToolSet) Register(registry *ToolRegistry) {
	prefix := c.prefix
	registry.Register("tool", minimalToolDef("tool"), func(_ context.Context, _ json.RawMessage) (string, error) {
		return prefix + "-result", nil
	})
}

func TestRegisterToolSetsWithDependencies(t *testing.T) {
	r := NewToolRegistry()
	set := &configToolSet{prefix: "myprefix"}
	RegisterToolSets(r, set)

	result, err := r.Execute(context.Background(), "tool", json.RawMessage("{}"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "myprefix-result" {
		t.Errorf("result = %q, want %q", result, "myprefix-result")
	}
}
