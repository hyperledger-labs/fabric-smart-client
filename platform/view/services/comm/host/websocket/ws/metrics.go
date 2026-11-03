/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ws

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
)

const (
	sideLabel  tracing.LabelName = "side"
	serverSide string            = "server"
	clientSide string            = "client"

	reasonLabel tracing.LabelName = "reason"
	// versionReason labels streams refused because of the peer's preamble.
	versionReason = "version"
)

type Metrics struct {
	OpenedSubConns   metrics.Counter
	ClosedSubConns   metrics.Counter
	OpenedWebsockets metrics.Counter
	ActiveSubConns   metrics.Gauge
	// RejectedStreams counts the sub-connections a server refused, by reason.
	RejectedStreams metrics.Counter
}

func newMetrics(p metrics.Provider) *Metrics {
	return &Metrics{
		OpenedSubConns: p.NewCounter(metrics.CounterOpts{
			Name:       "opened_subconns",
			Help:       "The number of open subconns",
			LabelNames: []string{sideLabel},
		}),
		ClosedSubConns: p.NewCounter(metrics.CounterOpts{
			Name:       "closed_subconns",
			Help:       "The number of closed subconns",
			LabelNames: []string{sideLabel},
		}),
		OpenedWebsockets: p.NewCounter(metrics.CounterOpts{
			Name:       "opened_websockets",
			Help:       "The number of open websockets",
			LabelNames: []string{sideLabel},
		}),
		RejectedStreams: p.NewCounter(metrics.CounterOpts{
			Name:       "rejected_streams",
			Help:       "The number of sub-connections refused, by reason",
			LabelNames: []string{reasonLabel},
		}),
		ActiveSubConns: p.NewGauge(metrics.GaugeOpts{
			Name:       "active_subconns",
			Help:       "The number of active subconns",
			LabelNames: []string{},
		}),
	}
}
