package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/bedrock"
	"github.com/anthropics/anthropic-sdk-go/option"

	contextmanager "github.com/dancsalo/arxiv-deep-research"
	"github.com/dancsalo/arxiv-deep-research/server"
)

type sdkAdapter struct {
	client *anthropic.Client
}

func (a *sdkAdapter) CreateMessage(ctx context.Context, params anthropic.MessageNewParams) (*anthropic.Message, error) {
	return a.client.Messages.New(ctx, params)
}

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	useBedrock := flag.Bool("bedrock", true, "use AWS Bedrock")
	model := flag.String("model", "", "model ID override")
	flag.Parse()

	ctx := context.Background()
	var opts []option.RequestOption
	if *useBedrock {
		opts = append(opts, bedrock.WithLoadDefaultConfig(ctx))
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

	factory := func(query string, logger *slog.Logger) (*contextmanager.AgenticLoop, error) {
		systemBlocks := []anthropic.TextBlockParam{
			{Text: "You are a helpful research assistant.", Type: "text"},
		}

		initialMsg := anthropic.NewUserMessage(anthropic.NewTextBlock(query))
		manager := contextmanager.NewContextManager(contextmanager.ContextManagerConfig{
			Estimator: contextmanager.NewTokenEstimator(nil, "", false),
			Budget: &contextmanager.ContextBudget{
				ModelContextLimit: 200000,
				MaxOutputTokens:   8192,
				SafetyMargin:      2000,
			},
			System:  systemBlocks,
			NowFunc: time.Now,
		}, initialMsg)

		registry := contextmanager.NewToolRegistry()

		registry.Register("finish", contextmanager.BuildFinishTool(),
			func(_ context.Context, input json.RawMessage) (string, error) {
				return string(input), nil
			})

		loop := contextmanager.NewAgenticLoop(
			&sdkAdapter{client: &apiClient},
			manager,
			registry,
			nil,
			contextmanager.AgenticLoopConfig{
				MaxTurns:        20,
				MaxCostUSD:      0.50,
				Model:           modelID,
				SessionID:       fmt.Sprintf("web-%d", time.Now().UnixMilli()),
				FinishTool:      "finish",
				DefaultPriority: contextmanager.PriorityCore,
				Logger:          logger,
			},
			systemBlocks,
		)
		return loop, nil
	}

	srv := server.NewServer(factory, *addr)
	fmt.Printf("Listening on %s\n", *addr)
	if err := srv.ListenAndServe(); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}
