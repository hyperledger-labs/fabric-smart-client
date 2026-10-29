/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package prometheus_test

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	prom "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	commonmetrics "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/prometheus"
)

var _ commonmetrics.Provider = &prometheus.Provider{}

// TestProvider runs its subtests one after another: Provider registers its
// collectors through prom.Register, so every subtest points the process-wide
// prom.DefaultRegisterer at a registry of its own.
//
//nolint:paralleltest // replaces the process-wide prom.DefaultRegisterer
func TestProvider(t *testing.T) {
	t.Run("NewCounter", func(t *testing.T) {
		t.Run("creates counters that support labels", func(t *testing.T) {
			p, scrape := newProvider(t)
			counter := p.NewCounter(counterOpts())
			counter.With("alpha", "a", "beta", "b").Add(1)
			counter.With("alpha", "aardvark", "beta", "b").Add(2)

			assertExposes(t, scrape(),
				`# HELP peer_playground_counter_name This is some help text for the counter`,
				`# TYPE peer_playground_counter_name counter`,
				`peer_playground_counter_name{alpha="a",beta="b"} 1`,
				`peer_playground_counter_name{alpha="aardvark",beta="b"} 2`,
			)
		})

		t.Run("does not need With when the counter is defined without labels", func(t *testing.T) {
			p, scrape := newProvider(t)
			opts := counterOpts()
			opts.LabelNames = nil
			p.NewCounter(opts).Add(1)

			assertExposes(t, scrape(), `peer_playground_counter_name 1`)
		})
	})

	t.Run("NewGauge creates gauges that support labels", func(t *testing.T) {
		p, scrape := newProvider(t)
		gauge := p.NewGauge(gaugeOpts())
		gauge.With("alpha", "a", "beta", "b").Add(1)
		gauge.With("alpha", "a", "beta", "b").Add(1)
		gauge.With("alpha", "aardvark", "beta", "b").Add(1)
		gauge.With("alpha", "aardvark", "beta", "bob").Set(99)

		assertExposes(t, scrape(),
			`# HELP peer_playground_gauge_name This is some help text for the gauge`,
			`# TYPE peer_playground_gauge_name gauge`,
			`peer_playground_gauge_name{alpha="a",beta="b"} 2`,
			`peer_playground_gauge_name{alpha="aardvark",beta="b"} 1`,
			`peer_playground_gauge_name{alpha="aardvark",beta="bob"} 99`,
		)
	})

	t.Run("NewHistogram", func(t *testing.T) {
		t.Run("creates histograms that support labels", func(t *testing.T) {
			p, scrape := newProvider(t)
			histogram := p.NewHistogram(histogramOpts())
			for _, limit := range prom.DefBuckets {
				histogram.With("alpha", "a", "beta", "b").Observe(limit)
			}
			histogram.With("alpha", "a", "beta", "b").Observe(prom.DefBuckets[len(prom.DefBuckets)-1] + 1)

			assertExposes(t, scrape(),
				`# HELP peer_playground_histogram_name This is some help text for the histogram`,
				`# TYPE peer_playground_histogram_name histogram`,
				`peer_playground_histogram_name_bucket{alpha="a",beta="b",le="0.005"} 1`,
				`peer_playground_histogram_name_bucket{alpha="a",beta="b",le="0.01"} 2`,
				`peer_playground_histogram_name_bucket{alpha="a",beta="b",le="0.025"} 3`,
				`peer_playground_histogram_name_bucket{alpha="a",beta="b",le="0.05"} 4`,
				`peer_playground_histogram_name_bucket{alpha="a",beta="b",le="0.1"} 5`,
				`peer_playground_histogram_name_bucket{alpha="a",beta="b",le="0.25"} 6`,
				`peer_playground_histogram_name_bucket{alpha="a",beta="b",le="0.5"} 7`,
				`peer_playground_histogram_name_bucket{alpha="a",beta="b",le="1"} 8`,
				`peer_playground_histogram_name_bucket{alpha="a",beta="b",le="2.5"} 9`,
				`peer_playground_histogram_name_bucket{alpha="a",beta="b",le="5"} 10`,
				`peer_playground_histogram_name_bucket{alpha="a",beta="b",le="10"} 11`,
				`peer_playground_histogram_name_bucket{alpha="a",beta="b",le="+Inf"} 12`,
				`peer_playground_histogram_name_sum{alpha="a",beta="b"} `,
				`peer_playground_histogram_name_count{alpha="a",beta="b"} 12`,
			)
		})

		t.Run("creates histograms with buckets that support labels", func(t *testing.T) {
			p, scrape := newProvider(t)
			opts := histogramOpts()
			opts.Buckets = []float64{1, 5}
			histogram := p.NewHistogram(opts)
			histogram.With("alpha", "a", "beta", "b").Observe(0.5)
			histogram.With("alpha", "a", "beta", "b").Observe(4.5)

			assertExposes(t, scrape(),
				`# HELP peer_playground_histogram_name This is some help text for the histogram`,
				`# TYPE peer_playground_histogram_name histogram`,
				`peer_playground_histogram_name_bucket{alpha="a",beta="b",le="1"} 1`,
				`peer_playground_histogram_name_bucket{alpha="a",beta="b",le="5"} 2`,
				`peer_playground_histogram_name_sum{alpha="a",beta="b"} 5`,
				`peer_playground_histogram_name_count{alpha="a",beta="b"} 2`,
			)
		})
	})

	t.Run("duplicate registration", func(t *testing.T) {
		// newCountingProvider wraps the test registry so a subtest can tell
		// cache hits from registry round-trips.
		newCountingProvider := func(t *testing.T) (*prometheus.Provider, func() string, *countingRegisterer) {
			t.Helper()
			p, scrape := newProvider(t)
			counting := &countingRegisterer{Registerer: prom.DefaultRegisterer}
			setDefaultRegisterer(t, counting)
			return p, scrape, counting
		}

		t.Run("returns counters backed by the collector registered first", func(t *testing.T) {
			p, scrape, _ := newCountingProvider(t)
			c1 := p.NewCounter(counterOpts())
			c2 := p.NewCounter(counterOpts())
			c1.With("alpha", "a", "beta", "b").Add(1)
			c2.With("alpha", "a", "beta", "b").Add(2)

			assertExposes(t, scrape(), `peer_playground_counter_name{alpha="a",beta="b"} 3`)
		})

		t.Run("returns gauges backed by the collector registered first", func(t *testing.T) {
			p, scrape, _ := newCountingProvider(t)
			g1 := p.NewGauge(gaugeOpts())
			g2 := p.NewGauge(gaugeOpts())
			g1.With("alpha", "a", "beta", "b").Set(1)
			g2.With("alpha", "a", "beta", "b").Add(2)

			assertExposes(t, scrape(), `peer_playground_gauge_name{alpha="a",beta="b"} 3`)
		})

		t.Run("returns histograms backed by the collector registered first", func(t *testing.T) {
			p, scrape, _ := newCountingProvider(t)
			h1 := p.NewHistogram(histogramOpts())
			h2 := p.NewHistogram(histogramOpts())
			h1.With("alpha", "a", "beta", "b").Observe(1)
			h2.With("alpha", "a", "beta", "b").Observe(2)

			assertExposes(t, scrape(),
				`peer_playground_histogram_name_count{alpha="a",beta="b"} 2`,
				`peer_playground_histogram_name_sum{alpha="a",beta="b"} 3`,
			)
		})

		t.Run("is not affected by callers mutating opts slices after creation", func(t *testing.T) {
			p, scrape, counting := newCountingProvider(t)
			labels := []string{"alpha", "beta"}
			opts := counterOpts()
			opts.LabelNames = labels
			c1 := p.NewCounter(opts)
			labels[0] = "gamma"

			var c2 commonmetrics.Counter
			require.NotPanics(t, func() { c2 = p.NewCounter(counterOpts()) })
			require.EqualValues(t, 1, counting.registrations.Load())

			c1.With("alpha", "a", "beta", "b").Add(1)
			c2.With("alpha", "a", "beta", "b").Add(2)

			assertExposes(t, scrape(), `peer_playground_counter_name{alpha="a",beta="b"} 3`)
		})

		t.Run("skips the registry for repeated requests", func(t *testing.T) {
			p, _, counting := newCountingProvider(t)
			for range 10 {
				p.NewCounter(counterOpts())
			}
			require.EqualValues(t, 1, counting.registrations.Load())
		})

		t.Run("registers the collector only once under concurrent requests", func(t *testing.T) {
			p, scrape, counting := newCountingProvider(t)
			var wg sync.WaitGroup
			for range 20 {
				wg.Go(func() {
					p.NewCounter(counterOpts()).With("alpha", "a", "beta", "b").Add(1)
				})
			}
			wg.Wait()

			require.EqualValues(t, 1, counting.registrations.Load())
			assertExposes(t, scrape(), `peer_playground_counter_name{alpha="a",beta="b"} 20`)
		})

		t.Run("panics when the same name is reused with different label names", func(t *testing.T) {
			p, _, _ := newCountingProvider(t)
			p.NewCounter(counterOpts())
			opts := counterOpts()
			opts.LabelNames = []string{"gamma"}
			require.Panics(t, func() { p.NewCounter(opts) })
		})

		t.Run("ignores differences in the doc-only LabelHelp field", func(t *testing.T) {
			p, scrape, _ := newCountingProvider(t)
			opts := counterOpts()
			opts.LabelHelp = map[string]string{"alpha": "first label"}
			c1 := p.NewCounter(opts)
			opts.LabelHelp = map[string]string{"alpha": "a different description"}
			var c2 commonmetrics.Counter
			require.NotPanics(t, func() { c2 = p.NewCounter(opts) })

			c1.With("alpha", "a", "beta", "b").Add(1)
			c2.With("alpha", "a", "beta", "b").Add(2)

			assertExposes(t, scrape(), `peer_playground_counter_name{alpha="a",beta="b"} 3`)
		})

		t.Run("returns a counter backed by an identical collector registered outside the provider", func(t *testing.T) {
			p, scrape, _ := newCountingProvider(t)
			opts := counterOpts()
			existing := prom.NewCounterVec(prom.CounterOpts{
				Namespace: opts.Namespace,
				Subsystem: opts.Subsystem,
				Name:      opts.Name,
				Help:      opts.Help,
			}, opts.LabelNames)
			require.NoError(t, prom.DefaultRegisterer.Register(existing))
			existing.WithLabelValues("a", "b").Add(1)

			var c commonmetrics.Counter
			require.NotPanics(t, func() { c = p.NewCounter(opts) })
			c.With("alpha", "a", "beta", "b").Add(2)

			assertExposes(t, scrape(), `peer_playground_counter_name{alpha="a",beta="b"} 3`)
		})

		t.Run("panics with a descriptive error when the same name is reused as a different metric type", func(t *testing.T) {
			p, _, _ := newCountingProvider(t)
			opts := counterOpts()
			p.NewCounter(opts)
			gaugeOpts := commonmetrics.GaugeOpts{
				Namespace:  opts.Namespace,
				Subsystem:  opts.Subsystem,
				Name:       opts.Name,
				Help:       opts.Help,
				LabelNames: opts.LabelNames,
			}
			require.ErrorContains(t, panicError(t, func() { p.NewGauge(gaugeOpts) }), "incompatible collector type")
		})

		t.Run("panics without poisoning the cache when a non-vector collector has the same descriptor", func(t *testing.T) {
			p, _, _ := newCountingProvider(t)
			scalarOpts := commonmetrics.CounterOpts{
				Namespace: "peer",
				Subsystem: "playground",
				Name:      "scalar_name",
				Help:      "This is some help text for the scalar counter",
			}
			require.NoError(t, prom.DefaultRegisterer.Register(prom.NewCounter(prom.CounterOpts{
				Namespace: scalarOpts.Namespace,
				Subsystem: scalarOpts.Subsystem,
				Name:      scalarOpts.Name,
				Help:      scalarOpts.Help,
			})))

			require.ErrorContains(t, panicError(t, func() { p.NewCounter(scalarOpts) }), "incompatible collector type")
			require.NotPanics(t, func() { p.NewCounter(counterOpts()) })
		})
	})

	t.Run("edge case behavior", func(t *testing.T) {
		t.Run("uses unknown for a label value missing from With", func(t *testing.T) {
			p, scrape := newProvider(t)
			p.NewCounter(counterOpts()).With("alpha", "a", "beta").Add(1)

			assertExposes(t, scrape(),
				`# HELP peer_playground_counter_name This is some help text for the counter`,
				`# TYPE peer_playground_counter_name counter`,
				`peer_playground_counter_name{alpha="a",beta="unknown"} 1`,
			)
		})

		t.Run("panics when With is called with an extra label", func(t *testing.T) {
			p, _ := newProvider(t)
			counter := p.NewCounter(counterOpts())
			err := panicError(t, func() { counter.With("alpha", "a", "beta", "b", "charlie", "c").Add(1) })
			require.Regexp(t, `inconsistent label cardinality: expected 2 label values but got 3 in prometheus.Labels\{.*\}`, err.Error())
		})

		t.Run("panics with a cardinality message when label values are not provided", func(t *testing.T) {
			p, _ := newProvider(t)
			counter := p.NewCounter(counterOpts())
			err := panicError(t, func() { counter.Add(1) })
			require.EqualError(t, err, `inconsistent label cardinality: expected 2 label values but got 0 in prometheus.Labels{}`)
		})
	})
}

