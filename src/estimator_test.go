package contextmanager

import (
	"context"
	"fmt"
	"testing"
	"unicode/utf8"

	"github.com/anthropics/anthropic-sdk-go"
)

type mockCounterClient struct {
	result *anthropic.MessageTokensCount
	err    error
	called bool
}

func (m *mockCounterClient) CountTokens(_ context.Context, _ anthropic.MessageCountTokensParams) (*anthropic.MessageTokensCount, error) {
	m.called = true
	return m.result, m.err
}

func TestEstimateFastProse(t *testing.T) {
	e := NewTokenEstimator(nil, "", false)
	text := "Hello world, this is a test."
	got := e.EstimateFast(text, ContentProse)
	want := int(float64(utf8.RuneCountInString(text)) / 4.0)
	if got != want {
		t.Errorf("EstimateFast prose: got %d, want %d", got, want)
	}
}

func TestEstimateFastJSON(t *testing.T) {
	e := NewTokenEstimator(nil, "", false)
	text := `{"key":"value","num":42}`
	got := e.EstimateFast(text, ContentJSON)
	want := int(float64(utf8.RuneCountInString(text)) / 3.0)
	if got != want {
		t.Errorf("EstimateFast JSON: got %d, want %d", got, want)
	}
}

func TestEstimateFastCode(t *testing.T) {
	e := NewTokenEstimator(nil, "", false)
	text := "func main() { fmt.Println() }"
	got := e.EstimateFast(text, ContentCode)
	want := int(float64(utf8.RuneCountInString(text)) / 3.5)
	if got != want {
		t.Errorf("EstimateFast code: got %d, want %d", got, want)
	}
}

func TestEstimateFastUnknownContentType(t *testing.T) {
	e := NewTokenEstimator(nil, "", false)
	text := "test"
	got := e.EstimateFast(text, ContentType(99))
	want := int(float64(utf8.RuneCountInString(text)) / 3.5)
	if got != want {
		t.Errorf("EstimateFast unknown: got %d, want %d", got, want)
	}
}

func TestEstimateFastEmptyString(t *testing.T) {
	e := NewTokenEstimator(nil, "", false)
	got := e.EstimateFast("", ContentProse)
	if got != 0 {
		t.Errorf("EstimateFast empty: got %d, want 0", got)
	}
}

func TestEstimateFastUnicode(t *testing.T) {
	e := NewTokenEstimator(nil, "", false)
	text := "αβγδ"
	got := e.EstimateFast(text, ContentProse)
	want := int(float64(4) / 4.0) // 4 runes, not 8 bytes
	if got != want {
		t.Errorf("EstimateFast unicode: got %d, want %d (rune count: %d, byte len: %d)",
			got, want, utf8.RuneCountInString(text), len(text))
	}
}

func TestCountExactNoDirect(t *testing.T) {
	e := NewTokenEstimator(nil, "", false)
	_, err := e.CountExact(context.Background(), nil, nil, nil)
	if err == nil {
		t.Fatal("expected error for Bedrock client")
	}
	if got := err.Error(); got != "exact counting unavailable on Bedrock" {
		t.Errorf("unexpected error: %s", got)
	}
}

func TestCountExactSuccess(t *testing.T) {
	mock := &mockCounterClient{
		result: &anthropic.MessageTokensCount{InputTokens: 150},
	}
	e := NewTokenEstimator(mock, anthropic.ModelClaudeHaiku4_5, true)

	msgs := []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock("hello"))}
	got, err := e.CountExact(context.Background(), msgs, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 150 {
		t.Errorf("CountExact: got %d, want 150", got)
	}
	if !mock.called {
		t.Error("expected API to be called")
	}
}

func TestCountExactAPIError(t *testing.T) {
	mock := &mockCounterClient{
		err: fmt.Errorf("rate limited"),
	}
	e := NewTokenEstimator(mock, anthropic.ModelClaudeHaiku4_5, true)

	_, err := e.CountExact(context.Background(), nil, nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if got := err.Error(); got != "count_tokens failed: rate limited" {
		t.Errorf("unexpected error: %s", got)
	}
}

func TestCalibrateUpdatesRatio(t *testing.T) {
	e := NewTokenEstimator(nil, "", false)
	// "abcdefghij" = 10 runes, 5 tokens → observed ratio = 2.0
	// ContentProse old ratio = 4.0
	// new = 0.8*4.0 + 0.2*2.0 = 3.6
	e.Calibrate("abcdefghij", 5, ContentProse)
	got := e.ratios[ContentProse]
	want := 3.6
	if got < want-0.01 || got > want+0.01 {
		t.Errorf("Calibrate: got ratio %f, want %f", got, want)
	}
}

func TestCalibrateZeroTokensNoop(t *testing.T) {
	e := NewTokenEstimator(nil, "", false)
	before := e.ratios[ContentProse]
	e.Calibrate("text", 0, ContentProse)
	after := e.ratios[ContentProse]
	if before != after {
		t.Errorf("Calibrate with 0 tokens changed ratio: %f → %f", before, after)
	}
}

func TestCalibrateConverges(t *testing.T) {
	e := NewTokenEstimator(nil, "", false)
	for i := 0; i < 20; i++ {
		e.Calibrate("abcdefghijklmnopqrstuvwxyzabcd", 10, ContentProse) // 30 runes / 10 tokens = 3.0
	}
	got := e.ratios[ContentProse]
	if got < 2.7 || got > 3.3 {
		t.Errorf("Calibrate convergence: got %f, want ~3.0", got)
	}
}

func TestClassifyContent(t *testing.T) {
	tests := []struct {
		name string
		text string
		want ContentType
	}{
		{"empty", "", ContentProse},
		{"prose", "The quick brown fox jumps over the lazy dog. This is a normal sentence with enough words to be clearly prose.", ContentProse},
		{"json", `{"key": "value", "arr": [1, 2, 3], "nested": {"a": "b"}}`, ContentJSON},
		{"code", "func main() {\n\tfmt.Println(\"hello world\")\n\tfor i := 0; i < 10; i++ {\n\t\tfmt.Println(i)\n\t}\n\tif err := run(); err != nil {\n\t\treturn err\n\t}\n\tresult := compute()\n\tfmt.Println(result)\n}", ContentCode},
		// JSON containing code-like strings should still be classified as JSON
		{"json_with_code_keywords", `{"description": "if x > 0, for each item, return result", "if ": true, "for ": "loop"}`, ContentJSON},
		// Single code indicator should NOT trigger code classification
		{"one_code_indicator", "This is prose with one func call mentioned.", ContentProse},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyContent(tt.text)
			if got != tt.want {
				t.Errorf("classifyContent(%q): got %d, want %d", tt.name, got, tt.want)
			}
		})
	}
}
