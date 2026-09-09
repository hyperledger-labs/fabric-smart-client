/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package disabled

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
)

type Provider struct{}

func (*Provider) NewCounter(_ metrics.CounterOpts) metrics.Counter       { return &Counter{} }
func (*Provider) NewGauge(_ metrics.GaugeOpts) metrics.Gauge             { return &Gauge{} }
func (*Provider) NewHistogram(_ metrics.HistogramOpts) metrics.Histogram { return &Histogram{} }

type Counter struct{}

func (*Counter) Add(_ float64) {}
func (c *Counter) With(_ ...string) metrics.Counter {
	return c
}

type Gauge struct{}

func (*Gauge) Add(_ float64) {}
func (*Gauge) Set(_ float64) {}
func (g *Gauge) With(_ ...string) metrics.Gauge {
	return g
}

type Histogram struct{}

func (*Histogram) Observe(_ float64) {}
func (h *Histogram) With(_ ...string) metrics.Histogram {
	return h
}
