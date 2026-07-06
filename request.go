package restkit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
)

// RequestHook mutates a built request before it is sent. It reaches headers and
// req.URL (query), covering auth, header pins, and query. Returning an error
// aborts the call as a *RequestError with Op == OpHook.
type RequestHook func(*http.Request) error

// WithQuery returns a hook that merges q into the request's existing query, so
// a client-wide pin and a per-call filter compose rather than clobber.
func WithQuery(q url.Values) RequestHook {
	return func(r *http.Request) error {
		if len(q) == 0 {
			return nil
		}
		merged := r.URL.Query()
		for k, vs := range q {
			for _, v := range vs {
				merged.Add(k, v)
			}
		}
		r.URL.RawQuery = merged.Encode()
		return nil
	}
}

// Values is a generic, nil-safe url.Values builder. Each setter skips a nil
// pointer and returns the receiver so calls chain. The embedded url.Values
// provides Set for required/bespoke values (e.g. a formatted Date).
type Values struct{ url.Values }

// NewValues returns an empty builder.
func NewValues() Values { return Values{url.Values{}} }

func (v Values) Str(key string, p *string) Values {
	if p != nil {
		v.Set(key, *p)
	}
	return v
}

func (v Values) Int(key string, p *int) Values {
	if p != nil {
		v.Set(key, strconv.Itoa(*p))
	}
	return v
}

func (v Values) Int32(key string, p *int32) Values {
	if p != nil {
		v.Set(key, strconv.FormatInt(int64(*p), 10))
	}
	return v
}

func (v Values) Int64(key string, p *int64) Values {
	if p != nil {
		v.Set(key, strconv.FormatInt(*p, 10))
	}
	return v
}

func (v Values) Float(key string, p *float64) Values {
	if p != nil {
		v.Set(key, strconv.FormatFloat(*p, 'f', -1, 64))
	}
	return v
}

func (v Values) Bool(key string, p *bool) Values {
	if p != nil {
		v.Set(key, strconv.FormatBool(*p))
	}
	return v
}

// Pathf builds a request path, percent-escaping each interpolated argument as a
// single path segment. It is the path-side counterpart to [Values] (which
// escapes query parameters): pass a format like "/v1/accounts/%s/orders/%s"
// with raw parameter values, and a value containing '/', '?', '#', or a space
// is escaped rather than corrupting the URL (injecting a segment, starting a
// query string, etc.).
//
// Every arg is interpolated with %s; use [fmt.Sprintf] directly if you need
// other verbs. An empty arg yields an empty segment (the server decides how to
// handle it) — Pathf validates escaping, not presence.
func Pathf(format string, args ...string) string {
	escaped := make([]any, len(args))
	for i, a := range args {
		escaped[i] = url.PathEscape(a)
	}
	return fmt.Sprintf(format, escaped...)
}

// Do issues the request and decodes the 2xx JSON body into T. A non-2xx is a
// *ResponseError; any other stage failure is a *RequestError naming it.
func Do[T any](
	ctx context.Context,
	c *Client,
	method, path string,
	body any,
	hooks ...RequestHook,
) (T, error) {
	var result T

	data, err := c.do(ctx, method, path, body, hooks)
	if err != nil {
		return result, err
	}

	if err := json.Unmarshal(data, &result); err != nil {
		return result, &RequestError{
			Name: c.name,
			Op:   OpUnmarshal,
			Err:  err,
		}
	}
	return result, nil
}

// do runs the request lifecycle and returns the raw 2xx body.
func (c *Client) do(
	ctx context.Context,
	method, path string,
	body any,
	hooks []RequestHook,
) ([]byte, error) {
	// A nil body sends no request body at all (not the JSON literal `null`),
	// so GETs stay body-less — some servers/CDNs reject a GET that carries a
	// body. http.NewRequestWithContext maps a nil io.Reader to no body.
	var bodyReader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, &RequestError{
				Name: c.name,
				Op:   OpMarshal,
				Err:  err,
			}
		}
		bodyReader = bytes.NewReader(raw)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		method,
		c.baseURL+path,
		bodyReader,
	)
	if err != nil {
		return nil, &RequestError{
			Name: c.name,
			Op:   OpBuild,
			Err:  err,
		}
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	// Client-wide hooks first, then per-call hooks (so per-call can override).
	for _, h := range slices.Concat(c.hooks, hooks) {
		if h == nil {
			continue
		}
		if e := h(req); e != nil {
			return nil, &RequestError{
				Name: c.name,
				Op:   OpHook,
				Err:  e,
			}
		}
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, &RequestError{
			Name: c.name,
			Op:   OpSend,
			Err:  err,
		}
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &RequestError{
			Name: c.name,
			Op:   OpRead,
			Err:  err,
		}
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &ResponseError{
			Name:       c.name,
			StatusCode: resp.StatusCode,
			Body:       string(data),
		}
	}
	return data, nil
}
