package contextmanager

import (
	"context"
	"fmt"
)

type GuardrailDecision struct {
	Proceed      bool
	ModifiedArgs map[string]any
	Reason       string
	Compacted    []int
}

func (m *ContextManager) PreToolGuardrail(
	ctx context.Context,
	toolName string,
	args map[string]any,
) (GuardrailDecision, error) {
	estimated := m.EstimateToolResult(toolName, args)
	currentTokens, err := m.GetTokenCount(ctx)
	if err != nil {
		return GuardrailDecision{}, err
	}
	remaining := m.budget.Remaining(currentTokens)

	if estimated < remaining-m.budget.SafetyMargin {
		return GuardrailDecision{Proceed: true}, nil
	}

	if reduced, ok := reduceToolArgs(toolName, args, remaining); ok {
		newEst := m.EstimateToolResult(toolName, reduced)
		if newEst < remaining-m.budget.SafetyMargin {
			return GuardrailDecision{
				Proceed:      true,
				ModifiedArgs: reduced,
				Reason:       fmt.Sprintf("reduced args to fit: %d tokens estimated", newEst),
			}, nil
		}
	}

	needed := m.SpaceNeeded(estimated)
	compactedIdxs, err := m.autoCompact(ctx, needed)
	if err != nil {
		return GuardrailDecision{
			Proceed: false,
			Reason:  fmt.Sprintf("compaction failed: %v", err),
		}, nil
	}

	currentTokens, _ = m.GetTokenCount(ctx)
	remaining = m.budget.Remaining(currentTokens)
	if estimated < remaining {
		return GuardrailDecision{
			Proceed:   true,
			Compacted: compactedIdxs,
			Reason:    fmt.Sprintf("compacted %d turns to free space", len(compactedIdxs)),
		}, nil
	}

	return GuardrailDecision{
		Proceed: false,
		Reason: fmt.Sprintf(
			"need ~%d tokens, only %d available after compaction",
			estimated, remaining,
		),
	}, nil
}

func (m *ContextManager) PostToolCheck(
	ctx context.Context,
	toolName string,
	resultText string,
) error {
	actualTokens := m.estimator.EstimateFast(resultText, ContentJSON)
	if m.toolSizeEst != nil {
		m.toolSizeEst.Record(toolName, actualTokens)
	}

	currentTokens, err := m.GetTokenCount(ctx)
	if err != nil {
		return err
	}
	if m.budget.Remaining(currentTokens) < m.budget.SafetyMargin {
		_, err := m.autoCompact(ctx, m.budget.SafetyMargin*2)
		if err != nil {
			return fmt.Errorf("post-tool compaction failed: %w", err)
		}
	}
	return nil
}

func (m *ContextManager) PreAPICheck(ctx context.Context) error {
	currentTokens, err := m.GetTokenCount(ctx)
	if err != nil {
		return err
	}
	if currentTokens > m.budget.Available() {
		target := int(float64(m.budget.Available()) * 0.8)
		_, err := m.autoCompact(ctx, currentTokens-target)
		if err != nil {
			return fmt.Errorf("pre-API compaction failed, context too large (%d tokens): %w",
				currentTokens, err)
		}
	}
	return nil
}

func (m *ContextManager) EstimateToolResult(toolName string, args map[string]any) int {
	if m.toolSizeEst != nil {
		return m.toolSizeEst.Estimate(toolName, args)
	}
	return 5000
}

func (m *ContextManager) SetToolSizeEstimator(est *ToolSizeEstimator) {
	m.toolSizeEst = est
}

func reduceToolArgs(toolName string, args map[string]any, remainingTokens int) (map[string]any, bool) {
	reduced := copyArgs(args)
	switch toolName {
	case "fetch_page_text":
		maxChars := int(float64(remainingTokens) * 0.5 * 3.5)
		if maxChars < 1000 {
			return nil, false
		}
		reduced["max_chars"] = maxChars
		return reduced, true
	case "search_arxiv":
		if v, ok := args["max_results"]; ok {
			switch n := v.(type) {
			case float64:
				if n > 5 {
					reduced["max_results"] = float64(5)
					return reduced, true
				}
			case int:
				if n > 5 {
					reduced["max_results"] = 5
					return reduced, true
				}
			}
		}
	}
	return nil, false
}

func copyArgs(args map[string]any) map[string]any {
	out := make(map[string]any, len(args))
	for k, v := range args {
		out[k] = v
	}
	return out
}
