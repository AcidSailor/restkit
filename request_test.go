package restkit_test

import (
	"context"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/acidsailor/restkit"
)

// newJSONServer returns a test server that replies once with status and body.
func newJSONServer(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_, _ = io.WriteString(w, body)
		}),
	)
}

func TestDo_DecodesJSON(t *testing.T) {
	t.Parallel()
	srv := newJSONServer(t, http.StatusOK, `{"a":1}`)
	defer srv.Close()
	c, err := restkit.New(srv.URL, restkit.WithName("moex"))
	require.NoError(t, err)

	out, err := restkit.Do[map[string]int](
		context.Background(),
		c,
		http.MethodGet,
		"/x",
		nil,
		nil,
	)
	require.NoError(t, err)
	assert.Equal(t, map[string]int{"a": 1}, out)
}

func TestDo_Non2xxIsResponseError(t *testing.T) {
	t.Parallel()
	srv := newJSONServer(t, http.StatusUnprocessableEntity, `{"detail":"x"}`)
	defer srv.Close()
	c, err := restkit.New(srv.URL, restkit.WithName("moex"))
	require.NoError(t, err)

	_, err = restkit.Do[map[string]int](
		context.Background(),
		c,
		http.MethodGet,
		"/x",
		nil,
		nil,
	)
	var re *restkit.ResponseError
	require.ErrorAs(t, err, &re)
	assert.Equal(t, 422, re.StatusCode)
	assert.Equal(t, `{"detail":"x"}`, re.Body)
	assert.Equal(t, "moex", re.Name)
	var reqErr *restkit.RequestError
	assert.NotErrorAs(t, err, &reqErr)
}

func TestDo_DecodeErrorOp(t *testing.T) {
	t.Parallel()
	srv := newJSONServer(t, http.StatusOK, `{"not":"an array"}`)
	defer srv.Close()
	c, err := restkit.New(srv.URL, restkit.WithName("moex"))
	require.NoError(t, err)

	_, err = restkit.Do[[]int](
		context.Background(),
		c,
		http.MethodGet,
		"/x",
		nil,
		nil,
	)
	var re *restkit.RequestError
	require.ErrorAs(t, err, &re)
	assert.Equal(t, restkit.OpUnmarshal, re.Op)
}

func TestDo_EncodeErrorOp(t *testing.T) {
	t.Parallel()
	c, err := restkit.New("https://example.test", restkit.WithName("moex"))
	require.NoError(t, err)

	_, err = restkit.Do[int](
		context.Background(),
		c,
		http.MethodPost,
		"/x",
		math.NaN(),
		nil,
	)
	var re *restkit.RequestError
	require.ErrorAs(t, err, &re)
	assert.Equal(t, restkit.OpMarshal, re.Op)
}

func TestDo_SendErrorOp_PreservesCause(t *testing.T) {
	t.Parallel()
	srv := newJSONServer(t, http.StatusOK, "[]")
	defer srv.Close()
	c, err := restkit.New(srv.URL, restkit.WithName("moex"))
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = restkit.Do[[]int](ctx, c, http.MethodGet, "/x", nil, nil)
	var re *restkit.RequestError
	require.ErrorAs(t, err, &re)
	assert.Equal(t, restkit.OpSend, re.Op)
	assert.ErrorIs(t, err, context.Canceled, "wrapped cause survives")
}

func TestDo_HookErrorOp(t *testing.T) {
	t.Parallel()
	srv := newJSONServer(t, http.StatusOK, "[]")
	defer srv.Close()
	c, err := restkit.New(srv.URL, restkit.WithName("moex"))
	require.NoError(t, err)

	boom := errors.New("boom")
	failing := func(*http.Request) error { return boom }
	_, err = restkit.Do[[]int](
		context.Background(),
		c,
		http.MethodGet,
		"/x",
		nil,
		failing,
	)
	var re *restkit.RequestError
	require.ErrorAs(t, err, &re)
	assert.Equal(t, restkit.OpHook, re.Op)
	assert.ErrorIs(t, err, boom)
}

