/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"

type Metrics struct {
	Sessions metrics.Gauge
}

func newMetrics(p metrics.Provider) *Metrics {
	return &Metrics{
		Sessions: p.NewGauge(metrics.GaugeOpts{
			Namespace:    "host",
			Name:         "sessions",
			Help:         "The number of open sessions on the client side",
			StatsdFormat: "%{#fqname}",
		}),
	}
}
