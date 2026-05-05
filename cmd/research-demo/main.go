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
	"github.com/anthropics/anthropic-sdk-go/bedrock"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/dancsalo/arxiv-deep-research/internal/agentic"
	"github.com/dancsalo/arxiv-deep-research/internal/ctxmgr"
	"github.com/dancsalo/arxiv-deep-research/internal/registry"
	"github.com/dancsalo/arxiv-deep-research/internal/tracing"
	"github.com/dancsalo/arxiv-deep-research/tools/research"
)

type sdkAdapter struct {
	client *anthropic.Client
}

func (a *sdkAdapter) CreateMessage(ctx context.Context, params anthropic.MessageNewParams) (*anthropic.Message, error) {
	return a.client.Messages.New(ctx, params)
}

func main() {
	query := flag.String("query", "retrieval augmented generation", "research query")
	model := flag.String("model", "us.anthropic.claude-3-5-haiku-20241022-v1:0", "model ID")
	maxTurns := flag.Int("max-turns", 10, "maximum agentic loop turns")
	useBedrock := flag.Bool("bedrock", true, "use AWS Bedrock")
	flag.Parse()

	start := time.Now()
	ctx := context.Background()

	var opts []option.RequestOption
	if *useBedrock {
		opts = append(opts, bedrock.WithLoadDefaultConfig(ctx))
	} else {
		apiKey := os.Getenv("ANTHROPIC_API_KEY")
		if apiKey == "" {
			fmt.Fprintln(os.Stderr, "error: ANTHROPIC_API_KEY environment variable is required")
			os.Exit(1)
		}
		opts = append(opts, option.WithAPIKey(apiKey))
	}
	apiClient := anthropic.NewClient(opts...)

	tp, shutdownTracer := tracing.InitTracer(ctx, tracing.Config{
		Endpoint:  os.Getenv("LANGFUSE_OTLP_ENDPOINT"),
		PublicKey: os.Getenv("LANGFUSE_PUBLIC_KEY"),
		SecretKey: os.Getenv("LANGFUSE_SECRET_KEY"),
	})
	defer shutdownTracer(ctx)

	tracingHooks, tracingState := tracing.NewTracingHooks(tp)

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

	var client agentic.MessageClient = &sdkAdapter{client: &apiClient}
	client = &tracing.TracedClient{Inner: client, Hooks: tracingState}

	loop := agentic.NewLoop(
		client,
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
			Hooks:           tracingHooks,
			Logger:          logger,
		},
		systemBlocks,
	)

	start = time.Now()
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
