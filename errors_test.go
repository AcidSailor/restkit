package restkit_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/acidsailor/restkit"
)

func TestConfigError(t *testing.T) {
	t.Parallel()
	err := error(&restkit.ConfigError{Name: "moex", Reason: "empty endpoint"})
	assert.Equal(t, "moex: invalid config: empty endpoint", err.Error())

	var ce *restkit.ConfigError
	assert.ErrorAs(t, err, &ce)
	assert.Equal(t, "empty endpoint", ce.Reason)
}

func TestRequestError_WrapsCause(t *testing.T) {
	t.Parallel()
	err := error(
		&restkit.RequestError{
			Name: "moex",
			Op:   restkit.OpSend,
			Err:  context.Canceled,
		},
	)
	assert.Equal(t, "moex: send: context canceled", err.Error())

	var re *restkit.RequestError
	assert.ErrorAs(t, err, &re)
	assert.Equal(t, restkit.OpSend, re.Op)
	// Unwrap exposes the cause to errors.Is.
	assert.ErrorIs(t, err, context.Canceled)
}

func TestResponseError(t *testing.T) {
	t.Parallel()
	err := error(
		&restkit.ResponseError{
			Name:       "moex",
			StatusCode: 422,
			Body:       `{"detail":"x"}`,
		},
	)
	assert.Equal(t, `moex: status 422, body: {"detail":"x"}`, err.Error())

	var re *restkit.ResponseError
	assert.ErrorAs(t, err, &re)
	assert.Equal(t, 422, re.StatusCode)
	assert.Equal(t, 422, re.GetStatusCode())
}
