package research

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type mockHTTPClient struct {
	doFunc func(req *http.Request) (*http.Response, error)
}

func (m *mockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	if m.doFunc != nil {
		return m.doFunc(req)
	}
	return nil, nil
}

func TestFetchArxivText_Success(t *testing.T) {
	// Mock HTML content
	htmlContent := `
		<!DOCTYPE html>
		<html>
		<head><title>Test Paper</title></head>
		<body>
			<article>
				<h1>Deep Learning for Computer Vision</h1>
				<p>This paper presents a novel approach to image classification using deep neural networks.</p>
				<p>Our experiments show significant improvements over existing methods.</p>
			</article>
		</body>
		</html>
	`

	mockClient := &mockHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			// Verify URL format
			expectedURL := "https://arxiv.org/html/2301.00001"
			if req.URL.String() != expectedURL {
				t.Errorf("unexpected URL: got %s, want %s", req.URL.String(), expectedURL)
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString(htmlContent)),
				Header:     make(http.Header),
			}, nil
		},
	}

	ts := newResearchToolSetWithBase(mockClient, "")

	input := `{"arxiv_id": "2301.00001", "max_length": 25000}`
	result, err := ts.handleFetchArxivText(context.Background(), json.RawMessage(input))

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed ArxivTextResult
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	// Verify result fields
	if parsed.ArxivID != "2301.00001" {
		t.Errorf("arxiv_id: got %s, want 2301.00001", parsed.ArxivID)
	}

	if parsed.TextContent == "" {
		t.Error("text_content is empty")
	}

	if parsed.Truncated {
		t.Error("truncated should be false for short content")
	}

	if parsed.Error != "" {
		t.Errorf("unexpected error field: %s", parsed.Error)
	}

	// Verify content extraction worked
	if !strings.Contains(parsed.TextContent, "novel approach") || !strings.Contains(parsed.TextContent, "significant improvements") {
		t.Errorf("text_content missing expected keywords: %s", parsed.TextContent)
	}
}
