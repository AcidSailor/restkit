# restkit

A dependency-light HTTP-JSON transport core for Go. It owns the repetitive
parts of talking to a JSON API — request building, sending, status checking,
JSON decoding, OpenTelemetry instrumentation, and typed errors — so hand-written
API clients can focus on their own endpoints and types. restkit knows nothing
about any specific API; everything API-specific (auth, header pins, query
defaults) is injected through a `RequestHook`.

## Features

- Generic `Do[T]` that marshals the body, builds the request, runs hooks, sends,
  checks for a 2xx, and decodes the response into `T`.
- Immutable, concurrency-safe `Client` with a 30s default timeout.
- Transport wrapped in [`otelhttp`](https://pkg.go.dev/go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp),
  so every call is traced and metered via the global OpenTelemetry providers.
- A single `RequestHook` extension point for auth, headers, and query — run
  client-wide first, then per-call.
- Built-in query helpers: `WithQuery` (merges, not clobbers) and a nil-safe
  `Values` builder.
- Three typed errors (`ConfigError`, `RequestError`, `ResponseError`), all
  matchable with `errors.As`.

## Install

```sh
go get github.com/acidsailor/restkit
```

Requires Go 1.26+.

## Quick start

```go
package main

import (
	"context"
	"fmt"

	"github.com/acidsailor/restkit"
)

type Quote struct {
	Symbol string  `json:"symbol"`
	Price  float64 `json:"price"`
}

func main() {
	client, err := restkit.New("https://api.example.com")
	if err != nil {
		panic(err)
	}

	q, err := restkit.Do[Quote](
		context.Background(),
		client,
		"GET", "/quotes/ACME",
		nil, // request body; marshaled to JSON when non-nil
	)
	if err != nil {
		panic(err)
	}
	fmt.Printf("%s: %.2f\n", q.Symbol, q.Price)
}
```

## Client construction

`New(endpoint, ...Option)` builds an immutable, concurrency-safe client. It
trims a trailing `/` from the endpoint and wraps the HTTP transport with
otelhttp.

```go
client, err := restkit.New(
	"https://api.example.com",
	restkit.WithName("example"),          // attaches to every error's Name field
	restkit.WithHTTPClient(myHTTPClient), // override the default (30s timeout)
	restkit.WithHook(authHook),           // client-wide hook, runs on every call
)
```

| Option           | Purpose                                                                     |
| ---------------- | --------------------------------------------------------------------------- |
| `WithName`       | Names this client; carried into every typed error (`"restkit"` by default). |
| `WithHTTPClient` | Replaces the default `*http.Client`. A nil client is ignored.               |
| `WithHook`       | Registers a client-wide `RequestHook`. Repeatable; runs in order.           |

## Hooks

A `RequestHook` is `func(*http.Request) error`. It can mutate headers **and**
`req.URL` (query), which makes it the single extension point for auth, header
pins, and query parameters. Returning an error aborts the call as a
`RequestError` with `Op == OpHook`.

Hooks run **client-wide first, then per-call**, so a per-call hook can override
a client-wide default.

```go
func authHook(r *http.Request) error {
	r.Header.Set("Authorization", "Bearer "+token)
	return nil
}

// Per-call hook passed as the trailing varargs of Do:
restkit.Do[Result](ctx, client, "GET", "/search", nil,
	restkit.WithQuery(url.Values{"q": {"acme"}}),
)
```

### Query helpers

`WithQuery` is a built-in hook that **merges** values into the request's
existing query rather than clobbering them, so a client-wide pin and a per-call
filter compose.

`Values` is a generic, nil-safe `url.Values` builder. Each setter is a no-op for
a nil pointer and returns the receiver, so calls chain — handy for building
queries from a struct of optional fields:

```go
q := restkit.NewValues().
	Str("symbol", filter.Symbol). // *string
	Int("limit", filter.Limit).   // *int
	Bool("active", filter.Active).// *bool
	Values                        // embedded url.Values

restkit.Do[Result](ctx, client, "GET", "/items", nil, restkit.WithQuery(q))
```

Setters: `Str`, `Int`, `Int32`, `Int64`, `Float`, `Bool`. For required or
bespoke values, use the embedded `url.Values.Set` directly.

## Errors

restkit returns three typed errors, all matchable with `errors.As`. Each carries
the client `Name`.

- **`ConfigError`** — invalid input to `New` (e.g. an empty endpoint).
- **`RequestError`** — a per-call failure before a usable result. Its `Op` field
  names the failing stage via the `Op*` constants (`OpMarshal`, `OpBuild`,
  `OpHook`, `OpSend`, `OpRead`, `OpUnmarshal`). The underlying cause is reachable
  with `errors.Is` / `errors.Unwrap`.
- **`ResponseError`** — a non-2xx response, carrying `StatusCode` and the raw
  response `Body` verbatim (the body is not decoded). It also exposes
  `GetStatusCode() int`.

```go
result, err := restkit.Do[Result](ctx, client, "GET", "/items", nil)
if err != nil {
	var respErr *restkit.ResponseError
	if errors.As(err, &respErr) {
		log.Printf("API returned %d: %s", respErr.StatusCode, respErr.Body)
	}

	var reqErr *restkit.RequestError
	if errors.As(err, &reqErr) && reqErr.Op == restkit.OpSend {
		// network/transport failure — safe to retry
	}
	return err
}
```

## License

Licensed under the GNU Affero General Public License v3. See [LICENSE](LICENSE).
