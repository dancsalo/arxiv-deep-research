package research

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleSearchGithub_Success(t *testing.T) {
	// Mock GitHub API response
	mockResponse := githubSearchResponse{
		TotalCount: 2,
		Items: []githubRepoItem{
			{
				Name:        "pytorch-transformer",
				FullName:    "user/pytorch-transformer",
				Description: "PyTorch implementation of Transformer",
				HTMLURL:     "https://github.com/user/pytorch-transformer",
				Stars:       5432,
				Language:    "Python",
				License:     &githubLicenseInfo{Key: "mit", Name: "MIT License"},
				Topics:      []string{"pytorch", "transformer", "nlp"},
				UpdatedAt:   "2024-05-01T00:00:00Z",
				Archived:    false,
			},
			{
				Name:        "attention-is-all-you-need",
				FullName:    "user/attention-is-all-you-need",
				Description: "Implementation of the paper",
				HTMLURL:     "https://github.com/user/attention-is-all-you-need",
				Stars:       3210,
				Language:    "Python",
				License:     &githubLicenseInfo{Key: "apache-2.0", Name: "Apache License 2.0"},
				Topics:      []string{"attention", "paper"},
				UpdatedAt:   "2024-04-15T00:00:00Z",
				Archived:    false,
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify headers
		if got := r.Header.Get("Accept"); got != "application/vnd.github+json" {
			t.Errorf("Accept header = %q, want 'application/vnd.github+json'", got)
		}
		if got := r.Header.Get("X-GitHub-Api-Version"); got != "2022-11-28" {
			t.Errorf("API version = %q, want '2022-11-28'", got)
		}
		if !strings.Contains(r.Header.Get("User-Agent"), "arxiv-deep-research") {
			t.Errorf("User-Agent missing or incorrect: %s", r.Header.Get("User-Agent"))
		}

		// Verify query params
		if r.URL.Query().Get("sort") != "stars" {
			t.Errorf("sort param = %q, want 'stars'", r.URL.Query().Get("sort"))
		}
		if r.URL.Query().Get("order") != "desc" {
			t.Errorf("order param = %q, want 'desc'", r.URL.Query().Get("order"))
		}

		// Verify quality filters were added to query
		query := r.URL.Query().Get("q")
		if !strings.Contains(query, "stars:>100") {
			t.Errorf("query missing stars filter: %s", query)
		}
		// Check for dynamic date filter (pushed:>YYYY-MM-DD format, not specific date)
		if !strings.Contains(query, "pushed:>") || !strings.Contains(query, "20") {
			t.Errorf("query missing recency filter (pushed:>YYYY-MM-DD): %s", query)
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-RateLimit-Remaining", "59")
		w.Header().Set("X-RateLimit-Reset", "1715000000")
		json.NewEncoder(w).Encode(mockResponse)
	}))
	defer server.Close()

	// Create toolset with mock server
	client := &http.Client{}
	toolset := newResearchToolSetWithBase(client, server.URL)

	input := `{"query": "transformer implementation", "max_results": 5}`

	result, err := toolset.handleSearchGithub(context.Background(), []byte(input))

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Parse result
	var repos []GitHubRepoResult
	if err := json.Unmarshal([]byte(result), &repos); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	// Verify results
	if len(repos) != 2 {
		t.Errorf("got %d results, want 2", len(repos))
	}

	if repos[0].Name != "pytorch-transformer" {
		t.Errorf("first result name = %q, want 'pytorch-transformer'", repos[0].Name)
	}
	if repos[0].License != "MIT License" {
		t.Errorf("first result license = %q, want 'MIT License'", repos[0].License)
	}
	if len(repos[0].Topics) != 3 {
		t.Errorf("first result topics count = %d, want 3", len(repos[0].Topics))
	}
	if repos[0].IsArchived {
		t.Error("first result should not be archived")
	}
}

func TestHandleSearchGithub_EmptyQuery(t *testing.T) {
	toolset := NewResearchToolSet(&http.Client{})
	input := `{"query": ""}`
	result, err := toolset.handleSearchGithub(context.Background(), []byte(input))

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should return error JSON
	if !strings.Contains(result, "error") || !strings.Contains(result, "required") {
		t.Errorf("expected error for empty query, got: %s", result)
	}
}

func TestHandleSearchGithub_MaxResultsCapped(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantMaxCap int
	}{
		{"default", `{"query": "test"}`, 5},
		{"explicit 3", `{"query": "test", "max_results": 3}`, 3},
		{"over limit 100", `{"query": "test", "max_results": 100}`, 5},
		{"negative", `{"query": "test", "max_results": -1}`, 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				perPage := r.URL.Query().Get("per_page")
				if perPage != "5" && perPage != "3" {
					t.Errorf("per_page = %q, want <= 5", perPage)
				}
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("X-RateLimit-Remaining", "59")
				w.Header().Set("X-RateLimit-Reset", "1715000000")
				json.NewEncoder(w).Encode(githubSearchResponse{Items: []githubRepoItem{}})
			}))
			defer server.Close()

			toolset := newResearchToolSetWithBase(&http.Client{}, server.URL)
			_, err := toolset.handleSearchGithub(context.Background(), []byte(tt.input))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestHandleSearchGithub_RateLimitExceeded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", "1715000000")
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"message": "API rate limit exceeded"}`))
	}))
	defer server.Close()

	toolset := newResearchToolSetWithBase(&http.Client{}, server.URL)
	result, err := toolset.executeGithubSearch(context.Background(), server.URL, "test")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should return recoverable error with reset time
	if !strings.Contains(result, "rate limit exceeded") {
		t.Errorf("expected rate limit error, got: %s", result)
	}
	if !strings.Contains(result, "2024-05-06") { // Unix 1715000000
		t.Errorf("expected formatted reset time, got: %s", result)
	}
}

func TestHandleSearchGithub_FilterArchived(t *testing.T) {
	mockResponse := githubSearchResponse{
		Items: []githubRepoItem{
			{
				Name:      "active-repo",
				FullName:  "user/active-repo",
				HTMLURL:   "https://github.com/user/active-repo",
				Stars:     1000,
				Archived:  false,
			},
			{
				Name:      "archived-repo",
				FullName:  "user/archived-repo",
				HTMLURL:   "https://github.com/user/archived-repo",
				Stars:     5000,
				Archived:  true,
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-RateLimit-Remaining", "59")
		w.Header().Set("X-RateLimit-Reset", "1715000000")
		json.NewEncoder(w).Encode(mockResponse)
	}))
	defer server.Close()

	toolset := newResearchToolSetWithBase(&http.Client{}, server.URL)
	result, err := toolset.executeGithubSearch(context.Background(), server.URL, "test")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var repos []GitHubRepoResult
	json.Unmarshal([]byte(result), &repos)

	if len(repos) != 1 {
		t.Errorf("got %d repos (expected archived filtered out), want 1", len(repos))
	}
	if repos[0].Name != "active-repo" {
		t.Errorf("wrong repo returned: %s", repos[0].Name)
	}
}

func TestHandleSearchGithub_RetryOn500(t *testing.T) {
	attemptCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++
		if attemptCount < 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-RateLimit-Remaining", "59")
		w.Header().Set("X-RateLimit-Reset", "1715000000")
		json.NewEncoder(w).Encode(githubSearchResponse{Items: []githubRepoItem{}})
	}))
	defer server.Close()

	toolset := newResearchToolSetWithBase(&http.Client{}, server.URL)
	result, err := toolset.executeGithubSearch(context.Background(), server.URL, "test")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if attemptCount != 2 {
		t.Errorf("expected 2 attempts (1 retry), got %d", attemptCount)
	}
	if strings.Contains(result, "error") {
		t.Errorf("expected success after retry, got error: %s", result)
	}
}

func TestHandleSearchGithub_EmptyResults(t *testing.T) {
	mockResponse := githubSearchResponse{
		TotalCount: 0,
		Items:      []githubRepoItem{},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-RateLimit-Remaining", "59")
		w.Header().Set("X-RateLimit-Reset", "1715000000")
		json.NewEncoder(w).Encode(mockResponse)
	}))
	defer server.Close()

	toolset := newResearchToolSetWithBase(&http.Client{}, server.URL)
	result, err := toolset.executeGithubSearch(context.Background(), server.URL, "nonexistent-query")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var repos []GitHubRepoResult
	if err := json.Unmarshal([]byte(result), &repos); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	if len(repos) != 0 {
		t.Errorf("got %d results, want 0 for empty response", len(repos))
	}
}

func TestHandleSearchGithub_NilTopics(t *testing.T) {
	mockResponse := githubSearchResponse{
		Items: []githubRepoItem{
			{
				Name:      "repo-without-topics",
				FullName:  "user/repo-without-topics",
				HTMLURL:   "https://github.com/user/repo-without-topics",
				Stars:     200,
				Topics:    nil, // GitHub may omit this field
				Archived:  false,
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-RateLimit-Remaining", "59")
		w.Header().Set("X-RateLimit-Reset", "1715000000")
		json.NewEncoder(w).Encode(mockResponse)
	}))
	defer server.Close()

	toolset := newResearchToolSetWithBase(&http.Client{}, server.URL)
	result, err := toolset.executeGithubSearch(context.Background(), server.URL, "test")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var repos []GitHubRepoResult
	if err := json.Unmarshal([]byte(result), &repos); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	if repos[0].Topics == nil {
		t.Error("Topics should be empty array, not nil")
	}
	if len(repos[0].Topics) != 0 {
		t.Errorf("Topics should be empty array, got length %d", len(repos[0].Topics))
	}
}

func TestHandleSearchGithub_MalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-RateLimit-Remaining", "59")
		w.Header().Set("X-RateLimit-Reset", "1715000000")
		w.Write([]byte(`{"items": [{"invalid json`))
	}))
	defer server.Close()

	toolset := newResearchToolSetWithBase(&http.Client{}, server.URL)
	result, err := toolset.executeGithubSearch(context.Background(), server.URL, "test")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result, "error") || !strings.Contains(result, "parse") {
		t.Errorf("expected parse error, got: %s", result)
	}
}

func TestHandleSearchGithub_RetryAfterHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "60")
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", "1715000000")
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"message": "rate limit"}`))
	}))
	defer server.Close()

	toolset := newResearchToolSetWithBase(&http.Client{}, server.URL)
	result, err := toolset.executeGithubSearch(context.Background(), server.URL, "test")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should prioritize Retry-After over X-RateLimit-Reset
	if !strings.Contains(result, "Retry after 60 seconds") {
		t.Errorf("expected Retry-After message, got: %s", result)
	}
}
