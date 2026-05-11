# ToolSet Interface: Domain Tool Extensibility

## Problem Statement

The `ToolRegistry` (planned in `agentic-loop.md`) provides `Register(name, def, handler)` for adding individual tools. But there's no pattern for organizing groups of related tools, composing multiple tool sets, or onboarding a new domain. A developer adding search tools, memory tools, and a finish tool must make N separate `Register` calls with no structure.

**Goal:** Define a `ToolSet` interface that standardizes how groups of related tools are packaged, making it easy to compose multiple tool sets into a single `ToolRegistry`.

## Requirements

1. A `ToolSet` groups related tools (definitions + handlers + dependencies) into a single unit.
2. Multiple tool sets compose into one `ToolRegistry` via a helper function.
3. Tool sets can hold configuration and dependencies as struct fields (API keys, DB clients).
4. Each tool set is independently testable.
5. The interface is minimal — one method.

## Specs

### Interface

```go
package contextmanager

// ToolSet is a group of related tools that register themselves
// into a ToolRegistry as a unit.
type ToolSet interface {
    // Register adds all tools in this set to the given registry.
    Register(registry *ToolRegistry)
}

// RegisterToolSets registers multiple tool sets into a single registry.
// If two tool sets register the same tool name, the last one wins
// (ToolRegistry overwrites on duplicate names).
func RegisterToolSets(registry *ToolRegistry, sets ...ToolSet) {
    for _, s := range sets {
        s.Register(registry)
    }
}
```

### Usage: Defining a Tool Set

```go
package mytools

type SearchToolSet struct {
    apiKey string
}

func NewSearchToolSet(apiKey string) *SearchToolSet {
    return &SearchToolSet{apiKey: apiKey}
}

func (s *SearchToolSet) Register(registry *contextmanager.ToolRegistry) {
    registry.Register("search_web", s.searchWebDef(), s.handleSearchWeb)
    registry.Register("fetch_page", s.fetchPageDef(), s.handleFetchPage)
}

func (s *SearchToolSet) searchWebDef() anthropic.ToolUnionParam { /* ... */ }
func (s *SearchToolSet) handleSearchWeb(ctx context.Context, input json.RawMessage) (string, error) { /* ... */ }
func (s *SearchToolSet) fetchPageDef() anthropic.ToolUnionParam { /* ... */ }
func (s *SearchToolSet) handleFetchPage(ctx context.Context, input json.RawMessage) (string, error) { /* ... */ }
```

### Usage: Composing Tool Sets

```go
registry := contextmanager.NewToolRegistry()
contextmanager.RegisterToolSets(registry,
    mytools.NewSearchToolSet(apiKey),
    memorytools.NewMemoryToolSet(memClient, sessionID),
    finishtools.NewFinishToolSet(),
)
// registry now contains all tools from all three sets
```

### Usage: Adapting Existing Memory Tools

The existing `tools.NewMemoryToolHandlers` returns `map[string]ToolHandler`. A memory tool set wraps this:

```go
package memorytools

type MemoryToolSet struct {
    client    *memoryclient.Client
    sessionID string
    turnFunc  func() int
}

func NewMemoryToolSet(client *memoryclient.Client, sessionID string, turnFunc func() int) *MemoryToolSet {
    return &MemoryToolSet{client: client, sessionID: sessionID, turnFunc: turnFunc}
}

func (m *MemoryToolSet) Register(registry *contextmanager.ToolRegistry) {
    handlers := tools.NewMemoryToolHandlers(m.client, m.sessionID, m.turnFunc)
    defs := map[string]anthropic.ToolUnionParam{
        "search_memories":    tools.BuildSearchMemoriesTool(),
        "get_memory_details": tools.BuildGetMemoryDetailsTool(),
        "get_memory_source":  tools.BuildGetMemorySourceTool(),
        "store_memory":       tools.BuildStoreMemoryTool(),
    }
    for name, handler := range handlers {
        registry.Register(name, defs[name], contextmanager.WrapLegacyHandler(handler))
    }
}
```

## Contracts

- `ToolSet.Register(registry)` is called once during setup.
- Tool sets hold their own dependencies as struct fields, initialized before registration.
- If two tool sets register the same tool name, the last one wins (ToolRegistry overwrites). No error.
- `Register` must not panic. If a tool set needs fallible initialization (e.g., API key validation), do it in the constructor, not in `Register`.

## Decisions & Tradeoffs

1. **Interface vs. registration function:** `ToolSet` is an interface rather than `func(*ToolRegistry)`. This gives tool sets a struct to hold configuration and dependencies. Tradeoff: slightly more ceremony, but enables dependency injection and testability.

2. **Same package as ToolRegistry:** `ToolSet` lives in `contextmanager` alongside `ToolRegistry`. This continues the package-scope tension noted in the agentic loop plan, but `ToolSet` is a one-interface, one-function addition tightly coupled to `ToolRegistry`. Moving it to a separate package would require exporting `ToolRegistry` internals.

3. **No duplicate detection:** `RegisterToolSets` does not warn on duplicate tool names. The `ToolRegistry.Register` method silently overwrites. This is simple and predictable. If callers need duplicate detection, they can check `registry.Definitions()` length before and after.

## Implementation Order

### Section 1: Interface & Helper
- Define `ToolSet` interface and `RegisterToolSets` function
- File: `src/toolset.go`
- Depends on: `ToolRegistry` from agentic loop plan Section 2

## Open Questions

None — this is a small, well-scoped addition.

## Revision Log

- v1 (initial): Extracted from `server-and-toolsets.md` into standalone plan.
