package contextmanager

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
)

type ToolHandler func(ctx context.Context, input json.RawMessage) (string, error)

type RegisteredTool struct {
	Definition anthropic.ToolUnionParam
	Handler    ToolHandler
}

// ToolRegistry is not safe for concurrent use. All Register calls must
// complete before any concurrent Execute or Definitions calls.
type ToolRegistry struct {
	tools map[string]RegisteredTool
	order []string
}

func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		tools: make(map[string]RegisteredTool),
	}
}

func (r *ToolRegistry) Register(name string, def anthropic.ToolUnionParam, handler ToolHandler) {
	if _, exists := r.tools[name]; !exists {
		r.order = append(r.order, name)
	}
	r.tools[name] = RegisteredTool{Definition: def, Handler: handler}
}

func (r *ToolRegistry) Definitions() []anthropic.ToolUnionParam {
	defs := make([]anthropic.ToolUnionParam, 0, len(r.order))
	for _, name := range r.order {
		defs = append(defs, r.tools[name].Definition)
	}
	return defs
}

func (r *ToolRegistry) Execute(ctx context.Context, name string, input json.RawMessage) (string, error) {
	tool, ok := r.tools[name]
	if !ok {
		return "", fmt.Errorf("unknown tool: %s", name)
	}
	return tool.Handler(ctx, input)
}

func WrapLegacyHandler(h func([]byte) (string, error)) ToolHandler {
	return func(_ context.Context, input json.RawMessage) (string, error) {
		return h([]byte(input))
	}
}
