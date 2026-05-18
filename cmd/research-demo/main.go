package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
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

func loadPrompt(variant string) (string, string, error) {
	// Only variant B is supported
	filename := "variant-b-metacognitive.txt"
	if variant != "B" {
		return "", "", fmt.Errorf("only variant B is supported")
	}

	// Construct path relative to this source file
	exePath, err := os.Executable()
	if err != nil {
		return "", "", fmt.Errorf("failed to get executable path: %w", err)
	}
	exeDir := filepath.Dir(exePath)

	// Try relative to executable first (for installed binary)
	promptPath := filepath.Join(exeDir, "prompts", filename)
	content, err := os.ReadFile(promptPath)
	if err != nil {
		// Fall back to relative to source (for development)
		promptPath = filepath.Join("cmd", "research-demo", "prompts", filename)
		content, err = os.ReadFile(promptPath)
		if err != nil {
			return "", "", fmt.Errorf("failed to read prompt file: %w", err)
		}
	}

	// Compute SHA256 hash, take first 8 hex chars
	hash := sha256.Sum256(content)
	hashStr := hex.EncodeToString(hash[:])[:8]

	return string(content), hashStr, nil
}

func main() {
	query := flag.String("query", "retrieval augmented generation", "research query")
	model := flag.String("model", "", "model ID override")
	maxTurns := flag.Int("max-turns", 3, "maximum agentic loop turns")
	traceDir := flag.String("trace-dir", ".traces", "directory for trace files (empty to disable)")
	useBedrock := flag.Bool("bedrock", true, "use AWS Bedrock")
	promptVariant := flag.String("prompt-variant", "B", "prompt variant (only B supported)")
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
		modelID = "us.anthropic.claude-sonnet-4-5-20250929-v1:0"
	} else {
		modelID = anthropic.ModelClaudeSonnet4_6
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	// Load prompt based on variant
	promptText, promptHash, err := loadPrompt(*promptVariant)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading prompt: %v\n", err)
		os.Exit(1)
	}

	sessionID := fmt.Sprintf("demo-%d", time.Now().UnixMilli())
	traceCfg := tracing.Config{
		Dir:           *traceDir,
		SessionID:     sessionID,
		Query:         *query,
		Model:         string(modelID),
		PromptVariant: *promptVariant,
		PromptHash:    promptHash,
		Logger:        logger,
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
		{Text: promptText, Type: "text"},
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
			MaxCostUSD:      1.00,
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

		// Print partial results if available
		if result != "" {
			fmt.Println("=== Partial Research Summary ===")
			fmt.Println("(Research incomplete due to error)")
			fmt.Println()
			fmt.Println(result)
			fmt.Println()
		}

		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		fmt.Printf("\n--- Stats ---\n")
		fmt.Printf("Elapsed: %s\n", elapsed.Round(time.Millisecond))
		fmt.Printf("Cost:    $%.4f\n", loop.TotalCost())
		if traceCfg.Enabled() {
			fmt.Printf("Trace:   %s/%s.json\n", traceCfg.Dir, sessionID)
		}
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
