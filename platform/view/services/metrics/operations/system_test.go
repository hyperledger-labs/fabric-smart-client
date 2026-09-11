/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package operations

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/prometheus"
)

// registration records one call to RegisterHandler.
type registration struct {
	path   string
	secure bool
}

// fakeServer records the handlers the operations system registers on it.
type fakeServer struct {
	registered []registration
}

func (s *fakeServer) RegisterHandler(path string, _ http.Handler, secure bool) {
	s.registered = append(s.registered, registration{path: path, secure: secure})
}

func (s *fakeServer) paths() []string {
	paths := make([]string, 0, len(s.registered))
	for _, r := range s.registered {
		paths = append(paths, r.path)
	}

	return paths
}

// recordingLogger captures what the system logs, so the provider branches can
// be told apart by the message they emit.
type recordingLogger struct {
	debugs []string
	infos  []any
	warns  []any
}

func (l *recordingLogger) Debugf(template string, _ ...any) {
	l.debugs = append(l.debugs, template)
}
func (l *recordingLogger) Info(args ...any)                { l.infos = append(l.infos, args...) }
func (l *recordingLogger) Warn(args ...any)                { l.warns = append(l.warns, args...) }
func (l *recordingLogger) Warnf(template string, _ ...any) { l.warns = append(l.warns, template) }

func newTestSystem(t *testing.T, o *Options, provider metrics.Provider) (*System, *fakeServer, *recordingLogger) {
	t.Helper()

	server := &fakeServer{}
	logger := &recordingLogger{}

	system, err := NewOperationSystem(server, NewOperationsLogger(logger), provider, o)
	require.NoError(t, err)
	require.NotNil(t, system)

	return system, server, logger
}

// TestNewOperationSystemRegistersLogspec checks the logspec endpoint is
// registered regardless of the metrics provider.
func TestNewOperationSystemRegistersLogspec(t *testing.T) {
	t.Parallel()

	_, server, _ := newTestSystem(t, &Options{}, &disabled.Provider{})

	require.Contains(t, server.paths(), "/logspec")
}

// TestNewOperationSystemRequireClientCert checks the flag is passed through to
// every handler the system registers.
func TestNewOperationSystemRequireClientCert(t *testing.T) {
	t.Parallel()

	for _, requireClientCert := range []bool{false, true} {
		_, server, _ := newTestSystem(t,
			&Options{Metrics: MetricsOptions{Provider: "prometheus"}, RequireClientCert: requireClientCert},
			&prometheus.Provider{},
		)

		require.NotEmpty(t, server.registered)
		for _, r := range server.registered {
			require.Equal(t, requireClientCert, r.secure, "handler %q", r.path)
		}
	}
}

// TestInitializeMetricsProviderPrometheus checks the metrics endpoint is
// registered only for the prometheus provider.
func TestInitializeMetricsProviderPrometheus(t *testing.T) {
	t.Parallel()

	system, server, _ := newTestSystem(t,
		&Options{Metrics: MetricsOptions{Provider: "prometheus"}},
		&prometheus.Provider{},
	)

	require.Contains(t, server.paths(), "/metrics")
	require.IsType(t, &prometheus.Provider{}, system.Provider)
}

// TestInitializeMetricsProviderDisabled checks an empty provider name disables
// metrics and says so, without registering an endpoint.
func TestInitializeMetricsProviderDisabled(t *testing.T) {
	t.Parallel()

	system, server, logger := newTestSystem(t, &Options{}, &disabled.Provider{})

	require.NotContains(t, server.paths(), "/metrics")
	require.Contains(t, logger.infos, "metrics disabled")
	require.Empty(t, logger.warns)
	require.IsType(t, &disabled.Provider{}, system.Provider)
}

// TestInitializeMetricsProviderUnknown checks an unrecognised provider name
// disables metrics with a warning rather than failing.
func TestInitializeMetricsProviderUnknown(t *testing.T) {
	t.Parallel()

	_, server, logger := newTestSystem(t,
		&Options{Metrics: MetricsOptions{Provider: "not-a-provider"}},
		&disabled.Provider{},
	)

	require.NotContains(t, server.paths(), "/metrics")
	require.NotEmpty(t, logger.warns, "an unknown provider is warned about")
}

// TestSystemStart publishes the version gauge; it is the only thing Start does,
// the endpoints having been registered during construction.
func TestSystemStart(t *testing.T) {
	t.Parallel()

	system, _, _ := newTestSystem(t, &Options{Version: "test-version"}, &disabled.Provider{})

	require.NoError(t, system.Start())
}

// TestSystemStop checks Stop halts the tickers and is safe both before Start
// and when called twice.
func TestSystemStop(t *testing.T) {
	t.Parallel()

	system, _, _ := newTestSystem(t, &Options{}, &disabled.Provider{})

	require.NoError(t, system.Stop(), "Stop is safe when Start was never called")

	system.collectorTicker = time.NewTicker(time.Hour)
	system.sendTicker = time.NewTicker(time.Hour)

	require.NoError(t, system.Stop())
	require.Nil(t, system.collectorTicker)
	require.Nil(t, system.sendTicker)

	require.NoError(t, system.Stop(), "Stop is idempotent")
}

// TestNewMetricsProvider checks the provider selected for each configured name.
func TestNewMetricsProvider(t *testing.T) {
	t.Parallel()

	require.IsType(t, &prometheus.Provider{}, NewMetricsProvider(MetricsOptions{Provider: "prometheus"}))
	require.IsType(t, &disabled.Provider{}, NewMetricsProvider(MetricsOptions{}))
	require.IsType(t, &disabled.Provider{}, NewMetricsProvider(MetricsOptions{Provider: "not-a-provider"}))
}

// TestVersionGauge checks the gauge is built from the provider it is given.
func TestVersionGauge(t *testing.T) {
	t.Parallel()

	gauge := versionGauge(&disabled.Provider{})
	require.NotNil(t, gauge)
	require.NotPanics(t, func() { gauge.With("version", "test-version").Set(1) })
}

// TestNewDisabledHistogram checks histograms are disabled while the rest of the
// wrapped provider is left alone.
func TestNewDisabledHistogram(t *testing.T) {
	t.Parallel()

	provider := NewDisabledHistogram(&prometheus.Provider{})
	require.NotNil(t, provider)

	histogram := provider.NewHistogram(metrics.HistogramOpts{
		Namespace: "test",
		Name:      "test_histogram",
		Help:      "a histogram that should be disabled",
	})
	require.NotNil(t, histogram)
	require.NotPanics(t, func() { histogram.Observe(1) })

	require.NotNil(t, provider.NewGauge(metrics.GaugeOpts{
		Namespace: "test",
		Name:      "test_gauge",
		Help:      "a gauge from the wrapped provider",
	}), "non-histogram metrics still come from the wrapped provider")
}

// TestNewOperationsLogger checks a nil logger falls back to the default rather
// than panicking on first use.
func TestNewOperationsLogger(t *testing.T) {
	t.Parallel()

	logger := &recordingLogger{}
	require.Equal(t, logger, NewOperationsLogger(logger).Logger)

	fallback := NewOperationsLogger(nil)
	require.NotNil(t, fallback)
	require.NotNil(t, fallback.Logger)
	require.NotPanics(t, func() { fallback.Info("logging through the fallback") })
}
