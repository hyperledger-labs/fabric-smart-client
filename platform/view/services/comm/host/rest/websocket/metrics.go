/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package websocket

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
)

const (
	sideLabel  tracing.LabelName = "side"
	serverSide string            = "server"
	clientSide string            = "client"
)

type Metrics struct {
	OpenedSubConns   metrics.Counter
	ClosedSubConns   metrics.Counter
	OpenedWebsockets metrics.Counter
	ActiveSubConns   metrics.Gauge
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
		ActiveSubConns: p.NewGauge(metrics.GaugeOpts{
			Name:       "active_subconns",
			Help:       "The number of active subconns",
			LabelNames: []string{},
		}),
	}
}
