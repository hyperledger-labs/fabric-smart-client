/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"

type Metrics struct {
	Sessions       metrics.Gauge
	StreamHashes   metrics.Gauge
	ActiveStreams  metrics.Gauge
	OpenedStreams  metrics.Counter
	ClosedStreams  metrics.Counter
	StreamHandlers metrics.Gauge
}

func newMetrics(p metrics.Provider) *Metrics {
	return &Metrics{
		Sessions: p.NewGauge(metrics.GaugeOpts{
			Name: "sessions",
			Help: "The number of open sessions on the client side",
		}),
		StreamHashes: p.NewGauge(metrics.GaugeOpts{
			Name: "stream_hashes",
			Help: "The number of hashes in the stream",
		}),
		ActiveStreams: p.NewGauge(metrics.GaugeOpts{
			Name: "active_streams",
			Help: "The number of streams on the client side",
		}),
		OpenedStreams: p.NewCounter(metrics.CounterOpts{
			Name: "opened_streams",
			Help: "The number of streams opened on the client side",
		}),
		ClosedStreams: p.NewCounter(metrics.CounterOpts{
			Name: "closed_streams",
			Help: "The number of streams closed on the client side",
		}),
		StreamHandlers: p.NewGauge(metrics.GaugeOpts{
			Name: "stream_handlers",
			Help: "The number of stream handlers on the client side",
		}),
	}
}
