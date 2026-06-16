# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

`restkit` is a dependency-light HTTP-JSON transport core (a Go library, not an
app) — the shared base for hand-written API clients (moexoptcalc, alor,
tinvest/rest). It owns the request lifecycle — JSON marshal, request build,
hooks, send, read, status check, JSON unmarshal — plus client construction with
otelhttp instrumentation, query helpers, and three typed errors. Everything
API-specific (auth, header/query pins) is injected through a `RequestHook`;
restkit knows nothing about any upstream API.

Single package (`package restkit`, module `github.com/acidsailor/restkit`),
Go 1.26. Three source files (`client.go`, `request.go`, `errors.go`) plus
`doc.go` and tests. No `internal/`, no subpackages, no `cmd/`.

## Common commands

The project uses [Task](https://taskfile.dev) (`taskfile.yml`):

- `task test` — run tests (`go test ./...`)
- `task lint` — `golangci-lint fmt` + `golangci-lint run --fix` (mutates files)
- `task ci` — read-only verification: `golangci-lint fmt --diff` + `golangci-lint run` (what CI runs, fail-fast)
- `task check` — composite: mutating lint then tests (use before committing)
- `task update` — resync go-scaffolds template tooling via `uvx copier update`

Run a single test: `go test -run TestName ./...` (or `-run 'TestX/subtest'`).

CI (`.github/workflows/ci.yml`) delegates to the reusable
`acidsailor/go-scaffolds/.github/workflows/go-ci.yml@v1` workflow.

Linting is golangci-lint v2: `standard` linters + `modernize`, formatted by
`gofumpt` (extra-rules) and `golines` at **80 columns**. Keep lines ≤80 or the
formatter wraps them.

## Architecture

The whole public API is three concepts wired together:

1. **`Client` (client.go)** — immutable, concurrency-safe; built with
   `New(endpoint, ...Option)`. Construction trims the trailing `/`, resolves the
   `*http.Client` (default 30s timeout), and wraps its transport in
   `otelhttp.NewTransport`, so every call is traced/metered via the global OTel
   providers. Options: `WithHTTPClient`, `WithName`, `WithHook`. A nil http
   client or nil hook is ignored, so `New` always yields a usable client.

2. **`Do[T]` (request.go)** — the generic package-level entry point:
   `Do[T](ctx, c, method, path, body, hooks...)`. Marshals `body` to JSON,
   builds the request against `baseURL+path`, sets `Accept: application/json`
   (and `Content-Type` when body is non-nil), runs hooks, sends, reads, and
   unmarshals a 2xx body into `T`. The unexported `Client.do` returns the raw
   2xx bytes; `Do` adds the JSON decode. Hooks run **client-wide first, then
   per-call** (`slices.Concat(c.hooks, hooks)`) so a per-call hook can override a
   client pin.

3. **`RequestHook` (request.go)** — `func(*http.Request) error`. The only
   extension point, and why restkit stays API-agnostic: it can mutate headers
   and `req.URL` (query), so auth, header pins, and query all go through it.
   `WithQuery(url.Values)` is a built-in hook that **merges** (not clobbers) into
   existing query params. `Values` / `NewValues()` is a generic, nil-safe
   `url.Values` builder whose setters (`Str`, `Int`, `Int32`, `Int64`, `Float`,
   `Bool`) skip nil pointers and chain.

### Error model (errors.go)

Three typed errors, all matched downstream with `errors.As`:

- **`ConfigError`** — bad input to `New` (empty endpoint).
- **`RequestError`** — any per-call stage failure before a usable result. `Op`
  names the stage via the `Op*` consts (`OpMarshal`, `OpBuild`, `OpHook`,
  `OpSend`, `OpRead`, `OpUnmarshal`) — match on the consts, not string literals.
  Mirrors `*url.Error`; wraps the cause via `Unwrap` (reachable with
  `errors.Is`).
- **`ResponseError`** — a non-2xx response carrying `StatusCode` and the raw
  `Body` verbatim (the error envelope is **not** decoded). Exposes
  `GetStatusCode() int` so callers can read the status via an
  `interface{ GetStatusCode() int }` probe without importing this package.

Every error carries `Name` (from `WithName`, default `"restkit"`) so a program
using several clients can attribute failures.

## Conventions

- Keep restkit API-agnostic: anything specific to one upstream API belongs in a
  hook supplied by the consuming client, never here.
- The tooling (taskfile, lint config, CI) is generated from the
  `acidsailor/go-scaffolds` copier template (`.copier-answers.yml`). Don't
  hand-edit generated tooling — run `task update` to resync; that file's
  `_commit` pins the template version.
