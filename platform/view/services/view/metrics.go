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
	// Draining is 1 while the node drains running views before it stops, and 0 otherwise.
	Draining metrics.Gauge
}

func NewMetrics(p metrics.Provider) *Metrics {
	return &Metrics{
		Contexts: p.NewGauge(metrics.GaugeOpts{
			Name: "contexts",
			Help: "The number of open contexts on the client side",
		}),
		// fsc_shutdown_draining, independent of this package's path.
		Draining: p.NewGauge(metrics.GaugeOpts{
			Namespace: "fsc",
			Subsystem: "shutdown",
			Name:      "draining",
			Help:      "1 while the node drains running views before it stops, 0 otherwise",
		}),
	}
}
