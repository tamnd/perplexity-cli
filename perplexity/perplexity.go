// Package perplexity is the library behind the pplx command line:
// the HTTP client and typed data models for the Perplexity AI API.
//
// Perplexity exposes an OpenAI-compatible chat completions endpoint at
// api.perplexity.ai. A bearer key (PPLX_API_KEY) is required. This package
// wraps the endpoint with a paced, retrying POST client and maps the response
// into a SearchResult record with the answer text and cited sources.
package perplexity

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// DefaultUserAgent identifies the client to Perplexity.
const DefaultUserAgent = "pplx/dev (+https://github.com/tamnd/perplexity-cli)"

// Host is the site this client talks to.
const Host = "perplexity.ai"

// APIBase is the root of the Perplexity API.
const APIBase = "https://api.perplexity.ai"

// DefaultModel is the model used when no --model flag is given.
const DefaultModel = "sonar"

// Config holds constructor parameters for Client.
type Config struct {
	APIBase   string
	APIKey    string
	UserAgent string
	Rate      time.Duration
	Retries   int
	Timeout   time.Duration
}

// DefaultConfig returns sensible defaults for the Perplexity API.
func DefaultConfig() Config {
	return Config{
		APIBase:   APIBase,
		UserAgent: DefaultUserAgent,
		Rate:      500 * time.Millisecond,
		Retries:   3,
		Timeout:   60 * time.Second,
	}
}

// Client talks to the Perplexity API over HTTPS. It paces requests, retries
// transient failures, and injects the bearer token on every request.
type Client struct {
	cfg  Config
	http *http.Client
	mu   sync.Mutex
	last time.Time
}

// NewClient returns a Client configured with cfg.
func NewClient(cfg Config) *Client {
	return &Client{
		cfg:  cfg,
		http: &http.Client{Timeout: cfg.Timeout},
	}
}

// SearchRequest is the body sent to POST /chat/completions.
type SearchRequest struct {
	Model                  string   `json:"model"`
	Messages               []Msg    `json:"messages"`
	MaxTokens              int      `json:"max_tokens,omitempty"`
	Temperature            float64  `json:"temperature"`
	TopP                   float64  `json:"top_p"`
	SearchDomainFilter     []string `json:"search_domain_filter,omitempty"`
	ReturnImages           bool     `json:"return_images"`
	ReturnRelatedQuestions bool     `json:"return_related_questions"`
	SearchRecencyFilter    string   `json:"search_recency_filter,omitempty"`
	Stream                 bool     `json:"stream"`
	PresencePenalty        float64  `json:"presence_penalty"`
	FrequencyPenalty       float64  `json:"frequency_penalty"`
}

// Msg is one message in the conversation.
type Msg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// apiResponse is the raw response from the Perplexity API.
type apiResponse struct {
	ID        string      `json:"id"`
	Model     string      `json:"model"`
	Created   int64       `json:"created"`
	Usage     apiUsage    `json:"usage"`
	Citations []string    `json:"citations"`
	Choices   []apiChoice `json:"choices"`
}

type apiUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type apiChoice struct {
	Index        int    `json:"index"`
	FinishReason string `json:"finish_reason"`
	Message      Msg    `json:"message"`
}

// Search sends a single-turn AI search query and returns the result.
func (c *Client) Search(ctx context.Context, query string, opts SearchOptions) (*SearchResult, error) {
	msgs := []Msg{}
	if opts.System != "" {
		msgs = append(msgs, Msg{Role: "system", Content: opts.System})
	}
	msgs = append(msgs, Msg{Role: "user", Content: query})

	model := opts.Model
	if model == "" {
		model = DefaultModel
	}

	maxTok := opts.MaxTokens
	if maxTok == 0 {
		maxTok = 1024
	}

	recency := opts.Recency
	if recency == "all" {
		recency = ""
	}

	req := SearchRequest{
		Model:                  model,
		Messages:               msgs,
		MaxTokens:              maxTok,
		Temperature:            0.2,
		TopP:                   0.9,
		SearchDomainFilter:     opts.Domains,
		ReturnImages:           false,
		ReturnRelatedQuestions: false,
		SearchRecencyFilter:    recency,
		Stream:                 false,
		PresencePenalty:        0,
		FrequencyPenalty:       1,
	}

	body, err := c.post(ctx, "/chat/completions", req)
	if err != nil {
		return nil, err
	}

	var resp apiResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("perplexity: empty choices in response")
	}

	citations := make([]Citation, len(resp.Citations))
	for i, u := range resp.Citations {
		citations[i] = Citation{Index: i + 1, URL: u}
	}

	return &SearchResult{
		ID:    resp.ID,
		Model: resp.Model,
		Query: query,
		Answer: resp.Choices[0].Message.Content,
		Citations: citations,
		Tokens: Usage{
			Prompt:     resp.Usage.PromptTokens,
			Completion: resp.Usage.CompletionTokens,
			Total:      resp.Usage.TotalTokens,
		},
		CreatedAt: resp.Created,
	}, nil
}

// post sends a POST request with a JSON body and returns the response body.
func (c *Client) post(ctx context.Context, path string, body any) ([]byte, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("encode request: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt <= c.cfg.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff(attempt)):
			}
		}
		data, retry, err := c.doPost(ctx, path, b)
		if err == nil {
			return data, nil
		}
		lastErr = err
		if !retry {
			return nil, err
		}
	}
	return nil, fmt.Errorf("post %s: %w", path, lastErr)
}

func (c *Client) doPost(ctx context.Context, path string, body []byte) ([]byte, bool, error) {
	c.pace()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.APIBase+path, bytes.NewReader(body))
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.cfg.UserAgent)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, true, err
	}

	switch {
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
		return nil, true, fmt.Errorf("http %d", resp.StatusCode)
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return nil, false, fmt.Errorf("http %d: check your PPLX_API_KEY", resp.StatusCode)
	case resp.StatusCode != http.StatusOK:
		msg := apiErrMsg(data)
		if msg != "" {
			return nil, false, fmt.Errorf("http %d: %s", resp.StatusCode, msg)
		}
		return nil, false, fmt.Errorf("http %d", resp.StatusCode)
	}
	return data, false, nil
}

// apiErrMsg tries to extract a human-readable error message from an API error body.
func apiErrMsg(data []byte) string {
	var e struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(data, &e) == nil && e.Error.Message != "" {
		return e.Error.Message
	}
	return ""
}

// pace blocks until at least Rate has passed since the last request.
func (c *Client) pace() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cfg.Rate <= 0 {
		return
	}
	if wait := c.cfg.Rate - time.Since(c.last); wait > 0 {
		time.Sleep(wait)
	}
	c.last = time.Now()
}

func backoff(attempt int) time.Duration {
	d := time.Duration(attempt) * time.Second
	if d > 5*time.Second {
		d = 5 * time.Second
	}
	return d
}
