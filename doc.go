// Package restkit is a dependency-light HTTP-JSON transport core shared by
// hand-written API clients (moexoptcalc, alor, tinvest/rest). It owns the
// request lifecycle, client construction with otelhttp instrumentation, the
// Values/WithQuery query helpers, and three typed errors (ConfigError,
// RequestError, ResponseError), all matched with errors.As. Per-client
// behaviour (auth, header/query pins) is injected through RequestHook.
package restkit
