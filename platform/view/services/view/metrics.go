/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package view

import "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"

// Metrics models the metrics for the view manager.
type Metrics struct {
	// Contexts returns the number of open contexts on the client side.
	Contexts metrics.Gauge
}

func newMetrics(p metrics.Provider) *Metrics {
	return &Metrics{
		Contexts: p.NewGauge(metrics.GaugeOpts{
			Name: "contexts",
			Help: "The number of open contexts on the client side",
		}),
	}
}
