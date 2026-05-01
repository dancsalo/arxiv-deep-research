package contextmanager

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/anthropics/anthropic-sdk-go"
)

type ContentType int

const (
	ContentProse ContentType = iota
	ContentJSON
	ContentCode
	ContentMixed
)

var defaultRatios = map[ContentType]float64{
	ContentProse: 4.0,
	ContentJSON:  3.0,
	ContentCode:  3.5,
	ContentMixed: 3.5,
}

type TokenCounterClient interface {
	CountTokens(ctx context.Context, params anthropic.MessageCountTokensParams) (*anthropic.MessageTokensCount, error)
}

type TokenEstimator struct {
	client    TokenCounterClient
	model     anthropic.Model
	ratios    map[ContentType]float64
	hasDirect bool
}

func NewTokenEstimator(client TokenCounterClient, model anthropic.Model, hasDirect bool) *TokenEstimator {
	ratios := make(map[ContentType]float64, len(defaultRatios))
	for k, v := range defaultRatios {
		ratios[k] = v
	}
	return &TokenEstimator{
		client:    client,
		model:     model,
		ratios:    ratios,
		hasDirect: hasDirect,
	}
}

func (e *TokenEstimator) EstimateFast(text string, ct ContentType) int {
	ratio, ok := e.ratios[ct]
	if !ok {
		ratio = 3.5
	}
	return int(float64(utf8.RuneCountInString(text)) / ratio)
}

func (e *TokenEstimator) CountExact(
	ctx context.Context,
	messages []anthropic.MessageParam,
	system []anthropic.TextBlockParam,
	tools []anthropic.MessageCountTokensToolUnionParam,
) (int, error) {
	if !e.hasDirect {
		return 0, fmt.Errorf("exact counting unavailable on Bedrock")
	}
	params := anthropic.MessageCountTokensParams{
		Model:    e.model,
		Messages: messages,
		Tools:    tools,
	}
	if len(system) > 0 {
		params.System.OfTextBlockArray = system
	}
	resp, err := e.client.CountTokens(ctx, params)
	if err != nil {
		return 0, fmt.Errorf("count_tokens failed: %w", err)
	}
	return int(resp.InputTokens), nil
}

func (e *TokenEstimator) Calibrate(text string, actualTokens int, ct ContentType) {
	if actualTokens == 0 {
		return
	}
	observed := float64(utf8.RuneCountInString(text)) / float64(actualTokens)
	old := e.ratios[ct]
	e.ratios[ct] = 0.8*old + 0.2*observed
}

func classifyContent(text string) ContentType {
	totalLen := utf8.RuneCountInString(text)
	if totalLen == 0 {
		return ContentProse
	}

	braceCount := strings.Count(text, "{") + strings.Count(text, "}")
	bracketCount := strings.Count(text, "[") + strings.Count(text, "]")
	quoteColonCount := strings.Count(text, `":`)
	jsonScore := float64(braceCount+bracketCount+quoteColonCount) / float64(totalLen)

	codeIndicators := strings.Count(text, "func ") +
		strings.Count(text, "if ") +
		strings.Count(text, "for ") +
		strings.Count(text, "return ") +
		strings.Count(text, ":=") +
		strings.Count(text, "import ") +
		strings.Count(text, "def ") +
		strings.Count(text, "class ")

	// JSON is structurally distinctive — check it first
	if jsonScore > 0.05 {
		return ContentJSON
	}

	if codeIndicators >= 2 {
		return ContentCode
	}

	return ContentProse
}
