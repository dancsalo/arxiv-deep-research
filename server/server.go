package server

import (
	"embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"

	"github.com/dancsalo/arxiv-deep-research/internal/agentic"
)

//go:embed static/index.html
var staticFS embed.FS

type LoopFactory func(query string, logger *slog.Logger) (*agentic.AgenticLoop, error)

type Server struct {
	factory LoopFactory
	addr    string

	mu      sync.Mutex
	running bool
}

func NewServer(factory LoopFactory, addr string) *Server {
	return &Server{factory: factory, addr: addr}
}

func (s *Server) ListenAndServe() error {
	return http.ListenAndServe(s.addr, s.Handler())
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.handleIndex)
	mux.HandleFunc("POST /query", s.handleQuery)
	return mux
}

func (s *Server) handleIndex(w http.ResponseWriter, _ *http.Request) {
	data, err := staticFS.ReadFile("static/index.html")
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(data)
}

func (s *Server) handleQuery(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		http.Error(w, "a query is already running", http.StatusConflict)
		return
	}
	s.running = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
	}()

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req struct {
		Query string `json:"query"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if req.Query == "" {
		http.Error(w, "query is required", http.StatusBadRequest)
		return
	}

	sseHandler, sseErr := NewSSEHandler(w, slog.LevelInfo)
	if sseErr != nil {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	logger := slog.New(sseHandler)
	loop, err := s.factory(req.Query, logger)
	if err != nil {
		http.Error(w, fmt.Sprintf("loop construction failed: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.(http.Flusher).Flush()

	answer, err := loop.Run(r.Context(), req.Query)
	if err != nil {
		if r.Context().Err() != nil {
			return
		}
		sseHandler.SendError(err)
		return
	}

	sseHandler.SendDone(answer)
}
