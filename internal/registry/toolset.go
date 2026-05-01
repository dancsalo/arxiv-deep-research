package registry

type ToolSet interface {
	Register(registry *ToolRegistry)
}

func RegisterToolSets(registry *ToolRegistry, sets ...ToolSet) {
	for _, s := range sets {
		s.Register(registry)
	}
}
