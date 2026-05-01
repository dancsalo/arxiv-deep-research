package registry

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

func minimalToolDef(name string) anthropic.ToolUnionParam {
	t := anthropic.ToolUnionParamOfTool(
		anthropic.ToolInputSchemaParam{
			Type:       "object",
			Properties: map[string]any{},
		},
		name,
	)
	return t
}

func TestRegistryRegisterAndDefinitions(t *testing.T) {
	r := NewToolRegistry()
	r.Register("alpha", minimalToolDef("alpha"), func(_ context.Context, _ json.RawMessage) (string, error) { return "", nil })
	r.Register("beta", minimalToolDef("beta"), func(_ context.Context, _ json.RawMessage) (string, error) { return "", nil })
	r.Register("gamma", minimalToolDef("gamma"), func(_ context.Context, _ json.RawMessage) (string, error) { return "", nil })

	defs := r.Definitions()
	if len(defs) != 3 {
		t.Fatalf("got %d definitions, want 3", len(defs))
	}

	names := []string{"alpha", "beta", "gamma"}
	for i, def := range defs {
		if def.OfTool.Name != names[i] {
			t.Errorf("defs[%d].Name = %q, want %q", i, def.OfTool.Name, names[i])
		}
	}
}

func TestRegistryExecuteKnownTool(t *testing.T) {
	r := NewToolRegistry()
	var receivedInput json.RawMessage
	r.Register("calc", minimalToolDef("calc"), func(_ context.Context, input json.RawMessage) (string, error) {
		receivedInput = input
		return "42", nil
	})

	input := json.RawMessage(`{"x":1}`)
	result, err := r.Execute(context.Background(), "calc", input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "42" {
		t.Errorf("result = %q, want %q", result, "42")
	}
	if string(receivedInput) != `{"x":1}` {
		t.Errorf("handler received %q, want %q", string(receivedInput), `{"x":1}`)
	}
}

func TestRegistryExecuteUnknownTool(t *testing.T) {
	r := NewToolRegistry()
	result, err := r.Execute(context.Background(), "nope", json.RawMessage("{}"))
	if err == nil {
		t.Fatal("expected error for unknown tool")
	}
	if result != "" {
		t.Errorf("result = %q, want empty", result)
	}
	if err.Error() != "unknown tool: nope" {
		t.Errorf("error = %q, want %q", err.Error(), "unknown tool: nope")
	}
}

type ctxKey string

func TestRegistryExecutePassesContext(t *testing.T) {
	r := NewToolRegistry()
	var gotValue string
	r.Register("tool", minimalToolDef("tool"), func(ctx context.Context, _ json.RawMessage) (string, error) {
		gotValue, _ = ctx.Value(ctxKey("key")).(string)
		return "", nil
	})

	ctx := context.WithValue(context.Background(), ctxKey("key"), "hello")
	_, err := r.Execute(ctx, "tool", json.RawMessage("{}"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotValue != "hello" {
		t.Errorf("context value = %q, want %q", gotValue, "hello")
	}
}

func TestWrapLegacyHandler(t *testing.T) {
	legacy := func(input []byte) (string, error) {
		return string(input), nil
	}
	wrapped := WrapLegacyHandler(legacy)

	input := json.RawMessage(`{"a":1}`)
	result, err := wrapped(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != `{"a":1}` {
		t.Errorf("result = %q, want %q", result, `{"a":1}`)
	}
}

func TestRegistryRegisterDuplicate(t *testing.T) {
	r := NewToolRegistry()
	r.Register("foo", minimalToolDef("foo"), func(_ context.Context, _ json.RawMessage) (string, error) {
		return "v1", nil
	})
	r.Register("foo", minimalToolDef("foo"), func(_ context.Context, _ json.RawMessage) (string, error) {
		return "v2", nil
	})

	defs := r.Definitions()
	if len(defs) != 1 {
		t.Fatalf("got %d definitions, want 1", len(defs))
	}

	result, err := r.Execute(context.Background(), "foo", json.RawMessage("{}"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "v2" {
		t.Errorf("result = %q, want %q (second handler should win)", result, "v2")
	}
}
