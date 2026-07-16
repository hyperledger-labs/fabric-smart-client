/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package prometheus_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	prom "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	commonmetrics "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/prometheus"
)

var _ = Describe("Provider", func() {
	var (
		server *httptest.Server
		client *http.Client
		p      *prometheus.Provider
	)

	BeforeEach(func() {
		// Note: These tests can't be run in parallel because go-kit uses
		// the global registry to manage metrics. This is something to revisit
		// in the future.
		registry := prom.NewRegistry()
		prom.DefaultRegisterer = registry
		prom.DefaultGatherer = registry

		server = httptest.NewServer(promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
		client = server.Client()

		p = &prometheus.Provider{}
	})

	AfterEach(func() {
		server.Close()
	})

	It("implements metrics.Provider", func() {
		var p commonmetrics.Provider = &prometheus.Provider{}
		Expect(p).NotTo(BeNil())
	})

	Describe("NewCounter", func() {
		var counterOpts commonmetrics.CounterOpts

		BeforeEach(func() {
			counterOpts = commonmetrics.CounterOpts{
				Namespace:  "peer",
				Subsystem:  "playground",
				Name:       "counter_name",
				Help:       "This is some help text for the counter",
				LabelNames: []string{"alpha", "beta"},
			}
		})

		It("creates counters that support labels", func() {
			counter := p.NewCounter(counterOpts)
			counter.With("alpha", "a", "beta", "b").Add(1)
			counter.With("alpha", "aardvark", "beta", "b").Add(2)

			resp, err := client.Get(fmt.Sprintf("http://%s/metrics", server.Listener.Addr().String()))
			Expect(err).NotTo(HaveOccurred())
			defer utils.IgnoreErrorFunc(resp.Body.Close)

			bytes, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(bytes)).To(ContainSubstring(`# HELP peer_playground_counter_name This is some help text for the counter`))
			Expect(string(bytes)).To(ContainSubstring(`# TYPE peer_playground_counter_name counter`))
			Expect(string(bytes)).To(ContainSubstring(`peer_playground_counter_name{alpha="a",beta="b"} 1`))
			Expect(string(bytes)).To(ContainSubstring(`peer_playground_counter_name{alpha="aardvark",beta="b"} 2`))
		})

		Context("when the counter is defined without labels", func() {
			BeforeEach(func() {
				counterOpts.LabelNames = nil
			})

			It("With does not need to be called", func() {
				counter := p.NewCounter(counterOpts)
				counter.Add(1)

				resp, err := client.Get(fmt.Sprintf("http://%s/metrics", server.Listener.Addr().String()))
				Expect(err).NotTo(HaveOccurred())
				defer utils.IgnoreErrorFunc(resp.Body.Close)

				bytes, err := io.ReadAll(resp.Body)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(bytes)).To(ContainSubstring(`peer_playground_counter_name 1`))
			})
		})
	})

	Describe("NewGauge", func() {
		var gaugeOpts commonmetrics.GaugeOpts

		BeforeEach(func() {
			gaugeOpts = commonmetrics.GaugeOpts{
				Namespace:  "peer",
				Subsystem:  "playground",
				Name:       "gauge_name",
				Help:       "This is some help text for the gauge",
				LabelNames: []string{"alpha", "beta"},
			}
		})

		It("creates gauges that support labels", func() {
			gauge := p.NewGauge(gaugeOpts)
			gauge.With("alpha", "a", "beta", "b").Add(1)
			gauge.With("alpha", "a", "beta", "b").Add(1)
			gauge.With("alpha", "aardvark", "beta", "b").Add(1)
			gauge.With("alpha", "aardvark", "beta", "bob").Set(99)

			resp, err := client.Get(fmt.Sprintf("http://%s/metrics", server.Listener.Addr().String()))
			Expect(err).NotTo(HaveOccurred())
			defer utils.IgnoreErrorFunc(resp.Body.Close)

			bytes, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(bytes)).To(ContainSubstring(`# HELP peer_playground_gauge_name This is some help text for the gauge`))
			Expect(string(bytes)).To(ContainSubstring(`# TYPE peer_playground_gauge_name gauge`))
			Expect(string(bytes)).To(ContainSubstring(`peer_playground_gauge_name{alpha="a",beta="b"} 2`))
			Expect(string(bytes)).To(ContainSubstring(`peer_playground_gauge_name{alpha="aardvark",beta="b"} 1`))
			Expect(string(bytes)).To(ContainSubstring(`peer_playground_gauge_name{alpha="aardvark",beta="bob"} 99`))
		})
	})

	Describe("NewHistogram", func() {
		var histogramOpts commonmetrics.HistogramOpts

		BeforeEach(func() {
			histogramOpts = commonmetrics.HistogramOpts{
				Namespace:  "peer",
				Subsystem:  "playground",
				Name:       "histogram_name",
				Help:       "This is some help text for the gauge",
				LabelNames: []string{"alpha", "beta"},
			}
		})

		It("creates histogram that support labels", func() {
			histogram := p.NewHistogram(histogramOpts)
			for _, limit := range prom.DefBuckets {
				histogram.With("alpha", "a", "beta", "b").Observe(limit)
			}
			histogram.With("alpha", "a", "beta", "b").Observe(prom.DefBuckets[len(prom.DefBuckets)-1] + 1)

			resp, err := client.Get(fmt.Sprintf("http://%s/metrics", server.Listener.Addr().String()))
			Expect(err).NotTo(HaveOccurred())
			defer utils.IgnoreErrorFunc(resp.Body.Close)

			bytes, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(bytes)).To(ContainSubstring(`# HELP peer_playground_histogram_name This is some help text for the gauge`))
			Expect(string(bytes)).To(ContainSubstring(`# TYPE peer_playground_histogram_name histogram`))
			Expect(string(bytes)).To(ContainSubstring(`peer_playground_histogram_name_bucket{alpha="a",beta="b",le="0.005"} 1`))
			Expect(string(bytes)).To(ContainSubstring(`peer_playground_histogram_name_bucket{alpha="a",beta="b",le="0.01"} 2`))
			Expect(string(bytes)).To(ContainSubstring(`peer_playground_histogram_name_bucket{alpha="a",beta="b",le="0.025"} 3`))
			Expect(string(bytes)).To(ContainSubstring(`peer_playground_histogram_name_bucket{alpha="a",beta="b",le="0.05"} 4`))
			Expect(string(bytes)).To(ContainSubstring(`peer_playground_histogram_name_bucket{alpha="a",beta="b",le="0.1"} 5`))
			Expect(string(bytes)).To(ContainSubstring(`peer_playground_histogram_name_bucket{alpha="a",beta="b",le="0.25"} 6`))
			Expect(string(bytes)).To(ContainSubstring(`peer_playground_histogram_name_bucket{alpha="a",beta="b",le="0.5"} 7`))
			Expect(string(bytes)).To(ContainSubstring(`peer_playground_histogram_name_bucket{alpha="a",beta="b",le="1"} 8`))
			Expect(string(bytes)).To(ContainSubstring(`peer_playground_histogram_name_bucket{alpha="a",beta="b",le="2.5"} 9`))
			Expect(string(bytes)).To(ContainSubstring(`peer_playground_histogram_name_bucket{alpha="a",beta="b",le="5"} 10`))
			Expect(string(bytes)).To(ContainSubstring(`peer_playground_histogram_name_bucket{alpha="a",beta="b",le="10"} 11`))
			Expect(string(bytes)).To(ContainSubstring(`peer_playground_histogram_name_bucket{alpha="a",beta="b",le="+Inf"} 12`))
			Expect(string(bytes)).To(ContainSubstring(`peer_playground_histogram_name_sum{alpha="a",beta="b"} `))
			Expect(string(bytes)).To(ContainSubstring(`peer_playground_histogram_name_count{alpha="a",beta="b"} 12`))
		})

		It("creates histogram with buckets that support labels", func() {
			histogramOpts.Buckets = []float64{1, 5}
			histogram := p.NewHistogram(histogramOpts)

			histogram.With("alpha", "a", "beta", "b").Observe(0.5)
			histogram.With("alpha", "a", "beta", "b").Observe(4.5)

			resp, err := client.Get(fmt.Sprintf("http://%s/metrics", server.Listener.Addr().String()))
			Expect(err).NotTo(HaveOccurred())
			defer utils.IgnoreErrorFunc(resp.Body.Close)

			bytes, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(bytes)).To(ContainSubstring(`# HELP peer_playground_histogram_name This is some help text for the gauge`))
			Expect(string(bytes)).To(ContainSubstring(`# TYPE peer_playground_histogram_name histogram`))
			Expect(string(bytes)).To(ContainSubstring(`peer_playground_histogram_name_bucket{alpha="a",beta="b",le="1"} 1`))
			Expect(string(bytes)).To(ContainSubstring(`peer_playground_histogram_name_bucket{alpha="a",beta="b",le="5"} 2`))
			Expect(string(bytes)).To(ContainSubstring(`peer_playground_histogram_name_sum{alpha="a",beta="b"} 5`))
			Expect(string(bytes)).To(ContainSubstring(`peer_playground_histogram_name_count{alpha="a",beta="b"} 2`))
		})
	})

	Describe("duplicate registration", func() {
		var (
			counterOpts commonmetrics.CounterOpts
			counting    *countingRegisterer
		)

		BeforeEach(func() {
			counterOpts = commonmetrics.CounterOpts{
				Namespace:  "peer",
				Subsystem:  "playground",
				Name:       "counter_name",
				Help:       "This is some help text for the counter",
				LabelNames: []string{"alpha", "beta"},
			}
			counting = &countingRegisterer{Registerer: prom.DefaultRegisterer}
			prom.DefaultRegisterer = counting
		})

		It("returns counters backed by the collector registered first", func() {
			c1 := p.NewCounter(counterOpts)
			c2 := p.NewCounter(counterOpts)
			c1.With("alpha", "a", "beta", "b").Add(1)
			c2.With("alpha", "a", "beta", "b").Add(2)

			resp, err := client.Get(fmt.Sprintf("http://%s/metrics", server.Listener.Addr().String()))
			Expect(err).NotTo(HaveOccurred())
			defer utils.IgnoreErrorFunc(resp.Body.Close)

			bytes, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(bytes)).To(ContainSubstring(`peer_playground_counter_name{alpha="a",beta="b"} 3`))
		})

		It("returns gauges backed by the collector registered first", func() {
			gaugeOpts := commonmetrics.GaugeOpts{
				Namespace:  "peer",
				Subsystem:  "playground",
				Name:       "gauge_name",
				Help:       "This is some help text for the gauge",
				LabelNames: []string{"alpha", "beta"},
			}
			g1 := p.NewGauge(gaugeOpts)
			g2 := p.NewGauge(gaugeOpts)
			g1.With("alpha", "a", "beta", "b").Set(1)
			g2.With("alpha", "a", "beta", "b").Add(2)

			resp, err := client.Get(fmt.Sprintf("http://%s/metrics", server.Listener.Addr().String()))
			Expect(err).NotTo(HaveOccurred())
			defer utils.IgnoreErrorFunc(resp.Body.Close)

			bytes, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(bytes)).To(ContainSubstring(`peer_playground_gauge_name{alpha="a",beta="b"} 3`))
		})

		It("returns histograms backed by the collector registered first", func() {
			histogramOpts := commonmetrics.HistogramOpts{
				Namespace:  "peer",
				Subsystem:  "playground",
				Name:       "histogram_name",
				Help:       "This is some help text for the histogram",
				LabelNames: []string{"alpha", "beta"},
			}
			h1 := p.NewHistogram(histogramOpts)
			h2 := p.NewHistogram(histogramOpts)
			h1.With("alpha", "a", "beta", "b").Observe(1)
			h2.With("alpha", "a", "beta", "b").Observe(2)

			resp, err := client.Get(fmt.Sprintf("http://%s/metrics", server.Listener.Addr().String()))
			Expect(err).NotTo(HaveOccurred())
			defer utils.IgnoreErrorFunc(resp.Body.Close)

			bytes, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(bytes)).To(ContainSubstring(`peer_playground_histogram_name_count{alpha="a",beta="b"} 2`))
			Expect(string(bytes)).To(ContainSubstring(`peer_playground_histogram_name_sum{alpha="a",beta="b"} 3`))
		})

		It("is not affected by callers mutating opts slices after creation", func() {
			labels := []string{"alpha", "beta"}
			counterOpts.LabelNames = labels
			c1 := p.NewCounter(counterOpts)
			labels[0] = "gamma"

			var c2 commonmetrics.Counter
			original := counterOpts
			original.LabelNames = []string{"alpha", "beta"}
			Expect(func() { c2 = p.NewCounter(original) }).NotTo(Panic())
			Expect(counting.registrations.Load()).To(BeEquivalentTo(1))

			c1.With("alpha", "a", "beta", "b").Add(1)
			c2.With("alpha", "a", "beta", "b").Add(2)

			resp, err := client.Get(fmt.Sprintf("http://%s/metrics", server.Listener.Addr().String()))
			Expect(err).NotTo(HaveOccurred())
			defer utils.IgnoreErrorFunc(resp.Body.Close)

			bytes, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(bytes)).To(ContainSubstring(`peer_playground_counter_name{alpha="a",beta="b"} 3`))
		})

		It("skips the registry for repeated requests", func() {
			for range 10 {
				p.NewCounter(counterOpts)
			}
			Expect(counting.registrations.Load()).To(BeEquivalentTo(1))
		})

		It("registers the collector only once under concurrent requests", func() {
			var wg sync.WaitGroup
			for range 20 {
				wg.Go(func() {
					defer GinkgoRecover()
					p.NewCounter(counterOpts).With("alpha", "a", "beta", "b").Add(1)
				})
			}
			wg.Wait()

			Expect(counting.registrations.Load()).To(BeEquivalentTo(1))

			resp, err := client.Get(fmt.Sprintf("http://%s/metrics", server.Listener.Addr().String()))
			Expect(err).NotTo(HaveOccurred())
			defer utils.IgnoreErrorFunc(resp.Body.Close)

			bytes, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(bytes)).To(ContainSubstring(`peer_playground_counter_name{alpha="a",beta="b"} 20`))
		})

		Context("when the same name is reused with different label names", func() {
			It("panics", func() {
				p.NewCounter(counterOpts)
				counterOpts.LabelNames = []string{"gamma"}
				Expect(func() { p.NewCounter(counterOpts) }).To(Panic())
			})
		})

		It("ignores differences in the doc-only LabelHelp field", func() {
			counterOpts.LabelHelp = map[string]string{"alpha": "first label"}
			c1 := p.NewCounter(counterOpts)
			counterOpts.LabelHelp = map[string]string{"alpha": "a different description"}
			var c2 commonmetrics.Counter
			Expect(func() { c2 = p.NewCounter(counterOpts) }).NotTo(Panic())

			c1.With("alpha", "a", "beta", "b").Add(1)
			c2.With("alpha", "a", "beta", "b").Add(2)

			resp, err := client.Get(fmt.Sprintf("http://%s/metrics", server.Listener.Addr().String()))
			Expect(err).NotTo(HaveOccurred())
			defer utils.IgnoreErrorFunc(resp.Body.Close)

			bytes, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(bytes)).To(ContainSubstring(`peer_playground_counter_name{alpha="a",beta="b"} 3`))
		})

		Context("when an identical collector is already registered outside the provider", func() {
			It("returns a counter backed by the existing collector", func() {
				existing := prom.NewCounterVec(prom.CounterOpts{
					Namespace: counterOpts.Namespace,
					Subsystem: counterOpts.Subsystem,
					Name:      counterOpts.Name,
					Help:      counterOpts.Help,
				}, counterOpts.LabelNames)
				Expect(prom.DefaultRegisterer.Register(existing)).To(Succeed())
				existing.WithLabelValues("a", "b").Add(1)

				var c commonmetrics.Counter
				Expect(func() { c = p.NewCounter(counterOpts) }).NotTo(Panic())
				c.With("alpha", "a", "beta", "b").Add(2)

				resp, err := client.Get(fmt.Sprintf("http://%s/metrics", server.Listener.Addr().String()))
				Expect(err).NotTo(HaveOccurred())
				defer utils.IgnoreErrorFunc(resp.Body.Close)

				bytes, err := io.ReadAll(resp.Body)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(bytes)).To(ContainSubstring(`peer_playground_counter_name{alpha="a",beta="b"} 3`))
			})
		})

		Context("when the same name is reused as a different metric type", func() {
			It("panics with a descriptive error", func() {
				p.NewCounter(counterOpts)
				gaugeOpts := commonmetrics.GaugeOpts{
					Namespace:  counterOpts.Namespace,
					Subsystem:  counterOpts.Subsystem,
					Name:       counterOpts.Name,
					Help:       counterOpts.Help,
					LabelNames: counterOpts.LabelNames,
				}
				Expect(func() { p.NewGauge(gaugeOpts) }).To(PanicWith(MatchError(ContainSubstring("incompatible collector type"))))
			})
		})

		Context("when a non-vector collector with the same descriptor is already registered", func() {
			var scalarOpts commonmetrics.CounterOpts

			BeforeEach(func() {
				scalarOpts = commonmetrics.CounterOpts{
					Namespace: "peer",
					Subsystem: "playground",
					Name:      "scalar_name",
					Help:      "This is some help text for the scalar counter",
				}
				scalar := prom.NewCounter(prom.CounterOpts{
					Namespace: scalarOpts.Namespace,
					Subsystem: scalarOpts.Subsystem,
					Name:      scalarOpts.Name,
					Help:      scalarOpts.Help,
				})
				Expect(prom.DefaultRegisterer.Register(scalar)).To(Succeed())
			})

			It("panics with a descriptive error and does not poison the cache", func() {
				Expect(func() { p.NewCounter(scalarOpts) }).To(PanicWith(MatchError(ContainSubstring("incompatible collector type"))))
				Expect(func() { p.NewCounter(counterOpts) }).NotTo(Panic())
			})
		})
	})

	Describe("edge case behavior", func() {
		var counterOpts commonmetrics.CounterOpts

		BeforeEach(func() {
			counterOpts = commonmetrics.CounterOpts{
				Namespace:  "peer",
				Subsystem:  "playground",
				Name:       "counter_name",
				Help:       "This is some help text for the counter",
				LabelNames: []string{"alpha", "beta"},
			}
		})
		Context("when With is called without a label value", func() {
			It("uses unknown for the missing value", func() {
				counter := p.NewCounter(counterOpts)
				counter.With("alpha", "a", "beta").Add(1)
				resp, err := client.Get(fmt.Sprintf("http://%s/metrics", server.Listener.Addr().String()))
				Expect(err).NotTo(HaveOccurred())
				defer utils.IgnoreErrorFunc(resp.Body.Close)

				bytes, err := io.ReadAll(resp.Body)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(bytes)).To(ContainSubstring(`# HELP peer_playground_counter_name This is some help text for the counter`))
				Expect(string(bytes)).To(ContainSubstring(`# TYPE peer_playground_counter_name counter`))
				Expect(string(bytes)).To(ContainSubstring(`peer_playground_counter_name{alpha="a",beta="unknown"} 1`))
			})
		})

		Context("when With is called with an extra label", func() {
			It("panics", func() {
				counter := p.NewCounter(counterOpts)
				panicMessage := func() (panicMessage any) {
					defer func() { panicMessage = recover() }()
					counter.With("alpha", "a", "beta", "b", "charlie", "c").Add(1)
					return panicMessage
				}()
				Expect(panicMessage).To(MatchError(MatchRegexp(`inconsistent label cardinality: expected 2 label values but got 3 in prometheus.Labels\{.*\}`)))
			})
		})

		Context("when label values are not provided", func() {
			It("it panics with a cardinaility message", func() {
				counter := p.NewCounter(counterOpts)
				panicMessage := func() (panicMessage any) {
					defer func() { panicMessage = recover() }()
					counter.Add(1)
					return panicMessage
				}()
				Expect(panicMessage).To(MatchError(`inconsistent label cardinality: expected 2 label values but got 0 in prometheus.Labels{}`))
			})
		})
	})
})

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
