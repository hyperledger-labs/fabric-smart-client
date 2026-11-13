/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package logging

import (
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// TestLogValueMatchesString checks each helper's value logs the same text through a
// structured handler as through String.
func TestLogValueMatchesString(t *testing.T) {
	t.Parallel()
	b := []byte("data\x00")
	for _, s := range []fmt.Stringer{
		Keys(map[string]int{"a": 1}), SHA256Base64(b), Base64(b), Identifier(t),
		Eval(func() int { return 1 }), Prefix(strings.Repeat("x", 30)), Printable(string(b)),
	} {
		assert.Equal(t, s.String(), s.(slog.LogValuer).LogValue().String())
	}
	assert.Implements(t, (*slog.LogValuer)(nil), Since(time.Now()))
}

// TestSHA256Base64_HashesWhenFormatted checks the hash is computed when the value is
// formatted, not when SHA256Base64 is called, so a disabled line doesn't hash.
func TestSHA256Base64_HashesWhenFormatted(t *testing.T) {
	t.Parallel()
	b := []byte("before")
	s := SHA256Base64(b)
	copy(b, "after!")
	assert.Equal(t, SHA256Base64([]byte("after!")).String(), s.String())
}
