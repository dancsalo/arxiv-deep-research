package server

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go"

	contextmanager "github.com/dancsalo/arxiv-deep-research"
)

type scriptedClient struct {
	responses []*anthropic.Message
	callIdx   int
}

func (s *scriptedClient) CreateMessage(_ context.Context, _ anthropic.MessageNewParams) (*anthropic.Message, error) {
	if s.callIdx >= len(s.responses) {
		return nil, fmt.Errorf("no more scripted responses")
	}
	resp := s.responses[s.callIdx]
	s.callIdx++
	return resp, nil
}

func testFactory(answer string) LoopFactory {
	return func(query string, logger *slog.Logger) (*contextmanager.AgenticLoop, error) {
		client := &scriptedClient{
			responses: []*anthropic.Message{
				{
					Content:    []anthropic.ContentBlockUnion{{Type: "text", Text: answer}},
					StopReason: "end_turn",
					Usage:      anthropic.Usage{InputTokens: 10, OutputTokens: 5},
				},
			},
		}
		estimator := contextmanager.NewTokenEstimator(nil, "", false)
		budget := &contextmanager.ContextBudget{
			ModelContextLimit: 200000,
			MaxOutputTokens:   8192,
			SafetyMargin:      2000,
		}
		initial := anthropic.NewUserMessage(anthropic.NewTextBlock(query))
		manager := contextmanager.NewContextManager(contextmanager.ContextManagerConfig{
			Estimator: estimator,
			Budget:    budget,
		}, initial)
		registry := contextmanager.NewToolRegistry()

		return contextmanager.NewAgenticLoop(client, manager, registry, nil, contextmanager.AgenticLoopConfig{
			MaxTurns:   5,
			MaxCostUSD: 1.0,
			Model:      anthropic.ModelClaudeHaiku4_5,
			Logger:     logger,
		}, nil), nil
	}
}

func errorFactory(err error) LoopFactory {
	return func(_ string, _ *slog.Logger) (*contextmanager.AgenticLoop, error) {
		return nil, err
	}
}

func blockingFactory(started chan struct{}, unblock chan struct{}) LoopFactory {
	var once sync.Once
	return func(query string, logger *slog.Logger) (*contextmanager.AgenticLoop, error) {
		if started != nil {
			once.Do(func() { close(started) })
		}
		<-unblock
		return testFactory("done")(query, logger)
	}
}

func TestGetIndexServesHTML(t *testing.T) {
	srv := NewServer(testFactory("test"), ":0")
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET / error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "<title>Agentic Loop</title>") {
		t.Error("body should contain <title>Agentic Loop</title>")
	}
}

