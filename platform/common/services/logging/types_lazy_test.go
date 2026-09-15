/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package logging

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"
)

// TestKeys checks the map keys are rendered, whatever their order.
func TestKeys(t *testing.T) {
	t.Parallel()

	t.Run("several keys", func(t *testing.T) {
		t.Parallel()

		rendered := Keys(map[string]int{"a": 1, "b": 2}).String()

		assert.Contains(t, rendered, "a")
		assert.Contains(t, rendered, "b")
	})

	t.Run("empty map", func(t *testing.T) {
		t.Parallel()

		assert.NotPanics(t, func() { _ = Keys(map[string]int{}).String() })
	})
}

// TestIdentifier covers the three shapes the identifier renders: a nil value, a pointer
// (which is dereferenced to its element type), and a plain struct.
func TestIdentifier(t *testing.T) {
	t.Parallel()

	t.Run("nil", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "<nil>", Identifier(nil).String())
	})

	t.Run("value", func(t *testing.T) {
		t.Parallel()

		rendered := Identifier(identifier{}).String()

		assert.Contains(t, rendered, "logging")
		assert.True(t, strings.HasSuffix(rendered, "/identifier"), "got %q", rendered)
	})

	t.Run("pointer is dereferenced to its element type", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, Identifier(identifier{}).String(), Identifier(&identifier{}).String())
	})
}

// TestEval checks the function is only called when the value is rendered.
func TestEval(t *testing.T) {
	t.Parallel()

	called := false
	lazy := Eval(func() string {
		called = true

		return "computed"
	})

	require.False(t, called, "the function is not called before rendering")
	assert.Equal(t, "computed", lazy.String())
	assert.True(t, called)
}

// TestSince checks the elapsed time is rendered as a duration.
func TestSince(t *testing.T) {
	t.Parallel()

	rendered := Since(time.Now().Add(-2 * time.Second)).String()

	elapsed, err := time.ParseDuration(rendered)
	require.NoError(t, err, "got %q", rendered)
	assert.GreaterOrEqual(t, elapsed, 2*time.Second)
}

// TestSHA256Base64 checks the hash is stable and rendered as base64.
func TestSHA256Base64(t *testing.T) {
	t.Parallel()

	rendered := SHA256Base64([]byte("some-input")).String()

	assert.Equal(t, SHA256Base64([]byte("some-input")).String(), rendered, "the same input hashes the same")
	assert.NotEqual(t, SHA256Base64([]byte("other-input")).String(), rendered)
	assert.NotEmpty(t, rendered)
}

// TestBase64 checks the encoding round-trips.
func TestBase64(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "c29tZS1ieXRlcw==", Base64([]byte("some-bytes")).String())
	assert.Empty(t, Base64(nil).String())
}

// TestPanicContextMethods checks the panic methods still panic, and record the message on
// the span first.
func TestPanicContextMethods(t *testing.T) { //nolint:paralleltest // mutates the shared global config
	t.Run("PanicfContext", func(t *testing.T) { //nolint:paralleltest // mutates the shared global config
		logger, _ := otelTestLogger(t, zapcore.DebugLevel, false)
		ctx, ended := recordingSpanCtx(t)

		require.Panics(t, func() { logger.PanicfContext(ctx, "boom %s", "now") })

		span := ended()
		require.Len(t, span.Events, 1)
		assert.Equal(t, "boom now", span.Events[0].Name)
	})

	t.Run("PanicwContext", func(t *testing.T) { //nolint:paralleltest // mutates the shared global config
		logger, _ := otelTestLogger(t, zapcore.DebugLevel, false)
		ctx, ended := recordingSpanCtx(t)

		require.Panics(t, func() { logger.PanicwContext(ctx, "boom") })

		span := ended()
		require.Len(t, span.Events, 1)
		assert.Equal(t, "boom", span.Events[0].Name)
	})
}
