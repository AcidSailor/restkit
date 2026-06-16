package restkit_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/acidsailor/restkit"
)

func TestNew_ConfigErrors(t *testing.T) {
	t.Parallel()

	_, err := restkit.New("", restkit.WithName("moex"))
	var ce *restkit.ConfigError
	require.ErrorAs(t, err, &ce, "empty endpoint should be a *ConfigError")
	assert.Equal(t, "moex", ce.Name)
	assert.Equal(t, "empty endpoint", ce.Reason)
}

// TestNew_NilHTTPClientFallsBackToDefault: WithHTTPClient(nil) is benign — New
// ignores it and keeps the default *http.Client.
func TestNew_NilHTTPClientFallsBackToDefault(t *testing.T) {
	t.Parallel()
	c, err := restkit.New("https://x", restkit.WithHTTPClient(nil))
	require.NoError(t, err, "nil *http.Client should fall back to the default")
	require.NotNil(t, c)
}

func TestNew_DefaultNameAndTrim(t *testing.T) {
	t.Parallel()
	c, err := restkit.New("https://example.test/api/")
	require.NoError(t, err)
	require.NotNil(t, c)
}

// TestNew_WrapsTransportWithOtel: the transport emits spans through the global
// TracerProvider (otelhttp wrapping).
func TestNew_WrapsTransportWithOtel(t *testing.T) {
	// No t.Parallel(): this test mutates the global TracerProvider.
	orig := otel.GetTracerProvider()
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { otel.SetTracerProvider(orig) })

	srv := newJSONServer(t, http.StatusOK, "[]")
	defer srv.Close()

	c, err := restkit.New(srv.URL)
	require.NoError(t, err)
	_, err = restkit.Do[[]int](
		context.Background(),
		c,
		http.MethodGet,
		"/x",
		nil,
		nil,
	)
	require.NoError(t, err)

	assert.NotEmpty(
		t,
		sr.Ended(),
		"otelhttp should have recorded at least one span",
	)
}
