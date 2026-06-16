package perplexity_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tamnd/perplexity-cli/perplexity"
)

// newTestClient returns a Client pointed at srv with the given API key.
func newTestClient(t *testing.T, srv *httptest.Server, key string) *perplexity.Client {
	t.Helper()
	cfg := perplexity.DefaultConfig()
	cfg.APIBase = srv.URL
	cfg.APIKey = key
	cfg.Rate = 0
	cfg.Timeout = 5 * time.Second
	return perplexity.NewClient(cfg)
}

func okResponse() map[string]any {
	return map[string]any{
		"id":        "resp-001",
		"model":     "sonar",
		"created":   int64(1720000000),
		"citations": []string{"https://example.com/a", "https://example.com/b"},
		"usage": map[string]int{
			"prompt_tokens":     20,
			"completion_tokens": 100,
			"total_tokens":      120,
		},
		"choices": []map[string]any{
			{
				"index":         0,
				"finish_reason": "stop",
				"message": map[string]string{
					"role":    "assistant",
					"content": "This is the answer.",
				},
			},
		},
	}
}

func TestSearch_success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(okResponse())
	}))
	defer srv.Close()

	c := newTestClient(t, srv, "test-key")
	res, err := c.Search(context.Background(), "test query", perplexity.SearchOptions{
		Model:  "sonar",
		System: "Be concise.",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Query != "test query" {
		t.Errorf("Query = %q, want %q", res.Query, "test query")
	}
	if res.Answer != "This is the answer." {
		t.Errorf("Answer = %q, want %q", res.Answer, "This is the answer.")
	}
	if res.ID != "resp-001" {
		t.Errorf("ID = %q, want %q", res.ID, "resp-001")
	}
	if len(res.Citations) != 2 {
		t.Errorf("Citations len = %d, want 2", len(res.Citations))
	}
	if res.Citations[0].Index != 1 || res.Citations[0].URL != "https://example.com/a" {
		t.Errorf("Citations[0] = %+v, wrong", res.Citations[0])
	}
	if res.Tokens.Total != 120 {
		t.Errorf("Tokens.Total = %d, want 120", res.Tokens.Total)
	}
}

func TestSearch_unauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := newTestClient(t, srv, "bad-key")
	_, err := c.Search(context.Background(), "q", perplexity.SearchOptions{})
	if err == nil {
		t.Fatal("expected error for 401, got nil")
	}
}

func TestSearch_rateLimited(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls < 2 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]any{
			"id": "resp-002", "model": "sonar", "created": int64(1720000000),
			"citations": []string{},
			"usage":     map[string]int{"prompt_tokens": 5, "completion_tokens": 10, "total_tokens": 15},
			"choices": []map[string]any{
				{"index": 0, "finish_reason": "stop", "message": map[string]string{"role": "assistant", "content": "ok"}},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	cfg := perplexity.DefaultConfig()
	cfg.APIBase = srv.URL
	cfg.APIKey = "key"
	cfg.Rate = 0
	cfg.Retries = 3
	cfg.Timeout = 5 * time.Second
	c := perplexity.NewClient(cfg)

	res, err := c.Search(context.Background(), "q", perplexity.SearchOptions{})
	if err != nil {
		t.Fatalf("expected retry to succeed, got: %v", err)
	}
	if res.Answer != "ok" {
		t.Errorf("Answer = %q, want ok", res.Answer)
	}
	if calls < 2 {
		t.Errorf("expected at least 2 calls (retry), got %d", calls)
	}
}

func TestSearch_serverError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	cfg := perplexity.DefaultConfig()
	cfg.APIBase = srv.URL
	cfg.APIKey = "key"
	cfg.Rate = 0
	cfg.Retries = 1
	cfg.Timeout = 5 * time.Second
	c := perplexity.NewClient(cfg)

	_, err := c.Search(context.Background(), "q", perplexity.SearchOptions{})
	if err == nil {
		t.Fatal("expected error for 500, got nil")
	}
}

func TestSearch_emptyCitations(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]any{
			"id": "r1", "model": "sonar", "created": int64(0),
			"citations": []string{},
			"usage":     map[string]int{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
			"choices": []map[string]any{
				{"index": 0, "finish_reason": "stop", "message": map[string]string{"role": "assistant", "content": "no sources"}},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := newTestClient(t, srv, "key")
	res, err := c.Search(context.Background(), "q", perplexity.SearchOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Citations == nil {
		t.Error("Citations should not be nil when response has empty array")
	}
	if len(res.Citations) != 0 {
		t.Errorf("Citations len = %d, want 0", len(res.Citations))
	}
}

func TestSearch_domainFilter(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]any{
			"id": "r1", "model": "sonar", "created": int64(0),
			"citations": []string{},
			"usage":     map[string]int{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
			"choices": []map[string]any{
				{"index": 0, "finish_reason": "stop", "message": map[string]string{"role": "assistant", "content": "ok"}},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := newTestClient(t, srv, "key")
	_, err := c.Search(context.Background(), "q", perplexity.SearchOptions{
		Domains: []string{"github.com", "reddit.com"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	domains, ok := gotBody["search_domain_filter"].([]any)
	if !ok {
		t.Fatalf("search_domain_filter not present or wrong type: %T", gotBody["search_domain_filter"])
	}
	if len(domains) != 2 {
		t.Errorf("search_domain_filter len = %d, want 2", len(domains))
	}
}