// Go's main package has no path separator in its fully qualified function name
// ("main.main"), so a binary that creates a metric directly from main reaches
// GetPackageName with a slash-free frame. Called straight from a test function,
// GetPackageName resolves to runtime.goexit, which is slash-free for the same
// reason.
func TestGetPackageNameWithoutPathSeparator(t *testing.T) {
	t.Parallel()

	name := prometheus.GetPackageName()
	require.NotEmpty(t, name)
	require.NotContains(t, name, "/")
}

// newProvider points prom.DefaultRegisterer at a fresh registry until t ends
// and returns a provider that registers into it, together with a function
// returning the registry's metrics in the text exposition format.
func newProvider(t *testing.T) (*prometheus.Provider, func() string) {
	t.Helper()
	registry := prom.NewRegistry()
	setDefaultRegisterer(t, registry)
	handler := promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
	scrape := func() string {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
		return rec.Body.String()
	}
	return &prometheus.Provider{}, scrape
}

// setDefaultRegisterer replaces prom.DefaultRegisterer with r until t ends.
func setDefaultRegisterer(t *testing.T, r prom.Registerer) {
	t.Helper()
	prev := prom.DefaultRegisterer
	prom.DefaultRegisterer = r
	t.Cleanup(func() { prom.DefaultRegisterer = prev })
}

