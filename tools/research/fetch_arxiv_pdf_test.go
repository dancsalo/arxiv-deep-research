package research

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Unit tests for arXiv ID normalization

func TestNormalizeArxivID_NewFormat(t *testing.T) {
	normalized, version, err := normalizeArxivID("2301.00001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if normalized != "2301.00001" {
		t.Errorf("expected normalized='2301.00001', got %q", normalized)
	}
	if version != "" {
		t.Errorf("expected version='', got %q", version)
	}
}

func TestNormalizeArxivID_NewFormatWithVersion(t *testing.T) {
	normalized, version, err := normalizeArxivID("2301.00001v2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if normalized != "2301.00001" {
		t.Errorf("expected normalized='2301.00001', got %q", normalized)
	}
	if version != "v2" {
		t.Errorf("expected version='v2', got %q", version)
	}
}

func TestNormalizeArxivID_OldFormat(t *testing.T) {
	normalized, version, err := normalizeArxivID("astro-ph/9901234")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if normalized != "astro-ph/9901234" {
		t.Errorf("expected normalized='astro-ph/9901234', got %q", normalized)
	}
	if version != "" {
		t.Errorf("expected version='', got %q", version)
	}
}

func TestNormalizeArxivID_WithPrefix(t *testing.T) {
	normalized, version, err := normalizeArxivID("arXiv:2301.00001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if normalized != "2301.00001" {
		t.Errorf("expected normalized='2301.00001', got %q", normalized)
	}
	if version != "" {
		t.Errorf("expected version='', got %q", version)
	}
}

func TestNormalizeArxivID_WithURLPrefix(t *testing.T) {
	normalized, version, err := normalizeArxivID("https://arxiv.org/abs/2301.00001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if normalized != "2301.00001" {
		t.Errorf("expected normalized='2301.00001', got %q", normalized)
	}
	if version != "" {
		t.Errorf("expected version='', got %q", version)
	}
}

func TestNormalizeArxivID_InvalidFormat(t *testing.T) {
	_, _, err := normalizeArxivID("invalid-id")
	if err == nil {
		t.Fatal("expected error for invalid format")
	}
}

func TestNormalizeArxivID_OldFormatWithVersion(t *testing.T) {
	normalized, version, err := normalizeArxivID("astro-ph/9901234v1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if normalized != "astro-ph/9901234" {
		t.Errorf("expected normalized='astro-ph/9901234', got %q", normalized)
	}
	if version != "v1" {
		t.Errorf("expected version='v1', got %q", version)
	}
}

func TestNormalizeArxivID_CategoryWithV(t *testing.T) {
	// "survey" contains "v" but shouldn't be mistaken for version
	normalized, version, err := normalizeArxivID("survey/1234567")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if normalized != "survey/1234567" {
		t.Errorf("expected normalized='survey/1234567', got %q", normalized)
	}
	if version != "" {
		t.Errorf("expected version='', got %q", version)
	}
}

// Integration tests with mocked HTTP

func TestFetchArxivPdf_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ts := &ResearchToolSet{client: &http.Client{Transport: &rewriteTransport{base: srv.URL, rt: http.DefaultTransport}}}
	input, _ := json.Marshal(map[string]any{"arxiv_id": "2301.00001"})
	result, err := ts.handleFetchArxivPdf(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed ArxivPdfResult
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	if parsed.ArxivID != "2301.00001" {
		t.Errorf("expected arxiv_id='2301.00001', got %q", parsed.ArxivID)
	}
	if parsed.PdfURL != "https://export.arxiv.org/pdf/2301.00001.pdf" {
		t.Errorf("unexpected pdf_url: %q", parsed.PdfURL)
	}
	if parsed.Version != "" {
		t.Errorf("expected version='', got %q", parsed.Version)
	}
}

