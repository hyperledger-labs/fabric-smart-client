/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package prometheus

import (
	kitmetrics "github.com/go-kit/kit/metrics"
	"github.com/go-kit/kit/metrics/prometheus"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	prom "github.com/prometheus/client_golang/prometheus"
)

type Provider struct {
	SkipRegisterErr bool
}

func (p *Provider) NewCounter(o metrics.CounterOpts) metrics.Counter {
	cv := prom.NewCounterVec(prom.CounterOpts{
		Namespace: o.Namespace,
		Subsystem: o.Subsystem,
		Name:      o.Name,
		Help:      o.Help,
	}, o.LabelNames)
	p.register(cv)
	return &Counter{Counter: prometheus.NewCounter(cv)}
}

func (p *Provider) NewGauge(o metrics.GaugeOpts) metrics.Gauge {
	gv := prom.NewGaugeVec(prom.GaugeOpts{
		Namespace: o.Namespace,
		Subsystem: o.Subsystem,
		Name:      o.Name,
		Help:      o.Help,
	}, o.LabelNames)
	p.register(gv)
	return &Gauge{Gauge: prometheus.NewGauge(gv)}
}

func (p *Provider) NewHistogram(o metrics.HistogramOpts) metrics.Histogram {
	hv := prom.NewHistogramVec(prom.HistogramOpts{
		Namespace: o.Namespace,
		Subsystem: o.Subsystem,
		Name:      o.Name,
		Help:      o.Help,
		Buckets:   o.Buckets,
	}, o.LabelNames)
	p.register(hv)
	return &Histogram{prometheus.NewHistogram(hv)}
}

func (p *Provider) register(c prom.Collector) {
	if err := prom.Register(c); err != nil && !p.SkipRegisterErr {
		panic(err)
	}
}

type Counter struct{ kitmetrics.Counter }

func (c *Counter) With(labelValues ...string) metrics.Counter {
	return &Counter{Counter: c.Counter.With(labelValues...)}
}

type Gauge struct{ kitmetrics.Gauge }

func (g *Gauge) With(labelValues ...string) metrics.Gauge {
	return &Gauge{Gauge: g.Gauge.With(labelValues...)}
}

type Histogram struct{ kitmetrics.Histogram }

func (h *Histogram) With(labelValues ...string) metrics.Histogram {
	return &Histogram{Histogram: h.Histogram.With(labelValues...)}
}
