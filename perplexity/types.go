package perplexity

// SearchOptions holds optional parameters for a search call.
type SearchOptions struct {
	Model     string
	System    string
	MaxTokens int
	Recency   string   // hour, day, week, month, all
	Domains   []string // restrict to these domains
}

// SearchResult is one record returned by a pplx search call.
type SearchResult struct {
	ID        string     `json:"id"`
	Model     string     `json:"model"`
	Query     string     `json:"query"`
	Answer    string     `json:"answer"`
	Citations []Citation `json:"citations"`
	Tokens    Usage      `json:"tokens"`
	CreatedAt int64      `json:"created_at"`
}

// Citation is one cited source URL from a SearchResult.
type Citation struct {
	Index int    `json:"index"`
	URL   string `json:"url"`
}

// Usage reports token counts for a search call.
type Usage struct {
	Prompt     int `json:"prompt"`
	Completion int `json:"completion"`
	Total      int `json:"total"`
}

// Model is metadata about a Perplexity model.
type Model struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Context     int    `json:"context_window"`
}

// KnownModels is the list of Perplexity models available via the API.
// Perplexity has no models-list API endpoint; this is a static list
// maintained from their model cards at docs.perplexity.ai.
var KnownModels = []Model{
	{Name: "sonar", Description: "Standard web search model, fast and cost-efficient", Context: 127072},
	{Name: "sonar-pro", Description: "Enhanced web search with deeper reasoning", Context: 200000},
	{Name: "sonar-reasoning", Description: "Chain-of-thought web search with reasoning trace", Context: 127072},
	{Name: "sonar-reasoning-pro", Description: "Extended reasoning, highest quality answers", Context: 200000},
	{Name: "sonar-deep-research", Description: "Multi-step research synthesis, longer latency", Context: 127072},
}

// ValidModel returns true when name is a known Perplexity model.
func ValidModel(name string) bool {
	for _, m := range KnownModels {
		if m.Name == name {
			return true
		}
	}
	return false
}