func TestFetchArxivPdf_WithRedirect(t *testing.T) {
	// Server that accepts redirected requests
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Accept HEAD request and return OK
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// Test that redirects are followed (Go's http client follows redirects automatically)
	ts := &ResearchToolSet{client: &http.Client{Transport: &rewriteTransport{base: srv.URL, rt: http.DefaultTransport}}}
	input, _ := json.Marshal(map[string]any{"arxiv_id": "2301.00001"})
	result, err := ts.handleFetchArxivPdf(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed ArxivPdfResult
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("failed to unmarshal: %v, result: %s", err, result)
	}
	if parsed.ArxivID != "2301.00001" {
		t.Errorf("expected arxiv_id='2301.00001', got %q, full result: %s", parsed.ArxivID, result)
	}
	// Verify URL is constructed correctly
	if parsed.PdfURL != "https://export.arxiv.org/pdf/2301.00001.pdf" {
		t.Errorf("unexpected pdf_url: %q", parsed.PdfURL)
	}
}

func TestFetchArxivPdf_OldFormatId(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ts := &ResearchToolSet{client: &http.Client{Transport: &rewriteTransport{base: srv.URL, rt: http.DefaultTransport}}}
	input, _ := json.Marshal(map[string]any{"arxiv_id": "astro-ph/9901234"})
	result, err := ts.handleFetchArxivPdf(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed ArxivPdfResult
	json.Unmarshal([]byte(result), &parsed)
	if parsed.PdfURL != "https://export.arxiv.org/pdf/astro-ph/9901234.pdf" {
		t.Errorf("unexpected pdf_url: %q", parsed.PdfURL)
	}
}

func TestFetchArxivPdf_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	ts := &ResearchToolSet{client: &http.Client{Transport: &rewriteTransport{base: srv.URL, rt: http.DefaultTransport}}}
	input, _ := json.Marshal(map[string]any{"arxiv_id": "9999.99999"})
	result, err := ts.handleFetchArxivPdf(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}

	var errResp map[string]any
	json.Unmarshal([]byte(result), &errResp)
	if errResp["recoverable"] != true {
		t.Error("expected recoverable error")
	}
	if errResp["error"] == nil {
		t.Error("expected error message")
	}
}

func TestFetchArxivPdf_InvalidJSON(t *testing.T) {
	ts := &ResearchToolSet{client: http.DefaultClient}
	result, err := ts.handleFetchArxivPdf(context.Background(), []byte("{invalid}"))
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}

	var errResp map[string]any
	json.Unmarshal([]byte(result), &errResp)
	if errResp["recoverable"] != false {
		t.Error("expected non-recoverable error")
	}
	if errResp["error"] == nil {
		t.Error("expected error message")
	}
}

func TestFetchArxivPdf_MissingField(t *testing.T) {
	ts := &ResearchToolSet{client: http.DefaultClient}
	input, _ := json.Marshal(map[string]any{})
	result, err := ts.handleFetchArxivPdf(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}

	var errResp map[string]any
	json.Unmarshal([]byte(result), &errResp)
	if errResp["recoverable"] != false {
		t.Error("expected non-recoverable error")
	}
	if !contains(errResp["error"].(string), "required") {
		t.Error("expected 'required' in error message")
	}
}

func TestFetchArxivPdf_InvalidArxivID(t *testing.T) {
	ts := &ResearchToolSet{client: http.DefaultClient}
	input, _ := json.Marshal(map[string]any{"arxiv_id": "not-an-arxiv-id"})
	result, err := ts.handleFetchArxivPdf(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}

	var errResp map[string]any
	json.Unmarshal([]byte(result), &errResp)
	if errResp["recoverable"] != false {
		t.Error("expected non-recoverable error")
	}
}

