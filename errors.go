package restkit

import "fmt"

// ConfigError reports invalid input to New (an empty endpoint). Match with
// errors.As.
type ConfigError struct {
	Name   string // client name from WithName; "restkit" by default
	Reason string // e.g. "empty endpoint"
}

func (e *ConfigError) Error() string {
	return fmt.Sprintf("%s: invalid config: %s", e.Name, e.Reason)
}

// Op values name the failed RequestError stage, following the net.OpError /
// fs.PathError convention (a short verb). Match on these consts, not literals.
const (
	OpMarshal   = "marshal"   // encoding the request body to JSON
	OpBuild     = "build"     // constructing the *http.Request
	OpHook      = "hook"      // a RequestHook returned an error
	OpSend      = "send"      // the HTTP round-trip
	OpRead      = "read"      // reading the response body
	OpUnmarshal = "unmarshal" // decoding the 2xx body into T
)

// RequestError reports a per-call failure before a usable result. Like
// *url.Error: Op names the stage, Err wraps the cause. Match with errors.As;
// the cause stays reachable via errors.Is (Unwrap).
type RequestError struct {
	Name string
	Op   string // one of the Op* consts
	Err  error
}

func (e *RequestError) Error() string {
	return fmt.Sprintf("%s: %s: %v", e.Name, e.Op, e.Err)
}

func (e *RequestError) Unwrap() error { return e.Err }

// ResponseError reports a non-2xx HTTP response, carrying the status and raw
// body verbatim (the error envelope is not decoded). Match with errors.As.
type ResponseError struct {
	Name       string
	StatusCode int
	Body       string
}

func (e *ResponseError) Error() string {
	return fmt.Sprintf("%s: status %d, body: %s", e.Name, e.StatusCode, e.Body)
}

// GetStatusCode lets callers read the status via an
// interface{ GetStatusCode() int } probe without importing this type.
func (e *ResponseError) GetStatusCode() int { return e.StatusCode }
