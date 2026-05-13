package tracing

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Recorder struct {
	cfg            Config
	trace          Trace
	currentTurn    *Turn
	pendingLLMCall *LLMCall
	toolStartStack []time.Time
	toolIndexStack []int
	prevCostUSD    float64
	mu             sync.Mutex
}

func NewRecorder(cfg Config) *Recorder {
	return &Recorder{
		cfg: cfg,
		trace: Trace{
			SessionID:     cfg.SessionID,
			PromptVariant: cfg.PromptVariant,
			PromptHash:    cfg.PromptHash,
			Query:         cfg.Query,
			Model:         cfg.Model,
			StartedAt:     time.Now(),
			Status:        "ok",
		},
	}
}

func (r *Recorder) SetError(err error) {
	if err == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.trace.Status = "error"
	r.trace.Error = err.Error()
}

func (r *Recorder) Flush() error {
	if !r.cfg.Enabled() {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.trace.EndedAt = time.Now()
	r.trace.DurationMs = r.trace.EndedAt.Sub(r.trace.StartedAt).Milliseconds()
	return r.writeTrace()
}

// flushIncremental writes the trace without updating end time (for in-progress traces)
func (r *Recorder) flushIncremental() error {
	if !r.cfg.Enabled() {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	// Update end time and duration for in-progress traces
	r.trace.EndedAt = time.Now()
	r.trace.DurationMs = r.trace.EndedAt.Sub(r.trace.StartedAt).Milliseconds()
	return r.writeTrace()
}

// writeTrace writes the trace to disk (must be called with r.mu held)
func (r *Recorder) writeTrace() error {
	if err := os.MkdirAll(r.cfg.Dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(r.trace, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(r.cfg.Dir, r.cfg.SessionID+".json"), data, 0o644)
}