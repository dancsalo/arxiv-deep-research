package ctxmgr

type RollingAvg struct {
	values []int
	sum    int
}

func NewRollingAvg() *RollingAvg {
	return &RollingAvg{}
}

func (r *RollingAvg) Record(v int) {
	r.values = append(r.values, v)
	r.sum += v
	if len(r.values) > 20 {
		r.sum -= r.values[0]
		r.values = r.values[1:]
	}
}

func (r *RollingAvg) Avg() int {
	if len(r.values) == 0 {
		return 0
	}
	return r.sum / len(r.values)
}

func (r *RollingAvg) Len() int {
	return len(r.values)
}

type ToolSizeEstimator struct {
	staticEstimates map[string]func(args map[string]any) int
	history         map[string]*RollingAvg
}

func NewToolSizeEstimator() *ToolSizeEstimator {
	return &ToolSizeEstimator{
		staticEstimates: make(map[string]func(args map[string]any) int),
		history:         make(map[string]*RollingAvg),
	}
}

func (e *ToolSizeEstimator) RegisterTool(name string, estimateFn func(args map[string]any) int) {
	e.staticEstimates[name] = estimateFn
}

func (e *ToolSizeEstimator) Estimate(toolName string, args map[string]any) int {
	if h, ok := e.history[toolName]; ok && h.Len() >= 3 {
		return h.Avg()
	}
	if fn, ok := e.staticEstimates[toolName]; ok {
		return fn(args)
	}
	return 5000
}

func (e *ToolSizeEstimator) Record(toolName string, tokens int) {
	h, ok := e.history[toolName]
	if !ok {
		h = NewRollingAvg()
		e.history[toolName] = h
	}
	h.Record(tokens)
}
