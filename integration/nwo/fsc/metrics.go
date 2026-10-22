/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fsc

import (
	dto "github.com/prometheus/client_model/go"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/connstate"
)

// ConnectionStateMetric is the gauge connstate exports; see package connstate.
const ConnectionStateMetric = "fsc_connection_state"

// ParseConnectionState returns the connection state of every endpoint of
// component, as reported in families, the metric families scraped from a node.
// The result is empty when the node exports no such series.
func ParseConnectionState(families map[string]*dto.MetricFamily, component string) map[string]connstate.State {
	states := map[string]connstate.State{}
	for _, m := range families[ConnectionStateMetric].GetMetric() {
		var comp, endpoint string
		for _, l := range m.GetLabel() {
			switch l.GetName() {
			case "component":
				comp = l.GetValue()
			case "endpoint":
				endpoint = l.GetValue()
			}
		}
		if comp == component {
			states[endpoint] = connstate.State(m.GetGauge().GetValue())
		}
	}
	return states
}
