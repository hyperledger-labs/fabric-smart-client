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

// Connection identifies one fsc_connection_state series by its labels. As a
// filter, an empty field matches any value.
type Connection struct {
	Network, Channel, Component, Endpoint string
}

// matches reports whether c, used as a filter, selects the series s.
func (c Connection) matches(s Connection) bool {
	return (c.Network == "" || c.Network == s.Network) &&
		(c.Channel == "" || c.Channel == s.Channel) &&
		(c.Component == "" || c.Component == s.Component) &&
		(c.Endpoint == "" || c.Endpoint == s.Endpoint)
}

// ParseConnectionState returns the state of every fsc_connection_state series
// in families, the metric families scraped from a node, that filter selects.
// Each series is keyed by all of its labels, so connections of different
// networks or channels to the same endpoint stay apart. The result is empty
// when the node exports no such series.
func ParseConnectionState(families map[string]*dto.MetricFamily, filter Connection) map[Connection]connstate.State {
	states := map[Connection]connstate.State{}
	for _, m := range families[ConnectionStateMetric].GetMetric() {
		var c Connection
		for _, l := range m.GetLabel() {
			switch l.GetName() {
			case "network":
				c.Network = l.GetValue()
			case "channel":
				c.Channel = l.GetValue()
			case "component":
				c.Component = l.GetValue()
			case "endpoint":
				c.Endpoint = l.GetValue()
			}
		}
		if filter.matches(c) {
			states[c] = connstate.State(m.GetGauge().GetValue())
		}
	}
	return states
}