func TestPostQueryMissingBody(t *testing.T) {
	srv := NewServer(testFactory("test"), ":0")
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/query", "application/json", nil)
	if err != nil {
		t.Fatalf("POST error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 400 {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestPostQueryEmptyQuery(t *testing.T) {
	srv := NewServer(testFactory("test"), ":0")
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	body := bytes.NewBufferString(`{"query": ""}`)
	resp, err := http.Post(ts.URL+"/query", "application/json", body)
	if err != nil {
		t.Fatalf("POST error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 400 {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestPostQueryOversizedBody(t *testing.T) {
	srv := NewServer(testFactory("test"), ":0")
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	big := strings.Repeat("x", 2<<20) // 2MB
	body := bytes.NewBufferString(fmt.Sprintf(`{"query": "%s"}`, big))
	resp, err := http.Post(ts.URL+"/query", "application/json", body)
	if err != nil {
		t.Fatalf("POST error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 400 {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestPostQueryFactoryError(t *testing.T) {
	srv := NewServer(errorFactory(fmt.Errorf("db down")), ":0")
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	body := bytes.NewBufferString(`{"query": "test"}`)
	resp, err := http.Post(ts.URL+"/query", "application/json", body)
	if err != nil {
		t.Fatalf("POST error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 500 {
		t.Errorf("status = %d, want 500", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if strings.Contains(ct, "text/event-stream") {
		t.Error("factory error should not return SSE content type")
	}
}

func TestPostQuerySuccess(t *testing.T) {
	srv := NewServer(testFactory("the answer"), ":0")
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	body := bytes.NewBufferString(`{"query": "test"}`)
	resp, err := http.Post(ts.URL+"/query", "application/json", body)
	if err != nil {
		t.Fatalf("POST error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/event-stream") {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}

	respBody, _ := io.ReadAll(resp.Body)
	sseBody := string(respBody)

	if !strings.Contains(sseBody, "event: log") {
		t.Error("response should contain event: log")
	}
	if !strings.Contains(sseBody, "event: done") {
		t.Error("response should contain event: done")
	}
	if !strings.Contains(sseBody, "the answer") {
		t.Error("response should contain the answer")
	}
}

func TestSingleFlightRejects(t *testing.T) {
	started := make(chan struct{})
	unblock := make(chan struct{})

	srv := NewServer(blockingFactory(started, unblock), ":0")
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// Start first request (will block in factory)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		body := bytes.NewBufferString(`{"query": "first"}`)
		resp, err := http.Post(ts.URL+"/query", "application/json", body)
		if err != nil {
			return
		}
		defer resp.Body.Close()
		io.ReadAll(resp.Body)
	}()

	// Wait for first request to start
	<-started

	// Second request should get 409
	body := bytes.NewBufferString(`{"query": "second"}`)
	resp, err := http.Post(ts.URL+"/query", "application/json", body)
	if err != nil {
		t.Fatalf("second POST error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 409 {
		t.Errorf("second request status = %d, want 409", resp.StatusCode)
	}

	// Unblock and wait for first request to complete
	close(unblock)
	wg.Wait()

	// Third request should succeed (slot freed)
	body3 := bytes.NewBufferString(`{"query": "third"}`)
	resp3, err := http.Post(ts.URL+"/query", "application/json", body3)
	if err != nil {
		t.Fatalf("third POST error: %v", err)
	}
	defer resp3.Body.Close()
	io.ReadAll(resp3.Body)

	if resp3.StatusCode != 200 {
		t.Errorf("third request status = %d, want 200", resp3.StatusCode)
	}
}

func TestClientDisconnectCancels(t *testing.T) {
	cancelled := make(chan struct{})

	factory := func(query string, logger *slog.Logger) (*contextmanager.AgenticLoop, error) {
		// Build a loop that blocks until context is cancelled
		client := &blockingClient{cancelled: cancelled}
		estimator := contextmanager.NewTokenEstimator(nil, "", false)
		budget := &contextmanager.ContextBudget{
			ModelContextLimit: 200000,
			MaxOutputTokens:   8192,
			SafetyMargin:      2000,
		}
		initial := anthropic.NewUserMessage(anthropic.NewTextBlock(query))
		manager := contextmanager.NewContextManager(contextmanager.ContextManagerConfig{
			Estimator: estimator,
			Budget:    budget,
		}, initial)
		registry := contextmanager.NewToolRegistry()

		return contextmanager.NewAgenticLoop(client, manager, registry, nil, contextmanager.AgenticLoopConfig{
			MaxTurns:   5,
			MaxCostUSD: 1.0,
			Model:      anthropic.ModelClaudeHaiku4_5,
			Logger:     logger,
		}, nil), nil
	}

	srv := NewServer(factory, ":0")
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, "POST", ts.URL+"/query", bytes.NewBufferString(`{"query": "test"}`))
	req.Header.Set("Content-Type", "application/json")

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	resp, err := http.DefaultClient.Do(req)
	if err == nil {
		resp.Body.Close()
	}

	select {
	case <-cancelled:
	case <-time.After(2 * time.Second):
		t.Error("expected cancellation to propagate to loop")
	}
}

type blockingClient struct {
	cancelled chan struct{}
}

func (b *blockingClient) CreateMessage(ctx context.Context, _ anthropic.MessageNewParams) (*anthropic.Message, error) {
	<-ctx.Done()
	close(b.cancelled)
	return nil, ctx.Err()
}

func TestHandlerRoutes(t *testing.T) {
	srv := NewServer(testFactory("test"), ":0")
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// GET / serves HTML (catch-all pattern)
	resp, err := http.Get(ts.URL + "/anything")
	if err != nil {
		t.Fatalf("GET error: %v", err)
	}
	resp.Body.Close()
	// GET / is a catch-all in Go 1.22+ ServeMux, serves HTML for all unmatched GETs
	if resp.StatusCode != 200 {
		t.Errorf("GET /anything status = %d, want 200 (served by catch-all)", resp.StatusCode)
	}

	// POST to / should get 405 (only GET is registered for /)
	body := bytes.NewBufferString(`{"query": "test"}`)
	resp2, err := http.Post(ts.URL+"/", "application/json", body)
	if err != nil {
		t.Fatalf("POST / error: %v", err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != 405 {
		t.Errorf("POST / status = %d, want 405", resp2.StatusCode)
	}
}
