package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func newTestHandler() (*SSEHandler, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	h, err := NewSSEHandler(w, slog.LevelInfo)
	if err != nil {
		panic(err)
	}
	return h, w
}

func TestNewSSEHandlerNonFlusher(t *testing.T) {
	w := &nonFlusherWriter{}
	_, err := NewSSEHandler(w, slog.LevelInfo)
	if err == nil {
		t.Error("expected error for non-Flusher writer")
	}
}

type nonFlusherWriter struct{}

func (n *nonFlusherWriter) Header() http.Header        { return http.Header{} }
func (n *nonFlusherWriter) Write(b []byte) (int, error) { return len(b), nil }
func (n *nonFlusherWriter) WriteHeader(int)             {}

func TestSSEHandlerWritesEventFormat(t *testing.T) {
	h, w := newTestHandler()

	r := slog.NewRecord(time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC), slog.LevelInfo, "test", 0)
	r.AddAttrs(slog.String("key1", "val1"))

	if err := h.Handle(context.Background(), r); err != nil {
		t.Fatalf("Handle error: %v", err)
	}

	body := w.Body.String()
	if !strings.HasPrefix(body, "event: log\ndata: ") {
		t.Errorf("expected SSE format, got: %q", body)
	}
	if !strings.HasSuffix(body, "\n\n") {
		t.Error("expected double newline suffix")
	}

	// Parse the JSON data
	dataLine := strings.TrimPrefix(body, "event: log\ndata: ")
	dataLine = strings.TrimSuffix(dataLine, "\n\n")
	var event map[string]any
	if err := json.Unmarshal([]byte(dataLine), &event); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if event["msg"] != "test" {
		t.Errorf("msg = %q, want %q", event["msg"], "test")
	}
	if event["key1"] != "val1" {
		t.Errorf("key1 = %q, want %q", event["key1"], "val1")
	}
	if _, ok := event["time"]; !ok {
		t.Error("expected time field")
	}
	if _, ok := event["level"]; !ok {
		t.Error("expected level field")
	}
}

func TestSSEHandlerLevelFiltering(t *testing.T) {
	h, _ := newTestHandler()
	h.level = slog.LevelWarn

	if h.Enabled(context.Background(), slog.LevelInfo) {
		t.Error("LevelInfo should be disabled when handler level is Warn")
	}
	if !h.Enabled(context.Background(), slog.LevelWarn) {
		t.Error("LevelWarn should be enabled")
	}
	if !h.Enabled(context.Background(), slog.LevelError) {
		t.Error("LevelError should be enabled")
	}
}

func TestSSEHandlerWithAttrsSharedMutex(t *testing.T) {
	h, w := newTestHandler()

	h2 := h.WithAttrs([]slog.Attr{slog.String("k", "v")})

	// Write via h2 — should include k=v
	r := slog.NewRecord(time.Now(), slog.LevelInfo, "from-h2", 0)
	h2.Handle(context.Background(), r)

	// Write via original h — should NOT include k=v
	r2 := slog.NewRecord(time.Now(), slog.LevelInfo, "from-h", 0)
	h.Handle(context.Background(), r2)

	body := w.Body.String()
	events := strings.Split(strings.TrimSpace(body), "\n\n")
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}

	// First event (from h2) should have k=v
	if !strings.Contains(events[0], `"k":"v"`) {
		t.Error("h2 event should contain k=v")
	}
	// Second event (from h) should NOT have k=v
	if strings.Contains(events[1], `"k":"v"`) {
		t.Error("original handler event should not contain k=v")
	}
}

func TestSSEHandlerWithGroupPrefixes(t *testing.T) {
	h, w := newTestHandler()
	h2 := h.WithGroup("grp")

	r := slog.NewRecord(time.Now(), slog.LevelInfo, "test", 0)
	r.AddAttrs(slog.String("key", "val"))
	h2.Handle(context.Background(), r)

	body := w.Body.String()
	if !strings.Contains(body, `"grp.key"`) {
		t.Errorf("expected group-prefixed key, got: %s", body)
	}
}

func TestSSEHandlerNestedGroups(t *testing.T) {
	h, w := newTestHandler()
	h2 := h.WithGroup("a").WithGroup("b")

	r := slog.NewRecord(time.Now(), slog.LevelInfo, "test", 0)
	r.AddAttrs(slog.String("key", "val"))
	h2.Handle(context.Background(), r)

	body := w.Body.String()
	if !strings.Contains(body, `"a.b.key"`) {
		t.Errorf("expected nested group prefix, got: %s", body)
	}
}

func TestSSEHandlerMarshalError(t *testing.T) {
	h, w := newTestHandler()

	r := slog.NewRecord(time.Now(), slog.LevelInfo, "test", 0)
	r.AddAttrs(slog.Any("bad", func() {}))

	err := h.Handle(context.Background(), r)
	if err != nil {
		t.Fatalf("Handle should not return error: %v", err)
	}

	body := w.Body.String()
	if !strings.Contains(body, "marshal_error") {
		t.Errorf("expected marshal_error in output, got: %s", body)
	}
}

func TestSSEHandlerSendDone(t *testing.T) {
	h, w := newTestHandler()
	h.SendDone("the answer")

	body := w.Body.String()
	expected := `event: done` + "\n" + `data: {"answer":"the answer"}` + "\n\n"
	if body != expected {
		t.Errorf("got %q, want %q", body, expected)
	}
}

func TestSSEHandlerSendError(t *testing.T) {
	h, w := newTestHandler()
	h.SendError(fmt.Errorf("boom"))

	body := w.Body.String()
	expected := `event: error` + "\n" + `data: {"error":"boom"}` + "\n\n"
	if body != expected {
		t.Errorf("got %q, want %q", body, expected)
	}
}

func TestSSEHandlerConcurrentSafety(t *testing.T) {
	h, w := newTestHandler()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r := slog.NewRecord(time.Now(), slog.LevelInfo, fmt.Sprintf("msg-%d", i), 0)
			h.Handle(context.Background(), r)
		}(i)
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		h.SendDone("final")
	}()

	wg.Wait()

	body := w.Body.String()
	events := strings.Split(strings.TrimSpace(body), "\n\n")
	if len(events) != 11 {
		t.Errorf("expected 11 events, got %d", len(events))
	}

	// Each event should be well-formed
	for i, event := range events {
		lines := strings.Split(event, "\n")
		if len(lines) < 2 {
			t.Errorf("event %d has %d lines, want >= 2", i, len(lines))
			continue
		}
		if !strings.HasPrefix(lines[0], "event: ") {
			t.Errorf("event %d first line should start with 'event: ', got: %q", i, lines[0])
		}
		if !strings.HasPrefix(lines[1], "data: ") {
			t.Errorf("event %d second line should start with 'data: ', got: %q", i, lines[1])
		}
	}
}
