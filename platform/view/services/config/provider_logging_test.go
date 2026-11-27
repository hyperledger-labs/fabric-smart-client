/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
)

// TestLoggingConfig checks that the logging keys of a core.yaml loaded with NewProvider,
// and their CORE_ environment overrides, change what the process logs.
func TestLoggingConfig(t *testing.T) { //nolint:paralleltest // changes the global logging configuration and the environment
	t.Run("format", func(t *testing.T) { //nolint:paralleltest // see TestLoggingConfig
		_, buf := loadLoggingConfig(t, "logging:\n  format: json\n", nil)
		logger.Info("hello")
		assertJSONLine(t, buf, "hello")
	})
	t.Run("spec", func(t *testing.T) { //nolint:paralleltest // see TestLoggingConfig
		_, buf := loadLoggingConfig(t, "logging:\n  spec: fsc.platform.view.services.config=debug:warn\n", nil)
		logger.Debug("in prefix")
		slog.New(logging.Handler("other")).Info("outside prefix")
		assert.Contains(t, buf.String(), "in prefix")
		assert.NotContains(t, buf.String(), "outside prefix")
	})
	t.Run("sanitize", func(t *testing.T) { //nolint:paralleltest // see TestLoggingConfig
		for _, sanitize := range []bool{true, false} {
			loadLoggingConfig(t, "logging:\n  spec: info\n  otel:\n    sanitize: "+strconv.FormatBool(sanitize)+"\n", nil)
			assert.Equal(t, sanitize, spanEventSanitized(t), "sanitize: %t", sanitize)
		}
	})
	t.Run("env format", func(t *testing.T) { //nolint:paralleltest // see TestLoggingConfig
		_, buf := loadLoggingConfig(t, "logging:\n  format: '%{message}'\n", map[string]string{"CORE_LOGGING_FORMAT": "json"})
		logger.Info("hello")
		assertJSONLine(t, buf, "hello")
	})
	t.Run("env spec", func(t *testing.T) { //nolint:paralleltest // see TestLoggingConfig
		_, buf := loadLoggingConfig(t, "logging:\n  spec: info\n", map[string]string{"CORE_LOGGING_SPEC": "warn"})
		logger.Info("hidden")
		logger.Warn("shown")
		assert.NotContains(t, buf.String(), "hidden")
		assert.Contains(t, buf.String(), "shown")
	})
	t.Run("env sanitize", func(t *testing.T) { //nolint:paralleltest // see TestLoggingConfig
		// The variable overrides sanitize whether core.yaml leaves it out or sets it.
		for _, c := range []struct{ inFile, env string }{{"", "true"}, {"false", "true"}, {"true", "false"}} {
			coreYAML := "logging:\n  spec: info\n"
			if c.inFile != "" {
				coreYAML += "  otel:\n    sanitize: " + c.inFile + "\n"
			}
			loadLoggingConfig(t, coreYAML, map[string]string{"CORE_LOGGING_OTEL_SANITIZE": c.env})
			assert.Equal(t, c.env == "true", spanEventSanitized(t), coreYAML)
		}
	})
	t.Run("ProvideFromRaw and MergeConfig keep logging", func(t *testing.T) { //nolint:paralleltest // see TestLoggingConfig
		p, _ := loadLoggingConfig(t, "logging:\n  spec: debug\n", nil)
		require.True(t, logger.IsEnabledFor(logging.DebugLevel))
		// No logging keys, as Fabric's AddNetwork merges.
		raw := []byte("fabric:\n  enabled: true\n")
		_, err := p.ProvideFromRaw(raw)
		require.NoError(t, err)
		require.NoError(t, p.MergeConfig(raw))
		assert.True(t, logger.IsEnabledFor(logging.DebugLevel))
	})
}

// loadLoggingConfig loads coreYAML with NewProvider, with the variables of the environment
// that change it cleared and env set. It returns the provider and the buffer the process
// then logs to, empty.
func loadLoggingConfig(t *testing.T, coreYAML string, env map[string]string) (*Provider, *bytes.Buffer) {
	t.Helper()
	for _, k := range []string{"FSCNODE_CFG_PATH", "FABRIC_LOGGING_SPEC", "CORE_LOGGING_SPEC", "CORE_LOGGING_FORMAT", "CORE_LOGGING_OTEL_SANITIZE"} {
		t.Setenv(k, env[k])
	}
	t.Cleanup(func() { require.NoError(t, logging.Init(logging.Config{Writer: os.Stderr})) })
	buf := &bytes.Buffer{}
	require.NoError(t, logging.Init(logging.Config{Writer: buf}))
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "core.yaml"), []byte(coreYAML), 0o600))
	p, err := NewProvider(dir)
	require.NoError(t, err)
	buf.Reset()
	return p, buf
}

// assertJSONLine checks that buf holds one JSON line with msg written by this package's logger.
func assertJSONLine(t *testing.T, buf *bytes.Buffer, msg string) {
	t.Helper()
	var line map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &line), buf.String())
	assert.Equal(t, "info", line["level"])
	assert.Equal(t, "fsc.platform.view.services.config", line["name"])
	assert.Equal(t, msg, line["msg"])
}

// spanEventSanitized logs a message with a non-printable character on a recording span,
// and reports whether the span event recorded for it was sanitized.
func spanEventSanitized(t *testing.T) bool {
	t.Helper()
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	t.Cleanup(func() { require.NoError(t, tp.Shutdown(context.Background())) })
	ctx, span := tp.Tracer("test").Start(context.Background(), "op")
	logger.InfofContext(ctx, "a\x00b")
	span.End()
	require.Len(t, rec.Ended(), 1)
	events := rec.Ended()[0].Events()
	require.Len(t, events, 1)
	return strings.HasPrefix(events[0].Name, "[nonprintable] ")
}
