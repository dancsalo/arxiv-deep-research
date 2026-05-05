package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/dancsalo/arxiv-deep-research/internal/agentic"
	"github.com/dancsalo/arxiv-deep-research/internal/ctxmgr"
	"github.com/dancsalo/arxiv-deep-research/internal/registry"
	"github.com/dancsalo/arxiv-deep-research/tools/research"
)

type sdkAdapter struct {
	client *anthropic.Client
}

func (a *sdkAdapter) CreateMessage(ctx context.Context, params anthropic.MessageNewParams) (*anthropic.Message, error) {
	return a.client.Messages.New(ctx, params)
}

func main() {
	query := flag.String("query", "retrieval augmented generation", "research topic")
	model := flag.String("model", string(anthropic.ModelClaudeHaiku4_5), "model ID")
	maxTurns := flag.Int("max-turns", 10, "maximum turns")
	flag.Parse()

	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		fmt.Fprintf(os.Stderr, "Error: ANTHROPIC_API_KEY environment variable is required\n")
		os.Exit(1)
	}

	httpClient := &http.Client{Timeout: 30 * time.Second}
	toolSet := research.NewResearchToolSet(httpClient)

	reg := registry.NewToolRegistry()
	registry.RegisterToolSets(reg, toolSet)
	reg.Register("finish_loop", agentic.BuildFinishTool(), func(_ context.Context, input json.RawMessage) (string, error) {
		return string(input), nil
	})

	tse := ctxmgr.NewToolSizeEstimator()
	for name, fn := range research.ResearchToolEstimators() {
		tse.RegisterTool(name, fn)
	}

	systemBlocks := []anthropic.TextBlockParam{
		{Text: "You are a research assistant. Search arXiv for preprints and OpenAlex for published works to find relevant papers on the given topic. Search both sources, then synthesize a summary with the most important findings. Include paper titles and authors in your summary.\nDo not call search_arxiv more than once.\nCall finish_loop with your final markdown summary when done.", Type: "text"},
	}

	initialMsg := anthropic.NewUserMessage(anthropic.NewTextBlock(
		fmt.Sprintf("Find and summarize recent research on: %s", *query),
	))

	manager := ctxmgr.NewContextManager(ctxmgr.ContextManagerConfig{
		Estimator: ctxmgr.NewTokenEstimator(nil, "", false),
		Budget: &ctxmgr.ContextBudget{
			ModelContextLimit: 200000,
			MaxOutputTokens:   8192,
			SafetyMargin:      2000,
		},
		System:  systemBlocks,
		NowFunc: time.Now,
	}, initialMsg)
	manager.SetToolSizeEstimator(tse)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	apiClient := anthropic.NewClient(option.WithAPIKey(apiKey))
	loop := agentic.NewLoop(
		&sdkAdapter{client: &apiClient},
		manager,
		reg,
		nil,
		agentic.LoopConfig{
			MaxTurns:        *maxTurns,
			MaxCostUSD:      0.50,
			Model:           anthropic.Model(*model),
			SessionID:       fmt.Sprintf("demo-%d", time.Now().UnixMilli()),
			FinishTool:      "finish_loop",
			DefaultPriority: ctxmgr.PriorityCore,
			Logger:          logger,
		},
		systemBlocks,
	)

	start := time.Now()
	result, err := loop.Run(context.Background(), *query)
	elapsed := time.Since(start)

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("=== Research Summary ===")
	fmt.Println(result)
	fmt.Println()
	fmt.Printf("--- Stats ---\n")
	fmt.Printf("Elapsed: %s\n", elapsed.Round(time.Millisecond))
	fmt.Printf("Cost:    $%.4f\n", loop.TotalCost())
}
