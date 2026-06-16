package perplexity

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/any-cli/kit/errs"
)

func init() { kit.Register(Domain{}) }

// Domain is the Perplexity driver. It carries no state; the per-run client is
// built by the factory Register hands kit.
type Domain struct{}

// Info describes the scheme and the identity the standalone binary inherits.
func (Domain) Info() kit.DomainInfo {
	return kit.DomainInfo{
		Scheme: "perplexity",
		Hosts:  []string{Host},
		Identity: kit.Identity{
			Binary: "pplx",
			Short:  "Query Perplexity AI from the command line.",
			Long: `pplx runs an AI-powered web search via the Perplexity API and prints
the answer with cited sources.

Requires a Perplexity API key in PPLX_API_KEY.
Get one at: https://perplexity.ai/api

Quick start:
  pplx search "how does TCP congestion control work"
  pplx search "latest Go release" --model sonar-pro --recency week
  pplx search "rust vs go" -o raw
  pplx models`,
			Site: Host,
			Repo: "https://github.com/tamnd/perplexity-cli",
		},
	}
}

// Register installs the client factory and operations onto app.
func (Domain) Register(app *kit.App) {
	app.SetClient(newClient)

	kit.Handle(app, kit.OpMeta{
		Name:    "search",
		Group:   "query",
		Summary: "Run an AI-powered web search",
		Args:    []kit.Arg{{Name: "query", Help: "search query"}},
	}, searchOp)

	kit.Handle(app, kit.OpMeta{
		Name:    "models",
		Group:   "info",
		Summary: "List available Perplexity models",
	}, modelsOp)
}

// newClient builds a Client from the kit Config, reading PPLX_API_KEY from env.
func newClient(_ context.Context, cfg kit.Config) (any, error) {
	key := os.Getenv("PPLX_API_KEY")
	if key == "" {
		return nil, fmt.Errorf("PPLX_API_KEY is not set; get a key at https://perplexity.ai/api")
	}
	c := DefaultConfig()
	c.APIKey = key
	if cfg.Rate > 0 {
		c.Rate = cfg.Rate
	}
	if cfg.Retries > 0 {
		c.Retries = cfg.Retries
	}
	if cfg.Timeout > 0 {
		c.Timeout = cfg.Timeout
	}
	if cfg.UserAgent != "" {
		c.UserAgent = cfg.UserAgent
	}
	return NewClient(c), nil
}

// --- inputs ---

type searchInput struct {
	Query    string   `kit:"arg"                    help:"search query"`
	Model    string   `kit:"flag"                   help:"model name (sonar, sonar-pro, sonar-reasoning, sonar-reasoning-pro, sonar-deep-research)" default:"sonar"`
	System   string   `kit:"flag"                   help:"system prompt" default:"Be precise and concise."`
	MaxTok   int      `kit:"flag,name=max-tokens"   help:"max output tokens" default:"1024"`
	Recency  string   `kit:"flag"                   help:"recency filter: hour, day, week, month, all" default:"month"`
	Domains  []string `kit:"flag,name=domain"       help:"restrict search to domain (repeatable)"`
	NoSystem bool     `kit:"flag,name=no-system"    help:"send no system prompt"`
	Client   *Client  `kit:"inject"`
}

type modelsInput struct{}

// --- handlers ---

func searchOp(ctx context.Context, in searchInput, emit func(*SearchResult) error) error {
	if in.Query == "" {
		return errs.Usage("pplx search: query is required")
	}
	sys := in.System
	if in.NoSystem {
		sys = ""
	}
	if in.Model != "" && !ValidModel(in.Model) {
		// warn but proceed -- Perplexity may add new models
		_, _ = fmt.Fprintf(os.Stderr, "warning: unknown model %q; proceeding anyway\n", in.Model)
	}

	opts := SearchOptions{
		Model:     in.Model,
		System:    sys,
		MaxTokens: in.MaxTok,
		Recency:   in.Recency,
		Domains:   in.Domains,
	}
	result, err := in.Client.Search(ctx, in.Query, opts)
	if err != nil {
		return mapErr(err)
	}
	return emit(result)
}

func modelsOp(_ context.Context, _ modelsInput, emit func(*Model) error) error {
	for i := range KnownModels {
		if err := emit(&KnownModels[i]); err != nil {
			return err
		}
	}
	return nil
}

// mapErr converts library errors to kit error kinds with the right exit code.
func mapErr(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	if strings.Contains(msg, "http 401") || strings.Contains(msg, "http 403") {
		return errs.RateLimited("%s", msg)
	}
	return err
}