// assertExposes checks that the scraped metrics contain every line in want.
func assertExposes(t *testing.T, scraped string, want ...string) {
	t.Helper()
	for _, line := range want {
		assert.Contains(t, scraped, line)
	}
}

// panicError runs f and returns the error it panics with. It fails t if f
// does not panic or panics with a value that is not an error.
func panicError(t *testing.T, f func()) error {
	t.Helper()
	var v any
	func() {
		defer func() { v = recover() }()
		f()
	}()
	err, ok := v.(error)
	require.Truef(t, ok, "expected a panic with an error, got %v", v)
	return err
}

func counterOpts() commonmetrics.CounterOpts {
	return commonmetrics.CounterOpts{
		Namespace:  "peer",
		Subsystem:  "playground",
		Name:       "counter_name",
		Help:       "This is some help text for the counter",
		LabelNames: []string{"alpha", "beta"},
	}
}

func gaugeOpts() commonmetrics.GaugeOpts {
	return commonmetrics.GaugeOpts{
		Namespace:  "peer",
		Subsystem:  "playground",
		Name:       "gauge_name",
		Help:       "This is some help text for the gauge",
		LabelNames: []string{"alpha", "beta"},
	}
}

func histogramOpts() commonmetrics.HistogramOpts {
	return commonmetrics.HistogramOpts{
		Namespace:  "peer",
		Subsystem:  "playground",
		Name:       "histogram_name",
		Help:       "This is some help text for the histogram",
		LabelNames: []string{"alpha", "beta"},
	}
}

// countingRegisterer counts Register calls so tests can tell cache hits from
// registry round-trips.
type countingRegisterer struct {
	prom.Registerer
	registrations atomic.Int32
}

func (r *countingRegisterer) Register(c prom.Collector) error {
	r.registrations.Add(1)
	return r.Registerer.Register(c)
}
