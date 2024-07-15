/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package websocket

import "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"

type Metrics struct {
	ClientWebsockets      metrics.Gauge
	ServerWebsockets      metrics.Gauge
	InstantiationDuration metrics.Histogram
}

func newMetrics(p metrics.Provider) *Metrics {
	return &Metrics{
		ClientWebsockets: p.NewGauge(metrics.GaugeOpts{
			Namespace:    "host",
			Name:         "client_websockets",
			Help:         "The number of open websockets on the client side",
			StatsdFormat: "%{#fqname}",
		}),
		ServerWebsockets: p.NewGauge(metrics.GaugeOpts{
			Namespace:    "host",
			Name:         "server_websockets",
			Help:         "The number of open websockets on the server side",
			StatsdFormat: "%{#fqname}",
		}),
		InstantiationDuration: p.NewHistogram(metrics.HistogramOpts{
			Namespace:    "host",
			Name:         "instantiation_duration",
			Help:         "The time it took to establish a connection to the server side",
			StatsdFormat: "%{#fqname}",
		}),
	}
}