// TestHookOrderAndQuery: client-wide hooks run before per-call hooks, and
// WithQuery merges (does not clobber) into existing query.
func TestHookOrderAndQuery(t *testing.T) {
	t.Parallel()
	var gotQuery, gotHeader string
	srv := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotQuery = r.URL.RawQuery
			gotHeader = r.Header.Get("X-Order")
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, "[]")
		}),
	)
	defer srv.Close()

	pin := restkit.WithQuery(url.Values{"format": {"Heavy"}})
	order := func(r *http.Request) error { r.Header.Set("X-Order", "client"); return nil }
	c, err := restkit.New(
		srv.URL,
		restkit.WithName("moex"),
		restkit.WithHook(pin),
		restkit.WithHook(order),
	)
	require.NoError(t, err)

	filter := restkit.WithQuery(url.Values{"strike": {"100"}})
	trail := func(r *http.Request) error { r.Header.Set("X-Order", "percall"); return nil }
	_, err = restkit.Do[[]int](
		context.Background(),
		c,
		http.MethodGet,
		"/x",
		nil,
		filter,
		trail,
	)
	require.NoError(t, err)

	assert.Equal(t, "format=Heavy&strike=100", gotQuery, "pin and filter merge")
	assert.Equal(
		t,
		"percall",
		gotHeader,
		"per-call hook runs after client hook",
	)
}

func TestDo_NilBodySendsNoRequestBody(t *testing.T) {
	t.Parallel()
	var gotBody, gotContentType string
	var gotContentLength int64
	srv := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			b, _ := io.ReadAll(r.Body)
			gotBody = string(b)
			gotContentType = r.Header.Get("Content-Type")
			gotContentLength = r.ContentLength
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, "{}")
		}),
	)
	defer srv.Close()

	c, err := restkit.New(srv.URL, restkit.WithName("moex"))
	require.NoError(t, err)

	_, err = restkit.Do[map[string]int](
		context.Background(), c, http.MethodGet, "/x", nil, nil,
	)
	require.NoError(t, err)
	assert.Empty(t, gotBody, "nil body must send no request body, not `null`")
	assert.Empty(t, gotContentType, "no Content-Type without a body")
	assert.Zero(t, gotContentLength)
}

func TestDo_NonNilBodyIsSent(t *testing.T) {
	t.Parallel()
	var gotBody, gotContentType string
	srv := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			b, _ := io.ReadAll(r.Body)
			gotBody = string(b)
			gotContentType = r.Header.Get("Content-Type")
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, "{}")
		}),
	)
	defer srv.Close()

	c, err := restkit.New(srv.URL, restkit.WithName("moex"))
	require.NoError(t, err)

	_, err = restkit.Do[map[string]int](
		context.Background(), c, http.MethodPost, "/x",
		map[string]int{"a": 1}, nil,
	)
	require.NoError(t, err)
	assert.JSONEq(t, `{"a":1}`, gotBody)
	assert.Equal(t, "application/json", gotContentType)
}

func TestValues_NilSafe(t *testing.T) {
	t.Parallel()
	s := "call"
	var omit *string
	f := 100.5
	var i int32 = 7
	v := restkit.NewValues().
		Str("option_type", &s).
		Str("series_type", omit).
		Float("strike", &f).
		Int32("rows", &i)
	assert.Equal(t, "option_type=call&rows=7&strike=100.5", v.Encode())
}

func TestValuesBool(t *testing.T) {
	t.Parallel()
	tr := true
	fa := false
	v := restkit.NewValues().
		Bool("a", &tr).
		Bool("b", &fa).
		Bool("c", nil)
	assert.Equal(t, "true", v.Get("a"))
	assert.Equal(t, "false", v.Get("b"))
	_, ok := v.Values["c"]
	assert.False(t, ok, "nil pointer omits the key")
}

func TestPathf_EscapesSegments(t *testing.T) {
	// A value with URL-significant characters is escaped, not interpolated raw.
	assert.Equal(t,
		"/v1/accounts/a%2Fb%20c/orders",
		restkit.Pathf("/v1/accounts/%s/orders", "a/b c"),
	)
	// Two params.
	assert.Equal(t,
		"/v1/accounts/acc%23/orders/ord%3F",
		restkit.Pathf("/v1/accounts/%s/orders/%s", "acc#", "ord?"),
	)
	// A legal path sub-delimiter (Finam's SBER@MISX form) is left intact.
	assert.Equal(t,
		"/v1/assets/SBER@MISX",
		restkit.Pathf("/v1/assets/%s", "SBER@MISX"),
	)
	// No args is a plain format string.
	assert.Equal(t, "/v1/assets", restkit.Pathf("/v1/assets"))
}
