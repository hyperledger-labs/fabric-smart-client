/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tracing_test

import (
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
)

type mockMetricsProvider struct {
	counters   map[string]*mockCounter
	histograms map[string]*mockHistogram
}

type mockCounter struct {
	values map[string]float64
}

type mockHistogram struct {
	values map[string][]float64
}

func (c *mockCounter) Add(delta float64) {
	c.values["total"] += delta
}

func (c *mockCounter) With(_ ...string) metrics.Counter {
	return c
}

func (h *mockHistogram) Observe(value float64) {
	h.values["observations"] = append(h.values["observations"], value)
}

func (h *mockHistogram) With(_ ...string) metrics.Histogram {
	return h
}

func (mp *mockMetricsProvider) NewCounter(opts metrics.CounterOpts) metrics.Counter {
	key := fmt.Sprintf("%s_%s_%s", opts.Namespace, opts.Subsystem, opts.Name)
	c := &mockCounter{values: make(map[string]float64)}
	mp.counters[key] = c
	return c
}

func (mp *mockMetricsProvider) NewHistogram(opts metrics.HistogramOpts) metrics.Histogram {
	key := fmt.Sprintf("%s_%s_%s", opts.Namespace, opts.Subsystem, opts.Name)
	h := &mockHistogram{values: make(map[string][]float64)}
	mp.histograms[key] = h
	return h
}
