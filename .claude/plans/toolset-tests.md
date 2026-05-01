# ToolSet: Test Plan

Companion to `toolset.md`.

## Test File

`src/toolset_test.go` — same package (white-box).

## Tests

### Test: RegisterToolSets with single tool set

- Create a `ToolRegistry`
- Create a mock `ToolSet` that registers 2 tools
- Call `RegisterToolSets(registry, mockSet)`
- Verify `registry.Definitions()` returns 2 definitions

### Test: RegisterToolSets with multiple tool sets

- Create 3 mock tool sets registering 2, 1, and 3 tools respectively
- Call `RegisterToolSets(registry, set1, set2, set3)`
- Verify `registry.Definitions()` returns 6 definitions
- Verify all tool names are present

### Test: RegisterToolSets with zero tool sets

- Call `RegisterToolSets(registry)` with no sets
- Verify `registry.Definitions()` returns empty slice
- No panic

### Test: Duplicate tool name across tool sets

- Set1 registers tool "search" with handler returning "v1"
- Set2 registers tool "search" with handler returning "v2"
- Call `RegisterToolSets(registry, set1, set2)`
- `registry.Execute(ctx, "search", ...)` returns "v2" (last wins)
- `registry.Definitions()` has exactly 1 "search" entry

### Test: ToolSet with dependencies

- Create a tool set struct holding a mock dependency (e.g., a string config)
- Register it → verify the handler uses the dependency
- Confirms struct-based tool sets can hold and use state

### Mock ToolSet

```go
type mockToolSet struct {
    tools map[string]string // name → fixed response
}

func (m *mockToolSet) Register(registry *ToolRegistry) {
    for name, resp := range m.tools {
        resp := resp
        def := /* minimal tool def */
        registry.Register(name, def, func(ctx context.Context, input json.RawMessage) (string, error) {
            return resp, nil
        })
    }
}
```
