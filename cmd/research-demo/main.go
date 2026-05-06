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
	model := flag.String("model", "", "model ID override")
	maxTurns := flag.Int("max-turns", 10, "maximum agentic loop turns")
	traceDir := flag.String("trace-dir", ".traces", "directory for trace files (empty to disable)")
	useBedrock := flag.Bool("bedrock", true, "use AWS Bedrock")
	flag.Parse()

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

	var modelID anthropic.Model
	if *model != "" {
		modelID = anthropic.Model(*model)
	} else if *useBedrock {
		modelID = "us.anthropic.claude-3-5-haiku-20241022-v1:0"
	} else {
		modelID = anthropic.ModelClaudeHaiku4_5
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	sessionID := fmt.Sprintf("demo-%d", time.Now().UnixMilli())
	traceCfg := tracing.Config{
		Dir:       *traceDir,
		SessionID: sessionID,
		Query:     *query,
		Model:     string(modelID),
		Logger:    logger,
	}
	hooks, recorder := tracing.NewTracingHooks(traceCfg)

	var client agentic.MessageClient = &sdkAdapter{client: &apiClient}
	if traceCfg.Enabled() {
		client = &tracing.TracedClient{Inner: client, Recorder: recorder}
	}

	httpClient := &http.Client{Timeout: 30 * time.Second}
	researchTools := research.NewResearchToolSet(httpClient)

	reg := registry.NewToolRegistry()
	registry.RegisterToolSets(reg, researchTools)
	reg.Register("finish_loop", agentic.BuildFinishTool(), func(_ context.Context, input json.RawMessage) (string, error) {
		return string(input), nil
	})

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

	loop := agentic.NewLoop(
		client,
		manager,
		reg,
		nil,
		agentic.LoopConfig{
			MaxTurns:        *maxTurns,
			MaxCostUSD:      0.50,
			Model:           modelID,
			SessionID:       sessionID,
			FinishTool:      "finish_loop",
			DefaultPriority: ctxmgr.PriorityCore,
			Hooks:           hooks,
			Logger:          logger,
		},
		systemBlocks,
	)

	start := time.Now()
	result, err := loop.Run(context.Background(), *query)
	elapsed := time.Since(start)

	if err != nil {
		recorder.SetError(err)
		if flushErr := recorder.Flush(); flushErr != nil {
			logger.Error("failed to flush trace", "err", flushErr)
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if err := recorder.Flush(); err != nil {
		logger.Error("failed to flush trace", "err", err)
	}

	fmt.Println("=== Research Summary ===")
	fmt.Println(result)
	fmt.Println()
	fmt.Printf("--- Stats ---\n")
	fmt.Printf("Elapsed: %s\n", elapsed.Round(time.Millisecond))
	fmt.Printf("Cost:    $%.4f\n", loop.TotalCost())
	if traceCfg.Enabled() {
		fmt.Printf("Trace:   %s/%s.json\n", traceCfg.Dir, sessionID)
	}
}
