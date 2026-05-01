package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"
)

// SSEHandler is an slog.Handler that writes log records as SSE events.
// All writes are protected by a shared mutex. WithAttrs and WithGroup
// return new instances that share the same mutex and ResponseWriter.
//
// The handler must only be used while the HTTP handler that created it
// is still executing — the ResponseWriter is invalid after the handler returns.
type SSEHandler struct {
	w       http.ResponseWriter
	flusher http.Flusher
	mu      *sync.Mutex
	attrs   []slog.Attr
	groups  []string
	level   slog.Level
}

func NewSSEHandler(w http.ResponseWriter, level slog.Level) (*SSEHandler, error) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, fmt.Errorf("ResponseWriter does not implement http.Flusher")
	}
	return &SSEHandler{
		w:       w,
		flusher: flusher,
		mu:      &sync.Mutex{},
		level:   level,
	}, nil
}

func (h *SSEHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *SSEHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	event := map[string]any{
		"time":  r.Time.Format(time.RFC3339),
		"level": r.Level.String(),
		"msg":   r.Message,
	}

	prefix := strings.Join(h.groups, ".")
	for _, a := range h.attrs {
		key := a.Key
		if prefix != "" {
			key = prefix + "." + key
		}
		event[key] = a.Value.Any()
	}

	r.Attrs(func(a slog.Attr) bool {
		key := a.Key
		if prefix != "" {
			key = prefix + "." + key
		}
		event[key] = a.Value.Any()
		return true
	})

	data, err := json.Marshal(event)
	if err != nil {
		data = []byte(fmt.Sprintf(`{"msg":"marshal_error","error":%q}`, err.Error()))
	}
	_, writeErr := fmt.Fprintf(h.w, "event: log\ndata: %s\n\n", data)
	h.flusher.Flush()
	return writeErr
}

func (h *SSEHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &SSEHandler{
		w: h.w, flusher: h.flusher, mu: h.mu,
		attrs:  append(slices.Clone(h.attrs), attrs...),
		groups: slices.Clone(h.groups),
		level:  h.level,
	}
}

func (h *SSEHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	return &SSEHandler{
		w: h.w, flusher: h.flusher, mu: h.mu,
		attrs:  slices.Clone(h.attrs),
		groups: append(slices.Clone(h.groups), name),
		level:  h.level,
	}
}

func (h *SSEHandler) SendDone(answer string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	data, _ := json.Marshal(map[string]string{"answer": answer})
	fmt.Fprintf(h.w, "event: done\ndata: %s\n\n", data)
	h.flusher.Flush()
}

func (h *SSEHandler) SendError(err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	data, _ := json.Marshal(map[string]string{"error": err.Error()})
	fmt.Fprintf(h.w, "event: error\ndata: %s\n\n", data)
	h.flusher.Flush()
}