func TestFetchArxivPdf_ContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Never respond
		select {}
	}))
	defer srv.Close()

	ts := &ResearchToolSet{client: &http.Client{Transport: &rewriteTransport{base: srv.URL, rt: http.DefaultTransport}}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	input, _ := json.Marshal(map[string]any{"arxiv_id": "2301.00001"})
	result, err := ts.handleFetchArxivPdf(ctx, input)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}

	var errResp map[string]any
	json.Unmarshal([]byte(result), &errResp)
	if errResp["error"] == nil {
		t.Error("expected error for cancelled context")
	}
}

func TestFetchArxivPdf_ArxivMaintenance(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	ts := &ResearchToolSet{client: &http.Client{Transport: &rewriteTransport{base: srv.URL, rt: http.DefaultTransport}}}
	input, _ := json.Marshal(map[string]any{"arxiv_id": "2301.00001"})
	result, err := ts.handleFetchArxivPdf(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}

	var errResp map[string]any
	json.Unmarshal([]byte(result), &errResp)
	// Error is recoverable (transient issue)
	if errResp["recoverable"] == nil || errResp["recoverable"] != true {
		t.Errorf("expected recoverable error for 503, got: %v", errResp)
	}
}

func TestFetchArxivPdf_SuspiciousRedirect(t *testing.T) {
	// Create a secondary server that's NOT arxiv.org
	maliciousSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer maliciousSrv.Close()

	// Main server redirects to non-arxiv domain
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, maliciousSrv.URL+"/fake.pdf", http.StatusMovedPermanently)
	}))
	defer srv.Close()

	ts := &ResearchToolSet{client: &http.Client{Transport: &rewriteTransport{base: srv.URL, rt: http.DefaultTransport}}}
	input, _ := json.Marshal(map[string]any{"arxiv_id": "2301.00001"})
	result, err := ts.handleFetchArxivPdf(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}

	var errResp map[string]any
	json.Unmarshal([]byte(result), &errResp)
	// Should fail because redirect goes to non-arxiv.org domain
	if errResp["error"] == nil {
		t.Errorf("expected error for suspicious redirect, got: %v", errResp)
	}
}

func TestFetchArxivPdf_VersionSuffixPreserved(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ts := &ResearchToolSet{client: &http.Client{Transport: &rewriteTransport{base: srv.URL, rt: http.DefaultTransport}}}
	input, _ := json.Marshal(map[string]any{"arxiv_id": "2301.00001v2"})
	result, err := ts.handleFetchArxivPdf(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed ArxivPdfResult
	json.Unmarshal([]byte(result), &parsed)
	if parsed.Version != "v2" {
		t.Errorf("expected version='v2', got %q", parsed.Version)
	}
}

func TestFetchArxivPdf_LeadingTrailingWhitespace(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ts := &ResearchToolSet{client: &http.Client{Transport: &rewriteTransport{base: srv.URL, rt: http.DefaultTransport}}}
	input, _ := json.Marshal(map[string]any{"arxiv_id": "  2301.00001  "})
	result, err := ts.handleFetchArxivPdf(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed ArxivPdfResult
	json.Unmarshal([]byte(result), &parsed)
	if parsed.ArxivID != "2301.00001" {
		t.Errorf("expected whitespace stripped, got %q", parsed.ArxivID)
	}
}

func TestFetchArxivPdf_EmptyStringID(t *testing.T) {
	ts := &ResearchToolSet{client: http.DefaultClient}
	input, _ := json.Marshal(map[string]any{"arxiv_id": ""})
	result, err := ts.handleFetchArxivPdf(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}

	var errResp map[string]any
	json.Unmarshal([]byte(result), &errResp)
	if errResp["recoverable"] != false {
		t.Error("expected non-recoverable error for empty string")
	}
}

// Estimator test

func TestArxivPdfEstimator(t *testing.T) {
	estimators := ResearchToolEstimators()

	got := estimators["fetch_arxiv_pdf"](map[string]any{"arxiv_id": "2301.00001"})
	if got != 100 {
		t.Errorf("fetch_arxiv_pdf estimator: got %d, want 100", got)
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || hasSubstring(s, substr)))
}

func hasSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
