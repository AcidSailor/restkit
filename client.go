package restkit

import (
	"net/http"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

const (
	defaultTimeout = 30 * time.Second
	defaultName    = "restkit"
)

// Client is an immutable, concurrency-safe HTTP-JSON transport core. Build with
// New; issue calls via the package-level Do.
type Client struct {
	baseURL    string
	httpClient *http.Client
	name       string
	hooks      []RequestHook
}

// Option configures a Client at construction time.
type Option func(*config)

type config struct {
	httpClient *http.Client
	name       string
	hooks      []RequestHook
}

// WithHTTPClient overrides the default *http.Client (30s timeout, stdlib
// transport). A nil client is ignored, leaving the default in place.
func WithHTTPClient(h *http.Client) Option {
	return func(c *config) {
		if h != nil {
			c.httpClient = h
		}
	}
}

// WithName sets the name carried into every typed error's Name field, so a
// program using several clients can attribute an error. Defaults to "restkit".
func WithName(name string) Option {
	return func(c *config) { c.name = name }
}

// WithHook registers a client-wide RequestHook, run on every call before any
// per-call hooks. Repeatable; runs in registration order. Use it for auth,
// header pins, or a query pin.
func WithHook(h RequestHook) Option {
	return func(c *config) {
		if h != nil {
			c.hooks = append(c.hooks, h)
		}
	}
}

// New trims the endpoint, resolves the *http.Client, wraps its transport with
// otelhttp (global tracer/meter providers), and builds the immutable Client.
// Returns a *ConfigError for an empty endpoint.
func New(endpoint string, opts ...Option) (*Client, error) {
	cfg := config{
		httpClient: &http.Client{Timeout: defaultTimeout},
		name:       defaultName,
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	if endpoint == "" {
		return nil, &ConfigError{Name: cfg.name, Reason: "empty endpoint"}
	}
	hc := *cfg.httpClient
	base := hc.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	hc.Transport = otelhttp.NewTransport(base)
	return &Client{
		baseURL:    strings.TrimSuffix(endpoint, "/"),
		httpClient: &hc,
		name:       cfg.name,
		hooks:      cfg.hooks,
	}, nil
}
