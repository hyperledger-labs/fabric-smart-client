/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"

type Metrics struct {
	Sessions     metrics.Gauge
	StreamHashes metrics.Gauge
	Streams      metrics.Gauge
}

func newMetrics(p metrics.Provider) *Metrics {
	return &Metrics{
		Sessions: p.NewGauge(metrics.GaugeOpts{
			Namespace:    "host",
			Name:         "sessions",
			Help:         "The number of open sessions on the client side",
			StatsdFormat: "%{#fqname}",
		}),
		StreamHashes: p.NewGauge(metrics.GaugeOpts{
			Namespace:    "host",
			Name:         "stream_hashes",
			Help:         "The number of hashes in the stream",
			StatsdFormat: "%{#fqname}",
		}),
		Streams: p.NewGauge(metrics.GaugeOpts{
			Namespace:    "host",
			Name:         "streams",
			Help:         "The number of streams on the client side",
			StatsdFormat: "%{#fqname}",
		}),
	}
}
